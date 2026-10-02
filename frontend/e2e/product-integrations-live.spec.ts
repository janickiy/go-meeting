import { randomUUID } from "node:crypto";
import { writeFile } from "node:fs/promises";
import {
  expect,
  test,
  type APIRequestContext,
  type BrowserContext,
  type Page,
} from "@playwright/test";
import type {
  Conference,
  ConferenceRecording,
  Items,
  Item,
  TranscriptView,
  SummaryView,
  SearchResult,
  OffsetPage,
  TranscriptSegment,
  CalendarSync,
} from "../src/types";

const uiURL = "https://localhost:15482";
const apiURL = "http://127.0.0.1:18085/api/v1";
test.skip(
  process.env.MEET_STAGE7_DOCKER_SMOKE !== "1",
  "Требуется явное разрешение изолированного Stage7 Docker smoke.",
);
type Actor = { id: string; token: string; email: string };

/**
 * Выполняет запрос строго к изолированному API, не выводя токены и приватные ссылки.
 * @args client — тестовый HTTP клиент; path — маршрут; actor — identity; method/data — команда.
 * @return Типизированный успешный JSON либо ошибка только с HTTP статусом.
 */
async function call<T>(
  client: APIRequestContext,
  path: string,
  actor?: Actor,
  method = "GET",
  data?: unknown,
): Promise<T> {
  const response = await client.fetch(`${apiURL}${path}`, {
    method,
    data,
    headers: actor ? { Authorization: `Bearer ${actor.token}` } : {},
  });
  if (!response.ok())
    throw new Error(
      `Stage7 ${method} ${path.split("?")[0]}: HTTP ${response.status()}`,
    );
  return response.status() === 204
    ? (undefined as T)
    : ((await response.json()) as T);
}

/**
 * Восстанавливает только созданную сценарием сессию в отдельном browser context.
 * @args context — изолированный контекст; actor — тестовая identity.
 * @return Новая страница с сессией без записи пароля в артефакты теста.
 */
async function actorPage(context: BrowserContext, actor: Actor): Promise<Page> {
  await context.addInitScript((token) => {
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({ token, expiresAt: Date.now() + 3_500_000 }),
    );
  }, actor.token);
  return context.newPage();
}

/**
 * Подключает синтетические устройства через штатные элементы страницы конференции.
 * @args page — страница одного тестового участника; conferenceId — тестовая встреча.
 * @return Завершение после реального подключения SFU этой вкладки.
 */
async function connectMedia(page: Page, conferenceId: string) {
  await page.goto(`${uiURL}/conferences/${conferenceId}`);
  await expect(page.getByTestId("connection-id")).toBeVisible();
  await page
    .getByRole("button", { name: "Включить камеру и микрофон", exact: true })
    .click();
  await expect(page.getByTestId("media-status")).toHaveText(
    "Медиасвязь подключена",
    { timeout: 40_000 },
  );
}

test("Stage7 isolated Docker: calendar → SFU MP4 → mock STT/AI → authorized search → real video seek", async ({
  browser,
  request: client,
}, info) => {
  const runId = `stage7-${randomUUID()}`;
  const actors: Actor[] = [];
  const contexts: BrowserContext[] = [];
  let conferenceId = "",
    recordingId = "",
    finished = false;
  const checks: string[] = [];
  try {
    const capabilities = await client.get(
      "http://127.0.0.1:18085/health/ready",
    );
    expect(capabilities.ok()).toBe(true);
    for (const name of ["Организатор", "Участник", "Посторонний"]) {
      const email = `${runId}-${actors.length}@smoke.invalid`;
      const password = `Stage7-${randomUUID()}`;
      const registration = await call<{ user: { id: string } }>(
        client,
        "/auth/register",
        undefined,
        "POST",
        { email, password, displayName: name },
      );
      const actor = { id: registration.user.id, token: "", email };
      actors.push(actor);
      actor.token = (
        await call<{ accessToken: string }>(
          client,
          "/auth/login",
          undefined,
          "POST",
          { email, password },
        )
      ).accessToken;
    }
    const [owner, participant, outsider] = actors;
    const integration = await call<{
      calendar: string;
      mockConnectAllowed: boolean;
    }>(client, "/integrations/capabilities", owner);
    expect(integration.calendar).toBe("mock");
    expect(integration.mockConnectAllowed).toBe(true);
    await call(client, "/notifications/preferences", owner, "PUT", {
      invitation: true,
      reminder: true,
      recording: true,
      summary: true,
      email: false,
      push: false,
    });
    await call(client, "/integrations/calendars/mock", owner, "POST", {});
    const scheduledAt = new Date(Date.now() + 10 * 60_000).toISOString();
    const created = await call<Item<Conference>>(
      client,
      "/conferences",
      owner,
      "POST",
      {
        title: runId,
        waitingRoomEnabled: false,
        scheduledAt,
        plannedDurationMin: 30,
      },
    );
    conferenceId = created.item.id;
    await expect
      .poll(
        async () =>
          (
            await call<Items<CalendarSync>>(
              client,
              `/conferences/${conferenceId}/calendar`,
              owner,
            )
          ).items.some((mapping) => mapping.syncStatus === "synced"),
        { timeout: 40_000 },
      )
      .toBe(true);
    await call(client, `/conferences/${conferenceId}/schedule`, owner, "PUT", {
      scheduledAt: new Date(Date.now() + 12 * 60_000).toISOString(),
      plannedDurationMin: 45,
    });
    await call(client, `/conferences/${conferenceId}/join`, owner, "POST", {});
    await call(client, `/conferences/${conferenceId}/start`, owner, "POST", {});
    await call(client, `/conferences/${conferenceId}/join`, owner, "POST", {});
    await call(
      client,
      `/conference-invites/${created.item.inviteCode}/join`,
      participant,
      "POST",
      {},
    );
    checks.push("mock calendar connected and scheduled meeting synced");
    for (let i = 0; i < 2; i++)
      contexts.push(
        await browser.newContext({
          ignoreHTTPSErrors: true,
          viewport: { width: 1440, height: 1100 },
        }),
      );
    const ownerPage = await actorPage(contexts[0], owner),
      participantPage = await actorPage(contexts[1], participant);
    await connectMedia(ownerPage, conferenceId);
    await connectMedia(participantPage, conferenceId);
    for (const page of [ownerPage, participantPage]) {
      await expect(page.getByTestId("remote-media")).toHaveCount(1);
      await expect
        .poll(
          () =>
            page
              .getByTestId("remote-media")
              .locator("video")
              .evaluate(
                (video: HTMLVideoElement) =>
                  video.readyState >= 2 && video.videoWidth > 0,
              ),
          { timeout: 30_000 },
        )
        .toBe(true);
    }
    checks.push(
      "two real browser SFU video receivers decoded synthetic frames",
    );
    const recording = await call<Item<ConferenceRecording>>(
      client,
      `/conferences/${conferenceId}/recordings`,
      owner,
      "POST",
      { segmentDurationSec: 5 },
    );
    recordingId = recording.item.uuid;
    await expect
      .poll(
        async () =>
          (
            await call<Item<ConferenceRecording>>(
              client,
              `/conferences/${conferenceId}/recordings/${recordingId}`,
              owner,
            )
          ).item.status,
        { timeout: 40_000 },
      )
      .toBe("recording");
    await expect(ownerPage.getByTestId("recording-indicator")).toContainText(
      "Идёт запись",
    );
    await expect(
      participantPage.getByTestId("recording-indicator"),
    ).toContainText("Идёт запись");
    // Это реальное накопление медиаданных, не замена API ответов или результата FFmpeg.
    await ownerPage.waitForTimeout(12_000);
    await call(
      client,
      `/conferences/${conferenceId}/recordings/${recordingId}/stop`,
      owner,
      "POST",
      {},
    );
    await expect
      .poll(
        async () => {
          const item = (
            await call<Item<ConferenceRecording>>(
              client,
              `/conferences/${conferenceId}/recordings/${recordingId}`,
              owner,
            )
          ).item;
          if (item.status === "failed")
            throw new Error("Составная запись завершилась ошибкой");
          return item.status;
        },
        { timeout: 100_000 },
      )
      .toBe("ready");
    checks.push("actual composite recording finalized to ready");
    await call(
      client,
      `/conferences/${conferenceId}/finish`,
      owner,
      "POST",
      {},
    );
    finished = true;
    await expect
      .poll(
        async () =>
          (
            await call<TranscriptView>(
              client,
              `/conferences/${conferenceId}/recordings/${recordingId}/transcript`,
              participant,
            )
          ).item?.status,
        { timeout: 80_000 },
      )
      .toBe("ready");
    await expect
      .poll(
        async () =>
          (
            await call<SummaryView>(
              client,
              `/conferences/${conferenceId}/recordings/${recordingId}/summary`,
              participant,
            )
          ).item?.status,
        { timeout: 80_000 },
      )
      .toBe("ready");
    const transcript = await call<TranscriptView>(
      client,
      `/conferences/${conferenceId}/recordings/${recordingId}/transcript`,
      participant,
    );
    const summary = await call<SummaryView>(
      client,
      `/conferences/${conferenceId}/recordings/${recordingId}/summary`,
      participant,
    );
    expect(transcript.providerMode).toBe("mock");
    expect(summary.providerMode).toBe("mock");
    checks.push(
      "mock transcript and AI summary ready via real asynchronous worker",
    );
    const segments = await call<OffsetPage<TranscriptSegment>>(
      client,
      `/conferences/${conferenceId}/recordings/${recordingId}/transcript/segments?limit=100&offset=0`,
      participant,
    );
    expect(segments.items.length).toBeGreaterThan(0);
    const timestamp = segments.items[0].startMs;
    await participantPage.goto(
      `${uiURL}/app/search?q=${encodeURIComponent("проекта")}&source=transcript&conferenceId=${conferenceId}`,
    );
    await expect(
      participantPage
        .getByRole("link", { name: "Открыть фрагмент записи" })
        .first(),
    ).toBeVisible({ timeout: 30_000 });
    await participantPage
      .getByRole("link", { name: "Открыть фрагмент записи" })
      .first()
      .click();
    await expect(participantPage).toHaveURL(
      new RegExp(`recording=${recordingId}.*t=${timestamp}`),
    );
    await expect(
      participantPage.getByText(
        /Тестовые данные: это демонстрационная расшифровка/,
      ),
    ).toBeVisible();
    const video = participantPage.locator("video.recording-video");
    await expect
      .poll(
        () =>
          video.evaluate(
            (element: HTMLVideoElement) =>
              element.readyState >= 2 &&
              element.duration > 3 &&
              element.videoWidth > 0,
          ),
        { timeout: 40_000 },
      )
      .toBe(true);
    await video.evaluate(async (element: HTMLVideoElement) => {
      element.muted = true;
      await element.play();
    });
    await expect
      .poll(
        () =>
          video.evaluate((element: HTMLVideoElement) => element.currentTime),
        { timeout: 15_000 },
      )
      .toBeGreaterThan(1.5);
    await video.evaluate((element: HTMLVideoElement) => element.pause());
    await participantPage
      .getByRole("button", { name: "Перейти к 00:00", exact: true })
      .click();
    await expect
      .poll(() =>
        video.evaluate((element: HTMLVideoElement) => element.currentTime),
      )
      .toBeLessThan(0.5);
    checks.push(
      "search UUID/timestamp deep link opened real MP4; play and transcript seek verified",
    );
    await participantPage
      .getByRole("tab", { name: "Итоги ИИ", exact: true })
      .click();
    await expect(
      participantPage.getByText(/Автоматические итоги могут содержать ошибки/),
    ).toBeVisible();
    await participantPage.screenshot({
      path: info.outputPath("stage7-summary.png"),
      fullPage: true,
    });
    const unrelated = await call<OffsetPage<SearchResult>>(
      client,
      `/search?q=${encodeURIComponent("проекта")}&source=all&conferenceId=${conferenceId}&limit=20&offset=0`,
      outsider,
    );
    expect(unrelated.items).toEqual([]);
    expect(unrelated.total).toBe(0);
    for (const suffix of [
      "",
      "/transcript",
      "/summary",
      "/transcript/segments",
    ]) {
      const response = await client.get(
        `${apiURL}/conferences/${conferenceId}/recordings/${recordingId}${suffix}`,
        { headers: { Authorization: `Bearer ${outsider.token}` } },
      );
      expect([403, 404]).toContain(response.status());
    }
    checks.push(
      "outsider recording/transcript/summary/segments denied and search empty",
    );
  } finally {
    if (conferenceId && actors[0]?.token && !finished) {
      await call(
        client,
        `/conferences/${conferenceId}/finish`,
        actors[0],
        "POST",
        {},
      ).catch(() => undefined);
    }
    await Promise.all(contexts.map((context) => context.close()));
    const manifest = JSON.stringify(
      {
        runId,
        conferenceId,
        recordingId,
        userIds: actors.map((actor) => actor.id),
        syntheticMedia: true,
        paidProviders: false,
        checks,
        cleanup:
          "Тестовые UUID сохранены в изолированном стенде для точечной очистки; основной Compose не используется.",
      },
      null,
      2,
    );
    const manifestPath = info.outputPath("acceptance.json");
    await writeFile(manifestPath, manifest);
    await info.attach("stage7-acceptance", {
      contentType: "application/json",
      path: manifestPath,
    });
  }
});
