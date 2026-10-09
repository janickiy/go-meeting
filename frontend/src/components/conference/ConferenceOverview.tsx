import { RealtimePanel } from "../RealtimePanel";
import { RecordingPanel } from "../RecordingPanel";
import { CaptionsPanel } from "../CaptionsPanel";
import { AnalyticsPanel } from "../AnalyticsPanel";
import { WaitingRoomPanel } from "../WaitingRoomPanel";
import { ChatPanel } from "../ChatPanel";
import { ConferenceCalendarStatus } from "../IntegrationsSettings";
import { ConferenceInviteContent } from "../ConferenceInvitations";
import { EditSchedule } from "../ConferenceModals";
import { Brand, Button, ErrorNotice, Modal } from "../ui";
import { ConferenceDetails } from "./ConferenceDetails";
import { ConferenceParticipants } from "./ConferenceParticipants";
import { ConferenceHistorySummary } from "./ConferenceHistorySummary";
import type { ConferenceViewProps } from "./types";
import "./conference-overview.css";

/**
 * Собирает представление ожидания, запланированной и завершённой встречи.
 * @args props — общие данные, команды и состояние интерфейса, созданные корневой страницей.
 * @return Представление раздела с прежними условиями доступа и монтирования панелей.
 */
export function ConferenceOverview(props: ConferenceViewProps) {
  const { id } = props;
  const {
    user,
    conference,
    membership,
    admitted,
    people,
    closed,
    live,
    capabilities,
    captionsEnabled,
    analyticsEnabled,
    activeMeeting,
    recordingAccess,
    canInvite,
  } = props.data;
  const { mutation } = props.commands;
  const {
    focusMessageId,
    latestMessages,
    utility,
    setUtility,
    invitationBusy,
    setInvitationBusy,
    confirm,
    setConfirm,
    editingSchedule,
    setEditingSchedule,
  } = props.controls;
  const awaitingAdmission =
    membership?.admissionState === "waiting" ||
    membership?.admissionState === "rejected" ||
    membership?.status === "waiting" ||
    membership?.status === "rejected";

  if (awaitingAdmission)
    return (
      <div className="conference-waiting-page">
        <Brand to={user?.guestConferenceId ? "/" : "/app"} />
        <div className="conference-waiting-content">
          <WaitingRoomPanel
            conferenceId={id}
            membership={membership}
            participants={people}
            active={conference.status === "active"}
            closed={closed}
            standalone
          />
          <Link
            className="button button-secondary conference-waiting-back"
            to={user?.guestConferenceId ? "/" : "/conferences"}
          >
            <ArrowLeft size={18} />
            {user?.guestConferenceId ? "На главную" : "К встречам"}
          </Link>
          {!closed &&
            membership?.status === "left" &&
            membership.admissionState === "waiting" &&
            conference.status !== "scheduled" && (
              <Link
                className="button button-primary"
                to={
                  user?.guestConferenceId
                    ? `/i/${conference.inviteCode}`
                    : `/conferences/${id}/join`
                }
              >
                Присоединиться снова
              </Link>
            )}
          <section className="content-card conference-waiting-context">
            <h2>{conference.title}</h2>
            <p className="muted">
              Имя во встрече: {membership?.displayName || user?.displayName}
            </p>
          </section>
          {!closed &&
            membership?.admissionState !== "rejected" &&
            membership?.status !== "rejected" && (
              <p className="conference-waiting-hint">
                Камера, микрофон и чат будут доступны после допуска.
              </p>
            )}
        </div>
      </div>
    );

  if (closed && admitted && membership)
    return (
      <div className="conference-history-chat">
        <Link
          className="back-link"
          to={user?.guestConferenceId ? "/" : `/history/${id}`}
        >
          <ArrowLeft size={17} />
          {user?.guestConferenceId ? "На главную" : "К материалам встречи"}
        </Link>
        <section className="page-heading">
          <div>
            <h1>{conference.title}</h1>
            <p>Чат завершённой встречи</p>
          </div>
          <span className="conference-readonly-badge">Только чтение</span>
        </section>
        <div className="conference-history-chat-card">
          <ChatPanel
            key={`${id}:${focusMessageId || "latest"}`}
            conferenceId={id}
            membership={membership}
            focusMessageId={focusMessageId}
            onLatest={latestMessages}
            readOnly
            readOnlyReason="Встреча завершена. Сообщения сохранены, отправка новых сообщений недоступна."
          />
          <p className="conference-history-chat-access">
            История доступна только участникам с сохранённым доступом.
          </p>
          <div className="conference-readonly-composer">
            <LockKeyhole size={18} aria-hidden="true" />
            <span>Чат завершённой встречи доступен только для чтения</span>
          </div>
        </div>
      </div>
    );

  return (
    <>
      <ConferenceDetails {...props} />
      {utility === "invite" && (
        <Modal
          title="Пригласить участников"
          onClose={() => {
            if (!invitationBusy) setUtility(null);
          }}
        >
          <ConferenceInviteContent
            conference={conference}
            canInvite={canInvite}
            onBusyChange={setInvitationBusy}
          />
        </Modal>
      )}
      <WaitingRoomPanel
        conferenceId={id}
        membership={membership}
        participants={people}
        active={conference.status === "active"}
        closed={closed}
      />
      {admitted &&
        membership?.status === "joined" &&
        !closed &&
        !activeMeeting && (
          <RealtimePanel
            conferenceId={id}
            membership={membership}
            live={live}
            shortcutsEnabled={false}
          />
        )}
      {membership?.status === "kicked" && (
        <ErrorNotice>
          Организатор исключил вас из конференции. Повторное присоединение
          недоступно.
        </ErrorNotice>
      )}
      {recordingAccess && (
        <RecordingPanel
          conference={conference}
          membership={membership}
          showInsights
        />
      )}
      {admitted && captionsEnabled && !activeMeeting && (
        <CaptionsPanel
          key={`captions-${id}`}
          conferenceId={id}
          active={
            conference.status === "active" && membership?.status === "joined"
          }
          live={live}
        />
      )}
      {admitted && analyticsEnabled && (
        <AnalyticsPanel
          key={`analytics-${id}`}
          conferenceId={id}
          active={conference.status === "active"}
        />
      )}
      {admitted && capabilities.isSuccess && !analyticsEnabled && (
        <p className="conference-feature-note">
          Аналитика встречи отключена для этой установки.
        </p>
      )}
      {admitted &&
        user &&
        (membership?.role === "owner" || membership?.role === "co_host") && (
          <ConferenceCalendarStatus
            conferenceId={conference.id}
            userId={user.id}
          />
        )}
      {admitted && membership && !activeMeeting && (
        <ChatPanel
          key={`${id}:${focusMessageId || "latest"}`}
          conferenceId={id}
          membership={membership}
          focusMessageId={focusMessageId}
          onLatest={latestMessages}
          readOnly={
            conference.status !== "active" || membership.status !== "joined"
          }
          readOnlyReason={
            closed
              ? undefined
              : "Присоединитесь к активной встрече, чтобы отправлять сообщения."
          }
        />
      )}
      <ConferenceHistorySummary {...props} />
      <ConferenceParticipants {...props} />
      {editingSchedule && (
        <EditSchedule
          conference={conference}
          onClose={() => setEditingSchedule(false)}
        />
      )}
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
            {confirm === "finish" &&
              " Текущая запись остановится и будет обработана в фоне."}
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
import { Link } from "react-router";
import { ArrowLeft, LockKeyhole } from "lucide-react";
