import { useState } from "react";
import { Link } from "react-router";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { Bell, Check } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { formatDate } from "../utils";
import type { Notification } from "../types";
import { Button, ErrorNotice, Loading, Modal } from "./ui";

/**
 * notificationLabel переводит тип и нагрузку уведомления в понятную подпись для списка.
 *
 * @parameters:
 *   - item (Notification) — элемент списка, который обрабатывает текущий шаг.
 *
 * @returns string — вычисленное значение: "Запись встречи готова"; "Скоро начнётся встреча"; "Вас пригласили войти во встречу"; "Запрос на вход отклонён"; "Вы исключены из встречи"; "Обновление вашей конференции".
 */
export function notificationLabel(item: Notification): string {
  if (item.type.includes("recording")) return "Запись встречи готова";
  if (item.type === "conference.soon") return "Скоро начнётся встреча";
  if (item.payload.admissionState === "admitted")
    return "Вас пригласили войти во встречу";
  if (item.payload.admissionState === "rejected")
    return "Запрос на вход отклонён";
  if (item.payload.admissionState === "kicked")
    return "Вы исключены из встречи";
  return "Обновление вашей конференции";
}
/**
 * NotificationBell показывает непрочитанные уведомления, поддерживает страницы и отметку прочтения.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function NotificationBell() {
  const { user } = useAuth();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const query = useInfiniteQuery({
    queryKey: ["notifications", user?.id],
    initialPageParam: undefined as string | undefined,
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @parameters:
     *   - объект параметров: pageParam — свойство текущего компонента; signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.notifications(pageParam, signal).
     */
    queryFn: ({ pageParam, signal }) => api.notifications(pageParam, signal),
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
  const read = useMutation({
    mutationFn: api.readNotification,
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["notifications", user?.id] });
    },
  });
  const unread = query.data?.pages[0]?.unreadCount || 0;
  const rows = [
    ...new Map(
      (
        query.data?.pages.flatMap(
          /**
           * Обработчик flatMap преобразует текущий элемент в данные или представление результирующего списка.
           *
           * @parameters:
           *   - page — изолированная страница Playwright.
           *
           * @returns преобразованное значение текущего элемента для результирующего набора.
           */ (page) => page.items,
        ) || []
      ).map(
        /**
         * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
         *
         * @parameters:
         *   - item — элемент списка, который обрабатывает текущий шаг.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (item) => [item.id, item],
      ),
    ).values(),
  ];
  return (
    <>
      <button
        className="icon-button notification-bell"
        aria-label="Уведомления"
        onClick={
          /**
           * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
           *
           *
           * @returns вычисленное значение: setOpen(true).
           */ () => setOpen(true)
        }
      >
        <Bell size={21} />
        {unread > 0 && (
          <span
            className="notification-count"
            aria-label={`${unread} непрочитанных уведомлений`}
          >
            {unread > 99 ? "99+" : unread}
          </span>
        )}
      </button>
      {open && (
        <Modal
          title="Уведомления"
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: setOpen(false).
             */ () => setOpen(false)
          }
          wide
        >
          <ErrorNotice error={query.error || read.error} />
          {query.isPending ? (
            <Loading />
          ) : (
            <>
              {!rows.length && !query.isError && (
                <p className="empty-state compact-empty">
                  Пока нет уведомлений. Здесь появятся приглашения и готовые
                  записи.
                </p>
              )}
              <div className="notification-list">
                {rows.map(
                  /**
                   * Обработчик rows.map преобразует один элемент набора в представление или данные следующего шага.
                   *
                   * @parameters:
                   *   - item — элемент списка, который обрабатывает текущий шаг.
                   *
                   * @returns преобразованное значение текущего элемента для результирующего набора.
                   */ (item) => (
                    <article
                      key={item.id}
                      className={`notification-row ${item.readAt ? "" : "notification-unread"}`}
                    >
                      <div>
                        <strong>{notificationLabel(item)}</strong>
                        <p className="field-hint">
                          {formatDate(item.createdAt)}
                        </p>
                        {item.payload.conferenceId && (
                          <Link
                            to={`/conferences/${encodeURIComponent(item.payload.conferenceId)}`}
                            onClick={
                              /**
                               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                               *
                               *
                               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                               */ () => {
                                if (!item.readAt) read.mutate(item.id);
                                setOpen(false);
                              }
                            }
                          >
                            Открыть встречу
                          </Link>
                        )}
                      </div>
                      {!item.readAt && (
                        <Button
                          variant="outline"
                          busy={read.isPending && read.variables === item.id}
                          onClick={
                            /**
                             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                             *
                             *
                             * @returns вычисленное значение: read.mutate(item.id).
                             */ () => read.mutate(item.id)
                          }
                          aria-label={`Прочитано: ${notificationLabel(item)}`}
                        >
                          <Check size={17} />
                        </Button>
                      )}
                    </article>
                  ),
                )}
              </div>
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
                  Ещё уведомления
                </Button>
              )}
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
                  Попробовать снова
                </Button>
              )}
            </>
          )}
        </Modal>
      )}
    </>
  );
}
