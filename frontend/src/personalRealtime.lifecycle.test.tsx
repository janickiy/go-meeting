import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import { usePersonalRealtime } from "./personalRealtime";
import type { DirectConversation } from "./types";
import { DirectConversationActions } from "./components/DirectConversationActions";

vi.mock("./auth", () => ({
  useAuth: () => ({ user: { id: "alice", displayName: "Алиса" } }),
}));

const initial: DirectConversation = {
  id: "direct",
  type: "direct",
  peer: { id: "bob", displayName: "Борис" },
  createdAt: "2026-10-08T10:00:00Z",
  lastMessageAt: null,
  lastMessageId: null,
  preview: "Старое превью",
  unreadCount: 3,
  notificationsEnabled: true,
  historyClearedThrough: 0,
};
class Socket {
  static latest: Socket;
  onopen: (() => void) | null = null;
  onmessage: ((message: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
  constructor() {
    Socket.latest = this;
  }
}
const clients: QueryClient[] = [];
function emit(type: string, data: Record<string, unknown>) {
  act(() =>
    Socket.latest.onmessage?.({
      data: JSON.stringify({
        type,
        data: { conversationId: "direct", type: "direct", ...data },
      }),
    }),
  );
}
function message(id: string, sequence = "10") {
  return {
    id,
    conversationId: "direct",
    sequence,
    senderId: "bob",
    senderName: "Борис",
    text: "Новое сообщение",
    version: 1,
  };
}
function setup(path = "/app") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  client.setQueryData(["personal-detail", "direct", "alice"], {
    status: "success",
    item: initial,
  });
  client.setQueryData(["personal-list", "alice", {}], {
    pages: [{ items: [initial], unreadCount: 3 }],
    pageParams: [undefined],
  });
  client.setQueryData(["personal-chat", "direct", "alice"], {
    pages: [{ items: [{ id: "old" }] }],
    pageParams: [undefined],
  });
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>{children}</MemoryRouter>
    </QueryClientProvider>
  );
  return { client, wrapper };
}
beforeEach(() => {
  if (Socket.latest) Socket.latest.onmessage = null;
  vi.stubGlobal("WebSocket", Socket);
  vi.spyOn(api, "userWSTicket").mockResolvedValue({
    ticket: "ticket",
    expiresAt: "2026-10-09T01:00:00Z",
  });
  vi.spyOn(api, "personalConversations").mockResolvedValue({
    status: "success",
    items: [initial],
    unreadCount: 3,
    nextCursor: null,
  });
});
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
describe("личные настройки в потоке событий", () => {
  it("публикует состояние существующего соединения и игнорирует позднее открытие после выхода", async () => {
    const { client, wrapper } = setup();
    const { unmount } = renderHook(() => usePersonalRealtime("alice"), {
      wrapper,
    });
    await waitFor(() => expect(Socket.latest.onopen).toBeTypeOf("function"));
    expect(client.getQueryData(["personal-connection", "alice"])).toBe(
      "connecting",
    );
    const lateOpen = Socket.latest.onopen!;
    act(() => lateOpen());
    expect(client.getQueryData(["personal-connection", "alice"])).toBe(
      "connected",
    );
    act(() => Socket.latest.onclose?.());
    expect(client.getQueryData(["personal-connection", "alice"])).toBe(
      "reconnecting",
    );
    unmount();
    expect(
      client.getQueryData(["personal-connection", "alice"]),
    ).toBeUndefined();
    act(() => lateOpen());
    expect(
      client.getQueryData(["personal-connection", "alice"]),
    ).toBeUndefined();
  });
  it("точный порог HTTP-удаления блокирует уже отправленный старый кадр после подтверждения", async () => {
    const { client, wrapper } = setup();
    vi.spyOn(api, "hidePersonalConversation").mockResolvedValue({
      status: "success",
      hidden: true,
      historyClearedThrough: 10,
    });
    function View() {
      const stream = usePersonalRealtime("alice");
      return (
        <>
          <DirectConversationActions conversation={initial} />
          <output data-testid="notice">{stream.notice?.text || ""}</output>
        </>
      );
    }
    render(<View />, { wrapper });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    fireEvent.click(
      screen.getByRole("button", { name: "Действия с перепиской: Борис" }),
    );
    const menu = await screen.findByRole("menu");
    fireEvent.click(
      within(menu).getByRole("menuitem", { name: "Удалить чат" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Удалить чат?" });
    vi.mocked(api.personalConversations).mockResolvedValue({
      status: "success",
      items: [],
      unreadCount: 0,
      nextCursor: null,
    });
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Удалить чат" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      client.getQueryData(["personal-history-cutoff", "direct", "alice"]),
    ).toBe(10);
    expect(
      client.getQueryData(["personal-detail", "direct", "alice"]),
    ).toBeUndefined();
    emit("message.created", {
      message: message("already-sent-before-hide", "8"),
      notificationsEnabled: true,
      historyClearedThrough: 0,
    });
    expect(screen.getByTestId("notice")).toHaveTextContent(/^$/);
  });
  it("скрытие без кеша деталей запоминает порог и блокирует задержанный старый кадр", async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => usePersonalRealtime("alice"), {
      wrapper,
    });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    client.removeQueries({ queryKey: ["personal-detail", "direct", "alice"] });
    emit("conversation.hidden", { hidden: true, historyClearedThrough: 10 });
    expect(
      client.getQueryData(["personal-history-cutoff", "direct", "alice"]),
    ).toBe(10);
    emit("message.created", {
      message: message("old-hidden", "8"),
      notificationsEnabled: true,
      historyClearedThrough: 0,
    });
    expect(result.current.notice).toBeNull();
    emit("message.created", {
      message: { ...message("unknown-hidden"), sequence: undefined },
      notificationsEnabled: true,
    });
    expect(result.current.notice).toBeNull();
  });
  it("личные метаданные нового сообщения повышают порог без полного снимка диалога", async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => usePersonalRealtime("alice"), {
      wrapper,
    });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    emit("message.created", {
      message: message("new-after-missed-clear", "12"),
      notificationsEnabled: true,
      historyClearedThrough: 10,
    });
    expect(
      client.getQueryData(["personal-chat", "direct", "alice"]),
    ).toBeUndefined();
    expect(
      client.getQueryData(["personal-history-cutoff", "direct", "alice"]),
    ).toBe(10);
    await waitFor(() => expect(result.current.notice).not.toBeNull());
    act(() => result.current.dismiss());
    emit("message.created", {
      message: message("old-after-missed-clear", "8"),
      notificationsEnabled: true,
      historyClearedThrough: 0,
    });
    expect(result.current.notice).toBeNull();
  });
  it("не показывает уведомление при выключенном или неизвестном разрешении, но обновляет историю и счётчик", async () => {
    const { client, wrapper } = setup();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => usePersonalRealtime("alice"), {
      wrapper,
    });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    await waitFor(() => expect(result.current.unread).toBe(3));
    emit("message.created", {
      message: message("muted"),
      notificationsEnabled: false,
    });
    expect(result.current.notice).toBeNull();
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["personal-chat", "direct"],
    });
    expect(result.current.unread).toBe(3);
    emit("message.created", { message: message("unknown") });
    expect(result.current.notice).toBeNull();
    emit("message.created", {
      message: message("enabled"),
      notificationsEnabled: true,
    });
    await waitFor(() =>
      expect(result.current.notice?.text).toBe("Новое сообщение"),
    );
  });
  it("синхронизирует настройку из другой вкладки и убирает уже открытое уведомление", async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => usePersonalRealtime("alice"), {
      wrapper,
    });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    emit("message.created", {
      message: message("enabled"),
      notificationsEnabled: true,
    });
    await waitFor(() => expect(result.current.notice).not.toBeNull());
    emit("conversation.preferences.updated", {
      item: { ...initial, notificationsEnabled: false },
    });
    await waitFor(() => expect(result.current.notice).toBeNull());
    expect(
      client.getQueryData<{ item: DirectConversation }>([
        "personal-detail",
        "direct",
        "alice",
      ])?.item.notificationsEnabled,
    ).toBe(false);
  });
  it("свежее разрешение WS включает уведомление вопреки старому кешированному отключению", async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => usePersonalRealtime("alice"), {
      wrapper,
    });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    client.setQueryData(["personal-detail", "direct", "alice"], {
      status: "success",
      item: { ...initial, notificationsEnabled: false },
    });
    emit("message.created", {
      message: message("fresh-unmuted"),
      notificationsEnabled: true,
    });
    await waitFor(() =>
      expect(result.current.notice?.text).toBe("Новое сообщение"),
    );
    expect(
      client.getQueryData<{ item: DirectConversation }>([
        "personal-detail",
        "direct",
        "alice",
      ])?.item.notificationsEnabled,
    ).toBe(true);
    emit("message.created", {
      message: message("fresh-muted"),
      notificationsEnabled: false,
    });
    await waitFor(() => expect(result.current.notice).toBeNull());
  });
  it("убирает старую историю после события очистки и отсекает задержанные сообщения", async () => {
    const { client, wrapper } = setup();
    const { result } = renderHook(() => usePersonalRealtime("alice"), {
      wrapper,
    });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    const cleared = {
      ...initial,
      historyClearedThrough: 10,
      unreadCount: 0,
      preview: "",
    };
    emit("conversation.history.cleared", {
      item: cleared,
      historyClearedThrough: 10,
    });
    expect(
      client.getQueryData(["personal-chat", "direct", "alice"]),
    ).toBeUndefined();
    expect(
      client.getQueryData(["personal-thread-version", "direct", "alice"]),
    ).toBe(1);
    emit("conversation.history.cleared", {
      item: cleared,
      historyClearedThrough: 10,
    });
    expect(
      client.getQueryData(["personal-thread-version", "direct", "alice"]),
    ).toBe(1);
    emit("message.created", {
      message: message("old", "9"),
      notificationsEnabled: true,
    });
    expect(result.current.notice).toBeNull();
    emit("message.created", {
      message: message("new", "11"),
      notificationsEnabled: true,
    });
    await waitFor(() => expect(result.current.notice).not.toBeNull());
  });
  it("обновлённые настройки с новым порогом очищают старый кеш даже без события самой очистки", async () => {
    const { client, wrapper } = setup();
    renderHook(() => usePersonalRealtime("alice"), { wrapper });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    emit("conversation.preferences.updated", {
      item: {
        ...initial,
        historyClearedThrough: 10,
        unreadCount: 0,
        preview: "",
      },
    });
    expect(
      client.getQueryData(["personal-chat", "direct", "alice"]),
    ).toBeUndefined();
    expect(
      client.getQueryData(["personal-thread-version", "direct", "alice"]),
    ).toBe(1);
  });
  it("после удаления в другой вкладке закрывает выбранный чат и очищает приватные кеши", async () => {
    const { client, wrapper } = setup("/personal/direct");
    function View() {
      usePersonalRealtime("alice");
      return <output>{useLocation().pathname}</output>;
    }
    render(<View />, { wrapper });
    await waitFor(() => expect(Socket.latest.onmessage).toBeTypeOf("function"));
    emit("conversation.hidden", { hidden: true, historyClearedThrough: 10 });
    await waitFor(() =>
      expect(screen.getByText("/personal")).toBeInTheDocument(),
    );
    expect(
      client.getQueryData(["personal-detail", "direct", "alice"]),
    ).toBeUndefined();
    expect(
      client.getQueryData(["personal-chat", "direct", "alice"]),
    ).toBeUndefined();
  });
});
