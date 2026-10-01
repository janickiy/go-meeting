import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "./api";
import { emptyMediaView, type MediaView } from "./media";
import { useMedia } from "./useMedia";
import type { useRealtime } from "./realtime";

const clients = vi.hoisted(
  () =>
    [] as {
      emit: (state: MediaView) => void;
      start: ReturnType<typeof vi.fn>;
    }[],
);
vi.mock("./media", async (original) => {
  const actual = await original<typeof import("./media")>();
  return {
    ...actual,
    ConferenceMediaClient: class {
      start = vi.fn(async () => {});
      constructor(
        _send: unknown,
        readonly emit: (state: MediaView) => void,
      ) {
        clients.push(this);
      }
      stop() {
        this.emit(actual.emptyMediaView());
      }
      setPolicy() {}
      handle() {}
    },
  };
});

function live(connectionId: string): ReturnType<typeof useRealtime> {
  return {
    state: { connectionId },
    onEvent: { current: null },
    send: vi.fn(),
  } as unknown as ReturnType<typeof useRealtime>;
}
const tick = () =>
  act(async () => {
    await vi.advanceTimersByTimeAsync(101);
  });
beforeEach(() => {
  vi.useFakeTimers();
  clients.length = 0;
  vi.spyOn(api, "setMediaState").mockResolvedValue(
    {} as Awaited<ReturnType<typeof api.setMediaState>>,
  );
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("per-connection media snapshots", () => {
  it("increments sequence across capture stop/restart on the same connection", async () => {
    const { result } = renderHook(() => useMedia(live("tab-a"), "room", {}));
    await tick();
    act(() => result.current.start(false));
    expect(clients[0].start).toHaveBeenCalledWith(false);
    act(() =>
      clients[0].emit({
        ...emptyMediaView(),
        active: true,
        microphoneEnabled: true,
      }),
    );
    await tick();
    act(() => result.current.stop());
    await tick();
    act(() => result.current.start());
    act(() =>
      clients[1].emit({
        ...emptyMediaView(),
        active: true,
        cameraEnabled: true,
      }),
    );
    await tick();
    const states = vi
      .mocked(api.setMediaState)
      .mock.calls.map(([, state]) => state);
    expect(states.map((state) => state.connectionId)).toEqual([
      "tab-a",
      "tab-a",
      "tab-a",
      "tab-a",
    ]);
    expect(states.map((state) => state.sequence)).toEqual([1, 2, 3, 4]);
    expect(states[2].microphoneEnabled).toBe(false);
    expect(states[3].cameraEnabled).toBe(true);
  });

  it("never relabels an old snapshot with a reconnected tab's identity", async () => {
    const { result, rerender } = renderHook(
      ({ id }) => useMedia(live(id), "room", {}),
      {
        initialProps: { id: "old-tab" },
      },
    );
    act(() => result.current.start());
    act(() =>
      clients[0].emit({
        ...emptyMediaView(),
        active: true,
        screenSharing: true,
      }),
    );
    await tick();
    rerender({ id: "new-tab" });
    await tick();
    const calls = vi.mocked(api.setMediaState).mock.calls;
    expect(calls[0][1]).toMatchObject({
      connectionId: "old-tab",
      screenSharing: true,
    });
    expect(calls.at(-1)?.[1]).toMatchObject({
      connectionId: "new-tab",
      screenSharing: false,
    });
    act(() =>
      clients[0].emit({
        ...emptyMediaView(),
        active: true,
        screenSharing: true,
      }),
    );
    await tick();
    expect(api.setMediaState).toHaveBeenCalledTimes(2);
  });
});
