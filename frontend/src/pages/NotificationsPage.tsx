import { Link } from "react-router";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { ArrowUpRight, Bell, Check } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { notificationLabel } from "../components/NotificationBell";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { notificationLink } from "../intelligence";
import { formatDate } from "../utils";
import "./history-notifications.css";

/** Полная история личных уведомлений с серверным курсором и отметкой прочтения. */
export function NotificationsPage() {
  const { user } = useAuth();
  const client = useQueryClient();
  const query = useInfiniteQuery({
    queryKey: ["notifications", user?.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => api.notifications(pageParam, signal),
    getNextPageParam: (last) => last.nextCursor || undefined,
  });
  const read = useMutation({
    mutationFn: api.readNotification,
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["notifications", user?.id] });
    },
  });
  const rows = [
    ...new Map(
      (query.data?.pages.flatMap((page) => page.items) || []).map((item) => [
        item.id,
        item,
      ]),
    ).values(),
  ];
  const unread = query.data?.pages[0]?.unreadCount || 0;

  return (
    <div className="notifications-page">
      <section className="page-heading">
        <div>
          <h1>Уведомления</h1>
          <p>Приглашения, напоминания и материалы встреч.</p>
        </div>
        <span className="notifications-count" role="status">
          Непрочитанных: {unread}
        </span>
      </section>
      <section className="content-card" aria-label="Список уведомлений">
        <ErrorNotice error={query.error || read.error} />
        {query.isPending ? (
          <Loading />
        ) : (
          <>
            {!rows.length && !query.isError && (
              <div className="notifications-empty">
                <Bell size={32} aria-hidden="true" />
                <h2>Пока тихо</h2>
                <p>
                  Пока нет уведомлений. Здесь появятся приглашения и новости о
                  ваших встречах.
                </p>
              </div>
            )}
            <div className="notification-list notifications-page-list">
              {rows.map((item) => (
                <article
                  key={item.id}
                  className={`notification-row ${item.readAt ? "" : "notification-unread"}`}
                >
                  <span className="notification-page-icon" aria-hidden="true">
                    <Bell size={22} />
                  </span>
                  <div className="notification-page-copy">
                    <div className="notification-page-title">
                      <strong>{notificationLabel(item)}</strong>
                      {!item.readAt && (
                        <span
                          className="notification-unread-dot"
                          aria-label="Непрочитанное"
                        />
                      )}
                    </div>
                    <time dateTime={item.createdAt}>
                      {formatDate(item.createdAt)}
                    </time>
                    <div className="notification-page-actions">
                      {item.payload.conferenceId && (
                        <Link
                          to={notificationLink(item)}
                          onClick={() => {
                            if (!item.readAt) read.mutate(item.id);
                          }}
                        >
                          Открыть встречу{" "}
                          <ArrowUpRight size={15} aria-hidden="true" />
                        </Link>
                      )}
                      {!item.readAt && (
                        <Button
                          variant="outline"
                          busy={read.isPending && read.variables === item.id}
                          onClick={() => read.mutate(item.id)}
                          aria-label={`Отметить как прочитанное: ${notificationLabel(item)}`}
                        >
                          <Check size={17} /> Прочитано
                        </Button>
                      )}
                    </div>
                  </div>
                </article>
              ))}
            </div>
            {query.hasNextPage && (
              <Button
                variant="outline"
                busy={query.isFetchingNextPage}
                onClick={() => void query.fetchNextPage()}
              >
                Ещё уведомления
              </Button>
            )}
            {query.isError && (
              <Button variant="outline" onClick={() => void query.refetch()}>
                Попробовать снова
              </Button>
            )}
          </>
        )}
      </section>
    </div>
  );
}
