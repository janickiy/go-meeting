import { personalBackground } from "./helpers/personal-background";
import { expect, test, type Page } from "@playwright/test";
import type { Conference } from "../src/types";

const user = {
  id: "calendar-owner",
  email: "calendar@example.test",
  displayName: "Александр",
  createdAt: "2026-10-01T10:00:00Z",
  updatedAt: "2026-10-01T10:00:00Z",
};
const meetings: Conference[] = [
  {
    id: "product",
    ownerId: user.id,
    title: "Продуктовая встреча",
    status: "scheduled",
    inviteCode: "A".repeat(32),
    inviteUrl: "",
    createdAt: user.createdAt,
    updatedAt: user.createdAt,
    startedAt: null,
    finishedAt: null,
    scheduledAt: "2026-10-06T07:00:00Z",
    plannedDurationMin: 30,
  },
  {
    id: "demo",
    ownerId: user.id,
    title: "Демо для клиента",
    status: "scheduled",
    inviteCode: "B".repeat(32),
    inviteUrl: "",
    createdAt: user.createdAt,
    updatedAt: user.createdAt,
    startedAt: null,
    finishedAt: null,
    scheduledAt: "2026-10-07T11:00:00Z",
    plannedDurationMin: 60,
  },
  {
    id: "retro",
    ownerId: user.id,
    title: "Ретроспектива команды",
    status: "scheduled",
    inviteCode: "C".repeat(32),
    inviteUrl: "",
    createdAt: user.createdAt,
    updatedAt: user.createdAt,
    startedAt: null,
    finishedAt: null,
    scheduledAt: "2026-10-09T13:30:00Z",
    plannedDurationMin: 45,
  },
];

/**
 * installCalendarFixture изолирует визуальную проверку от реальных аккаунтов и встреч.
 * @args page — браузерная страница; owner — разрешены ли текущему пользователю действия организатора.
 * @return Список фактически выполненных запросов для проверки подтверждения операций.
 */
async function installCalendarFixture(page: Page, owner = true) {
  await page.clock.setFixedTime(new Date("2026-10-05T08:00:00Z"));
  await page.addInitScript(() =>
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "calendar-visual-fixture",
        expiresAt: Date.now() + 1_800_000,
      }),
    ),
  );
  const requests: string[] = [];
  let cancelled = false;
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace("/api/v1", "");
    if (await personalBackground(route, path)) return;
    requests.push(`${request.method()} ${path}`);
    const respond = (body: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    if (["/auth/session", "/auth/refresh"].includes(path))
      return respond({
        status: "success",
        accessToken: "calendar-visual-fixture",
        expiresIn: 3600,
        user: { ...user, id: owner ? user.id : "calendar-member" },
      });
    if (path === "/auth/me")
      return respond({
        user: { ...user, id: owner ? user.id : "calendar-member" },
      });
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
        buildVersion: "calendar-visual",
      });
    if (path === "/notifications")
      return respond({ items: [], nextCursor: null, unreadCount: 0 });
    if (path === "/notifications/events")
      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: ": fixture\n\n",
      });
    if (path === "/me/conferences") {
      const rows =
        url.searchParams.get("view") === "past"
          ? meetings.map((meeting) => ({
              ...meeting,
              id: `past-${meeting.id}`,
              status: "finished",
              finishedAt: "2026-10-01T12:00:00Z",
            }))
          : cancelled
            ? meetings.slice(1)
            : meetings;
      return respond({ status: "success", items: rows, nextCursor: null });
    }
    if (path === "/conferences/product/cancel" && request.method() === "POST") {
      cancelled = true;
      return respond({ item: { ...meetings[0], status: "cancelled" } });
    }
    return respond({ message: "not found" }, 404);
  });
  return requests;
}

test("кабинет и недельный календарь: реальные переходы и визуальная проверка", async ({
  page,
}, testInfo) => {
  const requests = await installCalendarFixture(page);
  await page.goto("/app");
  await expect(
    page.getByRole("heading", { name: "Добро пожаловать, Александр!" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Недавние встречи" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Запланировать" }),
  ).toHaveAttribute("href", "/meetings/new?scheduled=1");
  await page.screenshot({
    path: testInfo.outputPath("dashboard-desktop.png"),
    fullPage: true,
  });
  await page.goto("/calendar");
  await expect(
    page.getByRole("heading", { name: "Календарь", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: /Продуктовая встреча,/ }).first(),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("calendar-week-desktop.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: /Продуктовая встреча,/ })
    .first()
    .click();
  await page
    .getByRole("button", { name: "Отменить встречу", exact: true })
    .click();
  expect(requests).not.toContain("POST /conferences/product/cancel");
  await page.getByRole("button", { name: "Да, отменить встречу" }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(
    page.getByRole("button", { name: /Продуктовая встреча,/ }),
  ).toHaveCount(0);
  expect(requests).toContain("POST /conferences/product/cancel");
  await page.getByRole("button", { name: "Месяц", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Месяц", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("region", { name: "Сетка месяца" }),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("calendar-month-desktop.png"),
    fullPage: true,
  });
});

test("мобильный календарь: список без горизонтального переполнения и без чужой модерации", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await installCalendarFixture(page, false);
  await page.goto("/calendar");
  await expect(
    page.getByRole("button", { name: /Продуктовая встреча,/ }).last(),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("calendar-mobile.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: /Продуктовая встреча,/ })
    .last()
    .click();
  await expect(
    page.getByRole("button", { name: "Изменить расписание" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Отменить встречу", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "Открыть встречу" }),
  ).toBeVisible();
});
