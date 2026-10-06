import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation } from "react-router";
import { api } from "./api";
import type { ChatMessage } from "./types";

export interface PersonalEvent {
  type: string;
  data: { conversationId?: string; type?: string; message?: ChatMessage };
}
// A bounded identity/version window suppresses duplicate notices, including send retries.
export function acceptPersonalEvent(
  raw: string,
  seen: Map<string, number>,
): PersonalEvent | null {
  let event: PersonalEvent;
  try {
    event = JSON.parse(raw) as PersonalEvent;
  } catch {
    return null;
  }
  if (
    !event ||
    ![
      "message.created",
      "message.updated",
      "message.deleted",
      "conversation.updated",
      "conversation.read.updated",
    ].includes(event.type) ||
    event.data?.type !== "direct" ||
    !event.data.conversationId
  )
    return null;
  const m = event.data.message;
  if (event.type.startsWith("message.")) {
    if (
      !m?.id ||
      m.conversationId !== event.data.conversationId ||
      !Number.isSafeInteger(m.version) ||
      m.version < 1
    )
      return null;
    if ((seen.get(m.id) ?? 0) >= m.version) return null;
    seen.delete(m.id);
    seen.set(m.id, m.version);
    if (seen.size > 256) seen.delete(seen.keys().next().value!);
  }
  return event;
}
export function usePersonalRealtime(userId?: string) {
  const client = useQueryClient();
  const location = useLocation();
  const pathname = useRef(location.pathname);
  pathname.current = location.pathname;
  const [notice, setNotice] = useState<{
    id: string;
    name: string;
    text: string;
  } | null>(null);
  const summary = useQuery({
    queryKey: ["personal-summary", userId],
    queryFn: ({ signal }) => api.personalConversations(undefined, signal, 1),
    enabled: !!userId,
    refetchInterval: 30000,
  });
  useEffect(() => {
    if (!notice) return;
    const timeout = setTimeout(() => setNotice(null), 6000);
    return () => clearTimeout(timeout);
  }, [notice]);
  useEffect(() => {
    if (!userId) return;
    const controller = new AbortController();
    const seen = new Map<string, number>();
    let socket: WebSocket | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let attempts = 0;
    const refresh = () => {
      void client.invalidateQueries({ queryKey: ["personal-list"] });
      void client.invalidateQueries({ queryKey: ["personal-summary"] });
      void client.invalidateQueries({ queryKey: ["personal-detail"] });
    };
    const retry = () => {
      if (controller.signal.aborted) return;
      timer = setTimeout(
        () => void connect(),
        Math.min(30000, 1000 * 2 ** Math.min(attempts++, 5)) +
          Math.random() * 250,
      );
    };
    const connect = async () => {
      try {
        const ticket = await api.userWSTicket(controller.signal);
        if (controller.signal.aborted) return;
        const url = new URL("/api/v1/ws", window.location.origin);
        url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
        url.searchParams.set("ticket", ticket.ticket);
        socket = new WebSocket(url);
        socket.onopen = () => {
          attempts = 0;
          refresh();
          void client.invalidateQueries({ queryKey: ["personal-chat"] });
          void client.invalidateQueries({ queryKey: ["personal-chat-read"] });
        };
        socket.onmessage = ({ data }) => {
          if (typeof data !== "string") return;
          const event = acceptPersonalEvent(data, seen);
          if (!event) return;
          refresh();
          const id = event.data.conversationId!;
          void client.invalidateQueries({ queryKey: ["personal-chat", id] });
          void client.invalidateQueries({
            queryKey: ["personal-chat-read", id],
          });
          const m = event.data.message;
          if (
            event.type === "message.created" &&
            m &&
            m.senderId !== userId &&
            !(
              document.visibilityState === "visible" &&
              pathname.current === `/personal/${id}`
            )
          )
            setNotice({
              id,
              name: m.senderName,
              text: m.text.slice(0, 120) || "Вложение",
            });
        };
        socket.onerror = () => socket?.close();
        socket.onclose = retry;
      } catch {
        retry();
      }
    };
    void connect();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
      if (socket) {
        socket.onclose = null;
        socket.close();
      }
      setNotice(null);
    };
  }, [userId, client]);
  return {
    unread: summary.data?.unreadCount ?? 0,
    notice,
    dismiss: () => setNotice(null),
  };
}
