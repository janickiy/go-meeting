import {
  expect,
  test,
  type APIRequestContext,
  type BrowserContext,
  type Page,
} from "@playwright/test";
import { randomUUID } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { resolve, sep } from "node:path";

test.skip(
  process.env.MEET_FRONTEND_LIVE_SMOKE !== "true",
  "Требуется явное разрешение локальной приёмки",
);
const origin = "https://localhost:25482";
const apiOrigin = "http://127.0.0.1:28085";
type Actor = { token: string; id: string; name: string };

/** Отправляет запрос только известному локальному стенду, не выводя учётные данные.
 * @args client — HTTP-клиент; path — путь API; actor — синтетическая сессия; method/body — запрос.
 * @return Проверенный JSON успешного ответа.
 */
async function api<T>(
  client: APIRequestContext,
  path: string,
  actor?: Actor,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await client.fetch(`${apiOrigin}/api/v1${path}`, {
    method,
    headers: actor ? { Authorization: `Bearer ${actor.token}` } : {},
    data: body,
    timeout: 15_000,
  });
  if (!response.ok())
    throw new Error(`${method} ${path}: HTTP ${response.status()}`);
  return response.json() as Promise<T>;
}

/** Накладывает сборку UI на тот же origin; API, WS и приватные загрузки остаются настоящими.
 * @args context — изолированный браузер; dist — проверенная директория сборки; actor — тестовая сессия.
 * @return Подготовленный браузер без публикации или замены контейнерных файлов.
 */
async function staticOverlay(
  context: BrowserContext,
  dist: string,
  actor: Actor,
) {
  await context.addInitScript(
    (token) =>
      sessionStorage.setItem(
        "meet.session.v1",
        JSON.stringify({ token, expiresAt: Date.now() + 1_800_000 }),
      ),
    actor.token,
  );
  await context.route(`${origin}/**`, async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname.startsWith("/api/") || request.method() !== "GET")
      return route.continue();
    const asset =
      /^\/(?:assets\/[^/]+|media\/[^/]+|brand-mark\.svg|favicon\.svg|version\.json)$/.test(
        url.pathname,
      );
    const document =
      request.resourceType() === "document" &&
      /^\/(?:conferences|meetings|history|app|login|register|notifications|settings|search)(?:\/[^.?#]*)?$/.test(
        url.pathname,
      );
    if (!asset && !document) return route.continue();
    const target = resolve(
      dist,
      document ? "index.html" : url.pathname.slice(1),
    );
    if (!target.startsWith(`${dist}${sep}`))
      throw new Error("Недопустимый путь сборки");
    const body = await readFile(target);
    const contentType = document
      ? "text/html; charset=utf-8"
      : target.endsWith(".js")
        ? "application/javascript"
        : target.endsWith(".css")
          ? "text/css"
          : target.endsWith(".woff2")
            ? "font/woff2"
            : target.endsWith(".woff")
              ? "font/woff"
              : /\.jpe?g$/.test(target)
                ? "image/jpeg"
                : target.endsWith(".svg")
                  ? "image/svg+xml"
                  : target.endsWith(".png")
                    ? "image/png"
                    : "application/json";
    if (document) {
      const upstream = await route.fetch();
      const headers = upstream.headers();
      delete headers["content-encoding"];
      delete headers["content-length"];
      delete headers.etag;
      await route.fulfill({
        status: upstream.status(),
        headers: { ...headers, "content-type": contentType },
        body,
      });
    } else await route.fulfill({ status: 200, contentType, body });
  });
}

/** Включает тестовые браузерные устройства обычной кнопкой production-интерфейса.
 * @args page — страница уже допущенного участника; @return установленная SFU-медиасвязь.
 */
async function connectMedia(page: Page) {
  const start = page.getByRole("button", {
    name: "Включить камеру и микрофон",
    exact: true,
  });
  await expect(start).toBeEnabled({ timeout: 15_000 });
  await start.click();
  await expect(page.getByTestId("media-status")).toHaveText(
    "Медиасвязь подключена",
    { timeout: 30_000 },
  );
  await expect(page.getByTestId("remote-media")).toHaveCount(1, {
    timeout: 30_000,
  });
}

test("новая сборка с настоящими SFU, чатом, приватным PDF и записью на локальном стенде", async ({
  browser,
  request,
}, info) => {
  const dist = resolve(process.env.MEET_FRONTEND_DIST || "");
  expect(process.env.MEET_FRONTEND_DIST).toBeTruthy();
  await readFile(resolve(dist, "index.html"));
  const namespace = `frontend-smoke-${randomUUID()}`;
  const password = `Local-${randomUUID()}`;
  const actors: Actor[] = [];
  const contexts: BrowserContext[] = [];
  let conferenceId = "";
  let recordingId = "";
  const checks: string[] = [];
  const networkErrors: string[] = [];
  const startedAt = Date.now();
  try {
    for (const [index, name] of [
      "Проверка интерфейса",
      "Проверка участника",
    ].entries()) {
      const email = `${namespace}-${index}@example.test`;
      await api(request, "/auth/register", undefined, "POST", {
        email,
        password,
        displayName: name,
      });
      const session = await api<{ accessToken: string; user: { id: string } }>(
        request,
        "/auth/login",
        undefined,
        "POST",
        { email, password },
      );
      actors.push({ token: session.accessToken, id: session.user.id, name });
    }
    const created = await api<{ item: { id: string; inviteCode: string } }>(
      request,
      "/conferences",
      actors[0],
      "POST",
      { title: namespace, waitingRoomEnabled: false },
    );
    conferenceId = created.item.id;
    await api(
      request,
      `/conferences/${conferenceId}/join`,
      actors[0],
      "POST",
      {},
    );
    await api(
      request,
      `/conference-invites/${created.item.inviteCode}/join`,
      actors[1],
      "POST",
      {},
    );
    await api(
      request,
      `/conferences/${conferenceId}/start`,
      actors[0],
      "POST",
      {},
    );
    const pages: Page[] = [];
    for (const actor of actors) {
      const context = await browser.newContext({
        ignoreHTTPSErrors: true,
        permissions: ["camera", "microphone"],
        viewport: { width: 1440, height: 1000 },
      });
      contexts.push(context);
      // Static overlay не наследует локальную сетевую зону исходного HTML: разрешение действует только для тестового origin.
      await context.grantPermissions(["local-network-access"], { origin });
      await staticOverlay(context, dist, actor);
      const page = await context.newPage();
      page.on("websocket", (socket) => {
        socket.on("socketerror", (message) => {
          networkErrors.push(
            message.replace(/([?&]ticket=)[^\s'"&]+/g, "$1[redacted]"),
          );
        });
      });
      page.on("console", (message) => {
        if (
          message.type() === "error" &&
          /WebSocket|Content Security Policy|net::ERR/.test(message.text())
        )
          networkErrors.push(
            message.text().replace(/([?&]ticket=)[^\s'"&]+/g, "$1[redacted]"),
          );
      });
      pages.push(page);
      await page.goto(`${origin}/conferences/${conferenceId}`);
      await expect(
        page.getByRole("heading", { name: namespace }),
      ).toBeVisible();
    }
    const [owner, member] = pages;
    await Promise.all(pages.map(connectMedia));
    for (const page of pages)
      await expect
        .poll(
          () =>
            page
              .getByTestId("remote-media")
              .locator("video")
              .evaluate(
                (video: HTMLVideoElement) =>
                  video.getVideoPlaybackQuality().totalVideoFrames,
              ),
          { timeout: 15_000 },
        )
        .toBeGreaterThan(5);
    checks.push("two-participant-sfu-video");
    await owner
      .getByLabel("Сообщение", { exact: true })
      .fill("Проверка настоящего чата");
    await owner.getByRole("button", { name: "Отправить", exact: true }).click();
    await expect(member.getByRole("log")).toContainText(
      "Проверка настоящего чата",
    );
    checks.push("persistent-chat-realtime");
    await owner.getByLabel("Выбрать файлы для сообщения").setInputFiles({
      name: "frontend-verification.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from(
        "%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n",
      ),
    });
    await expect(owner.getByText(/Готов к отправке/)).toBeVisible();
    await owner
      .getByLabel("Сообщение", { exact: true })
      .fill("Приватное вложение тестовой встречи");
    await owner.getByRole("button", { name: "Отправить", exact: true }).click();
    await expect(member.getByRole("log")).toContainText(
      "frontend-verification.pdf",
    );
    await member
      .getByRole("button", { name: "Получить ссылку", exact: true })
      .click();
    const fileURL = await member
      .getByRole("link", { name: "Скачать файл", exact: true })
      .getAttribute("href");
    expect(fileURL).toBeTruthy();
    const signed = await request.get(fileURL!, { ignoreHTTPSErrors: true });
    expect(signed.status()).toBe(200);
    const anonymousURL = new URL(fileURL!);
    anonymousURL.search = "";
    expect(
      (
        await request.get(anonymousURL.toString(), { ignoreHTTPSErrors: true })
      ).status(),
    ).toBe(403);
    checks.push("private-pdf-upload-download");
    await owner
      .getByRole("button", { name: "Записи конференции", exact: true })
      .click();
    await owner
      .getByRole("button", { name: "Начать запись", exact: true })
      .click();
    await owner
      .getByRole("dialog", { name: "Записи конференции" })
      .getByRole("button", { name: "Закрыть окно" })
      .click();
    await expect(member.getByTestId("recording-indicator")).toContainText(
      "Идёт запись",
      { timeout: 30_000 },
    );
    await owner.screenshot({
      path: info.outputPath("live-conference.png"),
      fullPage: true,
    });
    await owner.waitForTimeout(6500);
    await owner
      .getByRole("button", { name: "Записи конференции", exact: true })
      .click();
    await owner
      .getByRole("button", { name: "Остановить запись", exact: true })
      .click();
    await expect
      .poll(
        async () => {
          const rows = await api<{ items: { uuid: string; status: string }[] }>(
            request,
            `/conferences/${conferenceId}/recordings`,
            actors[0],
          );
          recordingId = rows.items[0]?.uuid || "";
          if (rows.items[0]?.status === "failed")
            throw new Error("Запись завершилась неуспешно");
          return rows.items[0]?.status;
        },
        { timeout: 90_000, intervals: [1000, 2000] },
      )
      .toBe("ready");
    await expect(
      owner.getByRole("link", { name: "Скачать MP4", exact: true }),
    ).toBeVisible();
    checks.push("recording-start-stop-ready");
    await owner
      .getByRole("dialog", { name: "Записи конференции" })
      .getByRole("button", { name: "Закрыть окно" })
      .click();
    await owner.getByRole("button", { name: /^Участники \(/ }).click();
    await owner
      .getByRole("button", { name: "Завершить конференцию", exact: true })
      .click();
    await owner
      .getByRole("button", { name: "Да, завершить", exact: true })
      .click();
    await owner.goto(`${origin}/history/${conferenceId}?section=recording`);
    await expect(owner.getByRole("heading", { name: namespace })).toBeVisible();
    const recording = owner.getByLabel("Запись встречи", { exact: true });
    await expect(recording).toBeVisible();
    await expect
      .poll(
        () => recording.evaluate((video: HTMLVideoElement) => video.readyState),
        { timeout: 20_000 },
      )
      .toBeGreaterThanOrEqual(1);
    await recording.evaluate((video: HTMLVideoElement) => video.play());
    await expect
      .poll(
        () =>
          recording.evaluate((video: HTMLVideoElement) => video.currentTime),
        { timeout: 15_000 },
      )
      .toBeGreaterThan(0.2);
    checks.push("history-authorized-player");
    await owner.screenshot({
      path: info.outputPath("live-history.png"),
      fullPage: true,
    });
  } finally {
    if (conferenceId && actors[0]) {
      try {
        const current = await api<{ item: { status: string } }>(
          request,
          `/conferences/${conferenceId}`,
          actors[0],
        );
        if (current.item.status === "active")
          await api(
            request,
            `/conferences/${conferenceId}/finish`,
            actors[0],
            "POST",
            {},
          );
      } catch {
        /* Сохраняем первоначальную ошибку; идентификатор тестовой встречи остаётся в отчёте. */
      }
    }
    await Promise.allSettled(contexts.map((context) => context.close()));
    await writeFile(
      info.outputPath("live-smoke-summary.json"),
      JSON.stringify(
        {
          namespace,
          conferenceId,
          recordingId,
          checks,
          networkErrors,
          durationMs: Date.now() - startedAt,
          staticOverlay: true,
          deployment: false,
          fixturesRetained: true,
        },
        null,
        2,
      ),
    );
  }
});
