import { useEffect, useRef, useState } from "react";
import {
  AudioActivityMonitor,
  type AudioActivitySource,
} from "./audioActivity";

const nobodySpeaking: ReadonlyMap<string, number> = new Map();

/** useSpeakingParticipants связывает общий детектор аудио с плитками React.
 * Обновления состава не пересоздают контекст; выход закрывает все ресурсы наблюдения.
 * @args sources — существующие микрофонные потоки; enabled — доступна ли медиасвязь.
 * @return нормированная громкость 0–1 для звучащих участников; в тишине ключ отсутствует.
 */
export function useSpeakingParticipants(
  sources: AudioActivitySource[],
  enabled: boolean,
) {
  const [speaking, setSpeaking] =
    useState<ReadonlyMap<string, number>>(nobodySpeaking);
  const monitor = useRef<AudioActivityMonitor | null>(null);
  useEffect(() => {
    const current = new AudioActivityMonitor(setSpeaking);
    monitor.current = current;
    return () => {
      monitor.current = null;
      current.stop();
    };
  }, []);
  // Входной массив может меняться при каждой перерисовке; наблюдатель сравнивает
  // дорожки по ссылке: неизменённые источники не перепривязываются и не замеряются повторно.
  useEffect(() => {
    monitor.current?.setSources(enabled ? sources : []);
  });
  return enabled ? speaking : nobodySpeaking;
}
