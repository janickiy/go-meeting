import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { MediaTile } from "../../src/components/RealtimePanel";
import { useSpeakingParticipants } from "../../src/useSpeakingParticipants";
import "../../src/styles.css";
import "../../src/pages/conference.css";

/** FixtureOptions выбирает проверку индикации речи либо режима показа экрана.
 * @params screenSharing — добавить переключение локального и удалённого экрана;
 * participantRoles — показать значки организатора и соорганизатора;
 * volumeControls — добавить проверку нескольких уровней громкости.
 */
interface FixtureOptions {
  screenSharing?: boolean;
  participantRoles?: boolean;
  volumeControls?: boolean;
}

/** Fixture проверяет реальные Web Audio и плитки без SFU и физических устройств.
 * Только тестовый Vite загружает этот модуль; в production-сборку он не импортируется.
 * @return управляемая комната с синтетическими звуками и обычными компонентами UI.
 */
function Fixture({
  screenSharing = false,
  participantRoles = false,
  volumeControls = false,
}: FixtureOptions) {
  const [streams, setStreams] = useState<MediaStream[]>([]);
  const [muted, setMuted] = useState(false);
  const [connected, setConnected] = useState(true);
  const [screenOwner, setScreenOwner] = useState<"local" | "remote" | null>(
    null,
  );
  const showingScreen = screenSharing && screenOwner !== null;
  const signals = useRef<GainNode[]>([]);
  const context = useRef<AudioContext | null>(null);
  const owned = useRef<MediaStream[]>([]);
  const sources = streams.slice(0, 2).map((stream, index) => ({
    id: String(index),
    participantId: String(index),
    stream,
    enabled: index !== 1 || !muted,
  }));
  const speaking = useSpeakingParticipants(sources, connected);
  useEffect(
    () => () => {
      owned.current.forEach((stream) =>
        stream.getTracks().forEach((track) => track.stop()),
      );
      void context.current?.close();
    },
    [],
  );

  /** start создаёт три настоящих потока с управляемым уровнем синтетического звука.
   * Дополнительный видеотрек проверяет подсветку видео, третий поток изображает экран.
   */
  async function start() {
    const audio = new AudioContext();
    context.current = audio;
    await audio.resume();
    const result = [0, 1, 2].map(() => {
      const oscillator = audio.createOscillator();
      const gain = audio.createGain();
      gain.gain.value = 0;
      oscillator.frequency.value = 440;
      const destination = audio.createMediaStreamDestination();
      oscillator.connect(gain);
      gain.connect(destination);
      oscillator.start();
      signals.current.push(gain);
      return destination.stream;
    });
    const canvas = document.createElement("canvas");
    canvas.width = 960;
    canvas.height = 540;
    const draw = canvas.getContext("2d")!;
    draw.fillStyle = "#223247";
    draw.fillRect(0, 0, 960, 540);
    draw.fillStyle = "#bac7de";
    draw.font = "100px sans-serif";
    draw.fillText("Б", 440, 310);
    const video = canvas.captureStream(1);
    owned.current = [...result, video];
    result[1].addTrack(video.getVideoTracks()[0]);
    const screenCanvas = document.createElement("canvas");
    screenCanvas.width = 1280;
    screenCanvas.height = 720;
    const screenDraw = screenCanvas.getContext("2d")!;
    screenDraw.fillStyle = "#edf2fa";
    screenDraw.fillRect(0, 0, 1280, 720);
    screenDraw.fillStyle = "#1766eb";
    screenDraw.fillRect(0, 0, 1280, 100);
    screenDraw.fillStyle = "#172137";
    screenDraw.font = "52px sans-serif";
    screenDraw.fillText("Общий экран", 64, 220);
    screenDraw.font = "32px sans-serif";
    screenDraw.fillText("Вместо всех видеоплиток участников", 64, 300);
    const display = screenCanvas.captureStream(1);
    result[2].addTrack(display.getVideoTracks()[0]);
    owned.current.push(display);
    setStreams(result);
  }

  /** setVolume меняет амплитуду всех синтетических аудиодорожек.
   * @args volume — линейная амплитуда генератора: ноль означает тишину.
   */
  function setVolume(volume: number) {
    signals.current.forEach((signal) => {
      signal.gain.value = volume;
    });
  }

  return (
    <main
      className="conference-room-page"
      style={{ position: "fixed", inset: 0, zIndex: 1000 }}
    >
      <header className="room-header">
        <h1>
          {screenSharing
            ? "Проверка демонстрации экрана"
            : "Проверка подсветки говорящего"}
        </h1>
      </header>
      <div className="meeting-actions">
        {!streams.length && (
          <button onClick={() => void start()}>
            Запустить тестовые потоки
          </button>
        )}
        <button onClick={() => setVolume(0.12)}>Звук</button>
        {volumeControls && (
          <>
            <button onClick={() => setVolume(0.04)}>Тихий звук</button>
            <button onClick={() => setVolume(0.12)}>Средний звук</button>
            <button onClick={() => setVolume(0.35)}>Громкий звук</button>
          </>
        )}
        <button onClick={() => setVolume(0)}>Тишина</button>
        <button onClick={() => setMuted((old) => !old)}>
          Переключить микрофон Бориса
        </button>
        <button onClick={() => setConnected(false)}>Отключить связь</button>
        {screenSharing && (
          <>
            <button onClick={() => setScreenOwner("local")}>
              Показать свой экран
            </button>
            <button onClick={() => setScreenOwner("remote")}>
              Показать экран Бориса
            </button>
            <button onClick={() => setScreenOwner(null)}>
              Остановить показ
            </button>
          </>
        )}
      </div>
      <div
        className={`media-grid ${showingScreen ? "media-grid-sharing" : ""}`}
      >
        <MediaTile
          local
          name="Алиса"
          stream={streams[0]}
          video={false}
          microphoneEnabled
          role={participantRoles ? "owner" : undefined}
          audioLevel={speaking.get("0") ?? 0}
          covered={showingScreen}
        />
        <MediaTile
          name="Борис"
          stream={streams[1]}
          video
          microphoneEnabled={!muted}
          role={participantRoles ? "co_host" : undefined}
          audioLevel={speaking.get("1") ?? 0}
          covered={showingScreen}
        />
        {(!screenSharing || showingScreen) && (
          <MediaTile
            name={screenOwner === "local" ? "Ваш экран" : "Экран · Борис"}
            local={screenOwner === "local"}
            stream={streams[2]}
            video={screenSharing}
            screen
            microphoneEnabled
            audioLevel={speaking.get(screenOwner === "local" ? "0" : "1") ?? 0}
          />
        )}
        {screenSharing && (
          <MediaTile name="Вера" video={false} covered={showingScreen} />
        )}
      </div>
    </main>
  );
}

/** install монтирует изолированную проверку на странице тестового браузера.
 * @args options — режим проверки; по умолчанию индикация речи.
 * @return тестовый UI без обращения к API и без захвата микрофона.
 */
export function install(options: FixtureOptions = {}) {
  const container = document.createElement("div");
  document.body.append(container);
  createRoot(container).render(<Fixture {...options} />);
}
