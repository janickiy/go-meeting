import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConferenceMediaClient } from "./media";
import type { MediaTrack } from "./media";
import type { RealtimeEvent } from "./types";
import type { ClientRealtimeType } from "./realtime";

const peerId = "00000000-0000-4000-8000-000000000001";
const remotePeer = "00000000-0000-4000-8000-000000000002";
const remoteParticipant = "00000000-0000-4000-8000-000000000003";
const audioId = "00000000-0000-4000-8000-000000000004";
const videoId = "00000000-0000-4000-8000-000000000005";
class FakeTrack {
  readyState = "live";
  onended: (() => void) | null = null;
  stop = vi.fn(() => {
    this.readyState = "ended";
  });
  applyConstraints = vi.fn(async (_constraints: MediaTrackConstraints) => {});
  constructor(
    public kind: string,
    public id: string,
  ) {}
}
class FakeStream {
  constructor(private list: FakeTrack[] = []) {}
  getTracks() {
    return [...this.list];
  }
  getAudioTracks() {
    return this.list.filter((track) => track.kind === "audio");
  }
  getVideoTracks() {
    return this.list.filter((track) => track.kind === "video");
  }
  addTrack(track: FakeTrack) {
    this.list.push(track);
  }
  removeTrack(track: FakeTrack) {
    this.list = this.list.filter((t) => t !== track);
  }
}
class FakePeer {
  static instances: FakePeer[] = [];
  connectionState = "new";
  iceConnectionState = "new";
  signalingState = "stable";
  localDescription: RTCSessionDescriptionInit | null = null;
  onicecandidate:
    ((event: { candidate: { toJSON(): object } | null }) => void) | null = null;
  ontrack:
    ((event: { track: FakeTrack; streams?: { id: string }[] }) => void) | null =
    null;
  onconnectionstatechange: (() => void) | null = null;
  oniceconnectionstatechange: (() => void) | null = null;
  onsignalingstatechange: (() => void) | null = null;
  transceivers: {
    receiver: { track: { kind: string } };
    setCodecPreferences: ReturnType<typeof vi.fn>;
    direction: string;
    mid: string;
    sender: { replaceTrack: ReturnType<typeof vi.fn> };
  }[] = [];
  constructor(public config: RTCConfiguration) {
    FakePeer.instances.push(this);
  }
  addTrack(track: FakeTrack) {
    this.addTransceiver(track.kind, { direction: "sendrecv" });
  }
  addTransceiver(track: string | FakeTrack, options: { direction: string }) {
    const item = {
      receiver: {
        track: { kind: typeof track === "string" ? track : track.kind },
      },
      mid: String(this.transceivers.length),
      sender: { replaceTrack: vi.fn(async (_track: unknown) => {}) },
      setCodecPreferences: vi.fn((_codecs: unknown[]) => {}),
      direction: options.direction,
    };
    this.transceivers.push(item);
    return item;
  }
  getTransceivers() {
    return this.transceivers;
  }
  createOffer = vi.fn(async () => ({
    type: "offer",
    sdp: "test-sdp-not-logged",
  }));
  setLocalDescription = vi.fn(async (sdp: RTCSessionDescriptionInit) => {
    this.localDescription = sdp;
    this.signalingState =
      sdp.type === "rollback" ? "stable" : "have-local-offer";
  });
  setRemoteDescription = vi.fn(async () => {
    this.signalingState = "stable";
  });
  addIceCandidate = vi.fn(async () => {});
  close = vi.fn(() => {
    this.connectionState = "closed";
  });
}
function event(type: string, data: unknown, replyTo?: string): RealtimeEvent {
  return {
    version: 1,
    id: crypto.randomUUID(),
    type,
    conferenceId: "room",
    timestamp: new Date().toISOString(),
    data,
    replyTo,
  };
}
function setup() {
  const localTracks = [
    new FakeTrack("audio", "local-audio"),
    new FakeTrack("video", "local-video"),
  ];
  const capture = vi.fn(
    async (_constraints: MediaStreamConstraints) => new FakeStream(localTracks),
  );
  vi.stubGlobal("navigator", { mediaDevices: { getUserMedia: capture } });
  const send = vi.fn((_type: ClientRealtimeType, _data: unknown) =>
    crypto.randomUUID(),
  );
  const change = vi.fn();
  return {
    client: new ConferenceMediaClient(send, change),
    send,
    change,
    capture,
    localTracks,
  };
}
async function joined(fixture: ReturnType<typeof setup>) {
  await fixture.client.start();
  fixture.client.handle(
    event(
      "media.joined",
      {
        mediaPeerId: peerId,
        workerId: "worker-test",
        maxPeers: 10,
        iceServers: [],
        tracks: [],
      },
      fixture.send.mock.results[0].value,
    ),
  );
  await vi.waitFor(() => expect(FakePeer.instances).toHaveLength(1));
  await vi.waitFor(() =>
    expect(fixture.send).toHaveBeenCalledWith(
      "media.offer",
      expect.objectContaining({ mediaPeerId: peerId }),
    ),
  );
  return FakePeer.instances[0];
}
function offer(fixture: ReturnType<typeof setup>) {
  return [...fixture.send.mock.calls]
    .reverse()
    .find(([type]) => type === "media.offer")?.[1] as { negotiationId: string };
}
beforeEach(() => {
  FakePeer.instances = [];
  vi.stubGlobal("RTCPeerConnection", FakePeer);
  vi.stubGlobal("MediaStream", FakeStream);
  vi.stubGlobal("RTCRtpReceiver", {
    getCapabilities: (kind: string) => ({
      codecs: [
        { mimeType: kind === "audio" ? "audio/opus" : "video/VP8" },
        { mimeType: "video/H264" },
      ],
    }),
  });
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("SFU media client", () => {
  it("requests camera/microphone only on explicit start, bounds capture, and cleans up once", async () => {
    const f = setup();
    expect(f.capture).not.toHaveBeenCalled();
    await joined(f);
    expect(FakePeer.instances[0].config.bundlePolicy).toBe("max-bundle");
    expect(f.capture).toHaveBeenCalledOnce();
    expect(f.capture.mock.calls[0][0]).toMatchObject({
      video: {
        width: { max: 1280 },
        height: { max: 720 },
        frameRate: { max: 30 },
      },
    });
    expect(
      FakePeer.instances[0].transceivers.filter(
        (t) => t.direction === "recvonly",
      ),
    ).toHaveLength(38);
    expect(
      FakePeer.instances[0].transceivers.every(
        (t) => t.setCodecPreferences.mock.calls[0][0].length === 1,
      ),
    ).toBe(true);
    f.client.stop();
    f.client.stop();
    f.localTracks.forEach((t) => expect(t.stop).toHaveBeenCalledOnce());
    expect(
      f.send.mock.calls.filter(([type]) => (type as unknown) === "media.leave"),
    ).toHaveLength(1);
    expect(f.client.snapshot().active).toBe(false);
  });
  it("stops capture that resolves after the user leaves instead of joining a zombie peer", async () => {
    const f = setup();
    let resolve!: (stream: FakeStream) => void;
    f.capture.mockImplementation(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const pending = f.client.start();
    f.client.stop();
    resolve(new FakeStream(f.localTracks));
    await pending;
    expect(f.send).not.toHaveBeenCalled();
    f.localTracks.forEach((t) => expect(t.stop).toHaveBeenCalledOnce());
  });
  it("reports denied permissions safely without opening a worker peer", async () => {
    const f = setup();
    f.capture.mockRejectedValue(
      new DOMException("private device detail", "NotAllowedError"),
    );
    await f.client.start();
    expect(f.client.snapshot().error).toContain(
      "Доступ к камере и микрофону запрещён",
    );
    expect(f.client.snapshot().error).not.toContain("private");
    expect(f.send).not.toHaveBeenCalled();
  });
  it("joins receive-only without requesting devices and can enable a single source later", async () => {
    const f = setup();
    await f.client.start(false);
    expect(f.capture).not.toHaveBeenCalled();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
        },
        f.send.mock.results[0].value,
      ),
    );
    await vi.waitFor(() => expect(FakePeer.instances).toHaveLength(1));
    const pc = FakePeer.instances[0];
    expect(pc.transceivers.every((t) => t.direction === "recvonly")).toBe(true);
    expect(f.client.snapshot().microphoneEnabled).toBe(false);
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    f.capture.mockResolvedValue(
      new FakeStream([new FakeTrack("audio", "mic-only")]),
    );
    await f.client.changeSource("microphone", true);
    expect(f.capture.mock.calls[0][0]).toMatchObject({ video: false });
    expect(f.client.snapshot().microphoneEnabled).toBe(true);
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    f.client.stop();
  });
  it("cancels an in-flight media join with authenticated session cleanup", async () => {
    const f = setup();
    await f.client.start();
    expect(f.send).toHaveBeenCalledWith("media.join", {});
    f.client.stop();
    expect(f.send).toHaveBeenCalledWith("media.leave", {});
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
        },
        f.send.mock.results[0].value,
      ),
    );
    await new Promise((r) => setTimeout(r, 5));
    expect(FakePeer.instances).toHaveLength(0);
  });
  it("applies the worker's lower video target before creating an offer", async () => {
    const f = setup();
    await f.client.start();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
          videoCapture: { maxWidth: 640, maxHeight: 360, maxFrameRate: 15 },
        },
        f.send.mock.results[0].value,
      ),
    );
    await vi.waitFor(() =>
      expect(f.send).toHaveBeenCalledWith(
        "media.offer",
        expect.objectContaining({ mediaPeerId: peerId }),
      ),
    );
    expect(f.localTracks[1].applyConstraints).toHaveBeenCalledWith({
      width: { ideal: 640, max: 640 },
      height: { ideal: 360, max: 360 },
      frameRate: { ideal: 15, max: 15 },
    });
    const offerIndex = f.send.mock.calls.findIndex(
      ([type]) => type === "media.offer",
    );
    expect(
      f.localTracks[1].applyConstraints.mock.invocationCallOrder[0],
    ).toBeLessThan(f.send.mock.invocationCallOrder[offerIndex]);
    f.client.stop();
  });
  it.each([
    { maxWidth: 1920, maxHeight: 720, maxFrameRate: 30 },
    { maxWidth: 1280, maxHeight: 1080, maxFrameRate: 30 },
    { maxWidth: 1280, maxHeight: 720, maxFrameRate: 60 },
    { maxWidth: 640, maxHeight: 360, maxFrameRate: 0 },
  ])("rejects an unsafe worker video target: %j", async (videoCapture) => {
    const f = setup();
    await f.client.start();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
          videoCapture,
        },
        f.send.mock.results[0].value,
      ),
    );
    await vi.waitFor(() => expect(f.client.snapshot().active).toBe(false));
    expect(FakePeer.instances).toHaveLength(0);
    expect(f.localTracks[1].applyConstraints).not.toHaveBeenCalled();
    expect(f.send).toHaveBeenCalledWith("media.leave", {});
  });
  it("does not recreate media after stopping during async camera constraints", async () => {
    const f = setup();
    let resolve!: () => void;
    f.localTracks[1].applyConstraints.mockImplementation(
      () =>
        new Promise<void>((r) => {
          resolve = r;
        }),
    );
    await f.client.start();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
        },
        f.send.mock.results[0].value,
      ),
    );
    await vi.waitFor(() =>
      expect(f.localTracks[1].applyConstraints).toHaveBeenCalledOnce(),
    );
    f.client.stop();
    resolve();
    await new Promise((r) => setTimeout(r, 5));
    expect(FakePeer.instances).toHaveLength(0);
    expect(f.send.mock.calls.some(([type]) => type === "media.offer")).toBe(
      false,
    );
    expect(f.send).toHaveBeenCalledWith("media.leave", { mediaPeerId: peerId });
  });
  it("serializes browser-owned offers, coalesces revisions, and ignores stale answers", async () => {
    const f = setup();
    const pc = await joined(f);
    const first = offer(f);
    for (const revision of [1, 2, 3])
      f.client.handle(
        event("media.renegotiate", { mediaPeerId: peerId, revision }),
      );
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: crypto.randomUUID(),
        sdp: "stale",
      }),
    );
    await new Promise((r) => setTimeout(r, 5));
    expect(pc.createOffer).toHaveBeenCalledOnce();
    expect(pc.setRemoteDescription).not.toHaveBeenCalled();
    expect(f.send.mock.calls.some(([type]) => type === "media.ready")).toBe(
      false,
    );
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: first.negotiationId,
        sdp: "answer",
      }),
    );
    await vi.waitFor(() => expect(pc.createOffer).toHaveBeenCalledTimes(2));
    const readyIndex = f.send.mock.calls.findIndex(
      ([type]) => type === "media.ready",
    );
    const nextOfferIndex = f.send.mock.calls.findIndex(
      ([type, data]) =>
        type === "media.offer" &&
        (data as { negotiationId: string }).negotiationId !==
          first.negotiationId,
    );
    expect(f.send.mock.calls[readyIndex]).toEqual([
      "media.ready",
      { mediaPeerId: peerId, negotiationId: first.negotiationId },
    ]);
    expect(pc.setRemoteDescription.mock.invocationCallOrder[0]).toBeLessThan(
      f.send.mock.invocationCallOrder[readyIndex],
    );
    expect(readyIndex).toBeLessThan(nextOfferIndex);
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: offer(f).negotiationId,
        sdp: "answer-2",
      }),
    );
    await vi.waitFor(() =>
      expect(pc.setRemoteDescription).toHaveBeenCalledTimes(2),
    );
    expect(pc.createOffer).toHaveBeenCalledTimes(2);
    f.client.stop();
  });
  it("buffers ICE until the matching remote SDP and supports end-of-candidates", async () => {
    const f = setup();
    const pc = await joined(f);
    f.client.handle(
      event("media.ice", {
        mediaPeerId: peerId,
        candidate: { candidate: "candidate:test" },
      }),
    );
    f.client.handle(
      event("media.ice", { mediaPeerId: peerId, candidate: null }),
    );
    await new Promise((r) => setTimeout(r, 5));
    expect(pc.addIceCandidate).not.toHaveBeenCalled();
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: offer(f).negotiationId,
        sdp: "answer",
      }),
    );
    await vi.waitFor(() => expect(pc.addIceCandidate).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(f.send).toHaveBeenCalledWith(
        "media.ready",
        expect.objectContaining({ mediaPeerId: peerId }),
      ),
    );
    const readyIndex = f.send.mock.calls.findIndex(
      ([type]) => type === "media.ready",
    );
    expect(pc.addIceCandidate.mock.invocationCallOrder[1]).toBeLessThan(
      f.send.mock.invocationCallOrder[readyIndex],
    );
    pc.onicecandidate!({ candidate: null });
    expect(f.send).toHaveBeenCalledWith("media.ice", {
      mediaPeerId: peerId,
      candidate: null,
    });
    f.client.stop();
  });
  it("stops media when the correlated readiness command is rejected", async () => {
    const f = setup();
    await joined(f);
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: offer(f).negotiationId,
        sdp: "answer",
      }),
    );
    await vi.waitFor(() =>
      expect(f.send).toHaveBeenCalledWith(
        "media.ready",
        expect.objectContaining({ mediaPeerId: peerId }),
      ),
    );
    const readyIndex = f.send.mock.calls.findIndex(
      ([type]) => type === "media.ready",
    );
    f.client.handle(
      event(
        "error",
        { code: "media_negotiation_conflict" },
        f.send.mock.results[readyIndex].value,
      ),
    );
    await vi.waitFor(() => expect(f.client.snapshot().active).toBe(false));
    expect(f.client.snapshot().error).toContain(
      "Медиасервис отклонил подключение",
    );
  });
  it("clears the readiness deadline on its matching acknowledgement", async () => {
    vi.useFakeTimers();
    const f = setup();
    const pc = await joined(f);
    f.client.handle(
      event("media.answer", {
        mediaPeerId: peerId,
        negotiationId: offer(f).negotiationId,
        sdp: "answer",
      }),
    );
    await vi.advanceTimersByTimeAsync(1);
    const readyIndex = f.send.mock.calls.findIndex(
      ([type]) => type === "media.ready",
    );
    expect(readyIndex).toBeGreaterThan(-1);
    f.client.handle(
      event(
        "ack",
        { type: "media.ready" },
        f.send.mock.results[readyIndex].value,
      ),
    );
    await vi.advanceTimersByTimeAsync(1);
    pc.connectionState = "connected";
    pc.onconnectionstatechange!();
    await vi.advanceTimersByTimeAsync(16000);
    expect(f.client.snapshot().active).toBe(true);
    expect(f.client.snapshot().error).toBeNull();
    f.client.stop();
  });
  it("uses authoritative track metadata for remote streams and removes stopped publishers", async () => {
    const f = setup();
    const pc = await joined(f);
    const catalog: MediaTrack[] = [
      {
        id: audioId,
        streamId: remotePeer,
        mediaPeerId: remotePeer,
        participantId: remoteParticipant,
        kind: "audio",
        source: "microphone",
      },
      {
        id: videoId,
        streamId: remotePeer,
        mediaPeerId: remotePeer,
        participantId: remoteParticipant,
        kind: "video",
        source: "camera",
      },
    ];
    // Chromium's preallocated RTCRtpReceiver can keep its generated ID instead
    // of the worker's SDP track ID, while SDP stream ID remains authoritative.
    pc.ontrack!({
      track: new FakeTrack("audio", "browser-audio-id"),
      streams: [{ id: remotePeer }],
    });
    pc.ontrack!({
      track: new FakeTrack("video", "browser-video-id"),
      streams: [{ id: remotePeer }],
    });
    expect(f.client.snapshot().remoteStreams).toHaveLength(0);
    f.client.handle(
      event("media.tracks", {
        mediaPeerId: peerId,
        revision: 2,
        tracks: catalog,
      }),
    );
    await vi.waitFor(() =>
      expect(f.client.snapshot().remoteStreams).toHaveLength(1),
    );
    expect(f.client.snapshot().remoteStreams[0].kinds).toEqual([
      "audio",
      "video",
    ]);
    f.client.handle(
      event("media.tracks", { mediaPeerId: peerId, revision: 3, tracks: [] }),
    );
    await vi.waitFor(() =>
      expect(f.client.snapshot().remoteStreams).toHaveLength(0),
    );
    f.client.handle(
      event("media.tracks", {
        mediaPeerId: peerId,
        revision: 2,
        tracks: catalog,
      }),
    );
    await new Promise((r) => setTimeout(r, 5));
    expect(f.client.snapshot().remoteStreams).toHaveLength(0);
    f.client.stop();
  });
  it("ignores stale joined responses and unrelated server errors", async () => {
    const f = setup();
    await f.client.start();
    f.client.handle(
      event(
        "media.joined",
        {
          mediaPeerId: peerId,
          workerId: "worker-test",
          maxPeers: 10,
          iceServers: [],
          tracks: [],
        },
        "stale-request",
      ),
    );
    f.client.handle(
      event("error", { code: "private_code" }, "not-this-media-request"),
    );
    await new Promise((r) => setTimeout(r, 5));
    expect(FakePeer.instances).toHaveLength(0);
    expect(f.client.snapshot().error).toBeNull();
    f.client.stop();
  });
  it("reports stopped local tracks with the worker-assigned track identity", async () => {
    const f = setup();
    await joined(f);
    f.client.handle(
      event("media.published", {
        mediaPeerId: peerId,
        track: {
          id: videoId,
          streamId: peerId,
          mediaPeerId: peerId,
          participantId: remoteParticipant,
          kind: "video",
          source: "camera",
        },
      }),
    );
    await new Promise((r) => setTimeout(r, 5));
    f.localTracks[1].readyState = "ended";
    f.localTracks[1].onended!();
    expect(f.send).toHaveBeenCalledWith("media.unpublish", {
      mediaPeerId: peerId,
      trackId: videoId,
    });
    f.client.stop();
  });
  it("releases devices and requires an explicit restart after ICE failure", async () => {
    const f = setup();
    const pc = await joined(f);
    pc.connectionState = "failed";
    pc.onconnectionstatechange!();
    expect(f.client.snapshot().active).toBe(false);
    expect(f.client.snapshot().localStream).toBeNull();
    expect(f.client.snapshot().error).toContain("Медиасвязь прервана");
    f.localTracks.forEach((track) => expect(track.stop).toHaveBeenCalledOnce());
    await f.client.start();
    expect(f.capture).toHaveBeenCalledOnce();
  });
  it("toggles and replaces capture without creating a new peer or leaking tracks", async () => {
    const f = setup();
    const pc = await joined(f);
    await f.client.changeSource("camera", false);
    expect(f.localTracks[1].stop).toHaveBeenCalledOnce();
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    const replacement = new FakeTrack("video", "replacement-camera");
    f.capture.mockResolvedValueOnce(new FakeStream([replacement]));
    await f.client.changeSource("camera", true, "device-b");
    expect(f.capture).toHaveBeenLastCalledWith(
      expect.objectContaining({
        audio: false,
        video: expect.objectContaining({ deviceId: { exact: "device-b" } }),
      }),
    );
    expect(pc.transceivers[1].sender.replaceTrack).toHaveBeenLastCalledWith(
      replacement,
    );
    expect(f.client.snapshot().cameraEnabled).toBe(true);
    expect(FakePeer.instances).toHaveLength(1);
    f.client.stop();
    expect(replacement.stop).toHaveBeenCalledOnce();
  });
  it("blocks capture on moderator policy without automatically enabling hardware on unblock", async () => {
    const f = setup();
    await joined(f);
    f.client.setPolicy({ microphoneBlocked: true, cameraBlocked: true });
    await vi.waitFor(() =>
      expect(f.client.snapshot().cameraEnabled).toBe(false),
    );
    expect(f.client.snapshot().microphoneEnabled).toBe(false);
    await f.client.changeSource("camera", true);
    expect(f.capture).toHaveBeenCalledOnce();
    f.client.setPolicy({});
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    f.client.stop();
  });
  it("keeps camera while screen shares and handles native screen end without requiring audio", async () => {
    const f = setup();
    await joined(f);
    const screen = new FakeTrack("video", "screen-video");
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(async () => new FakeStream([screen])),
    });
    await f.client.startScreen();
    expect(f.client.snapshot().screenSharing).toBe(true);
    expect(f.client.snapshot().cameraEnabled).toBe(true);
    expect(f.client.snapshot().localScreen?.getTracks()).toEqual([screen]);
    screen.readyState = "ended";
    screen.onended!();
    await vi.waitFor(() =>
      expect(f.client.snapshot().screenSharing).toBe(false),
    );
    expect(f.client.snapshot().localScreen).toBeNull();
    expect(f.localTracks[1].stop).not.toHaveBeenCalled();
    f.client.stop();
  });
  it("releases a screen capture that completes after leave", async () => {
    const f = setup();
    await joined(f);
    const screen = new FakeTrack("video", "late-screen");
    let resolve!: (stream: FakeStream) => void;
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(
        () =>
          new Promise<FakeStream>((r) => {
            resolve = r;
          }),
      ),
    });
    const pending = f.client.startScreen();
    f.client.stop();
    resolve(new FakeStream([screen]));
    await pending;
    expect(screen.stop).toHaveBeenCalled();
    expect(f.client.snapshot().localScreen).toBeNull();
  });
  it("rejects stale policy and stops screen audio on forced mute", async () => {
    const f = setup();
    await joined(f);
    const video = new FakeTrack("video", "screen-v");
    const audio = new FakeTrack("audio", "screen-a");
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(async () => new FakeStream([video, audio])),
    });
    await f.client.startScreen();
    f.client.setPolicy({ version: 3, microphoneBlocked: true });
    f.client.setPolicy({ version: 2, microphoneBlocked: false });
    await vi.waitFor(() => expect(audio.stop).toHaveBeenCalled());
    expect(video.stop).not.toHaveBeenCalled();
    await f.client.changeSource("microphone", true);
    expect(f.capture).toHaveBeenCalledOnce();
    f.client.stop();
  });
  it("cleans partially installed screen capture if a second sender rejects", async () => {
    const f = setup();
    const pc = await joined(f);
    const video = new FakeTrack("video", "screen-v");
    const audio = new FakeTrack("audio", "screen-a");
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(async () => new FakeStream([video, audio])),
    });
    pc.transceivers[3].sender.replaceTrack.mockRejectedValueOnce(
      new Error("replace failed"),
    );
    await f.client.startScreen();
    expect(video.stop).toHaveBeenCalled();
    expect(audio.stop).toHaveBeenCalled();
    expect(f.client.snapshot().localScreen).toBeNull();
    expect(f.client.snapshot().screenSharing).toBe(false);
    expect(f.client.snapshot().cameraEnabled).toBe(true);
    f.client.stop();
  });
  it("uses the newest receiver for a replaced publication and ignores old track end", async () => {
    const f = setup();
    const pc = await joined(f);
    f.client.handle(
      event("media.tracks", {
        mediaPeerId: peerId,
        revision: 1,
        tracks: [
          {
            id: videoId,
            streamId: remotePeer,
            mediaPeerId: remotePeer,
            participantId: remoteParticipant,
            kind: "video",
            source: "camera",
          },
        ],
      }),
    );
    const old = new FakeTrack("video", "browser-track");
    pc.ontrack!({ track: old, streams: [{ id: remotePeer }] });
    await vi.waitFor(() =>
      expect(f.client.snapshot().remoteStreams).toHaveLength(1),
    );
    const fresh = new FakeTrack("video", "browser-track");
    pc.ontrack!({ track: fresh, streams: [{ id: remotePeer }] });
    old.onended!();
    expect(
      f.client.snapshot().remoteStreams[0].stream.getVideoTracks(),
    ).toEqual([fresh]);
    f.client.stop();
  });
  it("keeps screen video if optional screen audio ends", async () => {
    const f = setup();
    await joined(f);
    const video = new FakeTrack("video", "screen-v");
    const audio = new FakeTrack("audio", "screen-a");
    Object.assign(navigator.mediaDevices, {
      getDisplayMedia: vi.fn(async () => new FakeStream([video, audio])),
    });
    await f.client.startScreen();
    audio.readyState = "ended";
    audio.onended!();
    await vi.waitFor(() =>
      expect(f.client.snapshot().localScreen?.getAudioTracks()).toHaveLength(0),
    );
    expect(f.client.snapshot().screenSharing).toBe(true);
    expect(video.stop).not.toHaveBeenCalled();
    f.client.stop();
  });
  it("preserves the peer as receive-only when a moderator policy races an offer", async () => {
    const f = setup();
    const pc = await joined(f);
    const index = f.send.mock.calls.findIndex(
      ([type]) => type === "media.offer",
    );
    f.client.handle(
      event(
        "error",
        { code: "media_policy_blocked" },
        f.send.mock.results[index].value,
      ),
    );
    await vi.waitFor(() => expect(pc.createOffer).toHaveBeenCalledTimes(2));
    expect(pc.close).not.toHaveBeenCalled();
    expect(f.client.snapshot().active).toBe(true);
    expect(f.client.snapshot().cameraEnabled).toBe(false);
    expect(f.client.snapshot().microphoneEnabled).toBe(false);
    expect(f.send).toHaveBeenLastCalledWith(
      "media.offer",
      expect.objectContaining({ publications: [] }),
    );
    f.client.stop();
  });
});
