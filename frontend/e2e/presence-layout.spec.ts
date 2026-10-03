import { expect, test, type Page, type WebSocketRoute } from "@playwright/test";
import type {
  ChatMessage,
  Participant,
  PresenceParticipant,
} from "../src/types";

const stamp = "2026-10-03T12:00:00Z";
const room = {
  id: "room-presence",
  title: "Проверка присутствия",
  status: "active",
  ownerId: "owner",
  createdAt: stamp,
  updatedAt: stamp,
  startedAt: null,
  finishedAt: null,
  inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWX12345678",
  waitingRoomEnabled: true,
};

/** Создаёт сохранённое членство без предположения о физическом присутствии.
 * @args id — членство; displayName — имя; changes — особые роль и состояние допуска.
 * @return Полный участник для тестовых HTTP-ответов.
 */
function member(
  id: string,
  displayName: string,
  changes: Partial<Participant> = {},
): Participant {
  return {
    id,
    userId: id,
    conferenceId: room.id,
    displayName,
    role: "participant",
    status: "joined",
    admissionState: "admitted",
    createdAt: stamp,
    updatedAt: stamp,
    joinedAt: stamp,
    leftAt: null,
    microphoneEnabled: false,
    cameraEnabled: false,
    ...changes,
  };
}

const owner = member("owner", "Алиса", { role: "owner" });
const bob = member("bob", "Борис Волков");
const vera = member("vera", "Вера Отключённая");
const waiting = member("waiting", "Глеб Ожидающий", {
  status: "waiting",
  admissionState: "waiting",
  joinedAt: null,
});
const people = [owner, bob, vera, waiting];
const previousMessage: ChatMessage = {
  id: "past-bob-message",
  sequence: "1",
  conferenceId: room.id,
  senderId: bob.userId!,
  senderName: bob.displayName,
  text: "Моё сообщение должно остаться после отключения.",
  replyTo: null,
  replyPreview: null,
  createdAt: stamp,
  updatedAt: stamp,
  deletedAt: null,
  version: 1,
  attachments: [],
};

/** Дополняет членство серверным подтверждением числа физических подключений.
 * @args person — членство; connections — число живых вкладок, ноль означает offline.
 * @return Авторитетный серверный снимок, независимый от сохранённого joined-статуса.
 */
function present(
  person: Participant,
  connections: number,
): PresenceParticipant {
  return {
    ...person,
    online: connections > 0,
    connections,
    connectionIds: Array.from(
      { length: connections },
      (_, index) => `connection-${person.id}-${index}`,
    ),
  };
}

/** Подменяет только ответы тестового сервера и управляет реальными WS-событиями настоящей страницы.
 * @args page — изолированная страница браузера.
 * @return Команда отправки нового присутствия Бориса; HTTP-членства и сохранённый чат не меняются.
 */
async function fixture(page: Page) {
  await page.addInitScript(() => {
    const runtime = window as Window & { presenceDeviceCalls?: number };
    runtime.presenceDeviceCalls = 0;
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "presence-only-token",
        expiresAt: Date.now() + 1_800_000,
      }),
    );
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: () => {
        runtime.presenceDeviceCalls = (runtime.presenceDeviceCalls ?? 0) + 1;
        throw new Error("Тест присутствия не должен запрашивать устройства.");
      },
    });
  });
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace("/api/v1", "");
    const respond = (data: unknown) => route.fulfill({ json: data });
    if (path === "/auth/me")
      return respond({
        status: "success",
        user: {
          id: owner.userId,
          email: "owner@example.test",
          displayName: owner.displayName,
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
        body: ": presence fixture\n\n",
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
      return respond({ status: "success", item: owner });
    if (path.endsWith("/participants/me/media"))
      return respond({ status: "success", item: owner });
    if (path.endsWith("/participants"))
      return respond({ status: "success", items: people, nextCursor: null });
    if (path.endsWith("/recordings"))
      return respond({ status: "success", items: [] });
    if (path.endsWith("/messages"))
      return respond({
        status: "success",
        items: [previousMessage],
        nextCursor: null,
      });
    if (path.endsWith("/chat/read"))
      return respond({
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: previousMessage.id },
      });
    if (path.endsWith("/ws-ticket"))
      return respond({
        ticket: "presence-ticket",
        expiresAt: "2099-01-01T00:00:00Z",
      });
    return route.fulfill({
      status: 404,
      json: { message: "Неизвестный тестовый маршрут" },
    });
  });
  let socket: WebSocketRoute | undefined;
  let eventSequence = 0;
  /** Передаёт авторитетный снимок без изменений сохранённого состава и сообщений.
   * @args connections — оставшиеся физические подключения Бориса; type — тип события серверного конверта.
   */
  const emit = (connections: number, type = "participant.connected") => {
    if (!socket) throw new Error("WebSocket страницы ещё не подключён.");
    socket.send(
      JSON.stringify({
        version: 1,
        id: `presence-${++eventSequence}`,
        type,
        conferenceId: room.id,
        timestamp: stamp,
        data: {
          connectionId: "connection-owner-0",
          participantId: owner.id,
          status: "active",
          participants: [
            present(owner, 1),
            present(bob, connections),
            present(vera, 0),
            present(waiting, 0),
          ],
        },
      }),
    );
  };
  await page.routeWebSocket(
    /\/api\/v1\/conferences\/room-presence\/ws/,
    (ws) => {
      socket = ws;
      emit(1, "conference.state");
    },
  );
  return emit;
}

test("живой состав скрывает offline без удаления чата и возвращает участника по новому снимку", async ({
  page,
}, info) => {
  const emit = await fixture(page);
  const mediaRequests: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (
      /\/media(?:\/|$)/.test(path) &&
      !path.endsWith("/participants/me/media")
    )
      mediaRequests.push(path);
  });
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByRole("heading", { name: room.title })).toBeVisible();
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(2);
  await expect(
    page.getByRole("tab", { name: "Участники (2)", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Участники (2)", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByTestId(`chat-message-${previousMessage.id}`),
  ).toContainText(previousMessage.text);
  await page.getByRole("tab", { name: "Участники (2)", exact: true }).click();
  const list = page.getByRole("region", {
    name: "Участники встречи",
    exact: true,
  });
  await expect(list.getByText("Алиса (вы)", { exact: true })).toBeVisible();
  await expect(list.getByText(bob.displayName, { exact: true })).toBeVisible();
  await expect(list.getByText(vera.displayName, { exact: true })).toHaveCount(
    0,
  );
  await expect(
    list.getByText(waiting.displayName, { exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", {
      name: `Допустить: ${waiting.displayName}`,
      exact: true,
    }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("presence-online.png"),
    fullPage: true,
  });

  emit(0, "participant.disconnected");
  await expect(
    page.getByRole("tab", { name: "Участники (1)", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Участники (1)", exact: true }),
  ).toBeVisible();
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(1);
  await expect(list.getByText(bob.displayName, { exact: true })).toHaveCount(0);
  await expect(page.getByLabel(`Управление: ${bob.displayName}`)).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("button", {
      name: `Допустить: ${waiting.displayName}`,
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Чат", exact: true }).click();
  await expect(
    page.getByTestId(`chat-message-${previousMessage.id}`),
  ).toContainText(previousMessage.text);
  await page.screenshot({
    path: info.outputPath("presence-offline-chat-retained.png"),
    fullPage: true,
  });

  emit(2);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(2);
  await expect(
    page.getByRole("tab", { name: "Участники (2)", exact: true }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Участники (2)", exact: true }).click();
  await expect(list.getByText(bob.displayName, { exact: true })).toHaveCount(1);
  emit(1, "participant.disconnected");
  await expect(
    page.getByRole("tab", { name: "Участники (2)", exact: true }),
  ).toBeVisible();
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(2);
  await expect(list.getByText(bob.displayName, { exact: true })).toHaveCount(1);
  await expect(list.getByText(vera.displayName, { exact: true })).toHaveCount(
    0,
  );
  expect(mediaRequests).toEqual([]);
  expect(
    await page.evaluate(
      () =>
        (window as Window & { presenceDeviceCalls?: number })
          .presenceDeviceCalls,
    ),
  ).toBe(0);
});
