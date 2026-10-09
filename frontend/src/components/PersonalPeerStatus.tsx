import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "../auth";
import { revokePersonalConversation } from "../personalRealtime";
import { usePersonalPeerPresence } from "../usePersonalPeerPresence";
import type { DirectConversation } from "../types";

/** Показывает живой статус собеседника вместо статической подписи личного диалога.
 * @args conversation — доступная переписка; onAccessDenied — закрытие отозванного диалога.
 * @return Доступная строка статуса, зелёная только при подтверждённом онлайн.
 */
export function PersonalPeerStatus({
  conversation,
  onAccessDenied,
}: {
  conversation: DirectConversation;
  onAccessDenied?: () => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const presence = usePersonalPeerPresence(conversation);
  const scope = JSON.stringify([
    user?.id,
    conversation.id,
    conversation.peer.id,
  ]);
  const notified = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (!presence.denied || !user || notified.current === scope) return;
    notified.current = scope;
    revokePersonalConversation(client, conversation.id, user.id);
    onAccessDenied?.();
  }, [presence.denied, user, client, conversation.id, scope, onAccessDenied]);
  const text = presence.isPending
    ? "Проверяем статус…"
    : presence.online === true
      ? "Онлайн"
      : presence.online === false
        ? "Не в сети"
        : "Статус недоступен";
  return (
    <small
      className={`personal-peer-status${presence.online === true ? " personal-peer-status-online" : ""}`}
      role="status"
      aria-live="polite"
    >
      {text}
    </small>
  );
}
