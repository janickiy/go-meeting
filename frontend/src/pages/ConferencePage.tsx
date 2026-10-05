import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  CalendarDays,
  Check,
  Circle,
  Link as LinkIcon,
  Info,
  LogIn,
  LogOut,
  MessageCircle,
  Play,
  Square,
  Users,
  Video,
  X,
} from "lucide-react";
import { api } from "../api";
import { RealtimePanel } from "../components/RealtimePanel";
import { ParticipantsPanel } from "../components/ParticipantsPanel";
import { RecordingPanel } from "../components/RecordingPanel";
import { RecordingNotice } from "../components/RecordingNotice";
import { CaptionsPanel } from "../components/CaptionsPanel";
import { AnalyticsPanel } from "../components/AnalyticsPanel";
import { ConferenceCalendarStatus } from "../components/IntegrationsSettings";
import type { ConferenceRecording, Items, ModerationAction } from "../types";
import { useAuth } from "../auth";
import { useConference, useParticipants, useMembership } from "../queries";
import { isAdmitted } from "../collaboration";
import { onlineParticipants } from "../presence";
import { useRealtime } from "../realtime";
import { WaitingRoomPanel } from "../components/WaitingRoomPanel";
import { ChatPanel } from "../components/ChatPanel";
import { useCapabilities } from "../useCapabilities";
import { meetingShortcut } from "../conferenceShortcuts";
import { EditSchedule } from "../components/ConferenceModals";
import { formatDate, initials, inviteLink } from "../utils";
import {
  Button,
  CopyLink,
  ErrorNotice,
  Loading,
  Modal,
  StatusBadge,
} from "../components/ui";
import "./conference.css";

/** Обновляет только длительность встречи, не перерисовывая сетку видеопотоков.
 * @args startedAt — фактическое серверное начало; @return доступный таймер или отсутствие значения.
 */
function MeetingClock({ startedAt }: { startedAt?: string | null }) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const start = Date.parse(startedAt || "");
  if (!Number.isFinite(start)) return null;
  const seconds = Math.max(0, Math.floor((now - start) / 1000));
  return (
    <span className="room-clock" aria-label="Длительность встречи">
      {[Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60]
        .map((part) => String(part).padStart(2, "0"))
        .join(":")}
    </span>
  );
}

/**
 * ConferencePage координирует сведения встречи, членство, единственный WebSocket, допуск, медиа, чат, запись и историю.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function ConferencePage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const { user } = useAuth();
  const client = useQueryClient();
  const query = useConference(id);
  const self = useMembership(id);
  const membership = self.data || undefined;
  const admitted = isAdmitted(membership);
  const participants = useParticipants(id, admitted);
  const people =
    participants.data?.pages.flatMap(
      /**
       * Обработчик flatMap преобразует текущий элемент в данные или представление результирующего списка.
       *
       * @args
       *   - page — изолированная страница Playwright.
       *
       * @returns преобразованное значение текущего элемента для результирующего набора.
       */ (page) => page.items,
    ) || [];
  const closed = ["finished", "cancelled"].includes(
    query.data?.item.status || "",
  );
  const live = useRealtime(
    id,
    admitted && membership?.status === "joined" && !closed,
  );
  const onlinePeople = onlineParticipants(people, live.state?.participants);
  const displayedPeople = closed ? people : onlinePeople;
  const capabilities = useCapabilities();
  const features =
    capabilities.isSuccess && !capabilities.isError
      ? capabilities.data.capabilities
      : undefined;
  const captionsEnabled = features?.liveCaptions === true;
  const analyticsEnabled = features?.meetingAnalytics === true;
  const activeMeeting =
    admitted &&
    membership?.status === "joined" &&
    query.data?.item.status === "active";
  const [stagePanel, setStagePanel] = useState<
    "chat" | "participants" | "captions"
  >("chat");
  const [panelOpen, setPanelOpen] = useState(
    () => typeof window === "undefined" || window.innerWidth > 900,
  );
  const panelTrigger = useRef<HTMLButtonElement | null>(null);
  const [reconnectTarget, setReconnectTarget] = useState<HTMLDivElement | null>(
    null,
  );
  const [utility, setUtility] = useState<"recording" | "invite" | null>(null);
  const recordingAccess = Boolean(user && !user.guestConferenceId);
  const recordingStatus = useQuery({
    queryKey: ["recordings", id],
    queryFn: ({ signal }) => api.recordings(id, signal),
    enabled: activeMeeting,
    refetchInterval: activeMeeting ? 3000 : false,
  });
  const stopRecording = useMutation({
    /**
     * Останавливает выбранную запись прямо из верхней панели, не открывая диалог.
     * @args recordingId — UUID текущей записи этой конференции.
     * @return Подтверждённая сервером карточка записи; отказ передаётся как ошибка.
     */
    mutationFn: (recordingId: string) => api.stopRecording(id, recordingId),
    /**
     * Сразу отражает подтверждённое состояние остановки в общем кеше панели и диалога.
     * @args response — серверный ответ с обновлённой карточкой записи.
     */
    onSuccess: (response) => {
      client.setQueryData<Items<ConferenceRecording>>(
        ["recordings", id],
        /**
         * Обновляет только остановленную запись, сохраняя остальные элементы списка.
         * @args cached — текущий ответ списка записей либо отсутствие загруженных данных.
         * @return Список с обновлённой карточкой; отсутствующий кеш не создаётся.
         */
        (cached) =>
          cached && {
            ...cached,
            items: cached.items.map(
              /**
               * Подставляет подтверждённую карточку по UUID, не затрагивая другие записи.
               * @args item — текущая карточка списка.
               * @return Серверная карточка остановленной записи либо прежний элемент.
               */ (item) =>
                item.uuid === response.item.uuid ? response.item : item,
            ),
          },
      );
    },
    /** После ответа или ошибки повторно сверяет записи с сервером. */
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["recordings", id] });
    },
  });
  useEffect(() => {
    if (!captionsEnabled && stagePanel === "captions") setStagePanel("chat");
  }, [captionsEnabled, stagePanel]);
  useEffect(() => {
    if (!activeMeeting) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (
        event.key === "Escape" &&
        !document.querySelector('[role="dialog"]')
      ) {
        setPanelOpen(false);
        panelTrigger.current?.focus();
        return;
      }
      const key = meetingShortcut(event, ["c"]);
      if (key === "c") {
        event.preventDefault();
        setStagePanel("chat");
        setPanelOpen(true);
        window.setTimeout(() => {
          document.getElementById(`chat-text-${id}`)?.focus();
        }, 0);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [activeMeeting, id]);
  const history = useQuery({
    queryKey: ["history", user?.id, id],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.history(id, signal).
     */
    queryFn: ({ signal }) => api.history(id, signal),
    enabled: admitted && closed,
  });
  const [confirm, setConfirm] = useState<"finish" | "cancel" | null>(null);
  const [editingSchedule, setEditingSchedule] = useState(false);
  const mutation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @args
     *   - action ("start" | "finish" | "cancel" | "join" | "leave") — разрешённое действие управления либо асинхронная операция.
     *
     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
     */
    mutationFn: (action: "start" | "finish" | "cancel" | "join" | "leave") =>
      action === "join" || action === "leave"
        ? api.membership(id, action).then(
            /**
             * Обработчик then выполняет переданный шаг вызова then в конференциях, расписании и истории.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {},
          )
        : api.transition(id, action).then(
            /**
             * Обработчик then выполняет переданный шаг вызова then в конференциях, расписании и истории.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {},
          ),
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["conference"] });
      void client.invalidateQueries({ queryKey: ["conferences"] });
      void client.invalidateQueries({ queryKey: ["participants"] });
      void client.invalidateQueries({ queryKey: ["membership"] });
      void client.invalidateQueries({ queryKey: ["history"] });
    },
    /**
     * onSuccess обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns вычисленное значение: setConfirm(null).
     */
    onSuccess: () => setConfirm(null),
  });
  const moderation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @args
     *   - объект параметров: participantId — идентификатор членства целевого участника; action — разрешённое действие управления либо асинхронная операция.
     *
     * @returns вычисленное значение: api.moderate(id, participantId, action).
     */
    mutationFn: ({
      participantId,
      action,
    }: {
      participantId: string;
      action: ModerationAction;
    }) => api.moderate(id, participantId, action),
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      void client.invalidateQueries({ queryKey: ["participants"] });
      void client.invalidateQueries({ queryKey: ["membership"] });
    },
  });
  if (query.isPending) return <Loading />;
  if (query.isError || !query.data)
    return (
      <div className="content-card">
        <ErrorNotice error={query.error || new Error()} />
        <Link
          to={user?.guestConferenceId ? "/" : "/conferences"}
          className="text-link"
        >
          <ArrowLeft size={16} />К моим конференциям
        </Link>
      </div>
    );
  const conference = query.data.item;
  const owner = conference.ownerId === user?.id;
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
  if (activeMeeting && membership) {
    const activeRecording = !recordingStatus.isError
      ? recordingStatus.data?.items.find(
          /**
           * Исключает чужую конференцию и уже завершённую обработку из действий остановки.
           * @args item — карточка записи из подтверждённого списка конференции.
           * @return Признак незавершённой записи именно этой встречи.
           */
          (item) =>
            item.conferenceId === conference.id &&
            ["starting", "recording", "degraded", "stopping"].includes(
              item.status,
            ),
        )
      : undefined;
    const recordingLabel =
      activeRecording?.status === "starting"
        ? "Запись запускается"
        : activeRecording?.status === "stopping"
          ? "Запись останавливается"
          : "Идёт запись";
    const panels = captionsEnabled
      ? (["chat", "participants", "captions"] as const)
      : (["chat", "participants"] as const);
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
            owner &&
            membership.role === "owner" && (
              <button
                type="button"
                className="room-header-action room-header-stop-recording"
                aria-label="Остановить запись"
                aria-busy={stopRecording.isPending || undefined}
                title={
                  stopRecording.isPending ||
                  activeRecording.status === "stopping"
                    ? "Запись останавливается"
                    : "Остановить запись"
                }
                disabled={
                  stopRecording.isPending ||
                  activeRecording.status === "stopping"
                }
                onClick={
                  /** Запрашивает остановку один раз; повторные нажатия и уже начатая остановка игнорируются. */
                  () => {
                    if (
                      !stopRecording.isPending &&
                      activeRecording.status !== "stopping"
                    )
                      stopRecording.mutate(activeRecording.uuid);
                  }
                }
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
                if (event.key === "ArrowRight")
                  next = (index + 1) % panels.length;
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
                conferenceId={id}
                membership={membership}
                readOnly={false}
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
            {capabilities.isSuccess && !captionsEnabled && (
              <p className="conference-feature-note">
                Субтитры отключены для этой установки.
              </p>
            )}
          </aside>
        </section>
        {utility && (utility !== "recording" || recordingAccess) && (
          <Modal
            title={
              utility === "recording"
                ? "Записи конференции"
                : "Пригласить участников"
            }
            onClose={() => setUtility(null)}
          >
            {utility === "recording" ? (
              <RecordingPanel
                conference={conference}
                membership={membership}
                showHistory={false}
                onStarted={() => setUtility(null)}
              />
            ) : (
              <>
                <p className="modal-description">
                  Отправьте ссылку участникам. Можно подключиться без аккаунта.
                </p>
                <CopyLink value={inviteLink(conference.inviteCode)} />
                <p className="field-hint">
                  {conference.waitingRoomEnabled
                    ? "Новые участники дождутся допуска организатора."
                    : "Участники со ссылкой смогут присоединиться к встрече."}
                </p>
              </>
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
              После завершения участники не смогут присоединиться. Текущая
              запись остановится и будет обработана в фоне.
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
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns значение не возвращается; открывает проверку устройств или завершает членство.
                     */ () => {
                      if (membership?.status === "joined")
                        mutation.mutate("leave");
                      else
                        navigate(
                          user?.guestConferenceId
                            ? `/i/${conference.inviteCode}`
                            : `/conferences/${id}/join`,
                        );
                    }
                  }
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
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: mutation.mutate("start").
                     */ () => mutation.mutate("start")
                  }
                >
                  <Play size={17} />
                  Начать конференцию
                </Button>
                <Button
                  variant="secondary"
                  disabled={mutation.isPending}
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: setConfirm("cancel").
                     */ () => setConfirm("cancel")
                  }
                >
                  <X size={17} />
                  Отменить конференцию
                </Button>
                {conference.status === "scheduled" && (
                  <Button
                    variant="secondary"
                    onClick={
                      /**
                       * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                       *
                       *
                       * @returns вычисленное значение: setEditingSchedule(true).
                       */ () => setEditingSchedule(true)
                    }
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
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns вычисленное значение: setConfirm("finish").
                   */ () => setConfirm("finish")
                }
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
          conferenceId={id}
          membership={membership}
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
      {admitted && closed && (
        <section
          className="content-card history-summary"
          aria-label="История встречи"
        >
          <h2>Итоги встречи</h2>
          <ErrorNotice error={history.error} />
          {history.isPending ? (
            <Loading />
          ) : (
            history.data && (
              <dl>
                <div>
                  <dt>Организатор</dt>
                  <dd>
                    {history.data.item.owner.displayName ||
                      "Организатор встречи"}
                  </dd>
                </div>
                <div>
                  <dt>Длительность</dt>
                  <dd>
                    {history.data.item.durationSec === null
                      ? "—"
                      : `${Math.max(1, Math.round(history.data.item.durationSec / 60))} мин`}
                  </dd>
                </div>
                <div>
                  <dt>Участников</dt>
                  <dd>{history.data.item.participantCount}</dd>
                </div>
                <div>
                  <dt>Записи</dt>
                  <dd>
                    {history.data.item.recordings.ready} готово ·{" "}
                    {history.data.item.recordings.processing} обрабатывается
                  </dd>
                </div>
              </dl>
            )
          )}
        </section>
      )}
      {admitted && (
        <section
          className="content-card participants-card"
          id="conference-participants"
        >
          <div className="section-heading">
            <h2>
              <Users size={20} />
              Участники{" "}
              <span className="count-badge">
                {displayedPeople.length}
                {closed && participants.hasNextPage ? "+" : ""}
              </span>
            </h2>
            <span className="muted small">Обновляется автоматически</span>
          </div>
          <ErrorNotice error={participants.error} />
          {participants.isPending ? (
            <Loading />
          ) : (
            <div className="participant-list">
              {displayedPeople.map(
                /**
                 * Обработчик people.map преобразует один элемент набора в представление или данные следующего шага.
                 *
                 * @args
                 *   - person — целевое членство участника.
                 *
                 * @returns преобразованное значение текущего элемента для результирующего набора.
                 */ (person) => (
                  <div className="participant-row" key={person.id}>
                    <span
                      className={`avatar ${person.role === "owner" ? "avatar-owner" : ""}`}
                    >
                      {initials(person.displayName)}
                    </span>
                    {!closed &&
                      membership?.status === "joined" &&
                      person.id !== membership.id &&
                      person.role !== "owner" &&
                      isAdmitted(person) &&
                      person.status !== "kicked" &&
                      (membership.role === "owner" ||
                        (membership.role === "co_host" &&
                          person.role === "participant")) && (
                        <div
                          className="participant-controls"
                          aria-label={`Управление: ${person.displayName}`}
                        >
                          {(
                            [
                              ["mute", "микрофон", !!person.microphoneBlocked],
                              ["camera", "видео", !!person.cameraBlocked],
                              ["screen", "экран", !!person.screenBlocked],
                            ] as const
                          )
                            .filter(
                              /**
                               * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
                               *
                               * @args
                               *   - [action] — элементы записи набора, извлечённые по указанным позициям.
                               *
                               * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                               */
                              ([action]) =>
                                action !== "camera" ||
                                membership.role === "owner",
                            )
                            .map(
                              /**
                               * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
                               *
                               * @args
                               *   - [action, label, blocked] — элементы записи набора, извлечённые по указанным позициям.
                               *
                               * @returns преобразованное значение текущего элемента для результирующего набора.
                               */ ([action, label, blocked]) => (
                                <Button
                                  key={action}
                                  variant="secondary"
                                  disabled={moderation.isPending}
                                  onClick={
                                    /**
                                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                                     *
                                     *
                                     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                                     */ () =>
                                      moderation.mutate({
                                        participantId: person.id,
                                        action: { action, blocked: !blocked },
                                      })
                                  }
                                >
                                  {blocked ? "Разрешить" : "Отключить"} {label}
                                </Button>
                              ),
                            )}
                          {membership.role === "owner" && (
                            <Button
                              variant="outline"
                              disabled={moderation.isPending}
                              onClick={
                                /**
                                 * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                                 *
                                 *
                                 * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                                 */ () =>
                                  moderation.mutate({
                                    participantId: person.id,
                                    action: {
                                      action: "role",
                                      role:
                                        person.role === "co_host"
                                          ? "participant"
                                          : "co_host",
                                    },
                                  })
                              }
                            >
                              {person.role === "co_host"
                                ? "Убрать соорганизатора"
                                : "Назначить соорганизатором"}
                            </Button>
                          )}
                          <Button
                            variant="danger"
                            disabled={moderation.isPending}
                            onClick={
                              /**
                               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                               *
                               *
                               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                               */ () => {
                                if (
                                  window.confirm(
                                    `Исключить ${person.displayName}? Повторное присоединение будет запрещено.`,
                                  )
                                )
                                  moderation.mutate({
                                    participantId: person.id,
                                    action: { action: "kick" },
                                  });
                              }
                            }
                          >
                            Исключить
                          </Button>
                        </div>
                      )}
                    <div className="participant-name">
                      <strong>
                        {person.displayName}
                        {person.userId === user?.id && (
                          <span className="muted"> (вы)</span>
                        )}
                      </strong>
                      <span>{roleNames[person.role]}</span>
                      {person.status === "joined" && (
                        <span className="participant-media-status">
                          {person.microphoneBlocked
                            ? "Звук запрещён"
                            : person.microphoneEnabled
                              ? "Микрофон включён"
                              : "Микрофон выключен"}
                          {" · "}
                          {person.cameraBlocked
                            ? "Видео запрещено"
                            : person.cameraEnabled
                              ? "Камера включена"
                              : "Камера выключена"}
                          {person.screenSharing ? " · Показывает экран" : ""}
                        </span>
                      )}
                    </div>
                    <span
                      className={`participant-status ${person.status === "joined" ? "participant-joined" : ""}`}
                    >
                      <span className="presence-dot" />
                      {person.joinedAt ||
                      ["waiting", "rejected", "kicked"].includes(person.status)
                        ? presence[person.status]
                        : "Ещё не присоединялся"}
                    </span>
                  </div>
                ),
              )}
            </div>
          )}
          {participants.hasNextPage && (
            <Button
              variant="outline"
              busy={participants.isFetchingNextPage}
              onClick={
                /**
                 * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                 *
                 *
                 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                 */ () => {
                  void participants.fetchNextPage();
                }
              }
            >
              Загрузить ещё участников
            </Button>
          )}
          <p className="field-hint">
            {closed
              ? "Сохранён состав участников завершённой встречи."
              : "Показаны только участники онлайн. После 5 секунд без связи участник исчезает из списка."}
          </p>
        </section>
      )}
      {editingSchedule && (
        <EditSchedule
          conference={conference}
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: setEditingSchedule(false).
             */ () => setEditingSchedule(false)
          }
        />
      )}
      {confirm && (
        <Modal
          title={
            confirm === "finish"
              ? "Завершить конференцию?"
              : "Отменить конференцию?"
          }
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {
              if (!mutation.isPending) setConfirm(null);
            }
          }
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
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: mutation.mutate(confirm).
               */ () => mutation.mutate(confirm)
            }
          >
            {confirm === "finish" ? "Да, завершить" : "Да, отменить"}
          </Button>
          <Button
            variant="secondary"
            className="full-width"
            disabled={mutation.isPending}
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: setConfirm(null).
               */ () => setConfirm(null)
            }
          >
            Вернуться к встрече
          </Button>
        </Modal>
      )}
    </>
  );
}
