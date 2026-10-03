import { act, renderHook } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { AudioActivitySource } from "./audioActivity";
import { useSpeakingParticipants } from "./useSpeakingParticipants";

const contexts: {
  reads: ReturnType<typeof vi.fn>;
  createMediaStreamSource: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
}[] = [];

/** installGraph предоставляет настоящий монитор с безопасным подставным аудиографом.
 * @args track — уже существующая дорожка, которой тест управляет без разрешения на микрофон.
 */
function installGraph(track: { level: number }) {
  vi.stubGlobal(
    "AudioContext",
    class {
      state = "running";
      destination = {};
      reads = vi.fn((samples: Float32Array) => samples.fill(track.level));
      createMediaStreamSource = vi.fn(() => ({
        connect: vi.fn(),
        disconnect: vi.fn(),
      }));
      close = vi.fn(async () => {
        this.state = "closed";
      });
      constructor() {
        contexts.push(this);
      }
      /** createGain создаёт неслышимый узел для проверки жизненного цикла наблюдения. */
      createGain() {
        return { gain: { value: 0 }, connect: vi.fn(), disconnect: vi.fn() };
      }
      /** createAnalyser возвращает измеритель с управляемым звуковым сигналом. */
      createAnalyser() {
        return {
          fftSize: 512,
          connect: vi.fn(),
          disconnect: vi.fn(),
          getFloatTimeDomainData: this.reads,
        };
      }
    },
  );
  vi.stubGlobal(
    "MediaStream",
    class {
      constructor(private tracks: MediaStreamTrack[]) {}
      /** getAudioTracks предоставляет заимствованные дорожки, не меняя их состояние. */
      getAudioTracks() {
        return this.tracks;
      }
    },
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  contexts.length = 0;
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("перезапускает наблюдение в StrictMode, не останавливая чужие дорожки и не создавая цикл отрисовки", () => {
  const track = {
    level: 0.028,
    readyState: "live",
    enabled: true,
    muted: false,
    stop: vi.fn(),
  };
  installGraph(track);
  const source: AudioActivitySource = {
    id: "local",
    participantId: "self",
    enabled: true,
    stream: { getAudioTracks: () => [track] } as unknown as MediaStream,
  };
  const { result, rerender, unmount } = renderHook(
    () => useSpeakingParticipants([{ ...source }], true),
    { wrapper: StrictMode },
  );
  expect(contexts).toHaveLength(2);
  expect(contexts[0].close).toHaveBeenCalledTimes(1);
  expect(contexts[1].createMediaStreamSource).toHaveBeenCalledTimes(1);
  expect(result.current.size).toBe(0);
  act(() => vi.advanceTimersByTime(50));
  expect(result.current.get("self")).toBe(0.32);
  const displayed = result.current;
  for (let iteration = 0; iteration < 20; iteration++) rerender();
  expect(result.current).toBe(displayed);
  expect(contexts[1].reads).toHaveBeenCalledTimes(2);
  act(() => vi.advanceTimersByTime(500));
  expect(result.current).toBe(displayed);
  expect(contexts[1].createMediaStreamSource).toHaveBeenCalledTimes(1);
  unmount();
  expect(
    contexts.every((context) => context.close.mock.calls.length === 1),
  ).toBe(true);
  expect(track.stop).not.toHaveBeenCalled();
  expect(vi.getTimerCount()).toBe(0);
});

it("немедленно скрывает уровень при отключении и подтверждает его заново после возврата связи", () => {
  const track = {
    level: 0.085,
    readyState: "live",
    enabled: true,
    muted: false,
    stop: vi.fn(),
  };
  installGraph(track);
  const source: AudioActivitySource = {
    id: "local",
    participantId: "self",
    enabled: true,
    stream: { getAudioTracks: () => [track] } as unknown as MediaStream,
  };
  const { result, rerender, unmount } = renderHook(
    ({ enabled }) => useSpeakingParticipants([{ ...source }], enabled),
    { initialProps: { enabled: true } },
  );
  act(() => vi.advanceTimersByTime(50));
  expect(result.current.get("self")).toBe(0.66);
  rerender({ enabled: false });
  expect(result.current.size).toBe(0);
  expect(vi.getTimerCount()).toBe(0);
  rerender({ enabled: true });
  expect(result.current.size).toBe(0);
  act(() => vi.advanceTimersByTime(50));
  expect(result.current.get("self")).toBe(0.66);
  expect(contexts).toHaveLength(1);
  expect(contexts[0].createMediaStreamSource).toHaveBeenCalledTimes(2);
  unmount();
  expect(track.stop).not.toHaveBeenCalled();
});
