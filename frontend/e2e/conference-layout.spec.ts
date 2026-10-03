import { expect, test, type Page } from "@playwright/test";
import type { Participant } from "../src/types";

const stamp = "2026-10-03T12:00:00Z";
const room = {
  id: "room-visual",
  title: "Продуктовая встреча",
  status: "active",
  ownerId: "owner",
  createdAt: stamp,
  updatedAt: stamp,
  startedAt: null,
  finishedAt: null,
  inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWX12345678",
  waitingRoomEnabled: true,
};
const people = [
  "Алексей Петров",
  "Мария Соколова",
  "Иван Ким",
  "Елена Смирнова",
].map(
  (name, index) =>
    ({
      id: `participant-${index}`,
      userId: index ? `user-${index}` : "owner",
      conferenceId: room.id,
      displayName: name,
      role: index === 0 ? "owner" : "participant",
      status: "joined",
      admissionState: "admitted",
      createdAt: stamp,
      updatedAt: stamp,
      joinedAt: stamp,
      leftAt: null,
      microphoneEnabled: false,
      cameraEnabled: false,
      online: true,
      connections: 1,
      connectionIds: [`connection-${index}`],
    }) as Participant,
);

/** Подменяет только тестовые HTTP/WS ответы; не предоставляет фиктивные медиа production-сборке.
 * @args page — изолированная страница; @return подготовленная комната с настоящими UI-состояниями без физических устройств.
 */
async function fixture(page: Page) {
  await page.addInitScript(() =>
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "visual-only-token",
        expiresAt: Date.now() + 1_800_000,
      }),
    ),
  );
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace("/api/v1", "");
    const respond = (data: unknown) => route.fulfill({ json: data });
    if (path === "/auth/me")
      return respond({
        status: "success",
        user: {
          id: "owner",
          email: "owner@example.test",
          displayName: "Алексей Петров",
          createdAt: stamp,
          updatedAt: stamp,
        },
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
      });
    if (path === "/notifications/events")
      return route.fulfill({
        contentType: "text/event-stream",
        body: ": visual fixture\n\n",
      });
    if (path === "/notifications")
      return respond({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
      });
    if (path === `/conferences/${room.id}`)
      return respond({ status: "success", item: room });
    if (path.endsWith("/participants/me"))
      return respond({ status: "success", item: people[0] });
    if (path.endsWith("/participants"))
      return respond({ status: "success", items: people });
    if (path.endsWith("/recordings") || path.endsWith("/hands"))
      return respond({ status: "success", items: [] });
    if (path.endsWith("/messages"))
      return respond({ status: "success", items: [], nextCursor: null });
    if (path.endsWith("/chat/read"))
      return respond({
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: null },
      });
    if (path.endsWith("/ws-ticket"))
      return respond({
        ticket: "visual-ticket",
        expiresAt: "2099-01-01T00:00:00Z",
      });
    return route.fulfill({
      status: 404,
      json: { message: "unavailable test route" },
    });
  });
  await page.routeWebSocket(
    /\/api\/v1\/conferences\/room-visual\/ws/,
    (socket) => {
      socket.send(
        JSON.stringify({
          version: 1,
          id: "initial-state",
          type: "conference.state",
          conferenceId: room.id,
          timestamp: stamp,
          data: {
            connectionId: "connection-0",
            participantId: people[0].id,
            status: "active",
            participants: people,
            hands: [],
          },
        }),
      );
    },
  );
}

test("тёмная комната: настоящие пустые плитки, чат и разрешённая модерация", async ({
  page,
}, info) => {
  await fixture(page);
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByRole("heading", { name: room.title })).toBeVisible();
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(4);
  await expect(
    page.getByRole("navigation", { name: "Основная навигация" }),
  ).toBeHidden();
  await expect(
    page.getByRole("tab", { name: "Чат", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("conference-chat.png"),
    fullPage: true,
  });
  await page.getByRole("tab", { name: "Участники (4)", exact: true }).click();
  await expect(page.getByLabel("Управление: Мария Соколова")).toBeVisible();
  await expect(page.getByLabel("Управление: Алексей Петров")).toHaveCount(0);
  await page.screenshot({
    path: info.outputPath("conference-participants.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Записи конференции", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Записи конференции", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Начать запись", exact: true }),
  ).toBeVisible();
});

test("мобильная комната открывает и закрывает панель без горизонтального переполнения", async ({
  page,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await fixture(page);
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(4);
  await expect(
    page.getByRole("tab", { name: "Чат", exact: true }),
  ).toBeHidden();
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({
    path: info.outputPath("conference-mobile.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Чат", exact: true }).click();
  await expect(
    page.getByRole("tab", { name: "Чат", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("conference-mobile-chat.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Закрыть панель встречи" }).click();
  await expect(
    page.getByRole("tab", { name: "Чат", exact: true }),
  ).toBeHidden();
});

test("предпросмотр не запрашивает устройства до явного действия", async ({
  page,
}, info) => {
  await fixture(page);
  await page.addInitScript(() => {
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      value: () => {
        throw new Error("unexpected automatic device access");
      },
    });
  });
  await page.goto(`/conferences/${room.id}/join`);
  await expect(page.getByRole("heading", { name: room.title })).toBeVisible();
  await expect(
    page.getByText("Камера выключена", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Войти во встречу" }),
  ).toBeEnabled();
  await page.screenshot({
    path: info.outputPath("prejoin-desktop.png"),
    fullPage: true,
  });
});
