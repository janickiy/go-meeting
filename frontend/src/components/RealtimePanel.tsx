import { useEffect, useRef, useState } from "react";
import { Radio, RefreshCw, Video, VideoOff, Volume2 } from "lucide-react";
import { useRealtime } from "../realtime";
import { useMedia } from "../useMedia";
import { Button, ErrorNotice } from "./ui";

function MediaTile({
  stream,
  name,
  local = false,
  video = true,
}: {
  stream: MediaStream;
  name: string;
  local?: boolean;
  video?: boolean;
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
      className={`media-tile ${local ? "media-tile-local" : ""}`}
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

// Minimal development media screen. Browser permissions are requested only
// by the explicit start action; media goes to the SFU, never to a P2P mesh.
export function RealtimePanel({ conferenceId }: { conferenceId: string }) {
  const live = useRealtime(conferenceId, true);
  const media = useMedia(live);
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
          <span className="dev-badge">Stage 3 · dev</span>
        </div>
        <p className="field-hint">
          Звук и видео передаются через media-worker. Доступ к устройствам
          запрашивается только после нажатия кнопки. Запись конференции пока не
          включена.
        </p>
        <p role="status" data-testid="media-status">
          {media.view.status}
        </p>
        {media.view.error && <ErrorNotice>{media.view.error}</ErrorNotice>}
        <div className="meeting-actions">
          <Button disabled={!live.state || media.running} onClick={media.start}>
            <Video size={17} />
            {media.view.error
              ? "Подключить медиасвязь снова"
              : "Включить камеру и микрофон"}
          </Button>
          {media.running && (
            <Button variant="secondary" onClick={media.stop}>
              <VideoOff size={17} />
              Отключить медиа
            </Button>
          )}
        </div>
        {(media.view.localStream || media.view.remoteStreams.length > 0) && (
          <div className="media-grid">
            {media.view.localStream && (
              <MediaTile
                local
                stream={media.view.localStream}
                name="Локальное видео"
              />
            )}
            {media.view.remoteStreams.map((remote) => (
              <MediaTile
                key={remote.mediaPeerId}
                stream={remote.stream}
                video={remote.kinds.includes("video")}
                name={
                  live.state?.participants.find(
                    (p) => p.id === remote.participantId,
                  )?.displayName || "Участник"
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
