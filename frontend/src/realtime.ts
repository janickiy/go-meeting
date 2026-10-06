import { useCallback, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "./api";
import type { RealtimeEvent, RealtimeState } from "./types";

/**
 * ClientRealtimeType ограничивает виды сообщений, которые браузер вправе отправить по сокету.
 *
 */
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

/**
 * websocketURL строит адрес комнаты ws/wss по адресу страницы и одноразовому билету подключения.
 *
 * @args
 *   - conferenceId (string) — идентификатор конференции и области данных.
 *   - ticket (string) — входное значение ticket текущего шага обработки.
 *   - origin — входное значение origin текущего шага обработки (по умолчанию window.location.origin).
 *
 * @returns полный адрес ws/wss с краткоживущим билетом подключения.
 */
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
/**
 * parseRealtime проверяет формат, версию и размер входящего realtime-конверта до обработки.
 *
 * @args
 *   - raw (string) — входное значение raw текущего шага обработки.
 *   - conferenceId (string) — идентификатор конференции и области данных.
 *
 * @returns RealtimeEvent | null — проверенный конверт события либо null, если формат, версия или размер недопустимы.
 */
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

/**
 * useRealtime управляет одним WebSocket комнаты, переподключением, снимком присутствия и подписчиками событий.
 *
 * @args
 *   - conferenceId (string) — идентификатор конференции и области данных.
 *   - enabled (boolean) — разрешает выполнение запроса или подключение при выполненных условиях доступа.
 *
 * @returns состояние, данные или действия React-хука; ресурсы освобождаются при изменении зависимостей.
 */
export function useRealtime(conferenceId: string, enabled: boolean) {
  const queryClient = useQueryClient();
  const [state, setState] = useState<RealtimeState | null>(null);
  const [status, setStatus] = useState("Отключено");
  const [error, setError] = useState<Error | null>(null);
  const [events, setEvents] = useState<string[]>([]);
  const [generation, setGeneration] = useState(0);
  const socket = useRef<WebSocket | null>(null);
  const listener = useRef<
    /**
     * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
     *
     * @args
     *   - event (RealtimeEvent) — проверенный конверт события комнаты.
     *
     * @returns void — значение не возвращается; функция выполняет описанные действия.
     */ (event: RealtimeEvent) => void
  >(
    /**
     * Обработчик useRef выполняет переданный шаг вызова useRef в состоянии связи и WebRTC-медиа.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */ () => {},
  );
  const subscribers = useRef(
    new Set<
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       * @args
       *   - event (RealtimeEvent) — проверенный конверт события комнаты.
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ (event: RealtimeEvent) => void
    >(),
  );
  const subscribe = useCallback(
    /**
     * Обработчик useCallback выполняет действие с текущими зависимостями React-компонента.
     *
     * @args
     *   - callback ((event: RealtimeEvent) => void) — обработчик события или изменения наблюдаемого состояния.
     *
     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
     */ (
      callback: /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
       *
       * @args
       *   - event (RealtimeEvent) — проверенный конверт события комнаты.
       *
       * @returns void — значение не возвращается; функция выполняет описанные действия.
       */ (event: RealtimeEvent) => void,
    ) => {
      subscribers.current.add(callback);
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        subscribers.current.delete(callback);
      };
    },
    [],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
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
      let chatTimer: ReturnType<typeof setTimeout> | undefined;
      let chatDirty = false;
      /**
       * refreshChat объединяет обновления чата в ограниченное временное окно.
       *
       * @args
       *   - messages (boolean) — входное значение messages текущего шага обработки.
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      const refreshChat = (messages: boolean) => {
        chatDirty ||= messages;
        if (chatTimer) return;
        chatTimer = setTimeout(
          /**
           * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
           *
           *
           * @returns следующее состояние, рассчитанное из предыдущего значения.
           */ () => {
            chatTimer = undefined;
            if (disposed) return;
            if (chatDirty)
              void queryClient.invalidateQueries(
                { queryKey: ["chat", conferenceId] },
                { cancelRefetch: false },
              );
            chatDirty = false;
            void queryClient.invalidateQueries(
              { queryKey: ["chat-read", conferenceId] },
              { cancelRefetch: false },
            );
          },
          200,
        );
      };
      const abort = new AbortController();
      /**
       * invalidate обновляет связанные кеши после сохранённого изменения или события.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      const invalidate = () => {
        void queryClient.invalidateQueries({
          queryKey: ["conference"],
        });
        void queryClient.invalidateQueries({
          queryKey: ["participants"],
        });
        void queryClient.invalidateQueries({ queryKey: ["membership"] });
      };
      /**
       * connect открывает соединение, обрабатывает события и назначает повтор после временного отказа.
       *
       *
       * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
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
          ws.onopen =
            /**
             * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {
              if (!disposed) {
                setStatus("Подключено");
                setError(null);
                void queryClient.invalidateQueries({
                  queryKey: ["chat", conferenceId],
                });
                void queryClient.invalidateQueries({
                  queryKey: ["chat-read", conferenceId],
                });
                // После переподключения сверяем запись: события во время обрыва не повторяются.
                void queryClient.invalidateQueries({
                  queryKey: ["recordings", conferenceId],
                });
              }
            };
          ws.onmessage =
            /**
             * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
             *
             * @args
             *   - message — понятный текст ошибки или сообщение операции.
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ (message) => {
              if (disposed || typeof message.data !== "string") return;
              const e = parseRealtime(message.data, conferenceId);
              if (!e) return;
              // Показываем только имена событий; полные SDP, ICE и билеты не выводятся.
              if (e.type !== "reaction.created" && !e.type.startsWith("chat."))
                setEvents(
                  /**
                   * Обработчик setEvents вычисляет следующее React-состояние из предыдущего значения.
                   *
                   * @args
                   *   - old — предыдущее состояние перед вычислением нового.
                   *
                   * @returns следующее состояние, рассчитанное из предыдущего значения.
                   */ (old) => [e.type, ...old].slice(0, 8),
                );
              if (
                e.type.startsWith("participant.") &&
                !["participant.connected", "participant.disconnected"].includes(
                  e.type,
                )
              )
                invalidate();
              if (e.type.startsWith("recording."))
                void queryClient.invalidateQueries({
                  queryKey: ["recordings", conferenceId],
                });
              if (e.type.startsWith("chat.")) {
                refreshChat(e.type !== "chat.read.updated");
              }
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
                        /**
                         * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
                         *
                         * @args
                         *   - p — сведения об участнике конференции.
                         *
                         * @returns преобразованное значение текущего элемента для результирующего набора.
                         */
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
              for (const subscriber of subscribers.current) {
                try {
                  subscriber(e);
                } catch {
                  /* Второстепенные обработчики интерфейса не должны нарушать доставку сигнализации. */
                }
              }
            };
          ws.onerror =
            /**
             * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {
              if (!disposed)
                setError(
                  new Error("Не удалось подключиться к realtime-сервису."),
                );
            };
          ws.onclose =
            /**
             * Вложенный обработчик выполняет шаг «Вложенный обработчик» в состоянии связи и WebRTC-медиа.
             *
             * @args
             *   - event — проверенный конверт события комнаты.
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ (event) => {
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
                  "authentication_unavailable",
                ].includes(event.reason)
              ) {
                if (
                  ["authentication_expired", "authentication_revoked"].includes(
                    event.reason,
                  )
                )
                  void api.me().catch(
                    /**
                     * Обработчик catch выполняет переданный шаг вызова catch в состоянии связи и WebRTC-медиа.
                     *
                     *
                     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                     */ () => {},
                  );
                setError(
                  new Error(
                    "Подключение закрыто. Проверьте доступ к конференции.",
                  ),
                );
                return;
              }
              timer = setTimeout(
                /**
                 * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
                 *
                 *
                 * @returns следующее состояние, рассчитанное из предыдущего значения.
                 */
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
            /**
             * Обработчик setTimeout выполняет отложенную либо периодическую часть операции.
             *
             *
             * @returns следующее состояние, рассчитанное из предыдущего значения.
             */
            () => void connect(),
            Math.min(30000, 1000 * 2 ** Math.min(retry++, 5)),
          );
        }
      };
      void connect();
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        disposed = true;
        abort.abort();
        clearTimeout(timer);
        clearTimeout(chatTimer);
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
    },
    [conferenceId, enabled, generation, queryClient],
  );
  /**
   * send передаёт исходящее событие через действующее соединение.
   *
   * @args
   *   - type (ClientRealtimeType) — машинный тип события.
   *   - data (unknown) — нагрузка события, проверяемая перед чтением.
   *
   * @returns вычисленное значение: e.id.
   */
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
    subscribe,
    /**
     * reconnect закрывает старое соединение и получает новый билет и снимок комнаты.
     *
     *
     * @returns вычисленное значение: setGeneration( (n) => n + 1, ).
     */
    reconnect: () =>
      setGeneration(
        /**
         * Обработчик setGeneration вычисляет следующее React-состояние из предыдущего значения.
         *
         * @args
         *   - n — входное значение n текущего шага обработки.
         *
         * @returns следующее состояние, рассчитанное из предыдущего значения.
         */ (n) => n + 1,
      ),
  };
}
