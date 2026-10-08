import { Link, useNavigate } from "react-router";
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
import { Button, CopyLink, ErrorNotice, StatusBadge } from "../ui";
import { formatDate, inviteLink } from "../../utils";
import type { ConferenceViewProps } from "./types";

/**
 * Отображает карточку встречи, расписание и доступные действия до входа или после завершения.
 * @args props — общие данные, команды и состояние интерфейса, созданные корневой страницей.
 * @return Представление раздела с прежними условиями доступа и монтирования панелей.
 */
export function ConferenceDetails(props: ConferenceViewProps) {
  const { id } = props;
  const {
    user,
    self,
    conference,
    membership,
    admitted,
    closed,
    owner,
    canInvite,
  } = props.data;
  const { mutation, moderation } = props.commands;
  const { setUtility, setConfirm, setEditingSchedule } = props.controls;
  const navigate = useNavigate();
  return (
    <>
      <Link
        className="back-link"
        to={user?.guestConferenceId ? "/" : "/conferences"}
      >
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
          {conference.scheduledAt && (
            <p>
              Запланирована: {formatDate(conference.scheduledAt)}
              {conference.plannedDurationMin
                ? ` · ${conference.plannedDurationMin} мин`
                : ""}
            </p>
          )}
        </div>
        <StatusBadge status={conference.status} />
      </section>
      <ErrorNotice error={mutation.error} />
      <ErrorNotice error={moderation.error} />
      <ErrorNotice error={self.error} />
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
            {!closed &&
              conference.status !== "scheduled" &&
              !["kicked", "waiting", "rejected"].includes(
                membership?.status || "",
              ) && (
                <Button
                  busy={mutation.isPending}
                  variant={
                    membership?.status === "joined" ? "secondary" : "primary"
                  }
                  onClick={() => {
                    if (membership?.status === "joined")
                      mutation.mutate("leave");
                    else
                      navigate(
                        user?.guestConferenceId
                          ? `/i/${conference.inviteCode}`
                          : `/conferences/${id}/join`,
                      );
                  }}
                  disabled={self.isPending || self.isError}
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
            {owner && ["created", "scheduled"].includes(conference.status) && (
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
                {conference.status === "scheduled" && (
                  <Button
                    variant="secondary"
                    onClick={() => setEditingSchedule(true)}
                    disabled={mutation.isPending}
                  >
                    Изменить расписание
                  </Button>
                )}
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
          {conference.status === "scheduled" &&
            (admitted || membership?.admissionState === "waiting") && (
              <p className="membership-note">
                <Check size={15} />
                Встреча добавлена в ваш список.{" "}
                {admitted
                  ? "Войти можно после её начала."
                  : "Организатор рассмотрит запрос на вход после начала встречи."}
              </p>
            )}
          <div className="video-notice">
            <Info size={18} />
            <span>
              После присоединения можно включить камеру и микрофон в блоке
              медиасвязи. Организатор может включить общую запись встречи.
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
        {admitted && (
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
            {canInvite && (
              <Button variant="secondary" onClick={() => setUtility("invite")}>
                <Users size={17} />
                Пригласить участников
              </Button>
            )}
            <div className="invite-code">
              <span>Код приглашения</span>
              <code>{conference.inviteCode}</code>
            </div>
            <p className="field-hint">
              По ссылке можно присоединиться без аккаунта.
              {closed ? " Эта конференция уже закрыта." : ""}
            </p>
          </aside>
        )}
      </div>
    </>
  );
}
