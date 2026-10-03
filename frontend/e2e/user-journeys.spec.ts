import { expect, test, type Page } from "@playwright/test";
import type {
  Conference,
  ConferenceHistory,
  ConferenceRecording,
  Notification,
  Participant,
  User,
} from "../src/types";

const now = "2026-10-02T10:00:00Z";
const user: User = {
  id: "user-1",
  email: "alex@example.test",
  displayName: "Александр",
  createdAt: now,
  updatedAt: now,
};
const conference: Conference = {
  id: "meeting-1",
  ownerId: user.id,
  title: "План запуска",
  inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWX12345678",
  inviteUrl: "/i/ABCDEFGHIJKLMNOPQRSTUVWX12345678",
  status: "finished",
  createdAt: now,
  updatedAt: now,
  startedAt: now,
  finishedAt: "2026-10-02T11:00:00Z",
  waitingRoomEnabled: true,
};
const membership: Participant = {
  id: "member-1",
  conferenceId: conference.id,
  userId: user.id,
  displayName: user.displayName || "Участник",
  role: "participant",
  status: "left",
  admissionState: "admitted",
  joinedAt: now,
  leftAt: conference.finishedAt,
  createdAt: now,
  updatedAt: now,
};
const history: ConferenceHistory = {
  conference,
  owner: { id: user.id, displayName: user.displayName },
  durationSec: 3600,
  participantCount: 1,
  participants: [membership],
  participantsTruncated: false,
  recordings: { total: 1, ready: 1, processing: 0, failed: 0 },
  chatAvailable: false,
  chatReadOnly: true,
};
const recording: ConferenceRecording = {
  uuid: "record-1",
  conferenceId: conference.id,
  mode: "composite",
  status: "ready",
  createdAt: now,
  durationSec: 3600,
  files: [],
};
const notification: Notification = {
  id: "notice-1",
  userId: user.id,
  type: "summary.ready",
  version: 1,
  payload: {
    conferenceId: conference.id,
    recordingId: recording.uuid,
    summaryId: "summary-1",
  },
  createdAt: now,
  readAt: null,
};

type MockOptions = {
  admin?: boolean;
  denyAdmin?: boolean;
  prejoin?: boolean;
};

/** Все API-запросы этих браузерных тестов обрабатываются локально; реальные данные не затрагиваются. */
async function mockStage9(page: Page, options: MockOptions = {}) {
  await page.addInitScript(() => {
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "stage9-e2e",
        expiresAt: Date.now() + 1_800_000,
      }),
    );
  });
  const requested: string[] = [];
  let read = false;
  let joined = false;
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    const method = request.method();
    requested.push(`${method} ${path}`);
    const respond = (body: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    if (path === "/notifications/events")
      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: ": isolated test\n\n",
      });
    if (path === "/auth/me")
      return respond({
        status: "success",
        user: { ...user, isAdmin: options.admin === true },
      });
    if (path === "/notifications")
      return respond({
        status: "success",
        items: [{ ...notification, readAt: read ? now : null }],
        nextCursor: null,
        unreadCount: read ? 0 : 1,
      });
    if (
      path === `/notifications/${notification.id}/read` &&
      method === "POST"
    ) {
      read = true;
      return respond({
        status: "success",
        item: { ...notification, readAt: now },
      });
    }
    if (path === "/capabilities")
      return respond({
        status: "success",
        capabilities: {
          liveCaptions: false,
          transcription: false,
          aiSummary: true,
          semanticSearch: false,
          meetingAnalytics: false,
          recordingModes: ["composite"],
        },
        buildVersion: "stage9-test",
      });
    if (path === "/admin/summary")
      return respond({ message: "forbidden" }, options.denyAdmin ? 403 : 404);
    if (path === `/conferences/${conference.id}`)
      return respond({
        status: "success",
        item: options.prejoin
          ? { ...conference, status: "created", finishedAt: null }
          : conference,
      });
    if (path === `/conferences/${conference.id}/join` && method === "POST") {
      joined = true;
      return respond({
        status: "success",
        item: { ...membership, status: "joined", joinedAt: now, leftAt: null },
      });
    }
    if (path === `/conferences/${conference.id}/participants/me`)
      return options.prejoin && !joined
        ? respond({ message: "not found" }, 404)
        : respond({
            status: "success",
            item: joined
              ? { ...membership, status: "joined", joinedAt: now, leftAt: null }
              : membership,
          });
    if (path === `/conferences/${conference.id}/history`)
      return respond({ status: "success", item: history });
    if (path === `/conferences/${conference.id}/recordings`)
      return respond({ status: "success", items: [recording] });
    if (path === `/conferences/${conference.id}/recordings/${recording.uuid}`)
      return respond({ status: "success", item: recording });
    if (
      path ===
      `/conferences/${conference.id}/recordings/${recording.uuid}/transcript`
    )
      return respond({
        status: "success",
        item: null,
        enabled: false,
        canRetry: false,
        providerMode: "noop",
      });
    if (
      path ===
      `/conferences/${conference.id}/recordings/${recording.uuid}/summary`
    )
      return respond({
        status: "success",
        item: {
          id: "summary-1",
          conferenceId: conference.id,
          transcriptId: "transcript-1",
          status: "ready",
          summary: "Решили запустить пилот на следующей неделе.",
          keyPoints: ["Подготовить пилот"],
          actionItems: [],
          topics: ["Запуск"],
          provider: "mock",
          model: "test",
          promptVersion: "1",
          schemaVersion: "1",
        },
        enabled: true,
        canRegenerate: false,
        providerMode: "mock",
      });
    return respond({ message: "unexpected test request" }, 404);
  });
  return requested;
}

test("prejoin shows an actionable device denial without opening room media", async ({
  page,
}) => {
  const requested = await mockStage9(page, { prejoin: true });
  await page.addInitScript(() => {
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: async () => {
        throw new DOMException("Denied by test", "NotAllowedError");
      },
    });
  });
  await page.goto(`/conferences/${conference.id}/join`);
  await expect(
    page.getByRole("heading", { name: conference.title }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Проверить камеру" }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Разрешите камеру для этого сайта",
  );
  await expect(
    page.getByRole("button", { name: "Войти во встречу" }),
  ).toBeEnabled();
  expect(requested).not.toContain("POST /conferences/meeting-1/join");
  expect(requested.some((path) => path.includes("/ws-ticket"))).toBe(false);
  expect(requested.some((path) => path.includes("/webrtc/"))).toBe(false);
  await page.getByRole("button", { name: "Войти во встречу" }).click();
  await expect(page).toHaveURL(/\/conferences\/meeting-1$/);
  expect(requested).toContain("POST /conferences/meeting-1/join");
});

test("history deep link opens AI summary and preserves recording when switching tabs", async ({
  page,
}) => {
  const requested = await mockStage9(page);
  await page.goto(
    `/history/${conference.id}?section=summary&recording=${recording.uuid}&tab=summary`,
  );
  await expect(
    page.getByRole("heading", { name: conference.title }),
  ).toBeVisible();
  const sections = page.getByRole("tablist", { name: "Материалы встречи" });
  await expect(sections.getByRole("tab", { name: "Итоги ИИ" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(
    page.getByText("Решили запустить пилот на следующей неделе."),
  ).toBeVisible();
  await sections.getByRole("tab", { name: "Обзор" }).click();
  await expect(sections.getByRole("tab", { name: "Обзор" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page).toHaveURL(/section=overview/);
  await expect(page).toHaveURL(/recording=record-1/);
  expect(requested.some((path) => path.includes("/analytics"))).toBe(false);
});

test("notifications mark read and open an authorized history deep link", async ({
  page,
}) => {
  await mockStage9(page);
  await page.goto("/notifications");
  await expect(
    page.getByRole("heading", { name: "Уведомления" }),
  ).toBeVisible();
  await expect(page.getByText("Непрочитанных: 1")).toBeVisible();
  await page.getByRole("button", { name: /Отметить как прочитанное/ }).click();
  await expect(page.getByText("Непрочитанных: 0")).toBeVisible();
  await page.getByRole("link", { name: "Открыть встречу" }).click();
  await expect(page).toHaveURL(/\/history\/meeting-1\?/);
  await expect(page).toHaveURL(/section=summary/);
  await expect(
    page.getByText("Решили запустить пилот на следующей неделе."),
  ).toBeVisible();
});

test("non-admin account cannot fetch the operations summary", async ({
  page,
}) => {
  const requested = await mockStage9(page);
  await page.goto("/admin");
  await expect(
    page.getByRole("heading", { name: "Нет доступа к разделу операций" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Администрирование" }),
  ).toHaveCount(0);
  expect(requested).not.toContain("GET /admin/summary");
});

test("server revocation denies an account with stale local admin capability", async ({
  page,
}) => {
  const requested = await mockStage9(page, { admin: true, denyAdmin: true });
  await page.goto("/admin");
  await expect(
    page.getByRole("heading", { name: "Нет доступа к разделу операций" }),
  ).toBeVisible();
  expect(requested).toContain("GET /admin/summary");
});

test("mobile prejoin and history keep actions visible without page overflow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockStage9(page, { prejoin: true });
  const fitsViewport = () =>
    page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    );

  await page.goto(`/conferences/${conference.id}/join`);
  await expect(
    page.getByRole("button", { name: "Войти во встречу" }),
  ).toBeVisible();
  await expect.poll(fitsViewport).toBe(true);

  await page.getByRole("button", { name: "Войти во встречу" }).click();
  await expect(page).toHaveURL(/\/conferences\/meeting-1$/);
  await page.goto(`/history/${conference.id}?section=summary`);
  await expect(
    page
      .getByRole("tablist", { name: "Материалы встречи" })
      .getByRole("tab", { name: "Итоги ИИ" }),
  ).toBeVisible();
  await expect.poll(fitsViewport).toBe(true);
});
