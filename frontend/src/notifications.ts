import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "./api";

export interface NotificationEvent {
  version: 1;
  id: string;
  type: "notification.created" | "notification.read";
  data: { notification?: { payload?: { conferenceId?: string } }; id?: string };
}
export function takeNotificationFrames(buffer: string): {
  events: NotificationEvent[];
  rest: string;
} {
  const normalized = buffer.replace(/\r\n/g, "\n");
  const blocks = normalized.split("\n\n");
  const rest = blocks.pop() || "";
  const events: NotificationEvent[] = [];
  for (const block of blocks) {
    const raw = block
      .split("\n")
      .filter((line) => line.startsWith("data:"))
      .map((line) => line.slice(5).trimStart())
      .join("\n");
    if (!raw) continue;
    try {
      const event = JSON.parse(raw) as NotificationEvent;
      if (
        event.version === 1 &&
        typeof event.id === "string" &&
        ["notification.created", "notification.read"].includes(event.type) &&
        event.data &&
        typeof event.data === "object"
      )
        events.push(event);
    } catch {
      /* A malformed noncritical notification must not affect media. */
    }
  }
  return { events, rest };
}

// One authenticated fetch stream per mounted account layout. Never send a JWT
// in a query string; regular list polling reconciles missed events after reconnect.
export function useNotificationStream(userId?: string) {
  const client = useQueryClient();
  useEffect(() => {
    if (!userId) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let retry = 0;
    const seen = new Set<string>();
    const connect = async () => {
      try {
        const response = await api.notificationEvents(controller.signal);
        if (controller.signal.aborted) return;
        const reader = response.body!.getReader();
        const decoder = new TextDecoder();
        let pending = "";
        retry = 0;
        void client.invalidateQueries({ queryKey: ["notifications", userId] });
        try {
          while (!controller.signal.aborted) {
            const next = await reader.read();
            if (next.done) break;
            pending += decoder.decode(next.value, { stream: true });
            if (pending.length > 131072)
              throw new Error("notification_frame_too_large");
            const parsed = takeNotificationFrames(pending);
            pending = parsed.rest;
            for (const event of parsed.events) {
              const eventKey = `${event.type}:${event.id}`;
              if (seen.has(eventKey)) continue;
              seen.add(eventKey);
              if (seen.size > 256) seen.delete(seen.values().next().value!);
              void client.invalidateQueries({
                queryKey: ["notifications", userId],
              });
              const conferenceId =
                event.data.notification?.payload?.conferenceId;
              if (typeof conferenceId === "string") {
                void client.invalidateQueries({
                  queryKey: ["membership", userId, conferenceId],
                });
                void client.invalidateQueries({
                  queryKey: ["conference", userId, conferenceId],
                });
                void client.invalidateQueries({
                  queryKey: ["conferences", userId],
                });
                void client.invalidateQueries({
                  queryKey: ["recordings", conferenceId],
                });
              }
            }
          }
        } finally {
          await reader.cancel().catch(() => {});
          reader.releaseLock();
        }
      } catch {
        /* List polling remains available while the stream reconnects. */
      }
      if (!controller.signal.aborted)
        timer = setTimeout(
          () => void connect(),
          Math.min(30000, 1000 * 2 ** Math.min(retry++, 5)),
        );
    };
    void connect();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [userId, client]);
}
