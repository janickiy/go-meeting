import { useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Download,
  MessageCircle,
  Pencil,
  Reply,
  Send,
  Trash2,
  X,
} from "lucide-react";
import { api, errorMessage } from "../api";
import { useAuth } from "../auth";
import { mergeChatPages, formatBytes } from "../collaboration";
import { formatDate } from "../utils";
import type { ChatAttachment, ChatMessage, Participant } from "../types";
import { AttachmentUploader } from "./AttachmentUploader";
import { Button, ErrorNotice, Loading, Modal } from "./ui";

/**
 * ChatPanel управляет постоянной историей чата, ответами, изменениями, вложениями, повтором отправки и отложенным прочтением.
 *
 * @parameters:
 *   - объект параметров: conferenceId — идентификатор конференции и области данных; membership — свойство текущего компонента; readOnly — свойство текущего компонента; readOnlyReason — свойство текущего компонента.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function ChatPanel({
  conferenceId,
  membership,
  readOnly,
  readOnlyReason,
}: {
  conferenceId: string;
  membership: Participant;
  readOnly: boolean;
  readOnlyReason?: string;
}) {
  const { user } = useAuth();
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
  const [validation, setValidation] = useState("");
  const [downloadError, setDownloadError] = useState("");
  const [download, setDownload] = useState<{
    id: string;
    url: string;
    expiresAt: string;
  } | null>(null);
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (!download) return;
      const timer = setTimeout(
        /**
         * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
         *
         *
         * @returns следующее состояние, рассчитанное из предыдущего значения.
         */
        () => setDownload(null),
        Math.max(0, Date.parse(download.expiresAt) - Date.now() - 5000),
      );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns вычисленное значение: clearTimeout(timer).
       */
      return () => clearTimeout(timer);
    },
    [download],
  );
  const retryRequest = useRef<{ signature: string; id: string } | null>(null);
  const viewport = useRef<HTMLDivElement>(null);
  const lastMarker = useRef<HTMLSpanElement>(null);
  const markerVisible = useRef(false);
  const initialScroll = useRef(false);
  const newestVisible = useRef(false);
  const [readCandidate, setReadCandidate] = useState<string | null>(null);
  const lastRead = useRef<string | null>(null);
  const query = useInfiniteQuery({
    queryKey: ["chat", conferenceId, user?.id],
    initialPageParam: undefined as string | undefined,
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @parameters:
     *   - объект параметров: pageParam — свойство текущего компонента; signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.messages(conferenceId, pageParam, signal).
     */
    queryFn: ({ pageParam, signal }) =>
      api.messages(conferenceId, pageParam, signal),
    /**
     * getNextPageParam извлекает курсор продолжения серверной страницы.
     *
     * @parameters:
     *   - last — последняя загруженная страница, по которой определяется продолжение.
     *
     * @returns вычисленное значение: last.nextCursor || undefined.
     */
    getNextPageParam: (last) => last.nextCursor || undefined,
    refetchInterval: 15000,
  });
  const read = useQuery({
    queryKey: ["chat-read", conferenceId, user?.id],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @parameters:
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.chatRead(conferenceId, signal).
     */
    queryFn: ({ signal }) => api.chatRead(conferenceId, signal),
    refetchInterval: 15000,
  });
  const messages = mergeChatPages(query.data?.pages || []);
  const latest = messages.at(-1)?.id;
  /**
   * invalidate обновляет связанные кеши после сохранённого изменения или события.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ["chat", conferenceId] });
    void client.invalidateQueries({ queryKey: ["chat-read", conferenceId] });
  };
  const send = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     *
     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
     */
    mutationFn: () => {
      const body = {
        text: text.trim(),
        replyTo: reply?.id,
        attachmentIds: attachments.map(
          /**
           * Обработчик attachments.map преобразует один элемент набора в представление или данные следующего шага.
           *
           * @parameters:
           *   - file — выбранный пользователем файл для проверки или передачи.
           *
           * @returns преобразованное значение текущего элемента для результирующего набора.
           */ (file) => file.id,
        ),
      };
      const signature = JSON.stringify(body);
      if (retryRequest.current?.signature !== signature)
        retryRequest.current = { signature, id: crypto.randomUUID() };
      return api.sendMessage(conferenceId, {
        ...body,
        clientRequestId: retryRequest.current.id,
      });
    },
    /**
     * onSuccess обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSuccess: () => {
      setText("");
      setReply(null);
      setAttachments([]);
      setComposerGeneration(
        /**
         * Обработчик setComposerGeneration вычисляет следующее React-состояние из предыдущего значения.
         *
         * @parameters:
         *   - value — значение для проверки, преобразования или отображения.
         *
         * @returns следующее состояние, рассчитанное из предыдущего значения.
         */ (value) => value + 1,
      );
      retryRequest.current = null;
      invalidate();
    },
  });
  const edit = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     *
     * @returns вычисленное значение: api.editMessage(conferenceId, editing!.id, editText.trim()).
     */
    mutationFn: () =>
      api.editMessage(conferenceId, editing!.id, editText.trim()),
    /**
     * onSuccess обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSuccess: () => {
      setEditing(null);
      invalidate();
    },
    onError: invalidate,
  });
  const remove = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     *
     * @returns вычисленное значение: api.deleteMessage(conferenceId, deleting!.id).
     */
    mutationFn: () => api.deleteMessage(conferenceId, deleting!.id),
    /**
     * onSuccess обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSuccess: () => {
      setDeleting(null);
      invalidate();
    },
    onError: invalidate,
  });
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (readOnly) {
        setReply(null);
        setEditing(null);
        setDeleting(null);
      }
    },
    [readOnly],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (!open || !latest) return;
      const element = viewport.current;
      if (!element) return;
      /**
       * observe отслеживает видимость последних сообщений для отложенной отметки прочтения.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
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
      if (!initialScroll.current || newestVisible.current)
        element.scrollTop = element.scrollHeight;
      initialScroll.current = true;
      const observer =
        typeof IntersectionObserver === "undefined"
          ? undefined
          : new IntersectionObserver(
              /**
               * Вложенный обработчик выполняет шаг «Вложенный обработчик» в чате, файлах и совместной работе.
               *
               * @parameters:
               *   - entries — список изменений видимости наблюдаемых элементов.
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */
              (entries) => {
                markerVisible.current = entries.some(
                  /**
                   * Обработчик entries.some проверяет условие поиска элемента или соответствия элементов набора.
                   *
                   * @parameters:
                   *   - entry — состояние видимости одного наблюдаемого элемента.
                   *
                   * @returns логический признак соответствия элемента условию.
                   */
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
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        element.removeEventListener("scroll", observe);
        document.removeEventListener("visibilitychange", observe);
        observer?.disconnect();
      };
    },
    [open, latest, messages.length],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (
        !open ||
        !readCandidate ||
        readCandidate === lastRead.current ||
        readCandidate === read.data?.item.lastReadMessageId
      )
        return;
      const timer = setTimeout(
        /**
         * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
         *
         *
         * @returns следующее состояние, рассчитанное из предыдущего значения.
         */ () => {
          if (
            document.visibilityState !== "visible" ||
            !newestVisible.current ||
            !markerVisible.current
          )
            return;
          void api
            .markChatRead(conferenceId, readCandidate)
            .then(
              /**
               * Обработчик then выполняет переданный шаг вызова then в чате, файлах и совместной работе.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                lastRead.current = readCandidate;
                void client.invalidateQueries({
                  queryKey: ["chat-read", conferenceId],
                });
              },
            )
            .catch(
              /**
               * Обработчик catch выполняет переданный шаг вызова catch в чате, файлах и совместной работе.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {},
            );
        },
        2500,
      );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns вычисленное значение: clearTimeout(timer).
       */
      return () => clearTimeout(timer);
    },
    [
      open,
      readCandidate,
      read.data?.item.lastReadMessageId,
      conferenceId,
      client,
    ],
  );
  const unread =
    read.data?.item.unreadCount ?? query.data?.pages[0]?.unreadCount ?? 0;
  const mayModerate = ["owner", "co_host"].includes(membership.role);
  return (
    <section className="content-card chat-panel" aria-label="Чат конференции">
      <div className="section-heading">
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
          onClick={
            /**
             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: setOpen( (value) => !value, ).
             */ () =>
              setOpen(
                /**
                 * Обработчик setOpen вычисляет следующее React-состояние из предыдущего значения.
                 *
                 * @parameters:
                 *   - value — значение для проверки, преобразования или отображения.
                 *
                 * @returns следующее состояние, рассчитанное из предыдущего значения.
                 */ (value) => !value,
              )
          }
          aria-expanded={open}
        >
          {open ? "Свернуть чат" : "Открыть чат"}
        </Button>
      </div>
      <div hidden={!open}>
        {readOnly && (
          <p className="field-hint">
            {readOnlyReason ||
              "Встреча завершена. Чат сохранён и доступен только для чтения."}
          </p>
        )}
        <ErrorNotice error={query.error} />
        {query.isError && (
          <Button
            variant="outline"
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: void query.refetch().
               */ () => void query.refetch()
            }
          >
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
            aria-label="Сообщения встречи"
            aria-live="polite"
          >
            {query.hasNextPage && (
              <Button
                variant="outline"
                busy={query.isFetchingNextPage}
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns вычисленное значение: void query.fetchNextPage().
                   */ () => void query.fetchNextPage()
                }
              >
                Предыдущие сообщения
              </Button>
            )}
            {!messages.length && !query.isError && (
              <p className="compact-empty muted">
                Здесь пока тихо. Напишите первое сообщение.
              </p>
            )}
            {messages.map(
              /**
               * Обработчик messages.map преобразует один элемент набора в представление или данные следующего шага.
               *
               * @parameters:
               *   - message — текущий объект сообщения чата.
               *
               * @returns преобразованное значение текущего элемента для результирующего набора.
               */ (message) => (
                <article
                  key={message.id}
                  className={`chat-message ${message.senderId === user?.id ? "chat-message-own" : ""}`}
                  data-testid={`chat-message-${message.id}`}
                >
                  <header>
                    <strong>{message.senderName}</strong>
                    <time dateTime={message.createdAt}>
                      {formatDate(message.createdAt)}
                    </time>
                    {message.version > 1 && !message.deletedAt && (
                      <small>изменено</small>
                    )}
                  </header>
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
                  <p className={message.deletedAt ? "muted" : "chat-text"}>
                    {message.deletedAt ? "Сообщение удалено" : message.text}
                  </p>
                  {!message.deletedAt &&
                    message.attachments.map(
                      /**
                       * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
                       *
                       * @parameters:
                       *   - file — выбранный пользователем файл для проверки или передачи.
                       *
                       * @returns преобразованное значение текущего элемента для результирующего набора.
                       */ (file) => (
                        <div className="chat-attachment" key={file.id}>
                          <span>
                            <PaperclipLabel />
                            {file.filename}{" "}
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
                              onClick={
                                /**
                                 * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                                 *
                                 *
                                 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                                 */ () => {
                                  setDownloadError("");
                                  void api
                                    .attachmentDownload(conferenceId, file.id)
                                    .then(
                                      /**
                                       * Обработчик then выполняет переданный шаг вызова then в чате, файлах и совместной работе.
                                       *
                                       * @parameters:
                                       *   - result — результат завершённой операции.
                                       *
                                       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                                       */ (result) => {
                                        const url = new URL(
                                          result.url,
                                          window.location.origin,
                                        );
                                        if (
                                          !["http:", "https:"].includes(
                                            url.protocol,
                                          )
                                        )
                                          throw new Error(
                                            "invalid_download_url",
                                          );
                                        setDownload({
                                          id: file.id,
                                          url: url.toString(),
                                          expiresAt: result.expiresAt,
                                        });
                                      },
                                    )
                                    .catch(
                                      /**
                                       * Обработчик catch выполняет переданный шаг вызова catch в чате, файлах и совместной работе.
                                       *
                                       * @parameters:
                                       *   - error — пойманная ошибка API или сети.
                                       *
                                       * @returns вычисленное значение: setDownloadError(errorMessage(error)).
                                       */ (error) =>
                                        setDownloadError(errorMessage(error)),
                                    );
                                }
                              }
                            >
                              <Download size={15} />
                              Получить ссылку
                            </button>
                          )}
                        </div>
                      ),
                    )}
                  {!readOnly && !message.deletedAt && (
                    <div className="chat-message-actions">
                      <button
                        type="button"
                        className="text-link"
                        onClick={
                          /**
                           * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                           *
                           *
                           * @returns вычисленное значение: setReply(message).
                           */ () => setReply(message)
                        }
                      >
                        <Reply size={14} />
                        Ответить
                      </button>
                      {message.senderId === user?.id && (
                        <button
                          type="button"
                          className="text-link"
                          onClick={
                            /**
                             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                             *
                             *
                             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                             */ () => {
                              setEditing(message);
                              setEditText(message.text);
                              edit.reset();
                            }
                          }
                        >
                          <Pencil size={14} />
                          Изменить
                        </button>
                      )}
                      {(message.senderId === user?.id || mayModerate) && (
                        <button
                          type="button"
                          className="text-link"
                          onClick={
                            /**
                             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                             *
                             *
                             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                             */ () => {
                              setDeleting(message);
                              remove.reset();
                            }
                          }
                        >
                          <Trash2 size={14} />
                          Удалить
                        </button>
                      )}
                    </div>
                  )}
                </article>
              ),
            )}
            <span
              ref={lastMarker}
              className="chat-read-marker"
              aria-hidden="true"
            />
          </div>
        )}
        <ErrorNotice>{downloadError || null}</ErrorNotice>
        {!readOnly && (
          <form
            className="chat-composer"
            onSubmit={
              /**
               * onSubmit обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               * @parameters:
               *   - event — проверенный конверт события комнаты.
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ (event) => {
                event.preventDefault();
                setValidation("");
                if (
                  (!text.trim() && !attachments.length) ||
                  Array.from(text.trim()).length > 4000
                ) {
                  setValidation(
                    "Напишите сообщение до 4000 символов или прикрепите файл.",
                  );
                  return;
                }
                if (!uploadBusy) send.mutate();
              }
            }
          >
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
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: setReply(null).
                     */ () => setReply(null)
                  }
                >
                  <X size={16} />
                </button>
              </div>
            )}
            <label className="field" htmlFor={`chat-text-${conferenceId}`}>
              Сообщение
              <textarea
                id={`chat-text-${conferenceId}`}
                value={text}
                onChange={
                  /**
                   * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   * @parameters:
                   *   - event — проверенный конверт события комнаты.
                   *
                   * @returns вычисленное значение: setText(event.target.value).
                   */ (event) => setText(event.target.value)
                }
                rows={3}
                placeholder="Напишите участникам встречи…"
                disabled={send.isPending}
              />
            </label>
            <AttachmentUploader
              key={composerGeneration}
              conferenceId={conferenceId}
              value={attachments}
              onChange={setAttachments}
              onBusy={setUploadBusy}
              disabled={send.isPending}
            />
            <ErrorNotice error={send.error}>{validation || null}</ErrorNotice>
            <Button
              type="submit"
              busy={send.isPending}
              disabled={uploadBusy || (!text.trim() && !attachments.length)}
            >
              <Send size={17} />
              {send.isError ? "Повторить отправку" : "Отправить"}
            </Button>
            <p className="field-hint">
              Если связь прервётся, повторная отправка не создаст копию
              сообщения.
            </p>
          </form>
        )}
      </div>
      {editing && (
        <Modal
          title="Изменить сообщение"
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {
              if (!edit.isPending) setEditing(null);
            }
          }
        >
          <form
            onSubmit={
              /**
               * onSubmit обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               * @parameters:
               *   - event — проверенный конверт события комнаты.
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ (event) => {
                event.preventDefault();
                if (
                  (editText.trim() || editing.attachments.length) &&
                  Array.from(editText.trim()).length <= 4000
                )
                  edit.mutate();
              }
            }
          >
            <label className="field" htmlFor="edit-message">
              Текст
              <textarea
                id="edit-message"
                data-autofocus
                value={editText}
                onChange={
                  /**
                   * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   * @parameters:
                   *   - event — проверенный конверт события комнаты.
                   *
                   * @returns вычисленное значение: setEditText(event.target.value).
                   */ (event) => setEditText(event.target.value)
                }
                rows={4}
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
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {
              if (!remove.isPending) setDeleting(null);
            }
          }
        >
          <p className="modal-description">
            Текст и вложения станут недоступны другим участникам. В чате
            останется отметка об удалении.
          </p>
          <ErrorNotice error={remove.error} />
          <Button
            variant="danger"
            busy={remove.isPending}
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: remove.mutate().
               */ () => remove.mutate()
            }
          >
            Да, удалить сообщение
          </Button>
        </Modal>
      )}
    </section>
  );
}
/**
 * PaperclipLabel показывает подпись действия прикрепления файла в чате.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
function PaperclipLabel() {
  return <span aria-hidden="true">↳ </span>;
}
