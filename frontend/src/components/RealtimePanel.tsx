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
}: {
  stream: MediaStream;
  name: string;
  local?: boolean;
  video?: boolean;
  screen?: boolean;
}) {
  const element = useRef<HTMLMediaElement | null>(null);
  const [blocked, setBlocked] = useState(false);
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
    </div>
  );
}

// Hardware capture always requires an explicit user action.
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
}: {
  conferenceId: string;
  membership: Participant;
  live: ReturnType<typeof useRealtime>;
}) {
  const media = useMedia(live, conferenceId, {
    ...membership,
    version: membership.mediaPolicyVersion,
  });
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
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
  const busy = media.view.controlBusy || !media.view.mediaPeerId;
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
        <span className="participant-status">{live.status}</span>
      </div>
      <ErrorNotice error={live.error} />
      {live.state && (
        <>
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
        </>
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
                disabled={!live.state}
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @returns вычисленное значение: media.start().
                   */ () => media.start()
                }
              >
                <Video size={17} />
                {media.view.error
                  ? "Подключить медиасвязь снова"
                  : "Включить камеру и микрофон"}
              </Button>
              <Button
                variant="secondary"
                disabled={!live.state}
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
                    defaultValue=""
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
                       */ (event) =>
                        void (kind === "audioinput"
                          ? media.microphone(true, event.target.value)
                          : media.camera(true, event.target.value))
                    }
                  >
                    <option value="" disabled>
                      Выберите устройство
                    </option>
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
          <details className="media-diagnostics">
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
            </dl>
          </details>
        )}
        <p className="field-hint">
          После переподключения вкладки устройства автоматически не включаются.
          При ошибке ICE проверьте доступность медиа-портов и настройку
          STUN/TURN.
        </p>
      </div>
      <Button
        variant="secondary"
        onClick={
          /**
           * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
           *
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ () => {
            media.stop();
            live.reconnect();
          }
        }
      >
        <RefreshCw size={16} />
        Переподключиться
      </Button>
    </section>
  );
}
