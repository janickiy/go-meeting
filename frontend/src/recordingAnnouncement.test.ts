import { afterEach, beforeEach, expect, it, vi } from "vitest";

let element: {
  play: ReturnType<typeof vi.fn>;
  pause: ReturnType<typeof vi.fn>;
  volume: number;
  currentTime: number;
  preload: string;
};
beforeEach(() => {
  vi.resetModules();
  element = {
    play: vi.fn().mockResolvedValue(undefined),
    pause: vi.fn(),
    volume: 1,
    currentTime: 0,
    preload: "",
  };
  vi.stubGlobal(
    "Audio",
    vi.fn(function () {
      return element;
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

it("uses the bundled English phrase without speech synthesis or external services", async () => {
  const { playRecordingAnnouncement } = await import("./recordingAnnouncement");
  const cancel = playRecordingAnnouncement();
  await Promise.resolve();
  expect(Audio).toHaveBeenCalledWith(
    expect.stringContaining("recording-started-en.wav"),
  );
  expect(element.play).toHaveBeenCalledOnce();
  expect(element.volume).toBe(1);
  cancel();
  expect(element.pause).toHaveBeenCalledOnce();
});

it("retries blocked autoplay on a gesture exactly once", async () => {
  element.play.mockRejectedValueOnce(
    new DOMException("blocked", "NotAllowedError"),
  );
  const { playRecordingAnnouncement } = await import("./recordingAnnouncement");
  const cancel = playRecordingAnnouncement();
  await Promise.resolve();
  document.dispatchEvent(new Event("pointerdown"));
  await Promise.resolve();
  document.dispatchEvent(new Event("keydown"));
  expect(element.play).toHaveBeenCalledTimes(2);
  cancel();
});

it("does not play a deferred message after the recording stops or the user leaves", async () => {
  element.play.mockRejectedValueOnce(
    new DOMException("blocked", "NotAllowedError"),
  );
  const { playRecordingAnnouncement } = await import("./recordingAnnouncement");
  const cancel = playRecordingAnnouncement();
  await Promise.resolve();
  cancel();
  document.dispatchEvent(new Event("pointerdown"));
  expect(element.play).toHaveBeenCalledOnce();
});

it("silently primes the same element on entry and restores full volume", async () => {
  const { installRecordingAudioUnlock, playRecordingAnnouncement } =
    await import("./recordingAnnouncement");
  const dispose = installRecordingAudioUnlock();
  document.dispatchEvent(new Event("pointerdown"));
  expect(element.volume).toBe(0);
  await vi.waitFor(() => expect(element.volume).toBe(1));
  const cancel = playRecordingAnnouncement();
  expect(Audio).toHaveBeenCalledOnce();
  expect(element.play).toHaveBeenCalledTimes(2);
  cancel();
  dispose();
});

it("does not interrupt an announcement when an earlier unlock resolves", async () => {
  let resolve!: () => void;
  element.play.mockImplementationOnce(
    () =>
      new Promise<void>((done) => {
        resolve = done;
      }),
  );
  const { installRecordingAudioUnlock, playRecordingAnnouncement } =
    await import("./recordingAnnouncement");
  const dispose = installRecordingAudioUnlock();
  document.dispatchEvent(new Event("pointerdown"));
  const cancel = playRecordingAnnouncement();
  resolve();
  await Promise.resolve();
  await Promise.resolve();
  expect(element.pause).not.toHaveBeenCalled();
  expect(element.volume).toBe(1);
  cancel();
  dispose();
});
