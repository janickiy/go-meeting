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
  process.env.MEET_FRONTEND_LIVE_SMOKE !== "true" &&
    process.env.MEET_REMOTE_SMOKE !== "true",
  "Требуется явное разрешение приёмки",
);
const remote = process.env.MEET_REMOTE_SMOKE === "true";
const origin = remote
  ? "https://meeting.janickiy.com"
  : "https://localhost:25482";
const apiOrigin = remote ? origin : "http://127.0.0.1:28085";
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
  try {
    await expect(page.getByTestId("media-status")).toHaveText(
      "Медиасвязь подключена",
      { timeout: 30_000 },
    );
  } catch (error) {
    console.log(
      "Media UI errors:",
      await page.getByRole("alert").allTextContents(),
    );
    throw error;
  }
  await expect(page.getByTestId("remote-media")).toHaveCount(1, {
    timeout: 30_000,
  });
}

test("новая сборка с настоящими SFU, чатом, приватным PDF и записью на локальном стенде", async ({
  browser,
  request,
}, info) => {
  const dist = resolve(process.env.MEET_FRONTEND_DIST || "");
  if (!remote) {
    expect(process.env.MEET_FRONTEND_DIST).toBeTruthy();
    await readFile(resolve(dist, "index.html"));
  }
  const namespace = `frontend-smoke-${randomUUID()}`;
  const password = `Local-${randomUUID()}`;
  const actors: Actor[] = [];
  const contexts: BrowserContext[] = [];
  let conferenceId = "";
  let recordingId = "";
  const checks: string[] = [];
  const networkErrors: string[] = [];
  const relayEvidence: unknown[] = [];
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
        ignoreHTTPSErrors: !remote,
        permissions:
          info.project.name === "firefox" ? [] : ["camera", "microphone"],
        viewport: { width: 1440, height: 1000 },
      });
      contexts.push(context);
      // Static overlay не наследует локальную сетевую зону исходного HTML: разрешение действует только для тестового origin.
      if (!remote) {
        await context.grantPermissions(["local-network-access"], { origin });
        await staticOverlay(context, dist, actor);
      } else {
        await context.addInitScript(
          (token) =>
            sessionStorage.setItem(
              "meet.session.v1",
              JSON.stringify({ token, expiresAt: Date.now() + 1_800_000 }),
            ),
          actor.token,
        );
        const transport = process.env.MEET_REMOTE_TURN || "";
        await context.addInitScript((mode) => {
          const Native = window.RTCPeerConnection;
          const peers: RTCPeerConnection[] = [];
          (
            window as unknown as { __smokePeers: RTCPeerConnection[] }
          ).__smokePeers = peers;
          window.RTCPeerConnection = class extends Native {
            constructor(config?: RTCConfiguration) {
              const servers = mode
                ? (config?.iceServers || []).flatMap((server) => {
                    const urls = (
                      Array.isArray(server.urls) ? server.urls : [server.urls]
                    ).filter((url) =>
                      mode === "tls"
                        ? url.startsWith("turns:")
                        : url.startsWith("turn:") &&
                          url.includes(`transport=${mode}`),
                    );
                    return urls.length ? [{ ...server, urls }] : [];
                  })
                : config?.iceServers;
              super(
                mode
                  ? {
                      ...config,
                      iceServers: servers,
                      iceTransportPolicy: "relay",
                    }
                  : config,
              );
              peers.push(this);
            }
          };
        }, transport);
      }
      const page = await context.newPage();
      page.on("websocket", (socket) => {
        const messages = new Map<string, string>();
        socket.on("framesent", (frame) => {
          try {
            const event = JSON.parse(String(frame.payload));
            if (event.id) messages.set(event.id, event.type);
          } catch {
            /* Не журналируем бинарные сообщения или токены. */
          }
        });
        socket.on("framereceived", (frame) => {
          try {
            const event = JSON.parse(String(frame.payload));
            if (event.type === "error")
              networkErrors.push(
                `error:${event.data?.code || ""}:${messages.get(event.replyTo) || "unknown"}`,
              );
          } catch {
            /* Полные SDP, ICE-пароли и токены не выводятся. */
          }
        });
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
    if (remote)
      for (const page of pages)
        page.on("websocket", (socket) =>
          socket.on("framereceived", (frame) => {
            try {
              const event = JSON.parse(String(frame.payload));
              if (/error|fail/.test(event.type || ""))
                networkErrors.push(
                  `${event.type}:${event.data?.code || ""}:${event.data?.reason || ""}`,
                );
            } catch {
              /* Двоичные медиаданные не включаются в диагностический отчёт. */
            }
          }),
        );
    if (remote && info.project.name === "firefox")
      for (const page of pages)
        page.on("websocket", (socket) =>
          socket.on("framesent", (frame) => {
            try {
              const event = JSON.parse(String(frame.payload));
              if (event.type === "media.offer")
                console.log("Firefox SDP structure:", {
                  sections: String(event.data.sdp)
                    .split(/\r?\n/)
                    .filter((line) =>
                      /^(m=|a=mid:|a=bundle-only|a=group:BUNDLE|a=sendrecv|a=sendonly|a=recvonly|a=inactive)/.test(
                        line,
                      ),
                    ),
                  publications: (event.data.publications || []).map(
                    (p: { mid: string; source: string }) => ({
                      mid: p.mid,
                      source: p.source,
                    }),
                  ),
                });
            } catch {
              /* Полный SDP, ключи ICE и токены никогда не журналируются. */
            }
          }),
        );
    await Promise.all(pages.map(connectMedia));
    for (const page of pages) {
      // Firefox может требовать отдельный жест для воспроизведения удалённого звука.
      const play = page
        .getByTestId("remote-media")
        .getByRole("button", { name: "Включить воспроизведение", exact: true });
      if (await play.isVisible()) await play.click();
    }
    for (const page of pages) {
      if (remote) {
        // Firefox не учитывает WebRTC в totalVideoFrames. Проверяем декодирование
        // настоящего входящего RTP и запущенный DOM-плеер без замены медиаданных.
        await expect
          .poll(
            () =>
              page.evaluate(async () => {
                const peers = (
                  window as unknown as { __smokePeers: RTCPeerConnection[] }
                ).__smokePeers;
                for (const peer of peers) {
                  const stats = await peer.getStats();
                  if (
                    [...stats.values()].some(
                      (s) =>
                        s.type === "inbound-rtp" &&
                        (s.kind || s.mediaType) === "video" &&
                        s.framesDecoded > 5 &&
                        s.bytesReceived > 0,
                    )
                  )
                    return true;
                }
                return false;
              }),
            { timeout: 15_000 },
          )
          .toBe(true);
        await expect
          .poll(
            () =>
              page
                .getByTestId("remote-media")
                .locator("video")
                .evaluate(
                  (video: HTMLVideoElement) =>
                    !video.paused &&
                    video.videoWidth > 0 &&
                    video.currentTime > 0.2,
                ),
            { timeout: 15_000 },
          )
          .toBe(true);
        continue;
      }
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
    }
    checks.push("two-participant-sfu-video");
    if (remote) {
      for (const page of pages) {
        // Подтверждаем входящий звук по RTP, а не только появление плитки участника.
        await expect
          .poll(
            () =>
              page.evaluate(async () => {
                const peers = (
                  window as unknown as { __smokePeers: RTCPeerConnection[] }
                ).__smokePeers;
                for (const peer of peers) {
                  const stats = await peer.getStats();
                  if (
                    [...stats.values()].some(
                      (s) =>
                        s.type === "inbound-rtp" &&
                        (s.kind || s.mediaType) === "audio" &&
                        s.packetsReceived > 10 &&
                        s.bytesReceived > 0,
                    )
                  )
                    return true;
                }
                return false;
              }),
            { timeout: 15_000 },
          )
          .toBe(true);
        await expect
          .poll(
            () =>
              page.getByTestId("remote-media").evaluate((element) => {
                const media =
                  element.querySelector<HTMLMediaElement>("video, audio");
                return (
                  !!media &&
                  !media.muted &&
                  !media.paused &&
                  media.volume > 0 &&
                  (media.srcObject as MediaStream | null)
                    ?.getAudioTracks()
                    .some((track) => track.readyState === "live")
                );
              }),
            { timeout: 15_000 },
          )
          .toBe(true);
      }
      checks.push("two-participant-sfu-audio-rtp-and-playback");
    }
    if (remote && process.env.MEET_REMOTE_TURN) {
      for (const page of pages) {
        let evidence: Awaited<ReturnType<typeof readRelayEvidence>> = [];
        async function readRelayEvidence() {
          return page.evaluate(async () => {
            const peers = (
              window as unknown as { __smokePeers: RTCPeerConnection[] }
            ).__smokePeers;
            const results = [];
            for (const peer of peers) {
              const stats = await peer.getStats();
              const pairs = [...stats.values()]
                .filter(
                  (s) =>
                    s.type === "candidate-pair" &&
                    s.state === "succeeded" &&
                    s.nominated,
                )
                .map((pair) => ({
                  localType: stats.get(pair.localCandidateId)?.candidateType,
                  remoteType: stats.get(pair.remoteCandidateId)?.candidateType,
                  localRelayProtocol: stats.get(pair.localCandidateId)
                    ?.relayProtocol,
                  localURL: stats.get(pair.localCandidateId)?.url,
                  localAddress: stats.get(pair.localCandidateId)?.address,
                  localPort: stats.get(pair.localCandidateId)?.port,
                  relayCandidates: [...stats.values()]
                    .filter(
                      (s) =>
                        s.type === "local-candidate" &&
                        s.candidateType === "relay",
                    )
                    .map((s) => ({
                      address: s.address,
                      port: s.port,
                      relayProtocol: s.relayProtocol,
                      url: s.url,
                    })),
                  bytesReceived: pair.bytesReceived,
                  bytesSent: pair.bytesSent,
                }));
              results.push({
                policy: peer.getConfiguration().iceTransportPolicy,
                state: peer.connectionState,
                pairs,
              });
            }
            return results;
          });
        }
        await expect
          .poll(
            async () => {
              evidence = await readRelayEvidence();
              return evidence.some(
                (peer) =>
                  peer.policy === "relay" &&
                  peer.pairs.some(
                    (pair) =>
                      pair.localType === "relay" && pair.bytesReceived > 0,
                  ),
              );
            },
            { timeout: 20_000, intervals: [500, 1000] },
          )
          .toBe(true);
        relayEvidence.push(evidence);
      }
      checks.push(`selected-turn-${process.env.MEET_REMOTE_TURN}`);
    }
    await owner
      .getByLabel("Сообщение", { exact: true })
      .fill("Проверка настоящего чата");
    await owner.getByRole("button", { name: "Добавить смайлик" }).click();
    const emojiPicker = owner.getByRole("dialog", { name: "Смайлики" });
    await expect(
      emojiPicker
        .getByRole("group", { name: "Выберите смайлик" })
        .getByRole("button"),
    ).toHaveCount(64);
    await emojiPicker
      .getByRole("button", { name: "Огонь", exact: true })
      .click();
    await expect(owner.getByLabel("Сообщение", { exact: true })).toHaveValue(
      "Проверка настоящего чата🔥",
    );
    await owner.getByRole("button", { name: "Отправить", exact: true }).click();
    await expect(member.getByRole("log")).toContainText(
      "Проверка настоящего чата🔥",
    );
    checks.push("persistent-chat-realtime-64-emoji");
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
    const signed = await request.get(fileURL!, { ignoreHTTPSErrors: !remote });
    expect(signed.status()).toBe(200);
    const anonymousURL = new URL(fileURL!);
    anonymousURL.search = "";
    expect(
      (
        await request.get(anonymousURL.toString(), {
          ignoreHTTPSErrors: !remote,
        })
      ).status(),
    ).toBe(403);
    checks.push("private-pdf-upload-download");
    await owner
      .getByRole("button", { name: "Записи конференции", exact: true })
      .click();
    await owner
      .getByRole("button", { name: "Начать запись", exact: true })
      .click();
    await expect(
      owner.getByRole("dialog", { name: "Записи конференции" }),
    ).toBeHidden();
    await expect(
      owner.getByRole("button", { name: "Записи конференции", exact: true }),
    ).toBeFocused();
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
      .getByRole("dialog", { name: "Записи конференции" })
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
    const readyRecording = await api<{
      item: {
        uuid: string;
        status: string;
        files: { fileType: string; url?: string }[];
      };
    }>(
      request,
      `/conferences/${conferenceId}/recordings/${recordingId}`,
      actors[0],
    );
    expect(readyRecording.item.uuid).toBe(recordingId);
    expect(readyRecording.item.status).toBe("ready");
    expect(
      readyRecording.item.files.find(
        /** Выбирает MP4 только из авторизованной карточки этой записи. */
        (file) => file.fileType === "final_mp4",
      )?.url,
    ).toBeTruthy();
    // Диалог активной встречи содержит управление, а готовые файлы проверяются через API и историю ниже.
    const recordingDialog = owner.getByRole("dialog", {
      name: "Записи конференции",
    });
    await expect(recordingDialog.locator(".recording-row")).toHaveCount(0);
    await expect(
      recordingDialog.getByTestId(`recording-${recordingId}`),
    ).toHaveCount(0);
    await expect(
      recordingDialog.getByRole("link", { name: "Скачать MP4", exact: true }),
    ).toHaveCount(0);
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
    for (const context of contexts)
      for (const page of context.pages()) {
        try {
          console.log(
            "Final media counters:",
            JSON.stringify(
              await page.evaluate(async () => {
                const peers =
                  (window as unknown as { __smokePeers?: RTCPeerConnection[] })
                    .__smokePeers || [];
                const results = [];
                for (const peer of peers) {
                  const stats = await peer.getStats();
                  results.push({
                    state: peer.connectionState,
                    rtp: [...stats.values()]
                      .filter(
                        (s) =>
                          s.type === "inbound-rtp" || s.type === "outbound-rtp",
                      )
                      .map((s) => ({
                        type: s.type,
                        kind: s.kind || s.mediaType,
                        packetsReceived: s.packetsReceived,
                        bytesReceived: s.bytesReceived,
                        framesDecoded: s.framesDecoded,
                        packetsSent: s.packetsSent,
                        framesEncoded: s.framesEncoded,
                      })),
                  });
                }
                return results;
              }),
            ),
          );
        } catch {
          /* Закрытая страница не заменяет первоначальный результат проверки. */
        }
      }
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
          relayEvidence,
          durationMs: Date.now() - startedAt,
          staticOverlay: !remote,
          deployment: remote,
          fixturesRetained: true,
        },
        null,
        2,
      ),
    );
  }
});
