import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AudioActivityMonitor,
  AudioLevelEnvelope,
  normalizeAudioLevel,
  type AudioActivitySource,
} from "./audioActivity";

describe("normalizeAudioLevel", () => {
  it("различает тихий, обычный и громкий голос на логарифмической шкале", () => {
    const quiet = normalizeAudioLevel(0.028);
    const medium = normalizeAudioLevel(0.085);
    const loud = normalizeAudioLevel(0.247);
    expect(quiet).toBeGreaterThan(0.25);
    expect(quiet).toBeLessThan(medium);
    expect(medium).toBeGreaterThan(0.6);
    expect(medium).toBeLessThan(0.8);
    expect(medium).toBeLessThan(loud);
    expect(loud).toBeGreaterThan(0.95);
  });

  it("отсекает шум и некорректные замеры, ограничивая сильный звук единицей", () => {
    for (const rms of [0, 0.008, 0.01, -1, NaN, Infinity, -Infinity])
      expect(normalizeAudioLevel(rms)).toBe(0);
    expect(normalizeAudioLevel(0.25)).toBe(1);
    expect(normalizeAudioLevel(1)).toBe(1);
  });
});

describe("AudioLevelEnvelope", () => {
  it("отбрасывает шум и одиночный щелчок, но подтверждает звук за 50 мс", () => {
    const envelope = new AudioLevelEnvelope();
    expect(envelope.update(0.008, 0)).toBe(0);
    expect(envelope.update(0.1, 50)).toBe(0);
    expect(envelope.update(0, 100)).toBe(0);
    expect(envelope.update(0.1, 150)).toBe(0);
    expect(envelope.update(0.1, 200)).toBeCloseTo(0.72, 2);
  });

  it("меняет интенсивность вслед за звуком и гасит паузу за 100 мс без фиксированного свечения", () => {
    const envelope = new AudioLevelEnvelope();
    envelope.update(0.028, 0);
    const quiet = envelope.update(0.028, 50);
    const medium = envelope.update(0.085, 100);
    const loud = envelope.update(0.247, 150);
    expect(quiet).toBeGreaterThan(0);
    expect(medium).toBeGreaterThan(quiet);
    expect(loud).toBeGreaterThan(medium);
    const softer = envelope.update(0.028, 200);
    expect(softer).toBeLessThan(loud);
    expect(envelope.update(0, 250)).toBe(softer);
    expect(envelope.update(0, 300)).toBeLessThan(softer);
    expect(envelope.update(0, 350)).toBe(0);
    expect(envelope.update(0.028, 400)).toBe(0);
    expect(envelope.update(0.028, 450)).toBeGreaterThan(0);
  });

  it("сбрасывает уровень сразу при недоступности дорожки и ошибочном времени или сигнале", () => {
    const envelope = new AudioLevelEnvelope();
    for (const [rms, now, available] of [
      [0.1, 100, false],
      [NaN, 100, true],
      [Infinity, 100, true],
      [-1, 100, true],
      [0.1, NaN, true],
      [0.1, 49, true],
    ] as const) {
      envelope.update(0.1, 0);
      expect(envelope.update(0.1, 50)).toBeGreaterThan(0);
      expect(envelope.update(rms, now, available)).toBe(0);
      expect(envelope.level).toBe(0);
    }
  });
});

/** FakeTrack позволяет проверить реакцию на mute/ended без захвата устройств.
 * @params level — амплитуда; readyState/enabled/muted — состояние браузерной дорожки;
 * stop — проверка, что наблюдатель не останавливает заимствованный поток.
 */
class FakeTrack extends EventTarget {
  readyState = "live";
  enabled = true;
  muted = false;
  level = 0;
  stop = vi.fn();
}

/** FakeContext имитирует Web Audio-граф с управляемой амплитудой каждого источника.
 * @params instances — созданные контексты; analysers — графы источников;
 * pending — дорожка последнего подключаемого узла; state — состояние контекста.
 */
class FakeContext extends EventTarget {
  static instances: FakeContext[] = [];
  state = "running";
  destination = {};
  analysers: {
    disconnect: ReturnType<typeof vi.fn>;
    getFloatTimeDomainData: ReturnType<typeof vi.fn>;
  }[] = [];
  pending: FakeTrack | null = null;
  createGain = vi.fn(() => ({
    gain: { value: 1 },
    connect: vi.fn(),
    disconnect: vi.fn(),
  }));
  createMediaStreamSource = vi.fn((stream: MediaStream) => {
    this.pending = stream.getAudioTracks()[0] as unknown as FakeTrack;
    return { connect: vi.fn(), disconnect: vi.fn() };
  });
  createAnalyser = vi.fn(() => {
    const track = this.pending!;
    const node = {
      fftSize: 512,
      connect: vi.fn(),
      disconnect: vi.fn(),
      getFloatTimeDomainData: vi.fn((samples: Float32Array) =>
        samples.fill(track.level),
      ),
    };
    this.analysers.push(node);
    return node;
  });
  resume = vi.fn(async () => {
    this.state = "running";
  });
  close = vi.fn(async () => {
    this.state = "closed";
  });
  constructor() {
    super();
    FakeContext.instances.push(this);
  }
}

/** input оборачивает подставную дорожку в контракт существующего потока.
 * @args id — подключение; participantId — участник; track — управляемая дорожка.
 * @return микрофонный источник без запроса разрешений браузера.
 */
function input(
  id: string,
  participantId: string,
  track: FakeTrack,
): AudioActivitySource {
  return {
    id,
    participantId,
    enabled: true,
    stream: { getAudioTracks: () => [track] } as unknown as MediaStream,
  };
}

let monitor: AudioActivityMonitor;
beforeEach(() => {
  vi.useFakeTimers();
  FakeContext.instances = [];
  vi.stubGlobal("AudioContext", FakeContext);
  vi.stubGlobal(
    "MediaStream",
    class {
      constructor(private tracks: MediaStreamTrack[]) {}
      getAudioTracks() {
        return this.tracks;
      }
    },
  );
});
afterEach(() => {
  monitor?.stop();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("анализирует свои и удалённые микрофоны в одном контексте без повторного воспроизведения", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const local = new FakeTrack(),
    remote = new FakeTrack();
  local.level = remote.level = 0.1;
  monitor.setSources([
    input("local", "self", local),
    input("remote", "other", remote),
  ]);
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenLastCalledWith(
    new Map([
      ["self", 0.72],
      ["other", 0.72],
    ]),
  );
  expect(FakeContext.instances).toHaveLength(1);
  const context = FakeContext.instances[0];
  expect(context.createGain.mock.results[0].value.gain.value).toBe(0);
  monitor.setSources([
    input("local", "self", local),
    input("remote", "other", remote),
  ]);
  expect(context.createMediaStreamSource).toHaveBeenCalledTimes(2);
  remote.muted = true;
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenLastCalledWith(new Map([["self", 0.72]]));
  local.enabled = false;
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenLastCalledWith(new Map());
  monitor.stop();
  expect(context.close).toHaveBeenCalledTimes(1);
  expect(local.stop).not.toHaveBeenCalled();
  expect(remote.stop).not.toHaveBeenCalled();
  expect(vi.getTimerCount()).toBe(0);
});

it("не повторяет замеры и не вызывает цикл обновлений при одинаковых источниках и громкости", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const track = new FakeTrack();
  track.level = 0.028;
  monitor.setSources([input("a", "one", track)]);
  const analyser = FakeContext.instances[0].analysers[0];
  expect(analyser.getFloatTimeDomainData).toHaveBeenCalledTimes(1);
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenCalledTimes(1);
  expect(analyser.getFloatTimeDomainData).toHaveBeenCalledTimes(2);
  for (let render = 0; render < 20; render++)
    monitor.setSources([input("a", "one", track)]);
  expect(changed).toHaveBeenCalledTimes(1);
  expect(analyser.getFloatTimeDomainData).toHaveBeenCalledTimes(2);
  vi.advanceTimersByTime(500);
  expect(changed).toHaveBeenCalledTimes(1);
  expect(
    FakeContext.instances[0].createMediaStreamSource,
  ).toHaveBeenCalledTimes(1);
});

it("обновляет громкость каждые 50 мс и убирает тихий ключ не позднее чем через 150 мс", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const track = new FakeTrack();
  track.level = 0.028;
  monitor.setSources([input("a", "one", track)]);
  vi.advanceTimersByTime(50);
  const quiet = changed.mock.lastCall![0].get("one") as number;
  track.level = 0.085;
  vi.advanceTimersByTime(50);
  const medium = changed.mock.lastCall![0].get("one") as number;
  track.level = 0.247;
  vi.advanceTimersByTime(50);
  const loud = changed.mock.lastCall![0].get("one") as number;
  expect(quiet).toBeLessThan(medium);
  expect(medium).toBeLessThan(loud);
  expect(loud).toBeLessThanOrEqual(1);
  track.level = 0;
  vi.advanceTimersByTime(100);
  expect(changed.mock.lastCall![0].get("one")).toBeLessThan(loud);
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenLastCalledWith(new Map());
});

it("сразу очищает события mute/ended и освобождает завершённую дорожку без обновления React", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const track = new FakeTrack();
  track.level = 0.1;
  monitor.setSources([input("a", "one", track)]);
  vi.advanceTimersByTime(50);
  track.muted = true;
  track.dispatchEvent(new Event("mute"));
  expect(changed).toHaveBeenLastCalledWith(new Map());
  track.muted = false;
  vi.advanceTimersByTime(100);
  expect(changed.mock.lastCall![0].has("one")).toBe(true);
  track.readyState = "ended";
  track.dispatchEvent(new Event("ended"));
  expect(changed).toHaveBeenLastCalledWith(new Map());
  expect(
    FakeContext.instances[0].analysers[0].disconnect,
  ).toHaveBeenCalledTimes(1);
  expect(vi.getTimerCount()).toBe(0);
  track.dispatchEvent(new Event("ended"));
  expect(
    FakeContext.instances[0].analysers[0].disconnect,
  ).toHaveBeenCalledTimes(1);
});

it("освобождает завершённую дорожку без события браузера при следующем замере", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const track = new FakeTrack();
  track.level = 0.1;
  monitor.setSources([input("active", "one", track)]);
  vi.advanceTimersByTime(50);
  track.readyState = "ended";
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenLastCalledWith(new Map());
  expect(
    FakeContext.instances[0].analysers[0].disconnect,
  ).toHaveBeenCalledTimes(1);
  expect(vi.getTimerCount()).toBe(0);
});

it("игнорирует выключенные и завершённые источники и сразу очищает исчезнувших участников", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const track = new FakeTrack(),
    ended = new FakeTrack();
  track.level = ended.level = 0.1;
  ended.readyState = "ended";
  monitor.setSources([
    input("active", "one", track),
    { ...input("disabled", "two", track), enabled: false },
    input("ended", "three", ended),
  ]);
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenLastCalledWith(new Map([["one", 0.72]]));
  expect(
    FakeContext.instances[0].createMediaStreamSource,
  ).toHaveBeenCalledTimes(1);
  monitor.setSources([]);
  expect(changed).toHaveBeenLastCalledWith(new Map());
  expect(vi.getTimerCount()).toBe(0);
  expect(
    FakeContext.instances[0].analysers[0].disconnect,
  ).toHaveBeenCalledTimes(1);
});

it("берёт максимальную громкость участника и заменяет дорожку без лишнего контекста", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const first = new FakeTrack(),
    second = new FakeTrack();
  first.level = 0.028;
  second.level = 0.085;
  monitor.setSources([input("a", "one", first), input("b", "one", second)]);
  vi.advanceTimersByTime(50);
  const louder = changed.mock.lastCall![0].get("one") as number;
  expect(changed.mock.lastCall![0].size).toBe(1);
  second.muted = true;
  second.dispatchEvent(new Event("mute"));
  expect(changed.mock.lastCall![0].get("one")).toBeLessThan(louder);
  const replacement = new FakeTrack();
  monitor.setSources([input("a", "one", replacement)]);
  expect(changed).toHaveBeenLastCalledWith(new Map());
  expect(FakeContext.instances).toHaveLength(1);
  expect(
    FakeContext.instances[0].createMediaStreamSource,
  ).toHaveBeenCalledTimes(3);
});

it("очищает выключенную дорожку при согласовании источников без повторного анализа остальных", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const track = new FakeTrack();
  track.level = 0.1;
  monitor.setSources([input("a", "one", track)]);
  vi.advanceTimersByTime(50);
  track.enabled = false;
  monitor.setSources([input("a", "one", track)]);
  expect(changed).toHaveBeenLastCalledWith(new Map());
  expect(
    FakeContext.instances[0].analysers[0].getFloatTimeDomainData,
  ).toHaveBeenCalledTimes(2);
});

it("гасит приостановленный контекст и некорректные сэмплы без ожидания затухания", () => {
  const changed = vi.fn();
  monitor = new AudioActivityMonitor(changed);
  const track = new FakeTrack();
  track.level = 0.1;
  monitor.setSources([input("a", "one", track)]);
  vi.advanceTimersByTime(50);
  const context = FakeContext.instances[0];
  context.state = "suspended";
  context.dispatchEvent(new Event("statechange"));
  expect(changed).toHaveBeenLastCalledWith(new Map());
  context.state = "running";
  vi.advanceTimersByTime(100);
  expect(changed.mock.lastCall![0].has("one")).toBe(true);
  track.level = NaN;
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenLastCalledWith(new Map());
  track.level = 0.1;
  vi.advanceTimersByTime(100);
  context.analysers[0].getFloatTimeDomainData.mockImplementation(() => {
    throw new Error("Недоступный аудиограф");
  });
  vi.advanceTimersByTime(50);
  expect(changed).toHaveBeenLastCalledWith(new Map());
});

it("ожидает разрешение autoplay и не мешает звонку без Web Audio", async () => {
  monitor = new AudioActivityMonitor(vi.fn());
  const track = new FakeTrack();
  track.level = 0.1;
  monitor.setSources([input("a", "one", track)]);
  const context = FakeContext.instances[0];
  context.state = "suspended";
  document.dispatchEvent(new Event("pointerdown"));
  await Promise.resolve();
  expect(context.resume).toHaveBeenCalledTimes(1);
  expect(context.state).toBe("running");
  monitor.stop();
  document.dispatchEvent(new Event("pointerdown"));
  expect(context.resume).toHaveBeenCalledTimes(1);
  vi.stubGlobal("AudioContext", undefined);
  monitor = new AudioActivityMonitor(vi.fn());
  expect(() => monitor.setSources([input("a", "one", track)])).not.toThrow();
  expect(vi.getTimerCount()).toBe(0);
});
