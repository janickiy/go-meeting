import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import type { ReactNode } from "react";
import { api } from "../api";
import type { ChatMessage, Participant } from "../types";
import { ChatPanel } from "./ChatPanel";
import { WaitingRoomPanel } from "./WaitingRoomPanel";
import { NotificationBell } from "./NotificationBell";

vi.mock(
  "../auth",
  /**
   * Обработчик vi.mock выполняет переданный шаг вызова vi.mock в проверках клиентского поведения.
   *
   *
   * @returns новый объект вычисленных данных.
   */ () => ({
    /**
     * useAuth возвращает авторизацию текущего React-контекста и сообщает об использовании вне провайдера.
     *
     *
     * @returns состояние, данные или действия React-хука; ресурсы освобождаются при изменении зависимостей.
     */
    useAuth: () => ({ user: { id: "user", displayName: "Test" } }),
  }),
);
const member = {
  id: "self",
  role: "owner",
  status: "joined",
  admissionState: "admitted",
} as Participant;
const waiting = {
  id: "waiting",
  displayName: "Bob",
  role: "participant",
  status: "waiting",
  admissionState: "waiting",
} as Participant;
const message = {
  id: "message",
  sequence: "1",
  conferenceId: "room",
  senderId: "user",
  senderName: "Test",
  text: "Original",
  createdAt: "2026-10-01T10:00:00Z",
  updatedAt: "2026-10-01T10:00:00Z",
  deletedAt: null,
  version: 1,
  attachments: [],
} as ChatMessage;
const clients: QueryClient[] = [];
/**
 * show монтирует проверяемый компонент с изолированными провайдерами.
 *
 * @args
 *   - child (ReactNode) — вложенный React-элемент тестового компонента.
 *
 * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
 */
function show(child: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>{child}</MemoryRouter>
    </QueryClientProvider>,
  );
}
beforeEach(
  /**
   * Обработчик beforeEach выполняет переданный шаг вызова beforeEach в проверках клиентского поведения.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    vi.spyOn(api, "messages").mockResolvedValue({
      status: "success",
      items: [message],
      nextCursor: null,
      unreadCount: 1,
      lastReadMessageId: null,
    });
    vi.spyOn(api, "chatRead").mockResolvedValue({
      status: "success",
      item: { lastReadMessageId: null, unreadCount: 1 },
    });
  },
);
afterEach(
  /**
   * Обработчик afterEach выполняет переданный шаг вызова afterEach в проверках клиентского поведения.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    cleanup();
    for (const client of clients.splice(0)) client.clear();
    vi.restoreAllMocks();
  },
);

describe("waiting room", /**
 * Проверяет зал ожидания.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("allows only a joined moderator to decide and excludes withdrawn requests", /**
   * Проверяет принятие решений только присоединившимся модератором и исключение отозванных заявок.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const admit = vi.spyOn(api, "admit").mockResolvedValue({
      status: "success",
      item: { ...waiting, status: "joined", admissionState: "admitted" },
    });
    show(
      <WaitingRoomPanel
        conferenceId="room"
        membership={member}
        participants={[
          waiting,
          { ...waiting, id: "left", displayName: "Withdrawn", status: "left" },
        ]}
        active
      />,
    );
    expect(screen.queryByText("Withdrawn")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Допустить: Bob" }));
    await waitFor(
      /**
       * Обработчик waitFor выполняет переданный шаг вызова waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(admit).toHaveBeenCalledWith("room", "waiting", "admit").
       */ () => expect(admit).toHaveBeenCalledWith("room", "waiting", "admit"),
    );
  });
  it("has no private room controls and correctly explains a closed waiting request", /**
   * Проверяет отсутствие приватных элементов комнаты и корректное пояснение закрытой заявки на допуск.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    show(
      <WaitingRoomPanel
        conferenceId="room"
        membership={waiting}
        participants={[]}
        active={false}
        closed
      />,
    );
    expect(screen.getByText("Встреча закрыта")).toBeVisible();
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByText(/скоро рассмотрит/)).toBeNull();
  });
});
describe("persistent chat", /**
 * Проверяет постоянный чат.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("keeps draft on network failure and retries with the same idempotency key", /**
   * Проверяет сохранение черновика при сетевом сбое и повтор с тем же ключом идемпотентности.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const send = vi
      .spyOn(api, "sendMessage")
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ status: "success", item: message });
    show(
      <ChatPanel conferenceId="room" membership={member} readOnly={false} />,
    );
    await screen.findByText("Original");
    fireEvent.change(screen.getByLabelText("Сообщение"), {
      target: { value: "Retry safely" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Отправить" }));
    fireEvent.click(
      await screen.findByRole("button", { name: "Повторить отправку" }),
    );
    await waitFor(
      /**
       * Обработчик waitFor выполняет переданный шаг вызова waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(send).toHaveBeenCalledTimes(2).
       */ () => expect(send).toHaveBeenCalledTimes(2),
    );
    expect(send.mock.calls[0][1].clientRequestId).toBe(
      send.mock.calls[1][1].clientRequestId,
    );
    expect(send.mock.calls[0][1].text).toBe("Retry safely");
    await waitFor(
      /**
       * Обработчик waitFor выполняет переданный шаг вызова waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(screen.getByLabelText("Сообщение")).toHaveValue("").
       */ () => expect(screen.getByLabelText("Сообщение")).toHaveValue(""),
    );
  });
  it("supports reply/edit/delete with server-confirmed mutations", /**
   * Проверяет ответы, редактирование и удаление с подтверждением изменений сервером.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const edit = vi.spyOn(api, "editMessage").mockResolvedValue({
      status: "success",
      item: { ...message, text: "Edited", version: 2 },
    });
    const remove = vi.spyOn(api, "deleteMessage").mockResolvedValue({
      status: "success",
      item: { ...message, deletedAt: "2026-10-01T10:01:00Z" },
    });
    show(
      <ChatPanel conferenceId="room" membership={member} readOnly={false} />,
    );
    await screen.findByText("Original");
    fireEvent.click(
      screen.getByRole("button", { name: /Действия с сообщением:/ }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Ответить" }));
    expect(screen.getByText("Ответ: Test")).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", { name: /Действия с сообщением:/ }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Изменить" }));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Текст"), {
      target: { value: "Edited" },
    });
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Сохранить сообщение" }),
    );
    await waitFor(
      /**
       * Обработчик waitFor выполняет переданный шаг вызова waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(edit).toHaveBeenCalledWith("room", "message", "Edited").
       */ () => expect(edit).toHaveBeenCalledWith("room", "message", "Edited"),
    );
    await waitFor(
      /**
       * Обработчик waitFor выполняет переданный шаг вызова waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(screen.queryByRole("dialog")).toBeNull().
       */ () => expect(screen.queryByRole("dialog")).toBeNull(),
    );
    fireEvent.click(
      screen.getByRole("button", { name: /Действия с сообщением:/ }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Удалить" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Да, удалить сообщение" }),
    );
    await waitFor(
      /**
       * Обработчик waitFor выполняет переданный шаг вызова waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(remove).toHaveBeenCalledWith("room", "message").
       */ () => expect(remove).toHaveBeenCalledWith("room", "message"),
    );
  });
  it("renders finished history as plain text with no write or moderation controls", /**
   * Проверяет показ завершённой истории обычным текстом без отправки сообщений и модерации.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    vi.mocked(api.messages).mockResolvedValue({
      status: "success",
      items: [{ ...message, text: "<img src=x onerror=alert(1)>" }],
      nextCursor: null,
      unreadCount: 0,
      lastReadMessageId: null,
    });
    show(<ChatPanel conferenceId="room" membership={member} readOnly />);
    await screen.findByText("<img src=x onerror=alert(1)>");
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.queryByLabelText("Сообщение")).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: /Отправить|Изменить|Удалить|Ответить|Прикрепить/,
      }),
    ).toBeNull();
  });
});
describe("notifications", /**
 * Проверка: notifications выполняет тестовый сценарий «notifications» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("shows unread count and persists explicit acknowledgement", /**
   * Проверяет счётчик непрочитанного и сохранение явного подтверждения прочтения.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const notification = {
      id: "notification",
      userId: "user",
      type: "recording.ready",
      version: 1 as const,
      payload: { conferenceId: "room" },
      createdAt: "2026-10-01T10:00:00Z",
      readAt: null,
    };
    vi.spyOn(api, "notifications").mockResolvedValue({
      status: "success",
      items: [notification],
      nextCursor: null,
      unreadCount: 1,
    });
    const read = vi.spyOn(api, "readNotification").mockResolvedValue({
      status: "success",
      item: { ...notification, readAt: "2026-10-01T10:01:00Z" },
    });
    show(<NotificationBell />);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Уведомления" }),
      ).toHaveAccessibleDescription("1 непрочитанных уведомлений"),
    );
    fireEvent.click(screen.getByRole("button", { name: "Уведомления" }));
    expect(screen.getByText("Запись встречи готова")).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", { name: "Прочитано: Запись встречи готова" }),
    );
    await waitFor(
      /**
       * Обработчик waitFor выполняет переданный шаг вызова waitFor в проверках клиентского поведения.
       *
       *
       * @returns вычисленное значение: expect(read).toHaveBeenCalledWith("notification", expect.anything()).
       */ () =>
        expect(read).toHaveBeenCalledWith("notification", expect.anything()),
    );
  });
});
