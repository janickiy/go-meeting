import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "./api";
import type { RealtimeEvent, RealtimeState } from "./types";

export type ClientRealtimeType =
  | "webrtc.offer"
  | "webrtc.answer"
  | "webrtc.ice"
  | "media.join"
  | "media.offer"
  | "media.answer"
  | "media.ready"
  | "media.ice"
  | "media.leave"
  | "media.unpublish";

export function websocketURL(
  conferenceId: string,
  ticket: string,
  origin = window.location.origin,
) {
  const url = new URL(
    `/api/v1/conferences/${encodeURIComponent(conferenceId)}/ws`,
    origin,
  );
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.searchParams.set("ticket", ticket);
  return url.toString();
}
export function parseRealtime(
  raw: string,
  conferenceId: string,
): RealtimeEvent | null {
  try {
    if (raw.length > 1048576) return null;
    const e = JSON.parse(raw) as RealtimeEvent;
    if (
      e.version !== 1 ||
      e.conferenceId !== conferenceId ||
      typeof e.id !== "string" ||
      typeof e.type !== "string" ||
      typeof e.timestamp !== "string"
    )
      return null;
    return e;
  } catch {
    return null;
  }
}

export function useRealtime(conferenceId: string, enabled: boolean) {
  const queryClient = useQueryClient();
  const [state, setState] = useState<RealtimeState | null>(null);
  const [status, setStatus] = useState("Отключено");
  const [error, setError] = useState<Error | null>(null);
  const [events, setEvents] = useState<string[]>([]);
  const [generation, setGeneration] = useState(0);
  const socket = useRef<WebSocket | null>(null);
  const listener = useRef<(event: RealtimeEvent) => void>(() => {});
  useEffect(() => {
    setState(null);
    setError(null);
    setEvents([]);
    if (!enabled) {
      setStatus("Отключено");
      return;
    }
    let disposed = false;
    let retry = 0;
    let rosterKey = "";
    let timer: ReturnType<typeof setTimeout> | undefined;
    const abort = new AbortController();
    const invalidate = () => {
      void queryClient.invalidateQueries({
        queryKey: ["conference"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["participants"],
      });
    };
    const connect = async () => {
      if (disposed) return;
      setStatus(retry ? "Переподключение…" : "Подключение…");
      try {
        const { ticket } = await api.wsTicket(conferenceId, abort.signal);
        if (disposed) return;
        const ws = new WebSocket(
          websocketURL(conferenceId, ticket),
          "go-recorder.v1",
        );
        socket.current = ws;
        ws.onopen = () => {
          if (!disposed) {
            setStatus("Подключено");
            setError(null);
          }
        };
        ws.onmessage = (message) => {
          if (disposed || typeof message.data !== "string") return;
          const e = parseRealtime(message.data, conferenceId);
          if (!e) return;
          // Show only event names; never display full SDP/ICE/tickets.
          setEvents((old) => [e.type, ...old].slice(0, 8));
          if (
            e.type.startsWith("participant.") &&
            !["participant.connected", "participant.disconnected"].includes(
              e.type,
            )
          )
            invalidate();
          if (e.type.startsWith("recording."))
            void queryClient.invalidateQueries({ queryKey: ["recordings"] });
          if (
            [
              "conference.state",
              "participant.connected",
              "participant.disconnected",
            ].includes(e.type)
          ) {
            const next = e.data as RealtimeState;
            if (
              typeof next?.connectionId === "string" &&
              Array.isArray(next.participants)
            ) {
              setState(next);
              retry = 0;
              const key =
                next.status +
                next.participants
                  .map(
                    (p) =>
                      `${p.id}:${p.status}:${p.role}:${p.mediaPolicyVersion}`,
                  )
                  .join(",");
              if (key !== rosterKey) {
                rosterKey = key;
                invalidate();
              }
            }
          }
          listener.current(e);
        };
        ws.onerror = () => {
          if (!disposed)
            setError(new Error("Не удалось подключиться к realtime-сервису."));
        };
        ws.onclose = (event) => {
          if (socket.current === ws) socket.current = null;
          if (disposed) return;
          setState(null);
          setStatus("Связь потеряна");
          invalidate();
          if (
            event.code === 1008 &&
            ![
              "broker_unavailable",
              "slow_client",
              "state_unavailable",
            ].includes(event.reason)
          ) {
            if (event.reason === "authentication_expired")
              void api.me().catch(() => {});
            setError(
              new Error("Подключение закрыто. Проверьте доступ к конференции."),
            );
            return;
          }
          timer = setTimeout(
            () => void connect(),
            Math.min(30000, 1000 * 2 ** Math.min(retry++, 5)),
          );
        };
      } catch (cause) {
        if (disposed) return;
        setStatus("Не подключено");
        setError(
          cause instanceof Error ? cause : new Error("Ошибка подключения"),
        );
        if (
          cause instanceof ApiError &&
          [401, 403, 409].includes(cause.status)
        ) {
          invalidate();
          return;
        }
        timer = setTimeout(
          () => void connect(),
          Math.min(30000, 1000 * 2 ** Math.min(retry++, 5)),
        );
      }
    };
    void connect();
    return () => {
      disposed = true;
      abort.abort();
      clearTimeout(timer);
      const ws = socket.current;
      socket.current = null;
      if (ws) {
        ws.onmessage = null;
        ws.onclose = null;
        ws.onerror = null;
        ws.onopen = null;
        ws.close(1000, "page_closed");
      }
    };
  }, [conferenceId, enabled, generation, queryClient]);
  const send = (type: ClientRealtimeType, data: unknown) => {
    const ws = socket.current;
    if (!ws || ws.readyState !== WebSocket.OPEN || ws.bufferedAmount > 131072)
      throw new Error("Realtime-соединение недоступно или перегружено.");
    const e: RealtimeEvent = {
      version: 1,
      id: crypto.randomUUID(),
      type,
      conferenceId,
      timestamp: new Date().toISOString(),
      data,
    };
    ws.send(JSON.stringify(e));
    return e.id;
  };
  return {
    state,
    status,
    error,
    events,
    send,
    onEvent: listener,
    reconnect: () => setGeneration((n) => n + 1),
  };
}
