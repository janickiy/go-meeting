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

vi.mock("../auth", () => ({
  useAuth: () => ({ user: { id: "user", displayName: "Test" } }),
}));
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
beforeEach(() => {
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
});
afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.restoreAllMocks();
});

describe("waiting room", () => {
  it("allows only a joined moderator to decide and excludes withdrawn requests", async () => {
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
    await waitFor(() =>
      expect(admit).toHaveBeenCalledWith("room", "waiting", "admit"),
    );
  });
  it("has no private room controls and correctly explains a closed waiting request", () => {
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
describe("persistent chat", () => {
  it("keeps draft on network failure and retries with the same idempotency key", async () => {
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
    await waitFor(() => expect(send).toHaveBeenCalledTimes(2));
    expect(send.mock.calls[0][1].clientRequestId).toBe(
      send.mock.calls[1][1].clientRequestId,
    );
    expect(send.mock.calls[0][1].text).toBe("Retry safely");
    await waitFor(() =>
      expect(screen.getByLabelText("Сообщение")).toHaveValue(""),
    );
  });
  it("supports reply/edit/delete with server-confirmed mutations", async () => {
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
    fireEvent.click(screen.getByRole("button", { name: "Ответить" }));
    expect(screen.getByText("Ответ: Test")).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Изменить" }));
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Текст"), {
      target: { value: "Edited" },
    });
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Сохранить сообщение" }),
    );
    await waitFor(() =>
      expect(edit).toHaveBeenCalledWith("room", "message", "Edited"),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "Удалить" }));
    fireEvent.click(
      screen.getByRole("button", { name: "Да, удалить сообщение" }),
    );
    await waitFor(() => expect(remove).toHaveBeenCalledWith("room", "message"));
  });
  it("renders finished history as plain text with no write or moderation controls", async () => {
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
describe("notifications", () => {
  it("shows unread count and persists explicit acknowledgement", async () => {
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
    await screen.findByLabelText("1 непрочитанных уведомлений");
    fireEvent.click(screen.getByRole("button", { name: "Уведомления" }));
    expect(screen.getByText("Запись встречи готова")).toBeVisible();
    fireEvent.click(
      screen.getByRole("button", { name: "Прочитано: Запись встречи готова" }),
    );
    await waitFor(() =>
      expect(read).toHaveBeenCalledWith("notification", expect.anything()),
    );
  });
});
