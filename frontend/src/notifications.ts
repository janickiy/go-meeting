import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "./api";

/**
 * NotificationEvent ограничивает SSE-конверт событиями создания и прочтения уведомления.
 *
 * Состав:
 *   - version — версия изменения или протокола.
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - type — машинный тип события.
 *   - data — нагрузка события, проверяемая перед чтением.
 */
export interface NotificationEvent {
  version: 1;
  id: string;
  type: "notification.created" | "notification.read";
  data: { notification?: { payload?: { conferenceId?: string } }; id?: string };
}
/**
 * takeNotificationFrames выделяет полные SSE-блоки из накопленного текста, проверяет события и оставляет незавершённый хвост.
 *
 * @parameters:
 *   - buffer (string) — накопленный текст SSE, включая возможный незавершённый блок.
 *
 * @returns { events: NotificationEvent[]; rest: string; } — полные проверенные SSE-события и текст незавершённого хвоста для следующей порции.
 */
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
      .filter(
        /**
         * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @parameters:
         *   - line — строка входящего текстового потока.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */ (line) => line.startsWith("data:"),
      )
      .map(
        /**
         * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
         *
         * @parameters:
         *   - line — строка входящего текстового потока.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (line) => line.slice(5).trimStart(),
      )
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
      /* Повреждённое второстепенное уведомление не должно нарушать работу медиа. */
    }
  }
  return { events, rest };
}

// Один авторизованный поток на смонтированный интерфейс учётной записи. JWT не передаётся
// в адресе запроса; периодическая загрузка списка восстанавливает события, пропущенные при разрыве связи.
/**
 * useNotificationStream держит личный SSE-поток авторизованного пользователя, устраняет повторы и обновляет кеш связанных ресурсов.
 *
 * @parameters:
 *   - userId (string) — идентификатор текущего авторизованного пользователя (необязательный параметр).
 *
 * @returns значение не возвращается; хук поддерживает поток и обновляет кеш запросов, освобождая ресурсы при смене пользователя или удалении компонента.
 */
export function useNotificationStream(userId?: string) {
  const client = useQueryClient();
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (!userId) return;
      const controller = new AbortController();
      let timer: ReturnType<typeof setTimeout> | undefined;
      let retry = 0;
      const seen = new Set<string>();
      /**
       * connect открывает соединение, обрабатывает события и назначает повтор после временного отказа.
       *
       *
       * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      const connect = async () => {
        try {
          const response = await api.notificationEvents(controller.signal);
          if (controller.signal.aborted) return;
          const reader = response.body!.getReader();
          const decoder = new TextDecoder();
          let pending = "";
          retry = 0;
          void client.invalidateQueries({
            queryKey: ["notifications", userId],
          });
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
                  void client.invalidateQueries({
                    queryKey: ["transcript", userId, conferenceId],
                  });
                  void client.invalidateQueries({
                    queryKey: ["summary", userId, conferenceId],
                  });
                  void client.invalidateQueries({
                    queryKey: ["content-search", userId],
                  });
                }
              }
            }
          } finally {
            await reader.cancel().catch(
              /**
               * Обработчик catch выполняет переданный шаг вызова catch в личных уведомлениях.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {},
            );
            reader.releaseLock();
          }
        } catch {
          /* List polling remains available while the stream reconnects. */
        }
        if (!controller.signal.aborted)
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
      void connect();
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        controller.abort();
        clearTimeout(timer);
      };
    },
    [userId, client],
  );
}
