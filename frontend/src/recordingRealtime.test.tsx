import type { ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "./api";
import { useRealtime } from "./realtime";

/**
 * Подменяет единственный транспорт комнаты и позволяет проверять события без сетевых соединений.
 * @params sockets — созданные соединения; onopen/onmessage — обработчики проверяемого хука.
 */
class RoomSocket {
  static OPEN = 1;
  static sockets: RoomSocket[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  readyState = 1;
  bufferedAmount = 0;
  close = vi.fn();
  send = vi.fn();
  /** Сохраняет изолированное соединение, не открывая сеть. */
  constructor() {
    RoomSocket.sockets.push(this);
  }
}

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.forEach((client) => client.clear());
  clients.length = 0;
  RoomSocket.sockets = [];
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

/** Монтирует реальный хук комнаты с изолированным клиентом кеша и подменённым WebSocket. */
function room() {
  vi.stubGlobal("WebSocket", RoomSocket);
  vi.spyOn(api, "wsTicket").mockResolvedValue({
    ticket: "isolated-ticket",
    expiresAt: "2026-10-04T20:00:00Z",
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const hook = renderHook(() => useRealtime("room", true), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    ),
  });
  return { ...hook, invalidate };
}

it("сверяет запись при первом подключении и после переподключения", async () => {
  const { result, invalidate } = room();
  await waitFor(() => expect(RoomSocket.sockets).toHaveLength(1));
  act(() => RoomSocket.sockets[0].onopen?.());
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["recordings", "room"] });
  invalidate.mockClear();
  act(() => result.current.reconnect());
  await waitFor(() => expect(RoomSocket.sockets).toHaveLength(2));
  act(() => RoomSocket.sockets[1].onopen?.());
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["recordings", "room"] });
});

it("передаёт событие подписчикам и обновляет только записи текущей комнаты", async () => {
  const { result, invalidate } = room();
  await waitFor(() => expect(RoomSocket.sockets).toHaveLength(1));
  const listener = vi.fn();
  const unsubscribe = result.current.subscribe(listener);
  const event = {
    version: 1,
    id: "event-id",
    type: "recording.started",
    conferenceId: "room",
    timestamp: new Date().toISOString(),
    data: {
      recordingId: "record",
      conferenceId: "room",
      requestedBy: "owner",
      status: "recording",
    },
  };
  act(() => RoomSocket.sockets[0].onmessage?.({ data: JSON.stringify(event) }));
  expect(listener).toHaveBeenCalledWith(event);
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["recordings", "room"] });
  expect(invalidate).not.toHaveBeenCalledWith({ queryKey: ["recordings"] });
  invalidate.mockClear();
  listener.mockClear();
  act(() =>
    RoomSocket.sockets[0].onmessage?.({
      data: JSON.stringify({ ...event, conferenceId: "foreign" }),
    }),
  );
  expect(listener).not.toHaveBeenCalled();
  expect(invalidate).not.toHaveBeenCalled();
  unsubscribe();
  act(() => RoomSocket.sockets[0].onmessage?.({ data: JSON.stringify(event) }));
  expect(listener).not.toHaveBeenCalled();
});
