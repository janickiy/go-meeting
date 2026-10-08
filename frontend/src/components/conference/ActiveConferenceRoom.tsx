import { Link } from "react-router";
import {
  ArrowLeft,
  Circle,
  Link as LinkIcon,
  LogOut,
  MessageCircle,
  Square,
  Users,
} from "lucide-react";
import { RealtimePanel } from "../RealtimePanel";
import { RecordingNotice } from "../RecordingNotice";
import { RecordingPanel } from "../RecordingPanel";
import { ConferenceInviteContent } from "../ConferenceInvitations";
import { Button, ErrorNotice, Modal } from "../ui";
import { initials } from "../../utils";
import { MeetingClock } from "./MeetingClock";
import { ConferenceRoomSidebar } from "./ConferenceRoomSidebar";
import type { ActiveConferenceViewProps } from "./types";

/**
 * Отображает активную комнату: медиасвязь, верхнюю панель и диалоги записи/приглашения.
 * @args props — общие данные, команды и состояние интерфейса, созданные корневой страницей.
 * @return Представление раздела с прежними условиями доступа и монтирования панелей.
 */
export function ActiveConferenceRoom(props: ActiveConferenceViewProps) {
  const { id } = props;
  const {
    user,
    self,
    conference,
    membership,
    people,
    live,
    onlinePeople,
    recordingAccess,
    recordingStatus,
    activeRecording,
    owner,
    canInvite,
  } = props.data;
  const { mutation, moderation, stopRecording } = props.commands;
  const {
    stagePanel,
    setStagePanel,
    panelOpen,
    setPanelOpen,
    panelTrigger,
    reconnectTarget,
    setReconnectTarget,
    utility,
    setUtility,
    invitationBusy,
    setInvitationBusy,
    confirm,
    setConfirm,
  } = props.controls;
  const recordingLabel =
    activeRecording?.status === "starting"
      ? "Запись запускается"
      : activeRecording?.status === "stopping"
        ? "Запись останавливается"
        : "Идёт запись";
  return (
    <div
      className={`conference-room-page ${panelOpen ? "room-panel-open" : "room-panel-closed"}`}
    >
      <header className="room-header">
        <Link
          className="icon-button room-back"
          to={user?.guestConferenceId ? "/" : "/conferences"}
          aria-label="К моим конференциям"
        >
          <ArrowLeft size={19} />
        </Link>
        <div className="room-title">
          <h1>{conference.title}</h1>
          <span>
            {live.state ? "Встреча в эфире" : "Подключаемся к встрече"}
          </span>
        </div>
        <MeetingClock startedAt={conference.startedAt} />
        {activeRecording && (
          <span
            className="room-recording-status"
            role="status"
            data-testid="recording-indicator"
          >
            <Circle size={9} fill="currentColor" />
            {recordingLabel}
          </span>
        )}
        {activeRecording &&
          recordingAccess &&
          ((owner && membership.role === "owner") ||
            activeRecording.requestedBy === user?.id) && (
            <button
              type="button"
              className="room-header-action room-header-stop-recording"
              aria-label="Остановить запись"
              aria-busy={stopRecording.isPending || undefined}
              title={
                stopRecording.isPending || activeRecording.status === "stopping"
                  ? "Запись останавливается"
                  : "Остановить запись"
              }
              disabled={
                stopRecording.isPending || activeRecording.status === "stopping"
              }
              onClick={() => {
                if (
                  !stopRecording.isPending &&
                  activeRecording.status !== "stopping"
                )
                  stopRecording.mutate(activeRecording.uuid);
              }}
            >
              <Square size={17} fill="currentColor" aria-hidden="true" />
              <span>
                {stopRecording.isPending ||
                activeRecording.status === "stopping"
                  ? "Останавливаем…"
                  : "Остановить запись"}
              </span>
            </button>
          )}
        {recordingAccess && (
          <button
            className="room-header-action"
            onClick={() => setUtility("recording")}
            aria-label="Записи конференции"
          >
            <Circle size={17} />
            <span>Запись</span>
          </button>
        )}
        <button
          className="room-header-action"
          onClick={() => setUtility("invite")}
        >
          <LinkIcon size={17} />
          <span>Пригласить</span>
        </button>
        <span
          className="avatar avatar-small room-self-avatar"
          aria-label={membership.displayName}
        >
          {initials(membership.displayName)}
        </span>
        <div className="room-reconnect-slot" ref={setReconnectTarget} />
      </header>
      <RecordingNotice
        conferenceId={id}
        ownerId={conference.ownerId}
        userId={user?.id}
        participants={
          people.some((person) => person.id === membership.id)
            ? people
            : [...people, membership]
        }
        recordings={
          recordingStatus.isSuccess && !recordingStatus.isError
            ? recordingStatus.data.items
            : undefined
        }
        subscribe={live.subscribe}
      />
      <div className="room-errors">
        <ErrorNotice
          error={
            stopRecording.error ||
            recordingStatus.error ||
            mutation.error ||
            moderation.error ||
            self.error
          }
        />
      </div>
      <section className="conference-stage" aria-label="Активная встреча">
        <div className="conference-stage-main">
          <RealtimePanel
            conferenceId={id}
            membership={membership}
            live={live}
            participants={people}
            reconnectTarget={reconnectTarget}
            controls={
              <>
                <Button
                  variant="secondary"
                  aria-expanded={panelOpen && stagePanel === "participants"}
                  onClick={(event) => {
                    panelTrigger.current = event.currentTarget;
                    setStagePanel("participants");
                    setPanelOpen(true);
                  }}
                >
                  <Users size={18} />
                  Участники ({onlinePeople.length})
                </Button>
                <Button
                  variant="secondary"
                  aria-keyshortcuts="C"
                  aria-expanded={panelOpen && stagePanel === "chat"}
                  onClick={(event) => {
                    panelTrigger.current = event.currentTarget;
                    setStagePanel("chat");
                    setPanelOpen(true);
                  }}
                >
                  <MessageCircle size={18} />
                  Чат
                </Button>
                <Button
                  variant="danger"
                  busy={mutation.isPending}
                  aria-label="Покинуть конференцию"
                  onClick={() => mutation.mutate("leave")}
                >
                  <LogOut size={18} />
                  Выйти
                </Button>
              </>
            }
          />
        </div>
        <ConferenceRoomSidebar {...props} />
      </section>
      {utility && (utility !== "recording" || recordingAccess) && (
        <Modal
          title={
            utility === "recording"
              ? "Записи конференции"
              : "Пригласить участников"
          }
          onClose={() => {
            if (!invitationBusy) setUtility(null);
          }}
        >
          {utility === "recording" ? (
            <RecordingPanel
              conference={conference}
              membership={membership}
              showHistory={false}
              onStarted={() => setUtility(null)}
            />
          ) : (
            <ConferenceInviteContent
              conference={conference}
              canInvite={canInvite}
              onBusyChange={setInvitationBusy}
            />
          )}
        </Modal>
      )}
      {confirm && (
        <Modal
          title="Завершить конференцию?"
          onClose={() => {
            if (!mutation.isPending) setConfirm(null);
          }}
        >
          <p className="modal-description">
            После завершения участники не смогут присоединиться. Текущая запись
            остановится и будет обработана в фоне.
          </p>
          <ErrorNotice error={mutation.error} />
          <Button
            variant="danger"
            busy={mutation.isPending}
            onClick={() => mutation.mutate("finish")}
          >
            Да, завершить
          </Button>
          <Button
            variant="secondary"
            disabled={mutation.isPending}
            onClick={() => setConfirm(null)}
          >
            Вернуться к встрече
          </Button>
        </Modal>
      )}
    </div>
  );
}
