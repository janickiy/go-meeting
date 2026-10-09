import { personalBackground } from "./helpers/personal-background";
import { readFile } from "node:fs/promises";
import { expect, test, type Page } from "@playwright/test";

const stamp = "2026-10-02T09:00:00Z";
const conferenceId = "48e49ccb-f8f3-4b62-bc87-e1b6f472c170";
const secondConferenceId = "28965a42-784f-4a91-ab89-0a83f1b09b71";
const thirdConferenceId = "e20b4047-e6ba-42c5-9c13-08dd04a5ed5c";
const recordingId = "c641d277-d625-4fbf-9c07-3b79286a6f3a";
const secondRecordingId = "034fdaf7-4b44-42d9-9558-8c92e7c118a4";
const mediaPath = "/__recordings_fixture__/preview.mp4";
const previewPath = "/__recordings_fixture__/preview.jpg";
const user = {
  id: "3f5a878c-7a98-4300-a6d7-9e7315c53e99",
  email: "owner@example.test",
  displayName: "Алексей Петров",
  createdAt: stamp,
  updatedAt: stamp,
};
const conference = {
  id: conferenceId,
  title: "Стратегическая сессия Q2",
  ownerId: user.id,
  status: "finished",
  createdAt: stamp,
  updatedAt: stamp,
  startedAt: stamp,
  finishedAt: "2026-10-02T09:30:00Z",
  inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWX12345678",
  waitingRoomEnabled: false,
};
const secondConference = {
  ...conference,
  id: secondConferenceId,
  title: "Командный синк",
};
const thirdConference = {
  ...conference,
  id: thirdConferenceId,
  title: "Демо для клиента",
};
const recording = {
  uuid: recordingId,
  conferenceId,
  mode: "composite",
  status: "ready",
  createdAt: stamp,
  startedAt: stamp,
  endedAt: "2026-10-02T09:00:03Z",
  durationSec: 3,
  files: [
    {
      fileType: "final_mp4",
      url: `${mediaPath}?signature=fixture-private-link`,
      sizeBytes: 120_000,
    },
    {
      fileType: "preview_jpg",
      url: `${previewPath}?signature=fixture-preview-link`,
    },
  ],
};

/** recordingsFixture изолирует авторизованные продуктовые API и видео теста.
 * Неизвестные маршруты отклоняются; legacy API и запросы изменения фиксируются отдельно.
 * @args page — тестовая страница; mediaURL — ссылка файла для проверки безопасных схем;
 * paginated — вернуть полную исходную страницу с техническими состояниями.
 * @return списки запросов; клип создаётся локально и не обращается к внешним хранилищам.
 */
async function recordingsFixture(
  page: Page,
  mediaURL?: string,
  paginated = false,
) {
  const requests: string[] = [];
  const unexpected: string[] = [];
  const mutations: string[] = [];
  const recordingOffsets: number[] = [];
  const bytes = await readFile(
    new URL("./fixtures/recording-preview.mp4", import.meta.url),
  );
  const preview = await readFile(
    new URL("./fixtures/recording-preview.jpg", import.meta.url),
  );
  await page.addInitScript(() => {
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "recordings-visual-fixture",
        expiresAt: Date.now() + 1_800_000,
      }),
    );
    // Проверяем содержимое копирования, не меняя системный буфер обмена пользователя.
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: {
        writeText: async (value: string) => {
          (
            window as unknown as { copiedRecordingLink: string }
          ).copiedRecordingLink = value;
        },
      },
    });
  });
  await page.route(`**${previewPath}*`, (route) =>
    route.fulfill({ contentType: "image/jpeg", body: preview }),
  );
  await page.route(`**${mediaPath}*`, (route) => {
    const range = /^bytes=(\d+)-(\d*)$/u.exec(
      route.request().headers().range || "",
    );
    const start = range ? Number(range[1]) : 0;
    const end = range?.[2]
      ? Math.min(Number(range[2]), bytes.length - 1)
      : bytes.length - 1;
    if (start > end || start >= bytes.length)
      return route.fulfill({
        status: 416,
        headers: { "Content-Range": `bytes */${bytes.length}` },
      });
    return route.fulfill({
      status: range ? 206 : 200,
      contentType: "video/mp4",
      headers: {
        "Cache-Control": "no-store",
        "Accept-Ranges": "bytes",
        ...(range
          ? { "Content-Range": `bytes ${start}-${end}/${bytes.length}` }
          : {}),
      },
      body: bytes.subarray(start, end + 1),
    });
  });
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (await personalBackground(route, path)) return;
    requests.push(path);
    if (
      request.method() !== "GET" &&
      !["/auth/session", "/auth/refresh"].includes(path)
    )
      mutations.push(path);
    const respond = (data: unknown, status = 200) =>
      route.fulfill({ status, json: data });
    if (
      (path === "/me/conferences" || path.startsWith("/conferences/")) &&
      request.headers().authorization !== "Bearer recordings-visual-fixture"
    ) {
      unexpected.push(`unauthorized:${path}`);
      return respond({ message: "Authentication required by fixture" }, 401);
    }
    if (["/auth/session", "/auth/refresh"].includes(path))
      return respond({
        status: "success",
        accessToken: "recordings-visual-fixture",
        expiresIn: 3600,
        user: user,
      });
    if (path === "/auth/me") return respond({ status: "success", user });
    if (path === "/capabilities")
      return respond({
        status: "success",
        capabilities: {
          liveCaptions: false,
          transcription: false,
          aiSummary: false,
          semanticSearch: false,
          meetingAnalytics: false,
          recordingModes: ["composite"],
        },
      });
    if (path === "/notifications/events")
      return route.fulfill({
        contentType: "text/event-stream",
        body: ": isolated recordings fixture\n\n",
      });
    if (path === "/notifications")
      return respond({
        status: "success",
        items: [],
        unreadCount: 0,
        nextCursor: null,
      });
    if (path === "/me/conferences")
      return respond({
        status: "success",
        items: url.searchParams.has("cursor")
          ? [thirdConference]
          : [conference, secondConference],
        nextCursor: url.searchParams.has("cursor") ? null : "fixture-next-page",
      });
    if (path === `/conferences/${conferenceId}`)
      return respond({ status: "success", item: conference });
    if (path === `/conferences/${secondConferenceId}`)
      return respond({ status: "success", item: secondConference });
    if (path === `/conferences/${thirdConferenceId}`)
      return respond({ status: "success", item: thirdConference });
    if (path === `/conferences/${conferenceId}/recordings` && paginated) {
      const offset = Number(url.searchParams.get("offset") || 0);
      recordingOffsets.push(offset);
      return respond({
        status: "success",
        items:
          offset === 0
            ? [
                recording,
                { ...recording, uuid: secondRecordingId },
                ...Array.from({ length: 18 }, (_, index) => ({
                  ...recording,
                  uuid: `00000000-0000-4000-8000-${String(index).padStart(12, "0")}`,
                  status: "processing",
                  files: [],
                })),
              ]
            : [{ ...recording, uuid: "874292e2-9ea3-4124-88b9-10ff51f4f13f" }],
      });
    }
    if (path === `/conferences/${conferenceId}/recordings`)
      return respond({
        status: "success",
        items: [
          {
            ...recording,
            files: [
              {
                ...recording.files[0],
                url: mediaURL ?? recording.files[0].url,
                sizeBytes: bytes.length,
              },
              recording.files[1],
            ],
          },
          {
            ...recording,
            uuid: secondRecordingId,
            createdAt: "2026-10-02T09:15:00Z",
            durationSec: 190,
            files: [
              {
                fileType: "final_mp4",
                url: `${mediaPath}?signature=fixture-second-file`,
              },
            ],
          },
          {
            ...recording,
            uuid: "d0955750-3c98-4e0f-bc58-8f20212d9fd0",
            status: "processing",
            files: [],
          },
        ],
      });
    if (
      path === `/conferences/${secondConferenceId}/recordings` ||
      path === `/conferences/${thirdConferenceId}/recordings`
    )
      return respond({ status: "success", items: [] });
    if (path === `/conferences/${conferenceId}/recordings/${recordingId}`)
      return respond({
        status: "success",
        item: {
          ...recording,
          files: [
            {
              ...recording.files[0],
              url: mediaURL ?? recording.files[0].url,
              sizeBytes: bytes.length,
            },
            recording.files[1],
          ],
        },
      });
    const historyConference = [
      conference,
      secondConference,
      thirdConference,
    ].find((meeting) => path === `/conferences/${meeting.id}/history`);
    if (historyConference)
      return respond({
        status: "success",
        item: {
          conference: historyConference,
          owner: { id: user.id, displayName: user.displayName },
          durationSec: 1800,
          participantCount: 4,
          participants: [],
          participantsTruncated: true,
          recordings: { total: 3, ready: 2, processing: 1, failed: 0 },
          chatAvailable: true,
          chatReadOnly: true,
        },
      });
    unexpected.push(path);
    return respond({ message: "Unexpected recordings test route" }, 404);
  });
  return { requests, unexpected, mutations, recordingOffsets };
}

/** expectNoOverflow проверяет границы документа после завершения разметки.
 * @args page — страница с текущим размером окна.
 */
async function expectNoOverflow(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
}

for (const viewport of [
  { width: 1440, height: 1000 },
  { width: 834, height: 1112 },
  { width: 390, height: 844 },
]) {
  for (const theme of ["light", "dark"] as const) {
    for (const textSize of [100, 200]) {
      test(`поиск и выбор встречи одинаковой высоты: ${viewport.width}px/${theme}/${textSize}%`, async ({
        page,
      }, info) => {
        await page.setViewportSize(viewport);
        await recordingsFixture(page);
        await page.addInitScript(
          ({ actor, theme, textSize }) => {
            localStorage.setItem(
              `go-recorder.appearance.v1:${encodeURIComponent(actor)}`,
              JSON.stringify({ version: 1, theme, textSize }),
            );
          },
          { actor: user.id, theme, textSize },
        );
        await page.goto(`/recordings?conference=${conferenceId}`);
        await expect(page.locator("html")).toHaveAttribute(
          "data-text-size",
          String(textSize),
        );
        const search = (await page
          .locator(".recordings-search")
          .boundingBox())!;
        const select = (await page
          .getByRole("combobox", { name: "Встреча", exact: true })
          .boundingBox())!;
        expect(Math.abs(search.height - select.height)).toBeLessThanOrEqual(
          0.5,
        );
        expect(search.height).toBeGreaterThanOrEqual(44);
        await expectNoOverflow(page);
        await page.screenshot({
          path: info.outputPath("recordings-matching-controls.png"),
          fullPage: true,
        });
      });
    }
  }
}

for (const viewport of [
  { width: 1440, height: 1000 },
  { width: 390, height: 844 },
  { width: 844, height: 390 },
]) {
  test(`записи и проигрыватель адаптивны и используют продуктовый API: ${viewport.width}×${viewport.height}`, async ({
    page,
  }, info) => {
    await page.setViewportSize(viewport);
    const fixture = await recordingsFixture(page);
    await page.goto(`/recordings?conference=${conferenceId}`);
    await expect(
      page.getByRole("heading", { name: "Записи", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("combobox", { name: "Встреча", exact: true }),
    ).toHaveValue(conferenceId);
    const watch = page.getByRole("link", { name: "Смотреть", exact: true });
    const previewLink = page.getByRole("link", {
      name: `Смотреть запись: ${conference.title}`,
      exact: true,
    });
    await expect(previewLink).toHaveCount(2);
    await expect(watch).toHaveCount(2);
    await expect(watch.first()).toBeVisible();
    await expect(watch.last()).toBeVisible();
    if (viewport.width < 600) {
      // В мобильной карточке явная кнопка просмотра остаётся доступной для касания.
      await watch.first().scrollIntoViewIfNeeded();
      await expect(watch.first()).toBeInViewport();
      const watchBox = await watch.first().boundingBox();
      expect(watchBox).not.toBeNull();
      expect(watchBox!.height).toBeGreaterThanOrEqual(44);
      expect(watchBox!.x).toBeGreaterThanOrEqual(0);
      expect(watchBox!.x + watchBox!.width).toBeLessThanOrEqual(viewport.width);
      await watch.first().click({ trial: true });
    }
    await expect(previewLink.first()).toHaveAttribute(
      "href",
      `/recordings/${recordingId}?conference=${conferenceId}`,
    );
    await expect(
      page.getByText("Обрабатываем запись", { exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: /Удалить|Избранное|Очистить/ }),
    ).toHaveCount(0);
    await expect(page.getByRole("tab")).toHaveCount(0);
    await expectNoOverflow(page);
    await page.screenshot({
      path: info.outputPath("recordings-list.png"),
      fullPage: true,
    });
    if (viewport.width < 600) await watch.first().click();
    else await previewLink.first().click();
    await expect(page).toHaveURL(
      `/recordings/${recordingId}?conference=${conferenceId}`,
    );
    await expect(
      page.getByRole("heading", { name: conference.title, exact: true }),
    ).toBeVisible();
    const video = page.locator(".recording-detail-page video");
    await expect(video).toBeVisible();
    await expect
      .poll(() =>
        video.evaluate((element: HTMLVideoElement) => element.readyState),
      )
      .toBeGreaterThanOrEqual(2);
    await video.evaluate(async (element: HTMLVideoElement) => {
      element.muted = true;
      await element.play();
    });
    await expect
      .poll(() =>
        video.evaluate((element: HTMLVideoElement) => element.currentTime),
      )
      .toBeGreaterThan(0.5);
    await video.evaluate((element: HTMLVideoElement) => element.pause());
    await expectNoOverflow(page);
    await page.screenshot({
      path: info.outputPath("recording-detail.png"),
      fullPage: true,
    });
    await page.getByRole("link", { name: "Назад к записям" }).click();
    await expect(page).toHaveURL(`/recordings?conference=${conferenceId}`);
    expect(fixture.requests.some((path) => path.startsWith("/records"))).toBe(
      false,
    );
    expect(fixture.requests).toContain(
      `/conferences/${conferenceId}/recordings`,
    );
    expect(fixture.requests).toContain(
      `/conferences/${conferenceId}/recordings/${recordingId}`,
    );
    expect(fixture.requests).not.toContain(
      `/conferences/${secondConferenceId}/recordings`,
    );
    expect(fixture.unexpected).toEqual([]);
    expect(fixture.mutations).toEqual([]);
  });
}

test("меню записи управляется клавиатурой и копирует адрес страницы без подписи файла", async ({
  page,
}) => {
  await recordingsFixture(page);
  await page.goto(`/recordings?conference=${conferenceId}`);
  const actions = page
    .getByRole("button", { name: /^Действия.*запис/ })
    .first();
  await actions.focus();
  await page.keyboard.press("Enter");
  const download = page.getByRole("menuitem", { name: "Скачать", exact: true });
  await expect(download).toBeVisible();
  await expect(download).toBeFocused();
  await expect(download).toHaveAttribute(
    "href",
    `${mediaPath}?signature=fixture-private-link`,
  );
  await page.keyboard.press("Escape");
  await expect(download).toBeHidden();
  await expect(actions).toBeFocused();
  await actions.click();
  await page.keyboard.press("End");
  await expect(
    page.getByRole("menuitem", { name: "Копировать ссылку", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Enter");
  const copied = await page.evaluate(
    () =>
      (window as unknown as { copiedRecordingLink: string })
        .copiedRecordingLink,
  );
  const url = new URL(copied);
  expect(url.pathname).toBe(`/recordings/${recordingId}`);
  expect(url.search).toBe(`?conference=${conferenceId}`);
  expect(copied).not.toContain("signature");
});

test("загрузка следующей страницы и локальный поиск не запрашивают записи всех встреч", async ({
  page,
}) => {
  const fixture = await recordingsFixture(page);
  await page.goto(`/recordings?conference=${conferenceId}`);
  const selector = page.getByRole("combobox", { name: "Встреча", exact: true });
  await expect(selector).toHaveValue(conferenceId);
  const search = page.getByLabel("Поиск по загруженным встречам", {
    exact: true,
  });
  await search.fill("Стратегическая");
  await expect(
    selector.locator("option").filter({ hasText: conference.title }),
  ).toHaveCount(1);
  await search.fill("");
  await page.getByRole("button", { name: "Ещё встречи", exact: true }).click();
  await expect(
    selector.locator("option").filter({ hasText: thirdConference.title }),
  ).toHaveCount(1);
  await search.fill("Демо");
  await selector.selectOption(thirdConferenceId);
  await expect(page).toHaveURL(`/recordings?conference=${thirdConferenceId}`);
  await expect(
    page.getByRole("link", { name: "Смотреть", exact: true }),
  ).toHaveCount(0);
  expect(fixture.requests).toContain(
    `/conferences/${thirdConferenceId}/recordings`,
  );
  expect(fixture.requests).not.toContain(
    `/conferences/${secondConferenceId}/recordings`,
  );
  expect(fixture.unexpected).toEqual([]);
  expect(fixture.mutations).toEqual([]);
});

test("небезопасная ссылка файла не попадает в проигрыватель или скачивание", async ({
  page,
}) => {
  await recordingsFixture(page, "javascript:alert('unsafe-recording')");
  await page.goto(`/recordings/${recordingId}?conference=${conferenceId}`);
  await expect(
    page.getByRole("heading", { name: conference.title, exact: true }),
  ).toBeVisible();
  await expect(page.locator(".recording-detail-page video[src]")).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("link", { name: "Скачать запись", exact: true }),
  ).toHaveCount(0);
  await expect(page.locator('a[href^="javascript:"]')).toHaveCount(0);
});

test("пагинация записей считается по исходной странице до скрытия технических состояний", async ({
  page,
}) => {
  const fixture = await recordingsFixture(page, undefined, true);
  await page.goto(`/recordings?conference=${conferenceId}`);
  const watch = page.getByRole("link", { name: "Смотреть", exact: true });
  await expect(watch).toHaveCount(2);
  await page.getByRole("button", { name: "Ещё записи", exact: true }).click();
  await expect(watch).toHaveCount(3);
  await expect(
    page.getByRole("button", { name: "Ещё записи", exact: true }),
  ).toHaveCount(0);
  expect(fixture.recordingOffsets).toEqual([0, 20]);
  expect(fixture.unexpected).toEqual([]);
  expect(fixture.mutations).toEqual([]);
});

test("мобильный отказ воспроизведения не перекрывает сведения о записи", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const fixture = await recordingsFixture(
    page,
    "javascript:alert('unsafe-recording')",
  );
  await page.goto(`/recordings/${recordingId}?conference=${conferenceId}`);
  const error = page.locator(
    ".recording-detail-player .recording-detail-unavailable",
  );
  await expect(
    error.getByRole("heading", { name: "Запись пока недоступна", exact: true }),
  ).toBeVisible();
  const retry = error.getByRole("button", {
    name: "Обновить запись",
    exact: true,
  });
  await expect(retry).toBeVisible();
  const infoCard = page.locator(".recording-detail-section").filter({
    has: page.getByRole("heading", { name: "О записи", exact: true }),
  });
  await expect(infoCard).toBeVisible();
  const errorBox = await error.boundingBox();
  const infoBox = await infoCard.boundingBox();
  const retryBox = await retry.boundingBox();
  expect(errorBox).not.toBeNull();
  expect(infoBox).not.toBeNull();
  expect(retryBox).not.toBeNull();
  expect(errorBox!.y + errorBox!.height).toBeLessThanOrEqual(infoBox!.y);
  expect(retryBox!.y).toBeGreaterThanOrEqual(errorBox!.y);
  expect(retryBox!.y + retryBox!.height).toBeLessThanOrEqual(
    errorBox!.y + errorBox!.height,
  );
  expect(retryBox!.x).toBeGreaterThanOrEqual(0);
  expect(retryBox!.x + retryBox!.width).toBeLessThanOrEqual(390);
  // Проверяем доступность кнопки поверх фиксированной навигации, не отправляя запрос обновления.
  await retry.click({ trial: true });
  await expectNoOverflow(page);
  await page.screenshot({
    path: info.outputPath("recording-detail-unavailable-mobile.png"),
    fullPage: true,
  });
  expect(fixture.unexpected).toEqual([]);
  expect(fixture.mutations).toEqual([]);
});
