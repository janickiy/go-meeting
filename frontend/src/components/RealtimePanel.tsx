import {
  memo,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import {
  Mic,
  MicOff,
  Settings,
  ShieldCheck,
  MonitorUp,
  Radio,
  RefreshCw,
  Video,
  VideoOff,
} from "lucide-react";
import type { useRealtime } from "../realtime";
import { useMedia } from "../useMedia";
import { Button, ErrorNotice } from "./ui";
import type { Participant } from "../types";
import { useAuth } from "../auth";
import { useAccountSettings } from "./AccountSettingsContext";
import {
  subscribeDevicePreferences,
  hasDevicePreferences,
  readDevicePreferences,
  consumeMediaEntry,
} from "../prejoinDevices";
import { meetingShortcut } from "../conferenceShortcuts";
import { initials } from "../utils";
import { useSpeakingParticipants } from "../useSpeakingParticipants";
import { onlineParticipants } from "../presence";

/**
 * MediaTile воспроизводит существующий поток и показывает состояние участника.
 * Уровень звука синхронно меняет яркость рамки и заполнение значка микрофона.
 * Экран, выключенный микрофон и восстанавливаемое соединение не получают
 * подсветку. Смена потока снимает
 * только DOM-привязку, не останавливая общие дорожки WebRTC.
 * @args stream — поток; name — подпись и инициалы; local — собственная плитка
 * без воспроизведения звука; video — показывать видео вместо инициалов;
 * screen — демонстрация экрана; sinkId — устройство воспроизведения;
 * microphoneEnabled — состояние микрофона; audioLevel — уровень звука от 0 до 1;
 * role — роль участника; reconnecting — восстанавливается ли медиасвязь;
 * covered — скрыть плитку на время показа экрана, сохранив воспроизведение звука.
 * @return видеоплитка или аудиоплитка с подписью и индикаторами состояния.
 */
export const MediaTile = memo(function MediaTile({
  stream,
  name,
  local = false,
  video = true,
  screen = false,
  sinkId = "",
  microphoneEnabled = false,
  audioLevel = 0,
  role,
  reconnecting = false,
  covered = false,
}: {
  stream?: MediaStream;
  name: string;
  local?: boolean;
  video?: boolean;
  screen?: boolean;
  sinkId?: string;
  microphoneEnabled?: boolean;
  audioLevel?: number;
  role?: Participant["role"];
  reconnecting?: boolean;
  covered?: boolean;
}) {
  const element = useRef<HTMLMediaElement | null>(null);
  const [blocked, setBlocked] = useState(false);
  const [sinkError, setSinkError] = useState(false);
  const level =
    microphoneEnabled && !screen && !reconnecting && Number.isFinite(audioLevel)
      ? Math.max(0, Math.min(1, audioLevel))
      : 0;
  const speakingNow = level > 0;
  /**
   * play запускает воспроизведение потока, учитывая ограничения браузера.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  const play = async () => {
    try {
      await element.current?.play();
      setBlocked(false);
    } catch {
      setBlocked(true);
    }
  };
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      const media = element.current;
      if (!media || !stream) return;
      media.srcObject = stream;
      let active = true;
      void media.play().catch(
        /**
         * Обработчик catch выполняет переданный шаг вызова catch в интерфейсе Meet.
         *
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ () => {
          if (active) setBlocked(true);
        },
      );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        active = false;
        media.srcObject = null;
      };
    },
    [stream, video],
  );
  useEffect(() => {
    const media = element.current as
      (HTMLMediaElement & { setSinkId?: (id: string) => Promise<void> }) | null;
    if (local || !media?.setSinkId) return;
    let active = true;
    void media.setSinkId(sinkId).then(
      () => {
        if (active) setSinkError(false);
      },
      () => {
        if (active) setSinkError(true);
        void media.setSinkId?.("").catch(() => {});
      },
    );
    return () => {
      active = false;
    };
  }, [local, sinkId, stream, video]);
  return (
    <>
      <div
        className={`media-tile ${local ? "media-tile-local" : ""} ${screen ? "media-tile-screen" : ""} ${speakingNow ? "media-tile-speaking" : ""}`}
        hidden={covered}
        data-speaking={speakingNow}
        data-audio-level={level}
        style={{ "--audio-level": level } as CSSProperties}
        data-testid={
          stream
            ? local
              ? "local-media"
              : "remote-media"
            : "participant-placeholder"
        }
      >
        {video && stream ? (
          <video
            ref={
              /**
               * ref сохраняет DOM-ссылку для медиа или отслеживания видимости.
               *
               * @args
               *   - node — DOM-элемент, к которому привязывается медиапоток.
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ (node) => {
                element.current = node;
              }
            }
            autoPlay
            playsInline
            muted={local}
            aria-label={name}
          />
        ) : (
          <>
            <div className="media-audio-symbol" aria-label="Камера выключена">
              <span className="participant-tile-initials">
                {initials(name)}
              </span>
            </div>
            {stream && (
              <audio
                ref={
                  /**
                   * ref сохраняет DOM-ссылку для медиа или отслеживания видимости.
                   *
                   * @args
                   *   - node — DOM-элемент, к которому привязывается медиапоток.
                   *
                   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                   */ (node) => {
                    element.current = node;
                  }
                }
                autoPlay
                muted={local}
                aria-label={name}
              />
            )}
          </>
        )}
        <div className="media-tile-label">
          <span>
            {name}
            {local ? " · вы" : ""}
          </span>
          {!screen && (
            <span className="media-tile-badges">
              {(role === "owner" || role === "co_host") && (
                <ShieldCheck
                  size={15}
                  aria-label={
                    role === "owner" ? "Организатор" : "Соорганизатор"
                  }
                />
              )}
              <span
                className={`media-tile-microphone ${speakingNow ? "media-microphone-speaking" : ""}`}
                aria-label={
                  speakingNow
                    ? `Говорит: ${name}`
                    : microphoneEnabled
                      ? "Микрофон включён"
                      : "Микрофон выключен"
                }
                title={
                  speakingNow
                    ? "Говорит"
                    : microphoneEnabled
                      ? "Микрофон включён"
                      : "Микрофон выключен"
                }
              >
                {microphoneEnabled ? (
                  <Mic size={15} aria-hidden="true" />
                ) : (
                  <MicOff size={15} aria-hidden="true" />
                )}
              </span>
            </span>
          )}
        </div>
        {reconnecting && (
          <span className="media-tile-reconnecting" role="status">
            Восстанавливаем связь
          </span>
        )}
        {blocked && !covered && (
          <Button
            variant="secondary"
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns вычисленное значение: void play().
               */ () => void play()
            }
          >
            Включить воспроизведение
          </Button>
        )}
        {sinkError && (
          <p className="field-hint" role="status">
            Выбранный динамик недоступен. Используется системный.
          </p>
        )}
      </div>
      {covered && blocked && !local && (
        <Button
          variant="secondary"
          className="media-audio-unlock"
          onClick={() => void play()}
        >
          Включить звук: {name}
        </Button>
      )}
    </>
  );
});

// Захват с физических устройств всегда требует явного действия пользователя.
/**
 * RealtimePanel показывает медиапотоки, присутствие и прямые кнопки управления встречей.
 * Настройки открываются общим диалогом без пересоздания медиа; переподключение сначала
 * освобождает текущую медиасессию и затем обновляет канал встречи.
 * @args conferenceId — идентификатор встречи; membership — членство и ограничения устройств;
 * live — общий канал событий; shortcutsEnabled — разрешение горячих клавиш микрофона и камеры;
 * participants — доступные сведения об участниках; controls — кнопки чата и участников;
 * reconnectTarget — необязательный отдельный контейнер кнопки переподключения;
 * controlsTarget — контейнер общей панели управления вне видеоколонки;
 * endControls — завершающие действия, например выход из встречи.
 * @return Панель потоков и состояния связи; кнопки монтируются в указанные контейнеры
 * или остаются внутри панели, если контейнеры не заданы.
 */
export function RealtimePanel({
  conferenceId,
  membership,
  live,
  shortcutsEnabled = true,
  participants = [],
  controls,
  reconnectTarget,
  controlsTarget,
  endControls,
}: {
  conferenceId: string;
  membership: Participant;
  live: ReturnType<typeof useRealtime>;
  shortcutsEnabled?: boolean;
  participants?: Participant[];
  controls?: ReactNode;
  reconnectTarget?: HTMLElement | null;
  controlsTarget?: HTMLElement | null;
  endControls?: ReactNode;
}) {
  const { user } = useAuth();
  const openSettings = useAccountSettings();
  const media = useMedia(live, conferenceId, {
    ...membership,
    version: membership.mediaPolicyVersion,
  });
  const [preferences, setPreferences] = useState(() =>
    readDevicePreferences(user?.id || "guest"),
  );
  useEffect(() => {
    const userId = user?.id || "guest";
    setPreferences(readDevicePreferences(userId));
    return subscribeDevicePreferences(userId, setPreferences);
  }, [user?.id]);
  useEffect(() => {
    if (media.running) media.configure?.(preferences);
  }, [
    media.configure,
    media.running,
    media.view.controlBusy,
    media.view.mediaPeerId,
    preferences,
  ]);
  useEffect(() => {
    if (
      !live.state?.connectionId ||
      membership.status !== "joined" ||
      membership.admissionState !== "admitted"
    )
      return;
    if (consumeMediaEntry(conferenceId)) media.start(true, preferences);
  }, [
    conferenceId,
    live.state?.connectionId,
    membership.status,
    membership.admissionState,
  ]);
  const hasSelection = hasDevicePreferences(user?.id || "guest");
  const [online, setOnline] = useState(() => navigator.onLine);
  useEffect(() => {
    const refresh = () => setOnline(navigator.onLine);
    window.addEventListener("online", refresh);
    window.addEventListener("offline", refresh);
    return () => {
      window.removeEventListener("online", refresh);
      window.removeEventListener("offline", refresh);
    };
  }, []);
  const connectionProblem =
    !online ||
    !live.state ||
    ["failed", "disconnected"].includes(media.view.connectionState) ||
    ["failed", "disconnected"].includes(media.view.iceState);
  const busy =
    media.view.controlBusy || !media.view.mediaPeerId || connectionProblem;
  const roster = onlineParticipants(participants, live.state?.participants);
  const onlineIds = new Set(roster.map((person) => person.id));
  const localStream =
    preferences.showSelf && onlineIds.has(membership.id)
      ? media.view.localStream
      : null;
  const localScreen = onlineIds.has(membership.id)
    ? media.view.localScreen
    : null;
  const visibleRemoteStreams = media.view.remoteStreams.filter((remote) =>
    onlineIds.has(remote.participantId),
  );
  const present = roster;
  const remoteParticipants = new Set(
    visibleRemoteStreams
      .filter((remote) => !remote.screen)
      .map((remote) => remote.participantId),
  );
  const showingScreen = Boolean(
    localScreen ||
    (!preferences.hideParticipantVideo &&
      visibleRemoteStreams.some((remote) => remote.screen)),
  );
  // Считаем именно отрисованные плитки: камеры, аудиопотоки и заглушки онлайн-участников.
  // Экран использует прежнюю полноразмерную раскладку, независимо от числа участников.
  const participantsWithoutStream = present.filter((person) =>
    person.id === membership.id
      ? preferences.showSelf && !localStream
      : !remoteParticipants.has(person.id),
  );
  const cameraTileCount =
    Number(Boolean(localStream)) +
    visibleRemoteStreams.filter((remote) => !remote.screen).length +
    participantsWithoutStream.length;
  const pairedTiles = !showingScreen && cameraTileCount === 2;
  const speaking = useSpeakingParticipants(
    [
      {
        id: "local",
        participantId: membership.id,
        stream: media.view.localStream,
        enabled: media.view.microphoneEnabled && !membership.microphoneBlocked,
      },
      ...visibleRemoteStreams
        .filter((remote) => !remote.screen)
        .map((remote) => {
          const person = roster.find(
            (person) => person.id === remote.participantId,
          );
          return {
            id: remote.id,
            participantId: remote.participantId,
            stream: remote.stream,
            enabled:
              !person?.microphoneBlocked &&
              (person?.microphoneEnabled ?? remote.kinds.includes("audio")),
          };
        }),
    ],
    media.running && !connectionProblem,
  );
  const localAudioLevel =
    media.view.microphoneEnabled &&
    !membership.microphoneBlocked &&
    !connectionProblem &&
    onlineIds.has(membership.id)
      ? (speaking.get(membership.id) ?? 0)
      : 0;
  useEffect(() => {
    if (!shortcutsEnabled || connectionProblem || !media.running || busy)
      return;
    const onKeyDown = (event: KeyboardEvent) => {
      const key = meetingShortcut(event, ["m", "v"]);
      if (key === "m" && !membership.microphoneBlocked) {
        event.preventDefault();
        void media.microphone(!media.view.microphoneEnabled);
      } else if (key === "v" && !membership.cameraBlocked) {
        event.preventDefault();
        void media.camera(!media.view.cameraEnabled);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [
    busy,
    connectionProblem,
    media,
    media.running,
    media.view.microphoneEnabled,
    media.view.cameraEnabled,
    membership.microphoneBlocked,
    membership.cameraBlocked,
    shortcutsEnabled,
  ]);
  const reconnectButton = (
    <Button
      variant="secondary"
      className={
        reconnectTarget
          ? "room-header-action room-header-reconnect"
          : "room-reconnect-control"
      }
      aria-label="Переподключиться"
      title="Переподключиться"
      disabled={!online}
      onClick={() => {
        media.stop();
        live.reconnect();
      }}
    >
      <RefreshCw size={20} aria-hidden="true" />
      <span className={reconnectTarget ? undefined : "sr-only"}>
        Переподключиться
      </span>
    </Button>
  );
  const renderControls = (children: ReactNode) =>
    controlsTarget ? createPortal(children, controlsTarget) : children;
  return (
    <section
      className="content-card realtime-panel"
      aria-label="Realtime-подключение"
    >
      <div className="section-heading">
        <h2>
          <Radio size={20} />
          Связь с участниками
        </h2>
        <div className="realtime-header-actions">
          <span
            className={`participant-status ${connectionProblem ? "conference-link-lost" : "conference-link-ready"}`}
          >
            {online ? live.status : "Нет сети"}
          </span>
        </div>
      </div>
      {reconnectTarget && createPortal(reconnectButton, reconnectTarget)}
      <ErrorNotice error={live.error} />
      {connectionProblem && (
        <div className="conference-connection-notice" role="status">
          <strong>
            {!online
              ? "Нет подключения к сети"
              : live.status === "Подключение…"
                ? "Соединяемся со встречей"
                : "Связь восстанавливается"}
          </strong>
          <span>
            {online
              ? "Звук и видео могут быть временно недоступны. После восстановления проверьте устройства."
              : "Проверьте интернет-соединение. При возврате сети подключение повторится автоматически."}
          </span>
        </div>
      )}
      {live.state && (
        <details className="conference-presence-details">
          <summary>В сети: {roster.length}</summary>
          <p className="field-hint">
            Подключение этой вкладки:{" "}
            <code data-testid="connection-id">{live.state.connectionId}</code>
          </p>
          <div className="realtime-people">
            {live.state.participants
              .filter((person) => onlineIds.has(person.id))
              .map(
                /**
                 * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
                 *
                 * @args
                 *   - p — сведения об участнике конференции.
                 *
                 * @returns преобразованное значение текущего элемента для результирующего набора.
                 */ (p) => (
                  <div
                    key={p.id}
                    className="realtime-person"
                    data-testid={`presence-${p.userId}`}
                  >
                    <span
                      className={`presence-dot ${p.online ? "online-dot" : ""}`}
                    />
                    <strong>{p.displayName}</strong>
                    <span>
                      {p.online
                        ? `Онлайн · подключений: ${p.connections}`
                        : "Не в сети"}
                    </span>
                  </div>
                ),
              )}
          </div>
        </details>
      )}
      <div className="conference-media" aria-label="Медиасвязь через SFU">
        <div className="section-heading">
          <h2>
            <Video size={20} />
            Камера и микрофон
          </h2>
        </div>
        <p className="field-hint">
          Доступ к устройствам запрашивается только после нажатия кнопки.
          Камеру, микрофон и демонстрацию экрана можно включать независимо.
        </p>
        <p role="status" data-testid="media-status">
          {media.view.status}
        </p>
        {media.view.error && <ErrorNotice>{media.view.error}</ErrorNotice>}
        {renderControls(
          <div
            className="meeting-actions conference-control-bar"
            aria-label="Управление медиасвязью"
          >
            {!media.running && (
              <>
                <Button
                  disabled={!live.state || !online}
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: media.start().
                     */ () => media.start(true, preferences)
                  }
                >
                  <Video size={17} />
                  {media.view.error
                    ? "Подключить медиасвязь снова"
                    : hasSelection
                      ? "Подключить с выбранными устройствами"
                      : "Включить камеру и микрофон"}
                </Button>
                <Button
                  variant="secondary"
                  disabled={!live.state || !online}
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: media.start(false).
                     */ () => media.start(false)
                  }
                >
                  Подключиться без камеры и микрофона
                </Button>
              </>
            )}
            {media.running && (
              <>
                <Button
                  variant="secondary"
                  disabled={busy || membership.microphoneBlocked}
                  aria-pressed={media.view.microphoneEnabled}
                  aria-label={
                    media.view.microphoneEnabled
                      ? "Выключить микрофон"
                      : "Включить микрофон"
                  }
                  aria-keyshortcuts={shortcutsEnabled ? "M" : undefined}
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: void media.microphone(!media.view.microphoneEnabled).
                     */ () =>
                      void media.microphone(!media.view.microphoneEnabled)
                  }
                >
                  <span
                    className={`media-tile-microphone media-control-microphone ${localAudioLevel > 0 ? "media-microphone-speaking" : ""}`}
                    style={
                      { "--audio-level": localAudioLevel } as CSSProperties
                    }
                    data-audio-level={localAudioLevel}
                    aria-hidden="true"
                  >
                    {media.view.microphoneEnabled ? (
                      <Mic size={17} />
                    ) : (
                      <MicOff size={17} />
                    )}
                  </span>
                  Микрофон
                </Button>
                <Button
                  variant="secondary"
                  disabled={busy || membership.cameraBlocked}
                  aria-pressed={media.view.cameraEnabled}
                  aria-label={
                    media.view.cameraEnabled
                      ? "Выключить камеру"
                      : "Включить камеру"
                  }
                  aria-keyshortcuts={shortcutsEnabled ? "V" : undefined}
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: void media.camera(!media.view.cameraEnabled).
                     */ () => void media.camera(!media.view.cameraEnabled)
                  }
                >
                  {media.view.cameraEnabled ? (
                    <Video size={17} />
                  ) : (
                    <VideoOff size={17} />
                  )}
                  Камера
                </Button>
                <Button
                  variant="secondary"
                  className="room-screen-control"
                  aria-pressed={media.view.screenSharing}
                  aria-label={
                    media.view.screenSharing
                      ? "Остановить демонстрацию"
                      : "Показать экран"
                  }
                  disabled={
                    busy || membership.screenBlocked || membership.cameraBlocked
                  }
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленное значение: void (media.view.screenSharing ? media.stopScreen() : media.startScreen()).
                     */ () =>
                      void (media.view.screenSharing
                        ? media.stopScreen()
                        : media.startScreen())
                  }
                >
                  <MonitorUp size={17} />
                  {media.view.screenSharing ? "Остановить экран" : "Экран"}
                </Button>
              </>
            )}
            {openSettings && (
              <Button
                variant="secondary"
                className="room-settings-control"
                aria-haspopup="dialog"
                onClick={(event) => openSettings(event.currentTarget)}
              >
                <Settings size={20} aria-hidden="true" />
                Настройки
              </Button>
            )}
            {controls}
            {!reconnectTarget && reconnectButton}
            {endControls}
          </div>,
        )}
        {(membership.microphoneBlocked ||
          membership.cameraBlocked ||
          membership.screenBlocked) && (
          <p className="field-hint">
            Организатор ограничил:{" "}
            {[
              membership.microphoneBlocked && "микрофон",
              membership.cameraBlocked && "видео (камеру и экран)",
              membership.screenBlocked && "демонстрацию экрана",
            ]
              .filter(Boolean)
              .join(", ")}
            . Разрешение не включает устройства автоматически.
          </p>
        )}
        {!localStream &&
          !localScreen &&
          visibleRemoteStreams.length === 0 &&
          present.length === 0 && (
            <div className="media-empty" data-testid="media-empty">
              <VideoOff size={36} aria-hidden="true" />
              <strong>Видео пока нет</strong>
              <span>
                {live.state
                  ? "Подключите камеру или дождитесь видео участников."
                  : "Ожидаем восстановления связи с конференцией."}
              </span>
            </div>
          )}
        {(localStream ||
          localScreen ||
          visibleRemoteStreams.length > 0 ||
          present.length > 0) && (
          <div
            className={`media-grid ${showingScreen ? "media-grid-sharing" : pairedTiles ? "media-grid-pair" : ""}`}
            role="region"
            aria-label={
              showingScreen
                ? "Демонстрация экрана"
                : pairedTiles
                  ? "Видео участников"
                  : "Медиапотоки встречи"
            }
            tabIndex={0}
          >
            {localScreen && (
              <MediaTile local screen stream={localScreen} name="Ваш экран" />
            )}
            {localStream && (
              <MediaTile
                local
                stream={localStream}
                video={media.view.cameraEnabled}
                name={membership.displayName || "Вы"}
                microphoneEnabled={media.view.microphoneEnabled}
                audioLevel={localAudioLevel}
                role={membership.role}
                reconnecting={connectionProblem}
                covered={showingScreen}
              />
            )}
            {visibleRemoteStreams.map(
              /**
               * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
               *
               * @args
               *   - remote — снимок медиа одного удалённого подключения.
               *
               * @returns преобразованное значение текущего элемента для результирующего набора.
               */ (remote) => (
                <MediaTile
                  key={remote.id}
                  screen={remote.screen && !preferences.hideParticipantVideo}
                  covered={showingScreen && !remote.screen}
                  stream={remote.stream}
                  audioLevel={speaking.get(remote.participantId) ?? 0}
                  video={
                    !preferences.hideParticipantVideo &&
                    remote.kinds.includes("video")
                  }
                  sinkId={preferences.audioOutputId}
                  microphoneEnabled={
                    roster.find((person) => person.id === remote.participantId)
                      ?.microphoneEnabled ?? remote.kinds.includes("audio")
                  }
                  role={
                    roster.find((person) => person.id === remote.participantId)
                      ?.role
                  }
                  reconnecting={connectionProblem}
                  name={
                    (remote.screen ? "Экран · " : "") +
                    (roster.find(
                      /**
                       * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
                       *
                       * @args
                       *   - p — сведения об участнике конференции.
                       *
                       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                       */
                      (p) => p.id === remote.participantId,
                    )?.displayName || "Участник")
                  }
                />
              ),
            )}
            {participantsWithoutStream.map((person) => (
              <MediaTile
                key={`waiting-${person.id}`}
                name={person.displayName || "Участник"}
                local={person.id === membership.id}
                video={false}
                covered={showingScreen}
                microphoneEnabled={
                  person.id === membership.id
                    ? media.view.microphoneEnabled
                    : person.microphoneEnabled
                }
                role={person.role}
                reconnecting={connectionProblem}
              />
            ))}
          </div>
        )}
        <p className="field-hint">
          После переподключения вкладки устройства автоматически не включаются.
          При ошибке ICE проверьте доступность медиа-портов и настройку
          STUN/TURN.
        </p>
      </div>
    </section>
  );
}
