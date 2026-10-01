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
import { useRealtime } from "../realtime";
import { useMedia } from "../useMedia";
import { Button, ErrorNotice } from "./ui";
import type { Participant } from "../types";

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
  const play = async () => {
    try {
      await element.current?.play();
      setBlocked(false);
    } catch {
      setBlocked(true);
    }
  };
  useEffect(() => {
    const media = element.current;
    if (!media) return;
    media.srcObject = stream;
    let active = true;
    void media.play().catch(() => {
      if (active) setBlocked(true);
    });
    return () => {
      active = false;
      media.srcObject = null;
    };
  }, [stream, video]);
  return (
    <div
      className={`media-tile ${local ? "media-tile-local" : ""} ${screen ? "media-tile-screen" : ""}`}
      data-testid={local ? "local-media" : "remote-media"}
    >
      {video ? (
        <video
          ref={(node) => {
            element.current = node;
          }}
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
            ref={(node) => {
              element.current = node;
            }}
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
        <Button variant="secondary" onClick={() => void play()}>
          Включить воспроизведение
        </Button>
      )}
    </div>
  );
}

// Hardware capture always requires an explicit user action.
export function RealtimePanel({
  conferenceId,
  membership,
}: {
  conferenceId: string;
  membership: Participant;
}) {
  const live = useRealtime(conferenceId, true);
  const media = useMedia(live, conferenceId, {
    ...membership,
    version: membership.mediaPolicyVersion,
  });
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  useEffect(() => {
    if (!media.running) {
      setDevices([]);
      return;
    }
    let active = true;
    const refresh = () => {
      void navigator.mediaDevices
        ?.enumerateDevices()
        .then((items) => {
          if (active) setDevices(items);
        })
        .catch(() => {});
    };
    refresh();
    navigator.mediaDevices?.addEventListener("devicechange", refresh);
    return () => {
      active = false;
      navigator.mediaDevices?.removeEventListener("devicechange", refresh);
    };
  }, [media.running, media.view.microphoneEnabled, media.view.cameraEnabled]);
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
            {live.state.participants.map((p) => (
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
            ))}
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
              <Button disabled={!live.state} onClick={() => media.start()}>
                <Video size={17} />
                {media.view.error
                  ? "Подключить медиасвязь снова"
                  : "Включить камеру и микрофон"}
              </Button>
              <Button
                variant="secondary"
                disabled={!live.state}
                onClick={() => media.start(false)}
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
                onClick={() =>
                  void media.microphone(!media.view.microphoneEnabled)
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
                onClick={() => void media.camera(!media.view.cameraEnabled)}
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
                onClick={() =>
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
            {(["audioinput", "videoinput"] as const).map((kind) => (
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
                  onChange={(event) =>
                    void (kind === "audioinput"
                      ? media.microphone(true, event.target.value)
                      : media.camera(true, event.target.value))
                  }
                >
                  <option value="" disabled>
                    Выберите устройство
                  </option>
                  {devices
                    .filter((device) => device.kind === kind)
                    .map((device, i) => (
                      <option
                        key={device.deviceId || i}
                        value={device.deviceId}
                      >
                        {device.label || `Устройство ${i + 1}`}
                      </option>
                    ))}
                </select>
              </label>
            ))}
          </div>
        )}
        {(media.view.localStream ||
          media.view.localScreen ||
          media.view.remoteStreams.length > 0) && (
          <div
            className={`media-grid ${media.view.localScreen || media.view.remoteStreams.some((stream) => stream.screen) ? "media-grid-sharing" : ""}`}
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
            {media.view.remoteStreams.map((remote) => (
              <MediaTile
                key={remote.id}
                screen={remote.screen}
                stream={remote.stream}
                video={remote.kinds.includes("video")}
                name={
                  (remote.screen ? "Экран · " : "") +
                  (live.state?.participants.find(
                    (p) => p.id === remote.participantId,
                  )?.displayName || "Участник")
                }
              />
            ))}
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
        onClick={() => {
          media.stop();
          live.reconnect();
        }}
      >
        <RefreshCw size={16} />
        Переподключиться
      </Button>
    </section>
  );
}
