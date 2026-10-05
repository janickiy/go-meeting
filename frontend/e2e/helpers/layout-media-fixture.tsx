import { useEffect, useRef, useState } from "react";
import { emptyMediaView } from "../../src/media";
import type { MediaView } from "../../src/media";
import type { useRealtime } from "../../src/realtime";

/** Создаёт настоящий видеотрек из canvas, не запрашивая камеру или экран компьютера.
 * @args name — подпись кадра; background — цвет синтетического источника.
 * @return Поток 16:9 и функция перерисовки для проверки непрерывного воспроизведения.
 */
function videoSource(name: string, background: string) {
  const canvas = document.createElement("canvas");
  canvas.width = 1280;
  canvas.height = 720;
  const draw = canvas.getContext("2d")!;
  let sequence = 0;
  const paint = () => {
    draw.fillStyle = background;
    draw.fillRect(0, 0, canvas.width, canvas.height);
    draw.fillStyle = "#d6e4f5";
    draw.fillRect(30, 30, 1220, 4);
    draw.fillRect(30, 686, 1220, 4);
    draw.font = "52px sans-serif";
    draw.fillText(name, 60, 330);
    draw.font = "28px sans-serif";
    draw.fillText(`1280 × 720 · кадр ${++sequence}`, 60, 390);
  };
  paint();
  return { stream: canvas.captureStream(2), paint };
}

/** Подменяется только сетевым маршрутом e2e; production никогда не импортирует этот модуль.
 * Настоящие RealtimePanel и CSS получают тестовые видеопотоки без SFU или физических устройств.
 * @args live — авторитетный тестовый состав из штатного realtime-хука.
 * @return Совместимый с useMedia интерфейс для проверки раскладки и показа экрана.
 */
export function useMedia(live: ReturnType<typeof useRealtime>) {
  const [view, setView] = useState<MediaView>(emptyMediaView);
  const [running, setRunning] = useState(false);
  const sources = useRef<ReturnType<typeof videoSource>[]>([]);
  const repaint = useRef<ReturnType<typeof setInterval> | null>(null);
  const roster = live.state?.participants ?? [];
  const rosterKey = roster.map((person) => person.id).join(",");
  useEffect(() => {
    if (!running) return;
    setView((old) => ({
      ...old,
      active: true,
      status: "Тестовые видеопотоки подключены",
      localStream: sources.current[0].stream,
      cameraEnabled: true,
      remoteStreams: roster.slice(1).map((person, index) => ({
        id: `layout-video-${person.id}`,
        participantId: person.id,
        mediaPeerId: `layout-peer-${person.id}`,
        stream: sources.current[index + 1].stream,
        kinds: ["video"],
        screen: false,
      })),
      mediaPeerId: "layout-local-peer",
      workerId: "layout-test-worker",
      connectionState: "connected",
      iceState: "connected",
      negotiationState: "stable",
    }));
  }, [running, rosterKey]);
  useEffect(
    () => () => {
      if (repaint.current !== null) clearInterval(repaint.current);
      sources.current.forEach((source) =>
        source.stream.getTracks().forEach((track) => track.stop()),
      );
    },
    [],
  );

  /** Начинает воспроизведение только синтетических видео; повторно дорожки не создаёт. */
  const start = () => {
    if (!sources.current.length) {
      sources.current = [
        videoSource("Алексей Петров", "#273e58"),
        videoSource("Мария Соколова", "#385746"),
        videoSource("Иван Ким", "#644b36"),
        videoSource("Елена Смирнова", "#4c3b65"),
        videoSource("Общий экран", "#234774"),
      ];
      repaint.current = setInterval(
        () => sources.current.forEach((source) => source.paint()),
        200,
      );
    }
    setRunning(true);
  };
  return {
    view,
    running,
    start,
    stop: () => {
      setRunning(false);
      setView(emptyMediaView());
    },
    diagnostics: async () => null,
    microphone: (enabled: boolean) =>
      setView((old) => ({ ...old, microphoneEnabled: enabled })),
    camera: (enabled: boolean) =>
      setView((old) => ({ ...old, cameraEnabled: enabled })),
    startScreen: () =>
      setView((old) => ({
        ...old,
        localScreen: sources.current[4].stream,
        screenSharing: true,
      })),
    stopScreen: () =>
      setView((old) => ({ ...old, localScreen: null, screenSharing: false })),
  };
}
