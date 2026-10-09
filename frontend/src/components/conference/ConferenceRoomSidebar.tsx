import { Link as LinkIcon, Square, X } from "lucide-react";
import { ParticipantsPanel } from "../ParticipantsPanel";
import { WaitingRoomPanel } from "../WaitingRoomPanel";
import { CaptionsPanel } from "../CaptionsPanel";
import { ChatPanel } from "../ChatPanel";
import { Button } from "../ui";
import type { ActiveConferenceViewProps } from "./types";

/**
 * Показывает чат, участников и субтитры без размонтирования скрытых вкладок.
 * @args props — общие данные, команды и состояние интерфейса, созданные корневой страницей.
 * @return Представление раздела с прежними условиями доступа и монтирования панелей.
 */
export function ConferenceRoomSidebar(props: ActiveConferenceViewProps) {
  const { id } = props;
  const {
    membership,
    participants,
    people,
    live,
    onlinePeople,
    captionsEnabled,
    owner,
  } = props.data;
  const { mutation, moderation } = props.commands;
  const {
    focusMessageId,
    latestMessages,
    stagePanel,
    setStagePanel,
    panelOpen,
    setPanelOpen,
    panelTrigger,
    setUtility,
    setConfirm,
  } = props.controls;
  const panels = captionsEnabled
    ? (["chat", "participants", "captions"] as const)
    : (["chat", "participants"] as const);
  return (
    <aside
      className="conference-stage-rail"
      aria-label="Панели встречи"
      hidden={!panelOpen}
    >
      <div className="room-panel-header">
        <strong>
          {stagePanel === "chat"
            ? "Чат встречи"
            : stagePanel === "participants"
              ? "Участники встречи"
              : "Субтитры"}
        </strong>
        <button
          className="icon-button"
          aria-label="Закрыть панель встречи"
          onClick={() => {
            setPanelOpen(false);
            panelTrigger.current?.focus();
          }}
        >
          <X size={18} />
        </button>
      </div>
      <div
        className="conference-stage-tabs"
        role="tablist"
        aria-label="Панель встречи"
        onKeyDown={(event) => {
          const index = panels.findIndex((panel) => panel === stagePanel);
          let next = index;
          if (event.key === "ArrowRight") next = (index + 1) % panels.length;
          else if (event.key === "ArrowLeft")
            next = (index - 1 + panels.length) % panels.length;
          else if (event.key === "Home") next = 0;
          else if (event.key === "End") next = panels.length - 1;
          else return;
          event.preventDefault();
          setStagePanel(panels[next]);
          document.getElementById(`meeting-tab-${panels[next]}`)?.focus();
        }}
      >
        {panels.map((panel) => (
          <button
            type="button"
            key={panel}
            role="tab"
            id={`meeting-tab-${panel}`}
            aria-controls={`meeting-panel-${panel}`}
            aria-selected={stagePanel === panel}
            tabIndex={stagePanel === panel ? 0 : -1}
            onClick={() => setStagePanel(panel)}
          >
            {panel === "chat"
              ? "Чат"
              : panel === "participants"
                ? `Участники (${onlinePeople.length})`
                : "Субтитры"}
          </button>
        ))}
      </div>
      <div
        id="meeting-panel-chat"
        role="tabpanel"
        aria-labelledby="meeting-tab-chat"
        className="conference-stage-panel conference-stage-panel-chat"
        hidden={stagePanel !== "chat"}
      >
        <ChatPanel
          key={`${id}:${focusMessageId || "latest"}`}
          conferenceId={id}
          membership={membership}
          readOnly={false}
          focusMessageId={focusMessageId}
          onLatest={latestMessages}
        />
      </div>
      <div
        id="meeting-panel-participants"
        role="tabpanel"
        aria-labelledby="meeting-tab-participants"
        className="conference-stage-panel"
        hidden={stagePanel !== "participants"}
      >
        <ParticipantsPanel
          participants={people}
          membership={membership}
          presence={live.state?.participants}
          loading={participants.isPending}
          error={participants.error}
          busy={moderation.isPending}
          onModerate={(participantId, action) =>
            moderation.mutate({ participantId, action })
          }
        />
        {participants.hasNextPage && (
          <Button
            variant="outline"
            busy={participants.isFetchingNextPage}
            onClick={() => void participants.fetchNextPage()}
          >
            Загрузить ещё участников
          </Button>
        )}
        <WaitingRoomPanel
          conferenceId={id}
          membership={membership}
          participants={people}
          active
          closed={false}
        />
        <Button variant="outline" onClick={() => setUtility("invite")}>
          <LinkIcon size={16} />
          Пригласить участников
        </Button>
        {owner && (
          <Button
            variant="danger"
            disabled={mutation.isPending}
            onClick={() => setConfirm("finish")}
          >
            <Square size={15} />
            Завершить конференцию
          </Button>
        )}
      </div>
      {captionsEnabled && (
        <div
          id="meeting-panel-captions"
          role="tabpanel"
          aria-labelledby="meeting-tab-captions"
          className="conference-stage-panel"
          hidden={stagePanel !== "captions"}
        >
          <CaptionsPanel
            key={`captions-${id}`}
            conferenceId={id}
            active
            live={live}
          />
        </div>
      )}
    </aside>
  );
}
