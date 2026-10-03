import { useEffect, useRef, useState } from "react";
import {
  Mic,
  MicOff,
  MonitorUp,
  Radio,
  RefreshCw,
  Video,
  VideoOff,
  Volume2,
} from "lucide-react";
import type { useRealtime } from "../realtime";
import { useMedia } from "../useMedia";
import { Button, ErrorNotice } from "./ui";
import type { Participant } from "../types";
import { useAuth } from "../auth";
import {
  hasDevicePreferences,
  readDevicePreferences,
  saveDevicePreferences,
} from "../prejoinDevices";
import { meetingShortcut } from "../conferenceShortcuts";
import {
  safeDiagnosticsReport,
  type RtcDiagnostics,
} from "../mediaDiagnostics";
import { useCapabilities } from "../useCapabilities";

/**
 * MediaTile привязывает MediaStream к аудио- или видеоэлементу и освобождает привязку при смене потока.
 *
 * @args
 *   - объект параметров: stream — свойство текущего компонента; name — отображаемое имя пользователя для инициалов; local — свойство текущего компонента; video — свойство текущего компонента; screen — свойство текущего компонента.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
function MediaTile({
  stream,
  name,
  local = false,
  video = true,
  screen = false,
  sinkId = "",
}: {
  stream: MediaStream;
  name: string;
  local?: boolean;
  video?: boolean;
  screen?: boolean;
  sinkId?: string;
}) {
  const element = useRef<HTMLMediaElement | null>(null);
  const [blocked, setBlocked] = useState(false);
  const [sinkError, setSinkError] = useState(false);
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
      if (!media) return;
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
    <div
      className={`media-tile ${local ? "media-tile-local" : ""} ${screen ? "media-tile-screen" : ""}`}
      data-testid={local ? "local-media" : "remote-media"}
    >
      {video ? (
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
          <div className="media-audio-symbol">
            <Volume2 size={34} />
          </div>
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
        </>
      )}
      <div className="media-tile-label">
        {name}
        {local ? " · вы" : ""}
      </div>
      {blocked && (
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
  );
}

// Захват с физических устройств всегда требует явного действия пользователя.
/**
 * RealtimePanel показывает состояние связи, локальные и удалённые медиа и действия устройств и экрана.
 *
 * @args
 *   - объект параметров: conferenceId — идентификатор конференции и области данных; membership — свойство текущего компонента; live — свойство текущего компонента.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function RealtimePanel({
  conferenceId,
  membership,
  live,
  shortcutsEnabled = true,
}: {
  conferenceId: string;
  membership: Participant;
  live: ReturnType<typeof useRealtime>;
  shortcutsEnabled?: boolean;
}) {
  const { user } = useAuth();
  const capabilities = useCapabilities();
  const media = useMedia(live, conferenceId, {
    ...membership,
    version: membership.mediaPolicyVersion,
  });
  const [preferences, setPreferences] = useState(() =>
    readDevicePreferences(user?.id || ""),
  );
  const [hasSelection] = useState(() => hasDevicePreferences(user?.id || ""));
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false);
  const [diagnostics, setDiagnostics] = useState<RtcDiagnostics | null>(null);
  const [diagnosticsError, setDiagnosticsError] = useState("");
  const [copyStatus, setCopyStatus] = useState("");
  useEffect(() => {
    setDiagnosticsOpen(false);
    setDiagnostics(null);
    setCopyStatus("");
  }, [media.view.mediaPeerId]);
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (!media.running) {
        setDevices([]);
        return;
      }
      let active = true;
      /**
       * refresh обновляет устройства или данные текущего компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      const refresh = () => {
        void navigator.mediaDevices
          ?.enumerateDevices()
          .then(
            /**
             * Обработчик then выполняет переданный шаг вызова then в интерфейсе Meet.
             *
             * @args
             *   - items — элементы результата для объединения или отображения.
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ (items) => {
              if (active) setDevices(items);
            },
          )
          .catch(
            /**
             * Обработчик catch выполняет переданный шаг вызова catch в интерфейсе Meet.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {},
          );
      };
      refresh();
      navigator.mediaDevices?.addEventListener("devicechange", refresh);
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        active = false;
        navigator.mediaDevices?.removeEventListener("devicechange", refresh);
      };
    },
    [media.running, media.view.microphoneEnabled, media.view.cameraEnabled],
  );
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
  useEffect(() => {
    if (!diagnosticsOpen || !media.running || !live.state) {
      setDiagnostics(null);
      return;
    }
    let active = true;
    let pending = false;
    const refresh = () => {
      if (pending) return;
      pending = true;
      void media
        .diagnostics()
        .then(
          (sample) => {
            if (!active) return;
            setDiagnostics(sample);
            setDiagnosticsError("");
          },
          () => {
            if (active)
              setDiagnosticsError("Статистика соединения недоступна.");
          },
        )
        .finally(() => {
          pending = false;
        });
    };
    refresh();
    const timer = window.setInterval(refresh, 5000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [
    diagnosticsOpen,
    media.running,
    media.diagnostics,
    live.state?.connectionId,
  ]);
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
        <span
          className={`participant-status ${connectionProblem ? "conference-link-lost" : "conference-link-ready"}`}
        >
          {online ? live.status : "Нет сети"}
        </span>
      </div>
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
          <Button
            variant="secondary"
            disabled={!online}
            onClick={() => {
              media.stop();
              live.reconnect();
            }}
          >
            <RefreshCw size={16} />
            Переподключиться
          </Button>
        </div>
      )}
      {live.state && (
        <details className="conference-presence-details">
          <summary>
            В сети:{" "}
            {live.state.participants.filter((person) => person.online).length}
          </summary>
          <p className="field-hint">
            Подключение этой вкладки:{" "}
            <code data-testid="connection-id">{live.state.connectionId}</code>
          </p>
          <div className="realtime-people">
            {live.state.participants.map(
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
        <div className="meeting-actions">
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
                   */ () =>
                    media.start(true, hasSelection ? preferences : undefined)
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
                aria-keyshortcuts={shortcutsEnabled ? "M" : undefined}
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns вычисленное значение: void media.microphone(!media.view.microphoneEnabled).
                   */ () => void media.microphone(!media.view.microphoneEnabled)
                }
              >
                {media.view.microphoneEnabled ? (
                  <Mic size={17} />
                ) : (
                  <MicOff size={17} />
                )}
                {media.view.microphoneEnabled
                  ? "Выключить микрофон"
                  : "Включить микрофон"}
              </Button>
              <Button
                variant="secondary"
                disabled={busy || membership.cameraBlocked}
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
                {media.view.cameraEnabled
                  ? "Выключить камеру"
                  : "Включить камеру"}
              </Button>
              <Button
                variant="secondary"
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
                {media.view.screenSharing
                  ? "Остановить демонстрацию"
                  : "Показать экран"}
              </Button>
              <Button variant="secondary" onClick={media.stop}>
                <VideoOff size={17} />
                Отключить медиа
              </Button>
            </>
          )}
        </div>
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
        {media.running && devices.length > 0 && (
          <div className="media-devices">
            {(["audioinput", "videoinput"] as const).map(
              /**
               * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
               *
               * @args
               *   - kind — вид устройства, медиаисточника или события, определяющий действие.
               *
               * @returns преобразованное значение текущего элемента для результирующего набора.
               */ (kind) => (
                <label key={kind}>
                  {kind === "audioinput" ? "Микрофон" : "Камера"}
                  <select
                    aria-label={
                      kind === "audioinput" ? "Выбор микрофона" : "Выбор камеры"
                    }
                    value={
                      kind === "audioinput"
                        ? preferences.audioInputId
                        : preferences.videoInputId
                    }
                    disabled={
                      busy ||
                      (kind === "audioinput"
                        ? membership.microphoneBlocked
                        : membership.cameraBlocked)
                    }
                    onChange={
                      /**
                       * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                       *
                       * @args
                       *   - event — проверенный конверт события комнаты.
                       *
                       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                       */ (event) => {
                        const selected = event.target.value;
                        const next = {
                          ...preferences,
                          [kind === "audioinput"
                            ? "audioInputId"
                            : "videoInputId"]: selected,
                        };
                        setPreferences(next);
                        saveDevicePreferences(user?.id || "", next);
                        void (kind === "audioinput"
                          ? media.microphone(true, selected)
                          : media.camera(true, selected));
                      }
                    }
                  >
                    <option value="">Системное устройство</option>
                    {(kind === "audioinput"
                      ? preferences.audioInputId
                      : preferences.videoInputId) &&
                      !devices.some(
                        (device) =>
                          device.kind === kind &&
                          device.deviceId ===
                            (kind === "audioinput"
                              ? preferences.audioInputId
                              : preferences.videoInputId),
                      ) && (
                        <option
                          value={
                            kind === "audioinput"
                              ? preferences.audioInputId
                              : preferences.videoInputId
                          }
                        >
                          Сохранённое устройство
                        </option>
                      )}
                    {devices
                      .filter(
                        /**
                         * Обработчик devices.filter проверяет, должен ли элемент войти в отфильтрованный набор.
                         *
                         * @args
                         *   - device — сведения браузера об одном доступном устройстве.
                         *
                         * @returns логический признак соответствия элемента условию.
                         */ (device) => device.kind === kind,
                      )
                      .map(
                        /**
                         * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
                         *
                         * @args
                         *   - device — сведения браузера об одном доступном устройстве.
                         *   - i — индекс элемента в текущем наборе.
                         *
                         * @returns преобразованное значение текущего элемента для результирующего набора.
                         */ (device, i) => (
                          <option
                            key={device.deviceId || i}
                            value={device.deviceId}
                          >
                            {device.label || `Устройство ${i + 1}`}
                          </option>
                        ),
                      )}
                  </select>
                </label>
              ),
            )}
            {typeof HTMLMediaElement !== "undefined" &&
              "setSinkId" in HTMLMediaElement.prototype && (
                <label>
                  Вывод звука
                  <select
                    value={preferences.audioOutputId}
                    onChange={(event) => {
                      const next = {
                        ...preferences,
                        audioOutputId: event.target.value,
                      };
                      setPreferences(next);
                      saveDevicePreferences(user?.id || "", next);
                    }}
                  >
                    <option value="">Системный динамик</option>
                    {preferences.audioOutputId &&
                      !devices.some(
                        (device) =>
                          device.kind === "audiooutput" &&
                          device.deviceId === preferences.audioOutputId,
                      ) && (
                        <option value={preferences.audioOutputId}>
                          Сохранённый динамик
                        </option>
                      )}
                    {devices
                      .filter((device) => device.kind === "audiooutput")
                      .map((device, index) => (
                        <option
                          key={device.deviceId || index}
                          value={device.deviceId}
                        >
                          {device.label || `Динамик ${index + 1}`}
                        </option>
                      ))}
                  </select>
                </label>
              )}
          </div>
        )}
        {!media.view.localStream &&
          !media.view.localScreen &&
          media.view.remoteStreams.length === 0 && (
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
        {(media.view.localStream ||
          media.view.localScreen ||
          media.view.remoteStreams.length > 0) && (
          <div
            className={`media-grid ${
              media.view.localScreen ||
              media.view.remoteStreams.some(
                /**
                 * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
                 *
                 * @args
                 *   - stream — поток браузерных медиа-дорожек.
                 *
                 * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                 */ (stream) => stream.screen,
              )
                ? "media-grid-sharing"
                : ""
            }`}
          >
            {media.view.localScreen && (
              <MediaTile
                local
                screen
                stream={media.view.localScreen}
                name="Ваш экран"
              />
            )}
            {media.view.localStream && (
              <MediaTile
                local
                stream={media.view.localStream}
                video={media.view.cameraEnabled}
                name="Локальное видео"
              />
            )}
            {media.view.remoteStreams.map(
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
                  screen={remote.screen}
                  stream={remote.stream}
                  video={remote.kinds.includes("video")}
                  sinkId={preferences.audioOutputId}
                  name={
                    (remote.screen ? "Экран · " : "") +
                    (live.state?.participants.find(
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
          </div>
        )}
        {media.view.mediaPeerId && (
          <details
            className="media-diagnostics"
            onToggle={(event) => setDiagnosticsOpen(event.currentTarget.open)}
          >
            <summary>Состояние медиасвязи</summary>
            <dl>
              <dt>Media peer</dt>
              <dd>
                <code data-testid="media-peer-id">
                  {media.view.mediaPeerId}
                </code>
              </dd>
              <dt>Worker</dt>
              <dd>{media.view.workerId}</dd>
              <dt>PeerConnection</dt>
              <dd>{media.view.connectionState}</dd>
              <dt>ICE</dt>
              <dd>{media.view.iceState}</dd>
              <dt>Negotiation</dt>
              <dd>{media.view.negotiationState}</dd>
              <dt>Удалённые потоки</dt>
              <dd>{media.view.remoteStreams.length}</dd>
              {diagnostics?.roundTripTimeMs !== undefined && (
                <>
                  <dt>Задержка RTT</dt>
                  <dd>{diagnostics.roundTripTimeMs} мс</dd>
                </>
              )}
              {diagnostics?.packetLossPercent !== undefined && (
                <>
                  <dt>Потери входящих пакетов</dt>
                  <dd>{diagnostics.packetLossPercent} %</dd>
                </>
              )}
              {diagnostics?.outboundKbps !== undefined && (
                <>
                  <dt>Исходящий поток</dt>
                  <dd>{diagnostics.outboundKbps} кбит/с</dd>
                </>
              )}
              {diagnostics?.inboundKbps !== undefined && (
                <>
                  <dt>Входящий поток</dt>
                  <dd>{diagnostics.inboundKbps} кбит/с</dd>
                </>
              )}
              {diagnostics?.route && (
                <>
                  <dt>Маршрут</dt>
                  <dd>
                    {diagnostics.route === "relay" ? "TURN relay" : "Прямой"}
                  </dd>
                </>
              )}
              <dt>Версия сборки</dt>
              <dd>{capabilities.data?.buildVersion || "Неизвестна"}</dd>
            </dl>
            {(!diagnostics || Object.keys(diagnostics).length === 0) &&
              !diagnosticsError && (
                <p className="field-hint">
                  Измерения появятся после обмена медиа.
                </p>
              )}
            {diagnosticsError && <p role="status">{diagnosticsError}</p>}
            <Button
              variant="outline"
              onClick={() => {
                const report = safeDiagnosticsReport({
                  buildVersion: capabilities.data?.buildVersion,
                  realtimeStatus: live.status,
                  mediaWorkerAvailable: !!media.view.mediaPeerId,
                  connectionState: media.view.connectionState,
                  iceState: media.view.iceState,
                  diagnostics,
                });
                if (!navigator.clipboard?.writeText) {
                  setCopyStatus("Буфер обмена недоступен в этом браузере.");
                  return;
                }
                void navigator.clipboard.writeText(report).then(
                  () => setCopyStatus("Обезличенный отчёт скопирован."),
                  () => setCopyStatus("Не удалось скопировать отчёт."),
                );
              }}
            >
              Скопировать диагностический отчёт
            </Button>
            <p className="field-hint">
              Отчёт содержит только агрегированные показатели и версию сборки,
              без адресов, SDP и токенов.
            </p>
            <p className="field-hint">
              Потери — накопленная доля входящих RTP-пакетов. Скорость считается
              по двум замерам; оценка качества не рассчитывается.
            </p>
            {copyStatus && <p role="status">{copyStatus}</p>}
          </details>
        )}
        <p className="field-hint">
          После переподключения вкладки устройства автоматически не включаются.
          При ошибке ICE проверьте доступность медиа-портов и настройку
          STUN/TURN.
        </p>
      </div>
      {!connectionProblem && (
        <Button
          variant="secondary"
          onClick={() => {
            media.stop();
            live.reconnect();
          }}
        >
          <RefreshCw size={16} />
          Переподключиться
        </Button>
      )}
    </section>
  );
}
