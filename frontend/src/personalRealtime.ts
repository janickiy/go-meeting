import { useEffect, useRef, useState } from "react";
import {
  useQuery,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
} from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router";
import { api } from "./api";
import { invalidateFolders, purgeFolder, purgeFolderTarget } from "./folders";
import {
  directConversationHistoryCutoff,
  observeDirectConversationNotifications,
  rememberDirectConversationHistoryCutoff,
  resetDirectConversationHistory,
  updateDirectConversation,
} from "./directConversations";
import type {
  ChatMessage,
  DirectConversation,
  PersonalConversation,
  PersonalPage,
} from "./types";

/** A revoke removes previews immediately, even when REST recovery is offline. */
export function revokePersonalConversation(
  client: QueryClient,
  id: string,
  userId: string,
) {
  void client.cancelQueries({ queryKey: ["personal-list", userId] });
  void client.cancelQueries({ queryKey: ["personal-summary", userId] });
  const cached = client.getQueriesData<InfiniteData<PersonalPage>>({
    queryKey: ["personal-list", userId],
  });
  let removedUnread =
    client.getQueryData<{ item: PersonalConversation }>([
      "personal-detail",
      id,
      userId,
    ])?.item.unreadCount || 0;
  for (const [, data] of cached)
    for (const page of data?.pages || [])
      for (const item of page.items)
        if (item.id === id)
          removedUnread = Math.max(removedUnread, item.unreadCount);
  client.setQueriesData<InfiniteData<PersonalPage>>(
    { queryKey: ["personal-list", userId] },
    (current) =>
      current
        ? {
            ...current,
            pages: current.pages.map((page) => ({
              ...page,
              items: page.items.filter((item) => item.id !== id),
              unreadCount: Math.max(0, page.unreadCount - removedUnread),
            })),
          }
        : current,
  );
  client.setQueryData<PersonalPage>(["personal-summary", userId], (current) =>
    current
      ? {
          ...current,
          items: current.items.filter((item) => item.id !== id),
          unreadCount: Math.max(0, current.unreadCount - removedUnread),
        }
      : current,
  );
  for (const key of [
    "personal-chat",
    "personal-chat-read",
    "personal-detail",
    "personal-peer-presence",
    "group-members",
  ]) {
    void client.cancelQueries({ queryKey: [key, id, userId] });
    client.removeQueries({ queryKey: [key, id, userId] });
  }
  purgeFolderTarget(client, userId, { type: "conversation", id });
}

export function acceptFolderEvent(
  raw: string,
): { type: string; data: { folderId?: string } } | null {
  try {
    const event = JSON.parse(raw);
    if (
      !event ||
      ![
        "folder.created",
        "folder.updated",
        "folder.deleted",
        "folder.items.updated",
        "folder.reordered",
      ].includes(event.type) ||
      !event.data ||
      typeof event.data !== "object" ||
      Array.isArray(event.data)
    )
      return null;
    if (
      event.type !== "folder.reordered" &&
      (typeof event.data.folderId !== "string" ||
        !event.data.folderId ||
        event.data.folderId.length > 128)
    )
      return null;
    return event;
  } catch {
    return null;
  }
}

export interface PersonalEvent {
  type: string;
  data: {
    conversationId?: string;
    type?: string;
    message?: ChatMessage;
    userId?: string;
    role?: string;
    notificationsEnabled?: boolean;
    historyClearedThrough?: number;
    hidden?: boolean;
    item?: DirectConversation;
  };
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
      "conversation.member.added",
      "conversation.member.updated",
      "conversation.member.removed",
      "conversation.preferences.updated",
      "conversation.history.cleared",
      "conversation.hidden",
    ].includes(event.type) ||
    !["direct", "group"].includes(event.data?.type || "") ||
    !event.data.conversationId
  )
    return null;
  if (
    [
      "conversation.preferences.updated",
      "conversation.history.cleared",
      "conversation.hidden",
    ].includes(event.type)
  ) {
    if (event.data.type !== "direct") return null;
    if (
      event.type === "conversation.hidden" &&
      (event.data.hidden !== true ||
        !Number.isSafeInteger(event.data.historyClearedThrough) ||
        event.data.historyClearedThrough! < 0)
    )
      return null;
    if (
      event.type !== "conversation.hidden" &&
      (event.data.item?.id !== event.data.conversationId ||
        event.data.item?.type !== "direct" ||
        !event.data.item.peer?.id ||
        typeof event.data.item.peer.displayName !== "string" ||
        typeof event.data.item.notificationsEnabled !== "boolean" ||
        !Number.isSafeInteger(event.data.item.historyClearedThrough) ||
        event.data.item.historyClearedThrough! < 0)
    )
      return null;
  }
  if (
    event.type.startsWith("conversation.member.") &&
    (event.data.type !== "group" || !event.data.userId)
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
  const navigate = useNavigate();
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
    const connectionKey = ["personal-connection", userId];
    client.setQueryData(connectionKey, "connecting");
    const seen = new Map<string, number>();
    let socket: WebSocket | undefined;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let attempts = 0;
    const refresh = () => {
      void client.invalidateQueries({ queryKey: ["notifications", userId] });
      void client.invalidateQueries({ queryKey: ["personal-list"] });
      void client.invalidateQueries({ queryKey: ["personal-summary"] });
      void client.invalidateQueries({ queryKey: ["personal-detail"] });
      void invalidateFolders(client, userId);
    };
    const retry = () => {
      if (controller.signal.aborted) return;
      client.setQueryData(connectionKey, "reconnecting");
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
          if (controller.signal.aborted) return;
          client.setQueryData(connectionKey, "connected");
          attempts = 0;
          refresh();
          void client.invalidateQueries({ queryKey: ["personal-chat"] });
          void client.invalidateQueries({ queryKey: ["personal-chat-read"] });
        };
        socket.onmessage = ({ data }) => {
          if (controller.signal.aborted) return;
          if (typeof data !== "string") return;
          const folderEvent = acceptFolderEvent(data);
          if (folderEvent) {
            if (folderEvent.type === "folder.deleted") {
              const folderId = folderEvent.data.folderId!;
              purgeFolder(client, userId, folderId);
              if (pathname.current === `/folders/${folderId}`)
                navigate("/folders", { replace: true });
            }
            void invalidateFolders(client, userId);
            return;
          }
          const event = acceptPersonalEvent(data, seen);
          if (!event) return;
          const id = event.data.conversationId!;
          if (event.type === "conversation.hidden") {
            rememberDirectConversationHistoryCutoff(
              client,
              id,
              userId,
              event.data.historyClearedThrough!,
            );
            resetDirectConversationHistory(client, id, userId);
            revokePersonalConversation(client, id, userId);
            setNotice((current) => (current?.id === id ? null : current));
            if (pathname.current === `/personal/${id}`)
              navigate("/personal", { replace: true });
            refresh();
            return;
          }
          if (
            event.data.item &&
            [
              "conversation.preferences.updated",
              "conversation.history.cleared",
            ].includes(event.type)
          ) {
            const item = event.data.item;
            const cutoff = directConversationHistoryCutoff(client, id, userId);
            if ((item.historyClearedThrough || 0) < cutoff) return;
            updateDirectConversation(client, userId, item);
            if (
              event.type === "conversation.history.cleared" ||
              item.notificationsEnabled === false
            )
              setNotice((current) => (current?.id === id ? null : current));
            refresh();
            return;
          }
          const m = event.data.message;
          if (
            event.data.type === "direct" &&
            Number.isSafeInteger(event.data.historyClearedThrough) &&
            event.data.historyClearedThrough! >= 0
          ) {
            if (
              rememberDirectConversationHistoryCutoff(
                client,
                id,
                userId,
                event.data.historyClearedThrough!,
              )
            )
              resetDirectConversationHistory(client, id, userId);
          }
          const cutoff = directConversationHistoryCutoff(client, id, userId);
          if (
            m &&
            cutoff > 0 &&
            (typeof m.sequence !== "string" ||
              !/^\d+$/.test(m.sequence) ||
              BigInt(m.sequence) <= BigInt(cutoff))
          )
            return;
          if (
            event.data.type === "direct" &&
            typeof event.data.notificationsEnabled === "boolean"
          ) {
            observeDirectConversationNotifications(
              client,
              id,
              userId,
              event.data.notificationsEnabled,
            );
            if (!event.data.notificationsEnabled)
              setNotice((current) => (current?.id === id ? null : current));
          }
          if (
            event.type === "conversation.member.removed" &&
            event.data.userId === userId
          ) {
            revokePersonalConversation(client, id, userId);
            setNotice((current) => (current?.id === id ? null : current));
            if (pathname.current === `/personal/${id}`)
              navigate("/personal", { replace: true });
          }
          refresh();
          if (event.type.startsWith("conversation.member."))
            void client.invalidateQueries({ queryKey: ["group-members", id] });
          void client.invalidateQueries({ queryKey: ["personal-chat", id] });
          void client.invalidateQueries({
            queryKey: ["personal-chat-read", id],
          });
          if (
            event.type === "message.created" &&
            m &&
            m.senderId !== userId &&
            // Не показываем уведомление, пока сервер не подтвердил актуальную личную настройку.
            event.data.notificationsEnabled === true &&
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
      client.removeQueries({ queryKey: connectionKey, exact: true });
    };
  }, [userId, client, navigate]);
  return {
    unread: summary.data?.unreadCount ?? 0,
    notice,
    dismiss: () => setNotice(null),
  };
}
