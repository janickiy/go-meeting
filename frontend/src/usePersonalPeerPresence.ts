import {
  createContext,
  createElement,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { skipToken, useQuery } from "@tanstack/react-query";
import { api, ApiError } from "./api";
import { useAuth } from "./auth";
import type { DirectConversation } from "./types";

/** Подтверждённое присутствие в строго заданной области аккаунта и переписки.
 * @params actorId — текущий аккаунт; conversationId — личный диалог; peerId — собеседник;
 * online — известное присутствие либо undefined; isPending — первоначальное ожидание;
 * denied — сервер отозвал доступ, дальнейший опрос этой области прекращён.
 */
interface PeerPresenceState {
  actorId?: string;
  conversationId?: string;
  peerId?: string;
  online?: boolean;
  isPending: boolean;
  denied: boolean;
}

const PeerPresenceContext = createContext<PeerPresenceState | undefined>(
  undefined,
);

/** Читает присутствие с одним видимым опросом и отменой запроса при смене области.
 * @args conversation — доступная личная переписка; shared — состояние владельца общего опроса.
 * @return Проверенное присутствие; ошибка или чужой DTO не становятся офлайн.
 */
function usePresenceQuery(
  conversation?: DirectConversation,
  shared?: PeerPresenceState,
): PeerPresenceState {
  const { user } = useAuth();
  const actorId = user?.id;
  const conversationId = conversation?.id;
  const peerId = conversation?.peer.id;
  const scope = JSON.stringify([actorId, conversationId, peerId]);
  const currentScope = useRef(scope);
  currentScope.current = scope;
  const [deniedScope, setDeniedScope] = useState<string>();
  const allowed = !!conversation && !!user && !user.guestConferenceId;
  const sharedMatches =
    allowed &&
    !!shared &&
    shared.actorId === actorId &&
    shared.conversationId === conversationId &&
    shared.peerId === peerId;
  const presence = useQuery({
    queryKey: ["personal-peer-presence", conversationId, actorId, peerId],
    queryFn: conversation
      ? async ({ signal }) => {
          try {
            return await api.personalPeerPresence(conversation.id, signal);
          } catch (error) {
            // Сначала блокируем опрос, чтобы очистка кеша не породила ещё один запрос к отозванному диалогу.
            if (
              !signal.aborted &&
              currentScope.current === scope &&
              error instanceof ApiError &&
              [403, 404].includes(error.status)
            )
              setDeniedScope(scope);
            throw error;
          }
        }
      : skipToken,
    enabled: allowed && !sharedMatches && deniedScope !== scope,
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchInterval: 1000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: "always",
    refetchOnMount: false,
  });
  const requestDenied =
    presence.error instanceof ApiError &&
    [403, 404].includes(presence.error.status);
  useEffect(() => {
    setDeniedScope((previous) => (previous === scope ? previous : undefined));
  }, [scope]);
  useEffect(() => {
    if (requestDenied && !sharedMatches) setDeniedScope(scope);
  }, [requestDenied, sharedMatches, scope]);
  if (sharedMatches) return shared;
  const denied = requestDenied || deniedScope === scope;
  const status =
    !allowed ||
    presence.isError ||
    denied ||
    presence.data?.status !== "success"
      ? undefined
      : presence.data.item;
  const online =
    conversation &&
    status &&
    status.conversationId === conversationId &&
    status.peerId === peerId &&
    typeof status.online === "boolean"
      ? status.online
      : undefined;
  return {
    actorId,
    conversationId,
    peerId,
    online,
    isPending: presence.isPending && !denied,
    denied,
  };
}

/** Делит единственный опрос открытого личного диалога между шапкой и окнами информации.
 * @args conversation — выбранная личная переписка, отсутствует для списка или группы; children — её интерфейс.
 * @return Контекст присутствия без дополнительного DOM-элемента и дублирующих таймеров.
 */
export function PersonalPeerPresenceProvider({
  conversation,
  children,
}: {
  conversation?: DirectConversation;
  children: ReactNode;
}) {
  const presence = usePresenceQuery(conversation);
  return createElement(
    PeerPresenceContext.Provider,
    { value: presence },
    children,
  );
}

/** Использует общий опрос своего диалога либо отдельный запрос вне открытой переписки.
 * @args conversation — собеседник и диалог, для которых отображается статус.
 * @return Состояние с проверенной привязкой к аккаунту, диалогу и собеседнику.
 */
export function usePersonalPeerPresence(conversation: DirectConversation) {
  return usePresenceQuery(conversation, useContext(PeerPresenceContext));
}
