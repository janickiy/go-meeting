import { useEffect, useMemo, useRef, useState } from "react";
import {
  useIsMutating,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router";
import { Bell, BellOff, Eraser, FolderPlus, Info, Trash2 } from "lucide-react";
import { api, ApiError } from "../api";
import { useAuth } from "../auth";
import { invalidateFolders } from "../folders";
import {
  directConversationHistoryCutoff,
  rememberDirectConversationHistoryCutoff,
  resetDirectConversationHistory,
  updateDirectConversation,
} from "../directConversations";
import { revokePersonalConversation } from "../personalRealtime";
import type { DirectConversation } from "../types";
import { ConversationAvatar } from "./GroupChats";
import { FolderPicker } from "./FolderPicker";
import { ItemActions } from "./ItemActions";
import { Button, ErrorNotice, Modal } from "./ui";

/**
 * Фиксирует автора, диалог и поколение интерфейса на момент подтверждения действия.
 * @params action — команда; actorId — автор; conversationId — целевой диалог;
 * scope — уникальное поколение учётной записи и диалога; notificationsEnabled — требуемая настройка;
 * controller — отмена только этого запроса при закрытии или смене области.
 */
interface DirectConversationOperation {
  action: "mute" | "clear" | "hide";
  actorId: string;
  conversationId: string;
  scope: object;
  notificationsEnabled: boolean;
  controller: AbortController;
}

/**
 * Показывает доступные публичные сведения собеседника в стандартном диалоге.
 * Электронная почта и присутствие не выдумываются: API личных чатов их не раскрывает.
 *
 * @args conversation — личный диалог; onClose — закрытие окна; returnFocus — элемент открытия.
 * @return Стандартное модальное окно с публичным именем и идентификатором собеседника.
 */
export function DirectUserInfoModal({
  conversation,
  onClose,
  returnFocus,
}: {
  conversation: DirectConversation;
  onClose: () => void;
  returnFocus?: () => HTMLElement | null;
}) {
  return (
    <Modal
      title="Информация о пользователе"
      className="personal-info-modal"
      onClose={onClose}
      returnFocus={returnFocus}
    >
      <div className="personal-user-info">
        <ConversationAvatar conversation={conversation} large />
        <h3>{conversation.peer.displayName}</h3>
        <p>Участник вашей переписки</p>
        <dl>
          <dt>Имя</dt>
          <dd>{conversation.peer.displayName}</dd>
          <dt>Идентификатор пользователя</dt>
          <dd className="personal-user-id">{conversation.peer.id}</dd>
        </dl>
      </div>
      <div className="personal-confirm-actions">
        <Button variant="outline" onClick={onClose}>
          Закрыть
        </Button>
      </div>
    </Modal>
  );
}

/**
 * Управляет только личным представлением диалога: уведомлениями, историей и видимостью.
 * Любое удаление подтверждается; сообщения и настройки собеседника не изменяются.
 *
 * @args conversation — диалог текущего участника с подтверждёнными серверными настройками.
 * @return Меню действий, подтверждения и безопасные сообщения об ошибках.
 */
export function DirectConversationActions({
  conversation,
}: {
  conversation: DirectConversation;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const location = useLocation();
  const navigate = useNavigate();
  const root = useRef<HTMLDivElement>(null);
  const mounted = useRef(false);
  const gate = useRef(false);
  const controller = useRef<AbortController | null>(null);
  const operation = useRef<DirectConversationOperation | null>(null);
  const scope = useMemo(() => ({}), [user?.id, conversation.id]);
  const activeScope = useRef(scope);
  activeScope.current = scope;
  const [dialog, setDialog] = useState<
    "info" | "folder" | "clear" | "hide" | null
  >(null);
  const mutationKey = ["personal-actions", conversation.id, user?.id];
  const otherBusy = useIsMutating({ mutationKey }) > 0;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
    };
  }, []);
  const returnFocus = () =>
    root.current?.querySelector<HTMLButtonElement>("button") || null;
  /** Проверяет, что ответ относится к текущей операции, а не к прежней учётной записи или диалогу. */
  const currentOperation = (pending: DirectConversationOperation) =>
    mounted.current &&
    activeScope.current === pending.scope &&
    operation.current === pending;
  const write = useMutation({
    mutationKey,
    mutationFn: (
      pending: DirectConversationOperation,
    ): Promise<{
      status: string;
      item?: DirectConversation;
      hidden?: boolean;
      historyClearedThrough?: number;
    }> => {
      if (!currentOperation(pending))
        throw new DOMException(
          "Операция предыдущей учётной записи отменена",
          "AbortError",
        );
      const signal = pending.controller.signal;
      signal.throwIfAborted();
      if (pending.action === "hide")
        return api.hidePersonalConversation(pending.conversationId, signal);
      if (pending.action === "clear")
        return api.clearPersonalConversationHistory(
          pending.conversationId,
          signal,
        );
      return api.setPersonalConversationNotifications(
        pending.conversationId,
        pending.notificationsEnabled,
        signal,
      );
    },
    onSuccess: (response, pending) => {
      if (!currentOperation(pending)) return;
      const action = pending.action;
      const actorId = pending.actorId;
      const id = pending.conversationId;
      if (action === "hide") {
        // Сохраняем и прежний известный порог, даже если сервер прислал неполное подтверждение.
        rememberDirectConversationHistoryCutoff(
          client,
          id,
          actorId,
          directConversationHistoryCutoff(client, id, actorId),
        );
        if (
          response.hidden === true &&
          Number.isSafeInteger(response.historyClearedThrough) &&
          response.historyClearedThrough! >= 0
        )
          rememberDirectConversationHistoryCutoff(
            client,
            id,
            actorId,
            response.historyClearedThrough!,
          );
        resetDirectConversationHistory(client, id, actorId);
        revokePersonalConversation(client, id, actorId);
        if (location.pathname === `/personal/${id}`)
          navigate(`/personal${location.search}`, { replace: true });
      } else if (response.item?.id === id && response.item.type === "direct") {
        const knownCutoff = directConversationHistoryCutoff(
          client,
          id,
          actorId,
        );
        const accepted = updateDirectConversation(
          client,
          actorId,
          response.item,
        );
        if (
          action === "clear" &&
          accepted &&
          (response.item.historyClearedThrough || 0) <= knownCutoff
        )
          resetDirectConversationHistory(client, id, actorId);
      }
      setDialog(null);
      void Promise.all([
        client.invalidateQueries({ queryKey: ["personal-list", actorId] }),
        client.invalidateQueries({ queryKey: ["personal-summary", actorId] }),
        invalidateFolders(client, actorId),
      ]);
    },
    onError: (error, pending) => {
      if (!currentOperation(pending)) return;
      if (error instanceof ApiError && [403, 404].includes(error.status)) {
        resetDirectConversationHistory(
          client,
          pending.conversationId,
          pending.actorId,
        );
        revokePersonalConversation(
          client,
          pending.conversationId,
          pending.actorId,
        );
        if (location.pathname === `/personal/${pending.conversationId}`)
          navigate(`/personal${location.search}`, { replace: true });
      }
    },
    onSettled: (_data, _error, pending) => {
      if (!currentOperation(pending)) return;
      operation.current = null;
      gate.current = false;
    },
  });
  useEffect(() => {
    write.reset();
    setDialog(null);
    return () => {
      const pending = operation.current;
      if (pending?.scope !== scope) return;
      pending.controller.abort();
      operation.current = null;
      controller.current = null;
      gate.current = false;
    };
  }, [scope, write.reset]);
  const busy = otherBusy || write.isPending;
  const run = (action: "mute" | "clear" | "hide") => {
    if (gate.current || busy || !user) return;
    gate.current = true;
    const pending: DirectConversationOperation = {
      action,
      actorId: user.id,
      conversationId: conversation.id,
      scope,
      notificationsEnabled: conversation.notificationsEnabled === false,
      controller: new AbortController(),
    };
    operation.current = pending;
    controller.current = pending.controller;
    write.mutate(pending);
  };
  const open = (value: typeof dialog) => {
    write.reset();
    setDialog(value);
  };
  const close = () => {
    if (!gate.current) setDialog(null);
  };
  const muted = conversation.notificationsEnabled === false;
  return (
    <div className="personal-direct-actions" ref={root}>
      <ItemActions
        label={`Действия с перепиской: ${conversation.peer.displayName}`}
        actions={[
          {
            label: "Информация",
            icon: <Info size={19} aria-hidden="true" />,
            disabled: busy,
            run: () => open("info"),
          },
          {
            label: muted ? "Включить уведомления" : "Без уведомлений",
            icon: muted ? (
              <Bell size={19} aria-hidden="true" />
            ) : (
              <BellOff size={19} aria-hidden="true" />
            ),
            disabled: busy,
            run: () => run("mute"),
          },
          {
            label: "Добавить в папку",
            icon: <FolderPlus size={19} aria-hidden="true" />,
            disabled: busy,
            run: () => open("folder"),
          },
          {
            label: "Очистить историю",
            icon: <Eraser size={19} aria-hidden="true" />,
            disabled: busy,
            run: () => open("clear"),
          },
          {
            label: "Удалить чат",
            icon: <Trash2 size={19} aria-hidden="true" />,
            danger: true,
            disabled: busy,
            run: () => open("hide"),
          },
        ]}
      />
      {!dialog && <ErrorNotice error={write.error} />}
      {!dialog && write.isPending && (
        <span className="sr-only" role="status">
          Сохраняем настройки переписки…
        </span>
      )}
      {dialog === "info" && (
        <DirectUserInfoModal
          conversation={conversation}
          onClose={close}
          returnFocus={returnFocus}
        />
      )}
      {dialog === "folder" && (
        <FolderPicker
          target={{ type: "conversation", id: conversation.id }}
          onClose={close}
          returnFocus={returnFocus}
        />
      )}
      {(dialog === "clear" || dialog === "hide") && (
        <Modal
          title={dialog === "clear" ? "Очистить историю?" : "Удалить чат?"}
          className="personal-confirm-modal"
          onClose={close}
          returnFocus={returnFocus}
        >
          <div className="personal-confirm-symbol" aria-hidden="true">
            {dialog === "clear" ? <Eraser size={26} /> : <Trash2 size={26} />}
          </div>
          <h3>Переписка с {conversation.peer.displayName}</h3>
          <p>
            {dialog === "clear"
              ? `Переписка с пользователем «${conversation.peer.displayName}» будет очищена только у вас. История собеседника сохранится. Отменить очистку нельзя.`
              : `Чат с пользователем «${conversation.peer.displayName}» будет удалён из вашего списка и папок, а его история — очищена только у вас. Новый входящий ответ снова появится в списке.`}
          </p>
          <ErrorNotice error={write.error} />
          <div className="personal-confirm-actions">
            <Button
              variant="secondary"
              data-autofocus
              disabled={busy}
              onClick={close}
            >
              Отмена
            </Button>
            <Button
              variant="danger"
              busy={write.isPending}
              disabled={busy && !write.isPending}
              onClick={() => run(dialog)}
            >
              {dialog === "clear" ? "Очистить историю" : "Удалить чат"}
            </Button>
          </div>
        </Modal>
      )}
    </div>
  );
}
