import { personalBackground } from "./helpers/personal-background";
import { expect, test, type Page } from "@playwright/test";

const createdAt = "2026-10-02T10:00:00Z";
const user = {
  id: "visual-user",
  email: "alex@example.test",
  displayName: "Алексей Петров",
  createdAt,
  updatedAt: createdAt,
};
const conference = {
  id: "visual-room",
  title: "Стратегическая сессия Q2",
  ownerId: user.id,
  status: "finished",
  createdAt,
  updatedAt: createdAt,
  startedAt: createdAt,
  finishedAt: "2026-10-02T11:28:00Z",
  inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWX12345678",
  inviteUrl: "/i/ABCDEFGHIJKLMNOPQRSTUVWX12345678",
  waitingRoomEnabled: true,
};
const member = {
  id: "visual-member",
  userId: user.id,
  conferenceId: conference.id,
  displayName: user.displayName,
  role: "owner",
  status: "left",
  admissionState: "admitted",
  joinedAt: createdAt,
  leftAt: conference.finishedAt,
  createdAt,
  updatedAt: createdAt,
};
const recording = {
  uuid: "visual-recording",
  conferenceId: conference.id,
  status: "ready",
  mode: "composite",
  createdAt,
  durationSec: 5280,
  files: [],
};

/** Изолирует визуальные проверки от рабочих данных; неизвестные API-запросы отклоняются.
 * @args page — браузерная страница теста.
 * @return Завершение установки фикстур API.
 */
async function visualFixture(page: Page) {
  await page.addInitScript(() => {
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "isolated-visual-test",
        expiresAt: Date.now() + 1800000,
      }),
    );
  });
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (await personalBackground(route, path)) return;
    const reply = (value: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(value),
      });
    if (["/auth/session", "/auth/refresh"].includes(path))
      return reply({
        status: "success",
        accessToken: "isolated-visual-test",
        expiresIn: 3600,
        user: user,
      });
    if (path === "/auth/me") return reply({ status: "success", user });
    if (path === "/capabilities")
      return reply({
        status: "success",
        buildVersion: "visual-test",
        capabilities: {
          liveCaptions: false,
          transcription: true,
          aiSummary: true,
          semanticSearch: false,
          meetingAnalytics: true,
          recordingModes: ["composite"],
        },
      });
    if (path === "/notifications/events")
      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: ": isolated visual fixture\n\n",
      });
    if (path === "/notifications")
      return reply({
        status: "success",
        items: [],
        unreadCount: 0,
        nextCursor: null,
      });
    if (path === "/me/conferences" && url.searchParams.get("view") === "active")
      return reply({ status: "success", items: [], nextCursor: null });
    if (path === "/me/conferences" && url.searchParams.get("view") === "past")
      return reply({
        status: "success",
        items: [
          conference,
          { ...conference, id: "second-room", title: "Демо продукта" },
          { ...conference, id: "third-room", title: "Командный синк" },
        ],
        nextCursor: null,
      });
    if (path === `/conferences/${conference.id}/participants/me`)
      return reply({ status: "success", item: member });
    if (path === `/conferences/${conference.id}/history`)
      return reply({
        status: "success",
        item: {
          conference,
          owner: { id: user.id, displayName: user.displayName },
          durationSec: 5280,
          participantCount: 3,
          participants: [member],
          participantsTruncated: true,
          recordings: { total: 1, ready: 1, processing: 0, failed: 0 },
          chatAvailable: true,
          chatReadOnly: true,
        },
      });
    if (path === `/conferences/${conference.id}/recordings`)
      return reply({ status: "success", items: [recording] });
    if (path === `/conferences/${conference.id}/recordings/visual-recording`)
      return reply({ status: "success", item: recording });
    if (path.endsWith("/transcript"))
      return reply({
        status: "success",
        enabled: true,
        providerMode: "http",
        canRetry: false,
        item: {
          id: "visual-transcript",
          status: "ready",
          conferenceId: conference.id,
          recordingId: recording.uuid,
          createdAt,
          updatedAt: createdAt,
          language: "ru",
          provider: "test",
        },
      });
    if (path.endsWith("/transcript/segments"))
      return reply({
        status: "success",
        offset: 0,
        limit: 100,
        total: 3,
        items: [
          {
            id: "segment-1",
            startMs: 0,
            endMs: 5000,
            speakerLabel: "Говорящий 1",
            text: "Обсудим итоги квартала и планы на следующий период.",
          },
          {
            id: "segment-2",
            startMs: 312000,
            endMs: 319000,
            speakerLabel: "Говорящий 2",
            text: "Продуктовая команда подготовила результаты и обратную связь пользователей.",
          },
          {
            id: "segment-3",
            startMs: 1125000,
            endMs: 1130000,
            speakerLabel: "Говорящий 1",
            text: "Следующий шаг — согласовать план работ и проверить ключевые сценарии.",
          },
        ],
      });
    if (path.endsWith("/analytics"))
      return reply({
        status: "success",
        item: {
          enabled: true,
          durationMs: 5280000,
          participantCount: 3,
          recordingAvailable: true,
          transcriptAvailable: true,
          timeline: [
            { atMs: 0, count: 1 },
            { atMs: 180000, count: 2 },
            { atMs: 420000, count: 3 },
            { atMs: 4200000, count: 2 },
            { atMs: 5280000, count: 0 },
          ],
          participants: [
            {
              participantId: "1",
              displayName: "Алексей Петров",
              participationMs: 5280000,
              speakingMs: 1530000,
              observedAudioMs: 5020000,
              screenMs: 760000,
              messageCount: 12,
            },
            {
              participantId: "2",
              displayName: "Мария Соколова",
              participationMs: 4500000,
              speakingMs: 1020000,
              observedAudioMs: 4300000,
              screenMs: 340000,
              messageCount: 8,
            },
            {
              participantId: "3",
              displayName: "Иван Ким",
              participationMs: 3890000,
              speakingMs: 780000,
              observedAudioMs: 3600000,
              screenMs: 0,
              messageCount: 6,
            },
          ],
        },
      });
    if (path === "/integrations/capabilities")
      return reply({
        email: "noop",
        push: "noop",
        calendar: "noop",
        calendarOAuthConfigured: false,
        mockConnectAllowed: false,
      });
    if (path === "/notifications/preferences")
      return reply({
        invitation: true,
        reminder: true,
        recording: true,
        summary: true,
        email: false,
        push: false,
      });
    if (path === "/integrations/calendars")
      return reply({ status: "success", items: [] });
    return reply({ message: "unexpected visual fixture request" }, 404);
  });
}

test("история, материалы, настройки и аналитика сохраняют адаптивную структуру", async ({
  page,
}, info) => {
  await visualFixture(page);
  await page.goto("/history");
  await expect(
    page.getByRole("link", { name: /Стратегическая сессия/ }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("history-desktop.png"),
    fullPage: true,
  });
  await page.goto(
    "/history/visual-room?recording=visual-recording&section=transcript&tab=transcript",
  );
  await expect(
    page.getByText("Обсудим итоги квартала и планы на следующий период."),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("history-materials-desktop.png"),
    fullPage: true,
  });
  await page.goto("/analytics?conference=visual-room");
  await expect(page.getByRole("table")).toBeVisible();
  await page.screenshot({
    path: info.outputPath("analytics-desktop.png"),
    fullPage: true,
  });
  await page
    .getByRole("combobox", { name: "Статус встречи" })
    .selectOption("active");
  await expect(page.getByText(/Сейчас нет активных встреч/)).toBeVisible();
  await expect(page.getByRole("table")).toHaveCount(0);
  await page.goto("/app/settings");
  const settings = page.getByRole("dialog", { name: "Настройки аккаунта" });
  await expect(settings).toBeVisible();
  await expect(
    settings.getByRole("textbox", { name: "Email", exact: true }),
  ).toHaveValue(user.email);
  await expect(
    settings.getByRole("tab", { name: "Интеграции", exact: true }),
  ).toHaveCount(0);
  await expect(
    settings.getByRole("tab", { name: "Безопасность", exact: true }),
  ).toHaveCount(0);
  await page.screenshot({
    path: info.outputPath("settings-desktop.png"),
    fullPage: false,
  });
  for (const [url, name] of [
    ["/history", "history-mobile"],
    ["/analytics?conference=visual-room", "analytics-mobile"],
    ["/app/settings", "settings-mobile"],
  ]) {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(url);
    if (url === "/app/settings") {
      await expect(settings).toBeVisible();
      await expect(
        settings.getByRole("heading", {
          name: "Настройки аккаунта",
          exact: true,
        }),
      ).toBeVisible();
      await expect(
        settings.getByRole("textbox", { name: "Email", exact: true }),
      ).toHaveValue(user.email);
    } else {
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    }
    await expect
      .poll(() =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      )
      .toBe(true);
    await page.screenshot({
      path: info.outputPath(`${name}.png`),
      fullPage: url !== "/app/settings",
    });
  }
});
