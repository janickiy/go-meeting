import { expect, test, type Page } from "@playwright/test";

const now = "2026-10-02T09:00:00Z";
const user = {
  id: "owner",
  displayName: "Александр",
  email: "owner@example.test",
  createdAt: now,
  updatedAt: now,
};
const conference = {
  id: "room",
  ownerId: user.id,
  title: "План выпуска продукта",
  inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef",
  inviteUrl: "/i/ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef",
  status: "finished",
  createdAt: now,
  updatedAt: now,
  startedAt: now,
  finishedAt: now,
};
const member = {
  id: "member",
  userId: user.id,
  conferenceId: "room",
  displayName: "Александр",
  role: "owner",
  status: "left",
  admissionState: "admitted",
  createdAt: now,
  updatedAt: now,
  joinedAt: now,
  leftAt: now,
};
const recording = {
  uuid: "record",
  conferenceId: "room",
  mode: "composite",
  status: "ready",
  createdAt: now,
  durationSec: 90,
  files: [
    {
      fileType: "final_mp4",
      url: "https://private-media.example.test/record.mp4?signature=private",
    },
  ],
};

/**
 * Изолирует UI от платных провайдеров и фиксирует только безопасные контракты API.
 * @args page — браузерная страница; disabled — режим отключённых интеграций.
 * @return Список проверяемых изменений предпочтений.
 */
async function mockStageSeven(page: Page, disabled = false) {
  const writes: unknown[] = [];
  await page.addInitScript(() =>
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({ token: "stage7-ui", expiresAt: Date.now() + 3_500_000 }),
    ),
  );
  await page.route("https://private-media.example.test/**", (route) =>
    route.abort(),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace("/api/v1", "");
    const reply = (body: unknown) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    if (path === "/auth/me") return reply({ status: "success", user });
    if (path === "/capabilities")
      return reply({
        status: "success",
        capabilities: {
          liveCaptions: false,
          transcription: !disabled,
          aiSummary: !disabled,
          semanticSearch: !disabled,
          meetingAnalytics: false,
          recordingModes: ["composite"],
        },
        buildVersion: "stage7-test",
      });
    if (path === "/notifications/events")
      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: ": heartbeat\n\n",
      });
    if (path === "/notifications")
      return reply({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
      });
    if (path === "/notifications/preferences") {
      if (request.method() === "PUT") {
        writes.push(request.postDataJSON());
        return reply(request.postDataJSON());
      }
      return reply({
        invitation: true,
        reminder: true,
        recording: true,
        summary: true,
        email: false,
        push: false,
      });
    }
    if (path === "/integrations/capabilities")
      return reply({
        email: disabled ? "noop" : "mock",
        push: "noop",
        calendar: disabled ? "noop" : "mock",
        calendarOAuthConfigured: false,
        mockConnectAllowed: !disabled,
      });
    if (path === "/integrations/calendars") return reply({ items: [] });
    if (path === "/integrations/calendars/mock")
      return reply({
        item: {
          id: "calendar",
          provider: "mock",
          calendarId: "primary",
          status: "connected",
          createdAt: now,
          updatedAt: now,
        },
      });
    if (path === "/me/conferences")
      return reply({
        status: "success",
        items: [conference],
        nextCursor: null,
      });
    if (path === "/search")
      return reply({
        status: "success",
        total: 1,
        offset: 0,
        limit: 20,
        items: [
          {
            type: "transcript",
            conferenceId: "room",
            conferenceTitle: conference.title,
            recordingId: "record",
            transcriptId: "transcript",
            segmentId: "segment",
            startMs: 42500,
            snippet: "Обсудили план выпуска и проверку качества.",
            rank: 1,
          },
        ],
      });
    if (path === "/conferences/room")
      return reply({ status: "success", item: conference });
    if (path === "/conferences/room/participants/me")
      return reply({ status: "success", item: member });
    if (path === "/conferences/room/participants")
      return reply({ status: "success", items: [member] });
    if (path === "/conferences/room/recordings")
      return reply({ status: "success", items: [recording] });
    if (path === "/conferences/room/recordings/record")
      return reply({ status: "success", item: recording });
    if (path.endsWith("/transcript"))
      return reply({
        status: "success",
        item: disabled
          ? null
          : {
              id: "transcript",
              conferenceId: "room",
              recordingId: "record",
              status: "ready",
              language: "ru",
              provider: "mock",
              createdAt: now,
              updatedAt: now,
            },
        enabled: !disabled,
        canRetry: false,
        providerMode: disabled ? "noop" : "mock",
      });
    if (path.endsWith("/transcript/segments"))
      return reply({
        status: "success",
        total: 2,
        offset: 0,
        limit: 100,
        items: [
          {
            id: "first",
            transcriptId: "transcript",
            startMs: 0,
            endMs: 3000,
            speakerLabel: "Говорящий 1",
            text: "Обсудим план следующего выпуска.",
          },
          {
            id: "segment",
            transcriptId: "transcript",
            startMs: 42500,
            endMs: 45000,
            speakerLabel: "Говорящий 2",
            text: "Проверку качества проведём до публикации. <script>Недоверенный текст</script>",
          },
        ],
      });
    if (path.endsWith("/summary"))
      return reply({
        status: "success",
        enabled: !disabled,
        canRegenerate: false,
        providerMode: disabled ? "noop" : "mock",
        item: disabled
          ? null
          : {
              id: "summary",
              conferenceId: "room",
              transcriptId: "transcript",
              status: "ready",
              summary:
                "Команда обсудила подготовку следующего выпуска продукта.",
              keyPoints: ["Проверка качества обязательна перед публикацией."],
              actionItems: [
                {
                  text: "Проверить качество выпуска",
                  assignee: null,
                  dueDate: null,
                  sourceSegmentIds: ["segment"],
                },
              ],
              topics: ["Выпуск", "Качество"],
              provider: "mock",
              model: "fixture",
              promptVersion: "1",
              schemaVersion: "1",
            },
      });
    if (path.endsWith("/calendar")) return reply({ items: [] });
    if (path.endsWith("/chat/read"))
      return reply({
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: null },
      });
    if (path.endsWith("/messages"))
      return reply({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
        lastReadMessageId: null,
      });
    if (path.endsWith("/history"))
      return reply({
        status: "success",
        item: {
          conference,
          owner: user,
          durationSec: 90,
          participantCount: 1,
          participants: [member],
          recordings: { total: 1, ready: 1, processing: 0, failed: 0 },
          chatAvailable: true,
          chatReadOnly: true,
        },
      });
    return reply({ status: "success", items: [] });
  });
  return writes;
}

test("Stage7: поиск открывает нужную запись, вкладки и временную метку", async ({
  page,
}, info) => {
  await mockStageSeven(page);
  await page.goto("/app/search");
  await page.getByLabel("Поисковый запрос").fill("выпуск");
  await page.getByLabel("Где искать").selectOption("transcript");
  await page.getByRole("button", { name: "Найти", exact: true }).click();
  await expect(
    page.getByText("Обсудили план выпуска и проверку качества."),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("search.png"),
    fullPage: true,
  });
  await page.getByRole("link", { name: "Открыть фрагмент записи" }).click();
  await expect(page).toHaveURL(/\/history\/room\?/);
  const target = new URL(page.url());
  expect(Object.fromEntries(target.searchParams)).toMatchObject({
    recording: "record",
    tab: "transcript",
    section: "transcript",
    t: "42500",
    segment: "segment",
  });
  const recordingTabs = page.getByRole("tablist", { name: "Материалы записи" });
  await expect(
    recordingTabs.getByRole("tab", { name: "Расшифровка", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    page.getByText(/Тестовые данные: это демонстрационная расшифровка/),
  ).toBeVisible();
  await expect(page.locator("video")).toHaveAttribute(
    "src",
    /private-media\.example\.test/,
  );
  await recordingTabs
    .getByRole("tab", { name: "Итоги ИИ", exact: true })
    .click();
  await expect(
    page.getByText("Ответственный: не указан · Срок: не указан"),
  ).toBeVisible();
  await page.getByRole("button", { name: "Источник 00:42" }).click();
  await expect(
    recordingTabs.getByRole("tab", { name: "Расшифровка", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await page.screenshot({
    path: info.outputPath("transcript.png"),
    fullPage: true,
  });
});

test("Stage7: настройки сохраняются, тестовые каналы помечены, mobile без переполнения", async ({
  page,
}, info) => {
  const writes = await mockStageSeven(page);
  await page.goto("/app/settings");
  const settings = page.getByRole("dialog", { name: "Настройки аккаунта" });
  await expect(settings).toBeVisible();
  await expect(
    settings.getByRole("tab", { name: "Интеграции", exact: true }),
  ).toHaveCount(0);
  await expect(
    settings.getByRole("tab", { name: "Безопасность", exact: true }),
  ).toHaveCount(0);
  await settings.getByRole("tab", { name: "Уведомления", exact: true }).click();
  await settings
    .getByRole("checkbox", { name: "Напоминания о встречах" })
    .uncheck();
  await settings.getByRole("button", { name: "Сохранить настройки" }).click();
  await expect(settings.getByText("Настройки сохранены.")).toBeVisible();
  expect(writes).toContainEqual(expect.objectContaining({ reminder: false }));
  await expect(
    settings.getByText("Тестовый канал не отправляет реальные сообщения."),
  ).toBeVisible();
  await expect(
    settings.getByText(
      "Тестовый календарь не создаёт события во внешнем сервисе.",
    ),
  ).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({
    path: info.outputPath("settings-mobile.png"),
    fullPage: false,
  });
});

test("Stage7: выключенные интеграции не имитируют готовность или обработку", async ({
  page,
}) => {
  await mockStageSeven(page, true);
  await page.goto("/app/settings");
  const settings = page.getByRole("dialog", { name: "Настройки аккаунта" });
  await expect(settings).toBeVisible();
  await settings.getByRole("tab", { name: "Уведомления", exact: true }).click();
  await expect(
    settings.getByRole("checkbox", { name: /Email/ }),
  ).toBeDisabled();
  await expect(
    settings.getByRole("button", { name: /Подключить/ }),
  ).toHaveCount(0);
  await page.goto("/conferences/room?recording=record");
  await expect(
    page.getByText(/Расшифровка отключена администратором/),
  ).toBeVisible();
  await expect(page.getByText("В очереди на обработку")).toHaveCount(0);
});
