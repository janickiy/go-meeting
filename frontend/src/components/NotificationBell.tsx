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
export function NotificationBell() {
  const { user } = useAuth();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const query = useInfiniteQuery({
    queryKey: ["notifications", user?.id],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) => api.notifications(pageParam, signal),
    getNextPageParam: (last) => last.nextCursor || undefined,
    refetchInterval: 15000,
  });
  const read = useMutation({
    mutationFn: api.readNotification,
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["notifications", user?.id] });
    },
  });
  const unread = query.data?.pages[0]?.unreadCount || 0;
  const rows = [
    ...new Map(
      (query.data?.pages.flatMap((page) => page.items) || []).map((item) => [
        item.id,
        item,
      ]),
    ).values(),
  ];
  return (
    <>
      <button
        className="icon-button notification-bell"
        aria-label="Уведомления"
        onClick={() => setOpen(true)}
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
        <Modal title="Уведомления" onClose={() => setOpen(false)} wide>
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
                {rows.map((item) => (
                  <article
                    key={item.id}
                    className={`notification-row ${item.readAt ? "" : "notification-unread"}`}
                  >
                    <div>
                      <strong>{notificationLabel(item)}</strong>
                      <p className="field-hint">{formatDate(item.createdAt)}</p>
                      {item.payload.conferenceId && (
                        <Link
                          to={`/conferences/${encodeURIComponent(item.payload.conferenceId)}`}
                          onClick={() => {
                            if (!item.readAt) read.mutate(item.id);
                            setOpen(false);
                          }}
                        >
                          Открыть встречу
                        </Link>
                      )}
                    </div>
                    {!item.readAt && (
                      <Button
                        variant="outline"
                        busy={read.isPending && read.variables === item.id}
                        onClick={() => read.mutate(item.id)}
                        aria-label={`Прочитано: ${notificationLabel(item)}`}
                      >
                        <Check size={17} />
                      </Button>
                    )}
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
        </Modal>
      )}
    </>
  );
}
