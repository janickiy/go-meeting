import { Fragment, useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Download,
  MessageCircle,
  Paperclip,
  Pencil,
  Reply,
  Send,
  Star,
  Trash2,
  X,
} from "lucide-react";
import { api, errorMessage, type ChatTransport } from "../api";
import { useAuth } from "../auth";
import { mergeChatPages, formatBytes } from "../collaboration";
import {
  chatDayKey,
  chatDayLabel,
  chatTime,
  insertChatEmoji,
} from "../chatPresentation";
import { formatDate, initials } from "../utils";
import type { ChatAttachment, ChatMessage, Participant } from "../types";
import { AttachmentUploader } from "./AttachmentUploader";
import { EmojiPicker } from "./EmojiPicker";
import { Button, ErrorNotice, Loading, Modal } from "./ui";

/**
 * Показывает историю чата и управляет отправкой, ответами, вложениями и правами на изменение сообщений.
 * @args conferenceId — идентификатор встречи; membership — роль текущего участника;
 * readOnly — запрет изменений; readOnlyReason — пояснение режима чтения.
 * @return Панель сообщений с редактором и диалогами изменения и удаления.
 */
export function ChatPanel({
  conferenceId,
  membership,
  readOnly,
  readOnlyReason,
  focusMessageId,
  onLatest,
}: {
  conferenceId: string;
  membership: Participant;
  readOnly: boolean;
  readOnlyReason?: string;
  focusMessageId?: string;
  onLatest?: () => void;
}) {
  return (
    <MessageThread
      scopeId={conferenceId}
      transport={api}
      mayModerate={["owner", "co_host"].includes(membership.role)}
      readOnly={readOnly}
      readOnlyReason={readOnlyReason}
      focusMessageId={focusMessageId}
      onLatest={onLatest}
    />
  );
}

export function MessageThread({
  scopeId,
  transport,
  mayModerate = false,
  readOnly = false,
  readOnlyReason,
  personal = false,
  focusMessageId,
  onLatest,
}: {
  scopeId: string;
  transport: ChatTransport;
  mayModerate?: boolean;
  readOnly?: boolean;
  readOnlyReason?: string;
  personal?: boolean;
  focusMessageId?: string;
  onLatest?: () => void;
}) {
  const historyKey = personal ? "personal-chat" : "chat";
  const readKey = personal ? "personal-chat-read" : "chat-read";
  const { user } = useAuth();
  const mayBookmark = !personal && !!user && !user.guestConferenceId;
  const focused = !personal && focusMessageId ? focusMessageId : undefined;
  const client = useQueryClient();
  const [open, setOpen] = useState(true);
  const [text, setText] = useState("");
  const [reply, setReply] = useState<ChatMessage | null>(null);
  const [editing, setEditing] = useState<ChatMessage | null>(null);
  const [deleting, setDeleting] = useState<ChatMessage | null>(null);
  const [editText, setEditText] = useState("");
  const [attachments, setAttachments] = useState<ChatAttachment[]>([]);
  const [uploadBusy, setUploadBusy] = useState(false);
  const [composerGeneration, setComposerGeneration] = useState(0);
  const [uploadDetailsTarget, setUploadDetailsTarget] =
    useState<HTMLDivElement | null>(null);
  const [validation, setValidation] = useState("");
  const [downloadError, setDownloadError] = useState("");
  const [download, setDownload] = useState<{
    id: string;
    url: string;
    expiresAt: string;
  } | null>(null);
  const composer = useRef<HTMLTextAreaElement>(null);
  const selection = useRef({ start: 0, end: 0 });
  const pendingCaret = useRef<number | null>(null);
  const retryRequest = useRef<{ signature: string; id: string } | null>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const lastMarker = useRef<HTMLSpanElement>(null);
  const markerVisible = useRef(false);
  const initialScroll = useRef(false);
  const previousLatest = useRef<string | undefined>(undefined);
  const olderAnchor = useRef<{ height: number; top: number } | null>(null);
  const [hasNewMessages, setHasNewMessages] = useState(false);
  const newestVisible = useRef(false);
  const [readCandidate, setReadCandidate] = useState<string | null>(null);
  const lastRead = useRef<string | null>(null);

  // Ссылка на приватный файл перестаёт отображаться немного раньше её серверного срока действия.
  useEffect(() => {
    if (!download) return;
    const timer = setTimeout(
      () => setDownload(null),
      Math.max(0, Date.parse(download.expiresAt) - Date.now() - 5000),
    );
    return () => clearTimeout(timer);
  }, [download]);

  const query = useInfiniteQuery({
    queryKey: focused
      ? [historyKey, scopeId, user?.id, "context", focused]
      : [historyKey, scopeId, user?.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      focused && !pageParam
        ? api.conferenceChatMessageContext(scopeId, focused, signal)
        : transport.messages(scopeId, pageParam, signal),
    getNextPageParam: (last) => last.nextCursor || undefined,
    refetchInterval: 15000,
  });
  const read = useQuery({
    queryKey: [readKey, scopeId, user?.id],
    queryFn: ({ signal }) => transport.chatRead(scopeId, signal),
    refetchInterval: 15000,
  });
  const messages = mergeChatPages(query.data?.pages || []);
  const latest = messages.at(-1)?.id;

  /** Обновляет историю и счётчик непрочитанных сообщений после изменения на сервере. */
  const invalidate = () => {
    void client.invalidateQueries({ queryKey: [historyKey, scopeId] });
    void client.invalidateQueries({ queryKey: [readKey, scopeId] });
  };
  const send = useMutation({
    mutationFn: () => {
      const body = {
        text: text.trim(),
        replyTo: reply?.id,
        attachmentIds: attachments.map((file) => file.id),
      };
      const signature = JSON.stringify(body);
      // Неизменённое сообщение при повторе получает прежний ключ, поэтому сбой сети не создаёт дубль.
      if (retryRequest.current?.signature !== signature)
        retryRequest.current = { signature, id: crypto.randomUUID() };
      return transport.sendMessage(scopeId, {
        ...body,
        clientRequestId: retryRequest.current.id,
      });
    },
    onSuccess: () => {
      newestVisible.current = true;
      setText("");
      setReply(null);
      setAttachments([]);
      setComposerGeneration((value) => value + 1);
      selection.current = { start: 0, end: 0 };
      retryRequest.current = null;
      invalidate();
      if (focused) onLatest?.();
    },
  });
  const edit = useMutation({
    mutationFn: () =>
      transport.editMessage(scopeId, editing!.id, editText.trim()),
    onSuccess: () => {
      setEditing(null);
      invalidate();
    },
    onError: invalidate,
  });
  const remove = useMutation({
    mutationFn: () => transport.deleteMessage(scopeId, deleting!.id),
    onSuccess: () => {
      setDeleting(null);
      invalidate();
    },
    onError: invalidate,
  });
  const bookmark = useMutation({
    mutationFn: (value: { messageId: string; important: boolean }) =>
      api.setConferenceChatImportant(scopeId, value.messageId, value.important),
    onSuccess: () => {
      invalidate();
      void client.invalidateQueries({
        queryKey: ["conference-chat-pins", scopeId],
      });
    },
  });

  useEffect(() => {
    if (readOnly) {
      setReply(null);
      setEditing(null);
      setDeleting(null);
      pendingCaret.current = null;
    }
  }, [readOnly]);

  // Изменение размера поля не двигает список сообщений; курсор возвращается после выбора смайлика.
  useEffect(() => {
    const element = composer.current;
    if (!element) return;
    element.style.height = "auto";
    element.style.height = `${Math.max(24, Math.min(120, element.scrollHeight))}px`;
    if (pendingCaret.current !== null) {
      element.focus();
      element.setSelectionRange(pendingCaret.current, pendingCaret.current);
      pendingCaret.current = null;
    }
  }, [text, open, readOnly]);

  useEffect(() => {
    if (!open || !latest) return;
    const element = viewport.current;
    if (!element) return;
    /** Проверяет, читает ли пользователь конец видимого списка, не прокручивая старую историю принудительно. */
    const observe = () => {
      newestVisible.current =
        element.scrollHeight - element.scrollTop - element.clientHeight < 40;
      setReadCandidate(
        document.visibilityState === "visible" &&
          newestVisible.current &&
          markerVisible.current
          ? latest
          : null,
      );
    };
    if (olderAnchor.current && !query.isFetchingNextPage) {
      element.scrollTop =
        olderAnchor.current.top +
        element.scrollHeight -
        olderAnchor.current.height;
      olderAnchor.current = null;
    } else if (!initialScroll.current && focused) {
      const target = Array.from(
        element.querySelectorAll<HTMLElement>("[data-testid]"),
      ).find((node) => node.dataset.testid === `chat-message-${focused}`);
      target?.scrollIntoView?.({ block: "center" });
      target?.focus({ preventScroll: true });
    } else if (!initialScroll.current || newestVisible.current) {
      element.scrollTop = element.scrollHeight;
      setHasNewMessages(false);
    } else if (previousLatest.current && previousLatest.current !== latest)
      setHasNewMessages(true);
    previousLatest.current = latest;
    initialScroll.current = true;
    const observer =
      typeof IntersectionObserver === "undefined"
        ? undefined
        : new IntersectionObserver(
            (entries) => {
              markerVisible.current = entries.some(
                (entry) => entry.isIntersecting,
              );
              observe();
            },
            { threshold: 1 },
          );
    if (lastMarker.current) observer?.observe(lastMarker.current);
    observe();
    element.addEventListener("scroll", observe);
    document.addEventListener("visibilitychange", observe);
    return () => {
      element.removeEventListener("scroll", observe);
      document.removeEventListener("visibilitychange", observe);
      observer?.disconnect();
    };
  }, [open, latest, messages.length, query.isFetchingNextPage, focused]);

  useEffect(() => {
    if (
      !open ||
      !readCandidate ||
      readCandidate === lastRead.current ||
      readCandidate === read.data?.item.lastReadMessageId
    )
      return;
    const timer = setTimeout(() => {
      const marker = lastMarker.current;
      const list = viewport.current;
      if (
        document.visibilityState !== "visible" ||
        !newestVisible.current ||
        !markerVisible.current ||
        !marker ||
        !list ||
        !marker.getClientRects().length
      )
        return;
      // Учитываем скрытие боковой панели и прокрутку, даже если observer ещё не успел обновить refs.
      const markerRect = marker.getBoundingClientRect();
      const listRect = list.getBoundingClientRect();
      if (
        markerRect.top < Math.max(0, listRect.top) ||
        markerRect.bottom > Math.min(window.innerHeight, listRect.bottom) ||
        markerRect.left < Math.max(0, listRect.left) ||
        markerRect.right > Math.min(window.innerWidth, listRect.right) ||
        list.scrollHeight - list.scrollTop - list.clientHeight >= 40
      )
        return;
      void transport
        .markChatRead(scopeId, readCandidate)
        .then(() => {
          lastRead.current = readCandidate;
          if (personal) {
            void client.invalidateQueries({ queryKey: ["personal-list"] });
            void client.invalidateQueries({ queryKey: ["personal-summary"] });
          }
          void client.invalidateQueries({
            queryKey: [readKey, scopeId],
          });
        })
        .catch(() => {});
    }, 2500);
    return () => clearTimeout(timer);
  }, [
    open,
    readCandidate,
    read.data?.item.lastReadMessageId,
    scopeId,
    client,
    transport,
    readKey,
    personal,
  ]);

  /** Сохраняет выделение до перевода фокуса с редактора на кнопку палитры. */
  const rememberSelection = () => {
    const element = composer.current;
    if (element)
      selection.current = {
        start: element.selectionStart,
        end: element.selectionEnd,
      };
  };

  /**
   * Заменяет выделенный текст смайликом, соблюдая серверный лимит, и возвращает курсор в редактор.
   * @args emoji — выбранная последовательность Unicode, включая составные смайлики.
   */
  const addEmoji = (emoji: string) => {
    if (readOnly || send.isPending) return;
    const result = insertChatEmoji(
      text,
      emoji,
      selection.current.start,
      selection.current.end,
    );
    if (!result) {
      setValidation("Сообщение не должно превышать 4000 символов.");
      return;
    }
    setValidation("");
    selection.current = { start: result.caret, end: result.caret };
    if (result.text === text) {
      composer.current?.focus();
      composer.current?.setSelectionRange(result.caret, result.caret);
    } else {
      pendingCaret.current = result.caret;
      setText(result.text);
    }
  };

  /** Проверяет черновик перед отправкой; вложения разрешены и без текстовой части. */
  const sendDraft = () => {
    if (readOnly || send.isPending || uploadBusy) return;
    setValidation("");
    if (
      (!text.trim() && !attachments.length) ||
      Array.from(text.trim()).length > 4000
    ) {
      setValidation("Добавьте текст (до 4000 символов) или файл.");
      return;
    }
    send.mutate();
  };

  /**
   * Получает временную ссылку на приватное вложение и проверяет допустимую схему адреса.
   * @args file — вложение сообщения, доступ к которому проверит сервер.
   */
  const prepareDownload = async (file: ChatAttachment) => {
    setDownloadError("");
    try {
      const result = await transport.attachmentDownload(scopeId, file.id);
      const url = new URL(result.url, window.location.origin);
      if (!["http:", "https:"].includes(url.protocol))
        throw new Error("invalid_download_url");
      setDownload({
        id: file.id,
        url: url.toString(),
        expiresAt: result.expiresAt,
      });
    } catch (error) {
      setDownloadError(errorMessage(error));
    }
  };

  const unread =
    read.data?.item.unreadCount ?? query.data?.pages[0]?.unreadCount ?? 0;
  return (
    <section
      className={`content-card chat-panel ${personal ? "personal-thread" : ""}`}
      aria-label={personal ? "Личная переписка" : "Чат конференции"}
    >
      <div className="section-heading chat-panel-heading" hidden={personal}>
        <h2>
          <MessageCircle size={20} />
          Чат{" "}
          {unread > 0 && (
            <span className="count-badge" data-testid="chat-unread">
              {unread}
            </span>
          )}
        </h2>
        <Button
          variant="outline"
          onClick={() => setOpen((value) => !value)}
          aria-expanded={open}
        >
          {open ? "Свернуть чат" : "Открыть чат"}
        </Button>
      </div>
      <div className="chat-panel-body" hidden={!open}>
        {focused && onLatest && (
          <Button variant="outline" onClick={onLatest}>
            К последним сообщениям
          </Button>
        )}
        <ErrorNotice error={bookmark.error} />
        {readOnly && (
          <p className="field-hint chat-readonly-note">
            {readOnlyReason ||
              "Встреча завершена. Чат сохранён и доступен только для чтения."}
          </p>
        )}
        <ErrorNotice error={query.error} />
        {query.isError && (
          <Button variant="outline" onClick={() => void query.refetch()}>
            Повторить загрузку чата
          </Button>
        )}
        {query.isPending ? (
          <Loading />
        ) : (
          <div
            ref={viewport}
            className="chat-messages"
            role="log"
            aria-label={personal ? "Личные сообщения" : "Сообщения встречи"}
            aria-live="polite"
          >
            {query.hasNextPage && (
              <Button
                variant="outline"
                busy={query.isFetchingNextPage}
                onClick={() => {
                  const list = viewport.current;
                  if (list) {
                    olderAnchor.current = {
                      height: list.scrollHeight,
                      top: list.scrollTop,
                    };
                    newestVisible.current = false;
                  }
                  void query.fetchNextPage().catch(() => {
                    olderAnchor.current = null;
                  });
                }}
              >
                Предыдущие сообщения
              </Button>
            )}
            {!messages.length && !query.isError && (
              <p className="compact-empty muted">
                Здесь пока тихо. Напишите первое сообщение.
              </p>
            )}
            {messages.map((message, index) => {
              const own = message.senderId === user?.id;
              const day = chatDayKey(message.createdAt);
              return (
                <Fragment key={message.id}>
                  {day &&
                    (!index ||
                      day !== chatDayKey(messages[index - 1].createdAt)) && (
                      <div className="chat-day-label">
                        {chatDayLabel(message.createdAt)}
                      </div>
                    )}
                  <article
                    className={`chat-message ${own ? "chat-message-own" : ""} ${message.id === focused ? "chat-message-focused" : ""}`}
                    data-testid={`chat-message-${message.id}`}
                    tabIndex={message.id === focused ? -1 : undefined}
                    aria-current={message.id === focused ? "true" : undefined}
                  >
                    {!own && (
                      <span className="chat-message-avatar" aria-hidden="true">
                        {initials(message.senderName)}
                      </span>
                    )}
                    <div className="chat-message-content">
                      {!own && (
                        <header>
                          <strong>{message.senderName}</strong>
                        </header>
                      )}
                      <div className="chat-message-bubble">
                        {message.replyPreview && (
                          <blockquote>
                            <strong>{message.replyPreview.senderName}</strong>
                            <span>
                              {message.replyPreview.deleted
                                ? "Сообщение удалено"
                                : message.replyPreview.text || "Вложение"}
                            </span>
                          </blockquote>
                        )}
                        {(message.deletedAt || message.text) && (
                          <p
                            className={
                              message.deletedAt
                                ? "chat-text muted"
                                : "chat-text"
                            }
                          >
                            {message.deletedAt
                              ? "Сообщение удалено"
                              : message.text}
                          </p>
                        )}
                        {!message.deletedAt &&
                          message.attachments.map((file) => (
                            <div className="chat-attachment" key={file.id}>
                              <span>
                                <Paperclip size={15} aria-hidden="true" />
                                {file.filename}
                                <small>{formatBytes(file.size)}</small>
                              </span>
                              {download?.id === file.id ? (
                                <a
                                  className="text-link"
                                  href={download.url}
                                  target="_blank"
                                  rel="noreferrer"
                                  download
                                >
                                  Скачать файл
                                </a>
                              ) : (
                                <button
                                  type="button"
                                  className="text-link"
                                  onClick={() => void prepareDownload(file)}
                                >
                                  <Download size={14} />
                                  Получить ссылку
                                </button>
                              )}
                            </div>
                          ))}
                        <footer className="chat-message-meta">
                          {message.version > 1 && !message.deletedAt && (
                            <span title="Сообщение изменено">
                              <Pencil size={11} aria-hidden="true" />
                              <span className="sr-only">изменено</span>
                            </span>
                          )}
                          <time
                            dateTime={message.createdAt}
                            title={formatDate(message.createdAt)}
                          >
                            {chatTime(message.createdAt)}
                          </time>
                        </footer>
                      </div>
                      {!message.deletedAt && (!readOnly || mayBookmark) && (
                        <div className="chat-message-actions">
                          {mayBookmark && (
                            <button
                              type="button"
                              aria-pressed={!!message.important}
                              disabled={bookmark.isPending}
                              onClick={() =>
                                bookmark.mutate({
                                  messageId: message.id,
                                  important: !message.important,
                                })
                              }
                            >
                              <Star
                                size={13}
                                fill={
                                  message.important ? "currentColor" : "none"
                                }
                              />
                              {message.important
                                ? "Убрать из важных"
                                : "В важные"}
                            </button>
                          )}
                          {!readOnly && (
                            <>
                              <button
                                type="button"
                                disabled={send.isPending}
                                onClick={() => {
                                  setReply(message);
                                  composer.current?.focus();
                                }}
                              >
                                <Reply size={13} />
                                Ответить
                              </button>
                              {own && (
                                <button
                                  type="button"
                                  onClick={() => {
                                    setEditing(message);
                                    setEditText(message.text);
                                    edit.reset();
                                  }}
                                >
                                  <Pencil size={13} />
                                  Изменить
                                </button>
                              )}
                              {(own || mayModerate) && (
                                <button
                                  type="button"
                                  onClick={() => {
                                    setDeleting(message);
                                    remove.reset();
                                  }}
                                >
                                  <Trash2 size={13} />
                                  Удалить
                                </button>
                              )}
                            </>
                          )}
                        </div>
                      )}
                    </div>
                  </article>
                </Fragment>
              );
            })}
            <span
              ref={lastMarker}
              className="chat-read-marker"
              aria-hidden="true"
            />
          </div>
        )}
        {hasNewMessages && (
          <Button
            variant="outline"
            onClick={() => {
              const list = viewport.current;
              if (list) list.scrollTop = list.scrollHeight;
              setHasNewMessages(false);
            }}
          >
            Новые сообщения ↓
          </Button>
        )}
        <ErrorNotice error={downloadError || null} />
        {!readOnly && (
          <form
            className="chat-composer"
            onSubmit={(event) => {
              event.preventDefault();
              sendDraft();
            }}
          >
            <div className="chat-composer-details">
              {reply && (
                <div className="chat-reply">
                  <div>
                    <strong>Ответ: {reply.senderName}</strong>
                    <p>{reply.text || "Вложение"}</p>
                  </div>
                  <button
                    type="button"
                    className="icon-button"
                    aria-label="Отменить ответ"
                    disabled={send.isPending}
                    onClick={() => setReply(null)}
                  >
                    <X size={16} />
                  </button>
                </div>
              )}
              <div
                className="chat-upload-details"
                ref={setUploadDetailsTarget}
              />
              <ErrorNotice error={send.error}>{validation || null}</ErrorNotice>
            </div>
            <div className="chat-compose-row">
              <AttachmentUploader
                key={composerGeneration}
                conferenceId={scopeId}
                transport={transport}
                value={attachments}
                onChange={setAttachments}
                onBusy={setUploadBusy}
                disabled={send.isPending}
                compact
                detailsTarget={uploadDetailsTarget}
              />
              <label
                className="chat-compose-field"
                htmlFor={`chat-text-${scopeId}`}
              >
                <span className="sr-only">Сообщение</span>
                <textarea
                  ref={composer}
                  aria-label="Сообщение"
                  id={`chat-text-${scopeId}`}
                  value={text}
                  rows={1}
                  placeholder="Напишите сообщение…"
                  disabled={send.isPending}
                  onSelect={rememberSelection}
                  onBlur={rememberSelection}
                  onChange={(event) => {
                    setText(event.target.value);
                    selection.current = {
                      start: event.target.selectionStart,
                      end: event.target.selectionEnd,
                    };
                  }}
                  onKeyDown={(event) => {
                    if (
                      event.key === "Enter" &&
                      !event.shiftKey &&
                      !event.nativeEvent.isComposing &&
                      event.keyCode !== 229
                    ) {
                      event.preventDefault();
                      sendDraft();
                    }
                  }}
                />
              </label>
              <EmojiPicker onSelect={addEmoji} disabled={send.isPending} />
              <Button
                type="submit"
                className="chat-send-button"
                busy={send.isPending}
                disabled={uploadBusy || (!text.trim() && !attachments.length)}
                aria-label={send.isError ? "Повторить отправку" : "Отправить"}
                title={send.isError ? "Повторить отправку" : "Отправить"}
              >
                {!send.isPending && <Send size={21} />}
              </Button>
            </div>
            <p className="sr-only">
              Enter — отправить, Shift + Enter — новая строка. Если связь
              прервётся, повторная отправка не создаст копию сообщения.
            </p>
          </form>
        )}
      </div>
      {editing && (
        <Modal
          title="Изменить сообщение"
          onClose={() => {
            if (!edit.isPending) setEditing(null);
          }}
        >
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (
                !edit.isPending &&
                !readOnly &&
                (editText.trim() || editing.attachments.length) &&
                Array.from(editText.trim()).length <= 4000
              )
                edit.mutate();
            }}
          >
            <label className="field" htmlFor="edit-message">
              Текст
              <textarea
                id="edit-message"
                data-autofocus
                value={editText}
                onChange={(event) => setEditText(event.target.value)}
                rows={4}
                disabled={edit.isPending}
              />
            </label>
            <ErrorNotice error={edit.error} />
            <Button
              type="submit"
              busy={edit.isPending}
              disabled={
                (!editText.trim() && !editing.attachments.length) ||
                Array.from(editText.trim()).length > 4000
              }
            >
              Сохранить сообщение
            </Button>
          </form>
        </Modal>
      )}
      {deleting && (
        <Modal
          title="Удалить сообщение?"
          onClose={() => {
            if (!remove.isPending) setDeleting(null);
          }}
        >
          <p className="modal-description">
            Текст и вложения станут недоступны другим участникам. В чате
            останется отметка об удалении.
          </p>
          <ErrorNotice error={remove.error} />
          <Button
            variant="danger"
            busy={remove.isPending}
            onClick={() => {
              if (!readOnly) remove.mutate();
            }}
          >
            Да, удалить сообщение
          </Button>
        </Modal>
      )}
    </section>
  );
}
