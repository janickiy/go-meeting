import { useEffect, useState } from "react";
import { Link, useParams } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  CalendarDays,
  Check,
  Info,
  LogIn,
  LogOut,
  Play,
  Square,
  Users,
  Video,
  X,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { useConference, useParticipants } from "../queries";
import { formatDate, initials, inviteLink } from "../utils";
import {
  Button,
  CopyLink,
  ErrorNotice,
  Loading,
  Modal,
  StatusBadge,
} from "../components/ui";

export function ConferencePage() {
  const { id = "" } = useParams();
  const { user } = useAuth();
  const client = useQueryClient();
  const query = useConference(id);
  const participants = useParticipants(id);
  const people = participants.data?.pages.flatMap((page) => page.items) || [];
  const membership = people.find((item) => item.userId === user?.id);
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = participants;
  useEffect(() => {
    // The current user's membership can be outside the first API page.
    if (
      !membership &&
      hasNextPage &&
      !isFetchingNextPage &&
      !participants.isError
    )
      void fetchNextPage();
  }, [
    membership,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
    participants.isError,
  ]);
  const [confirm, setConfirm] = useState<"finish" | "cancel" | null>(null);
  const mutation = useMutation({
    mutationFn: (action: "start" | "finish" | "cancel" | "join" | "leave") =>
      action === "join" || action === "leave"
        ? api.membership(id, action).then(() => {})
        : api.transition(id, action).then(() => {}),
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["conference"] });
      void client.invalidateQueries({ queryKey: ["conferences"] });
      void client.invalidateQueries({ queryKey: ["participants"] });
    },
    onSuccess: () => setConfirm(null),
  });
  if (query.isPending) return <Loading />;
  if (query.isError || !query.data)
    return (
      <div className="content-card">
        <ErrorNotice error={query.error || new Error()} />
        <Link to="/conferences" className="text-link">
          <ArrowLeft size={16} />К моим конференциям
        </Link>
      </div>
    );
  const conference = query.data.item;
  const owner = conference.ownerId === user?.id;
  const closed =
    conference.status === "finished" || conference.status === "cancelled";
  const roleNames = {
    owner: "Организатор",
    co_host: "Соорганизатор",
    participant: "Участник",
    guest: "Гость",
  };
  const presence = {
    joined: "Присоединился",
    left: "Вышел",
    waiting: "Ожидает",
    rejected: "Отклонён",
    kicked: "Исключён",
  };
  return (
    <>
      <Link className="back-link" to="/conferences">
        <ArrowLeft size={17} />
        Мои конференции
      </Link>
      <section className="page-heading conference-heading">
        <div>
          <span className="eyebrow">КОНФЕРЕНЦИЯ</span>
          <h1>{conference.title}</h1>
          <p>
            <CalendarDays size={15} />
            Создана {formatDate(conference.createdAt)}
          </p>
        </div>
        <StatusBadge status={conference.status} />
      </section>
      <ErrorNotice error={mutation.error} />
      <div className="conference-grid">
        <section className="content-card meeting-card">
          <div className="meeting-card-symbol">
            <Video size={35} />
          </div>
          <h2>
            {closed
              ? "Встреча закрыта"
              : conference.status === "active"
                ? "Конференция началась"
                : "Всё готово к встрече"}
          </h2>
          <p className="muted">
            {closed
              ? "Участники и история конференции сохранены."
              : owner
                ? "Управляйте встречей и приглашайте участников."
                : "Присоединитесь к встрече, когда будете готовы."}
          </p>
          <div className="meeting-actions">
            {!closed && (
              <Button
                busy={mutation.isPending}
                variant={
                  membership?.status === "joined" ? "secondary" : "primary"
                }
                onClick={() =>
                  mutation.mutate(
                    membership?.status === "joined" ? "leave" : "join",
                  )
                }
                disabled={
                  participants.isPending ||
                  participants.isError ||
                  (!membership && !!participants.hasNextPage)
                }
              >
                {membership?.status === "joined" ? (
                  <>
                    <LogOut size={18} />
                    Покинуть конференцию
                  </>
                ) : (
                  <>
                    <LogIn size={18} />
                    Присоединиться
                  </>
                )}
              </Button>
            )}
            {owner && conference.status === "created" && (
              <>
                <Button
                  variant="outline"
                  busy={mutation.isPending}
                  onClick={() => mutation.mutate("start")}
                >
                  <Play size={17} />
                  Начать конференцию
                </Button>
                <Button
                  variant="secondary"
                  disabled={mutation.isPending}
                  onClick={() => setConfirm("cancel")}
                >
                  <X size={17} />
                  Отменить конференцию
                </Button>
              </>
            )}
            {owner && conference.status === "active" && (
              <Button
                variant="danger"
                disabled={mutation.isPending}
                onClick={() => setConfirm("finish")}
              >
                <Square size={16} />
                Завершить конференцию
              </Button>
            )}
          </div>
          {membership?.status === "joined" && !closed && (
            <p className="membership-note">
              <Check size={15} />
              Вы присоединились к конференции
            </p>
          )}
          <div className="video-notice">
            <Info size={18} />
            <span>
              Сейчас доступно управление конференцией. Видеозвонки, камера и
              микрофон будут подключены на следующем этапе.
            </span>
          </div>
          {conference.startedAt && (
            <p className="field-hint">
              Начало: {formatDate(conference.startedAt)}
            </p>
          )}
          {conference.finishedAt && (
            <p className="field-hint">
              Завершение: {formatDate(conference.finishedAt)}
            </p>
          )}
        </section>
        <aside className="content-card invitation-card">
          <span className="eyebrow">ПРИГЛАСИТЕ КОЛЛЕГ</span>
          <h2>
            Встреча начинается
            <br />с приглашения
          </h2>
          <p className="muted">
            Отправьте ссылку тем, с кем хотите встретиться.
          </p>
          <CopyLink value={inviteLink(conference.inviteCode)} />
          <div className="invite-code">
            <span>Код приглашения</span>
            <code>{conference.inviteCode}</code>
          </div>
          <p className="field-hint">
            Для присоединения нужен аккаунт Meet.
            {closed ? " Эта конференция уже закрыта." : ""}
          </p>
        </aside>
      </div>
      <section className="content-card participants-card">
        <div className="section-heading">
          <h2>
            <Users size={20} />
            Участники{" "}
            <span className="count-badge">
              {people.length}
              {participants.hasNextPage ? "+" : ""}
            </span>
          </h2>
          <span className="muted small">Обновляется автоматически</span>
        </div>
        <ErrorNotice error={participants.error} />
        {participants.isPending ? (
          <Loading />
        ) : (
          <div className="participant-list">
            {people.map((person) => (
              <div className="participant-row" key={person.id}>
                <span
                  className={`avatar ${person.role === "owner" ? "avatar-owner" : ""}`}
                >
                  {initials(person.displayName)}
                </span>
                <div className="participant-name">
                  <strong>
                    {person.displayName}
                    {person.userId === user?.id && (
                      <span className="muted"> (вы)</span>
                    )}
                  </strong>
                  <span>{roleNames[person.role]}</span>
                </div>
                <span
                  className={`participant-status ${person.status === "joined" ? "participant-joined" : ""}`}
                >
                  <span className="presence-dot" />
                  {person.joinedAt
                    ? presence[person.status]
                    : "Ещё не присоединялся"}
                </span>
              </div>
            ))}
          </div>
        )}
        {participants.hasNextPage && (
          <Button
            variant="outline"
            busy={participants.isFetchingNextPage}
            onClick={() => {
              void participants.fetchNextPage();
            }}
          >
            Загрузить ещё участников
          </Button>
        )}
        <p className="field-hint">
          Статус отражает join/leave, а не подключение к видеосвязи.
        </p>
      </section>
      {confirm && (
        <Modal
          title={
            confirm === "finish"
              ? "Завершить конференцию?"
              : "Отменить конференцию?"
          }
          onClose={() => {
            if (!mutation.isPending) setConfirm(null);
          }}
        >
          <p className="modal-description">
            После этого участники не смогут присоединиться. Это действие нельзя
            отменить.
          </p>
          <ErrorNotice error={mutation.error} />
          <Button
            variant="danger"
            className="full-width"
            busy={mutation.isPending}
            onClick={() => mutation.mutate(confirm)}
          >
            {confirm === "finish" ? "Да, завершить" : "Да, отменить"}
          </Button>
          <Button
            variant="secondary"
            className="full-width"
            disabled={mutation.isPending}
            onClick={() => setConfirm(null)}
          >
            Вернуться к встрече
          </Button>
        </Modal>
      )}
    </>
  );
}
