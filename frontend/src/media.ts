import type { ClientRealtimeType } from "./realtime";
import type { RealtimeEvent } from "./types";

export interface MediaTrack {
  id: string;
  streamId: string;
  mediaPeerId: string;
  participantId: string;
  kind: "audio" | "video";
  source: "microphone" | "camera";
}
export interface RemoteMedia {
  mediaPeerId: string;
  participantId: string;
  stream: MediaStream;
  kinds: string[];
}
export interface MediaView {
  active: boolean;
  status: string;
  localStream: MediaStream | null;
  remoteStreams: RemoteMedia[];
  mediaPeerId: string;
  workerId: string;
  connectionState: string;
  iceState: string;
  negotiationState: string;
  error: string | null;
}
type Send = (type: ClientRealtimeType, data: unknown) => string;
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const initial = (): MediaView => ({
  active: false,
  status: "Камера и микрофон выключены",
  localStream: null,
  remoteStreams: [],
  mediaPeerId: "",
  workerId: "",
  connectionState: "new",
  iceState: "new",
  negotiationState: "stable",
  error: null,
});
function record(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}
function tracks(value: unknown): MediaTrack[] | null {
  if (!Array.isArray(value) || value.length > 128) return null;
  const result: MediaTrack[] = [];
  const ids = new Set<string>();
  for (const item of value) {
    const t = record(item);
    if (
      !t ||
      typeof t.id !== "string" ||
      !uuid.test(t.id) ||
      ids.has(t.id) ||
      typeof t.mediaPeerId !== "string" ||
      !uuid.test(t.mediaPeerId) ||
      t.streamId !== t.mediaPeerId ||
      typeof t.participantId !== "string" ||
      !uuid.test(t.participantId) ||
      !(
        (t.kind === "audio" && t.source === "microphone") ||
        (t.kind === "video" && t.source === "camera")
      )
    )
      return null;
    ids.add(t.id);
    result.push(t as unknown as MediaTrack);
  }
  return result;
}
function videoCapture(value: unknown) {
  if (value === undefined)
    return { maxWidth: 1280, maxHeight: 720, maxFrameRate: 30 };
  const target = record(value);
  if (
    !target ||
    !Number.isInteger(target.maxWidth) ||
    Number(target.maxWidth) < 1 ||
    Number(target.maxWidth) > 1280 ||
    !Number.isInteger(target.maxHeight) ||
    Number(target.maxHeight) < 1 ||
    Number(target.maxHeight) > 720 ||
    !Number.isInteger(target.maxFrameRate) ||
    Number(target.maxFrameRate) < 1 ||
    Number(target.maxFrameRate) > 30
  )
    throw new Error("invalid_video_capture");
  return {
    maxWidth: Number(target.maxWidth),
    maxHeight: Number(target.maxHeight),
    maxFrameRate: Number(target.maxFrameRate),
  };
}

// One controller = one Stage 2 connection and one server-side MediaPeer.
// No permission request or automatic capture occurs in the constructor.
export class ConferenceMediaClient {
  private view = initial();
  private disposed = false;
  private started = false;
  private pc: RTCPeerConnection | null = null;
  private local: MediaStream | null = null;
  private joinedRequest = "";
  private negotiation: {
    id: string;
    request: string;
    revision: number;
  } | null = null;
  private wantedRevision = 0;
  private answeredRevision = -1;
  private catalog: MediaTrack[] = [];
  private ownPublished = new Map<string, MediaTrack>();
  private catalogRevision = -1;
  // A preallocated browser receiver may retain its own track ID. SDP msid's
  // stream ID + kind binds it to the worker's canonical publication metadata.
  private received = new Map<
    string,
    { track: MediaStreamTrack; streamIds: string[] }
  >();
  private streams = new Map<string, MediaStream>();
  private remoteICE: (RTCIceCandidateInit | null)[] = [];
  private localICE: (RTCIceCandidateInit | null)[] = [];
  private remoteDescriptionReady = false;
  private hasSentOffer = false;
  private readyRequests = new Map<string, ReturnType<typeof setTimeout>>();
  private queue = Promise.resolve();
  private timer?: ReturnType<typeof setTimeout>;
  private disconnectedTimer?: ReturnType<typeof setTimeout>;

  constructor(
    private send: Send,
    private onChange: (view: MediaView) => void,
  ) {}

  snapshot() {
    return this.view;
  }
  private update(patch: Partial<MediaView>) {
    this.view = { ...this.view, ...patch };
    this.onChange(this.view);
  }
  private deadline(message: string, ms = 15000) {
    clearTimeout(this.timer);
    this.timer = setTimeout(() => this.fail(message), ms);
  }
  private fail(message: string) {
    if (this.disposed) return;
    this.stop();
    this.update({ status: "Медиасвязь остановлена", error: message });
  }
  stop() {
    if (this.disposed) return;
    this.disposed = true;
    clearTimeout(this.timer);
    clearTimeout(this.disconnectedTimer);
    for (const timer of this.readyRequests.values()) clearTimeout(timer);
    this.readyRequests.clear();
    if (this.view.mediaPeerId || this.joinedRequest) {
      try {
        this.send(
          "media.leave",
          this.view.mediaPeerId ? { mediaPeerId: this.view.mediaPeerId } : {},
        );
      } catch {
        /* WS cleanup also closes the worker peer. */
      }
    }
    this.pc?.close();
    this.pc = null;
    for (const track of this.local?.getTracks() || []) track.stop();
    this.local = null;
    this.received.clear();
    this.ownPublished.clear();
    this.streams.clear();
    this.remoteICE = [];
    this.localICE = [];
    this.update({
      active: false,
      localStream: null,
      remoteStreams: [],
      status: "Камера и микрофон выключены",
      connectionState: "closed",
      iceState: "closed",
    });
  }

  async start() {
    if (this.started || this.disposed) return;
    this.started = true;
    this.update({
      active: true,
      status: "Запрашиваем доступ к камере и микрофону…",
      error: null,
    });
    try {
      if (!navigator.mediaDevices?.getUserMedia)
        throw new Error("secure_context");
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
        video: {
          width: { ideal: 1280, max: 1280 },
          height: { ideal: 720, max: 720 },
          frameRate: { ideal: 30, max: 30 },
        },
      });
      if (this.disposed) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      this.local = stream;
      this.update({
        localStream: stream,
        status: "Подключаемся к media-worker…",
      });
      this.deadline(
        "Media-worker не ответил. Проверьте сервис и повторите подключение.",
      );
      this.joinedRequest = this.send("media.join", {});
    } catch (error) {
      const info = record(error);
      const name = typeof info?.name === "string" ? info.name : "";
      this.fail(
        error instanceof Error && error.message === "secure_context"
          ? "Камера доступна только на HTTPS или localhost. Откройте защищённую страницу."
          : name === "NotAllowedError" || name === "SecurityError"
            ? "Доступ к камере и микрофону запрещён. Разрешите их для этого сайта в браузере и повторите."
            : name === "NotFoundError"
              ? "Камера или микрофон не найдены. Подключите устройства и повторите."
              : name === "NotReadableError"
                ? "Не удалось открыть камеру или микрофон. Возможно, устройство занято другим приложением."
                : "Не удалось включить медиасвязь. Проверьте устройства и media-worker, затем повторите.",
      );
    }
  }

  handle(event: RealtimeEvent) {
    if (
      this.disposed ||
      (!event.type.startsWith("media.") &&
        event.type !== "error" &&
        event.type !== "ack")
    )
      return;
    this.queue = this.queue
      .then(() => this.process(event))
      .catch(() => {
        this.fail(
          "Ошибка обмена медиа. Переподключите камеру и микрофон; может требоваться TURN.",
        );
      });
  }
  private async process(event: RealtimeEvent) {
    if (this.disposed) return;
    const data = record(event.data);
    if (!data) return;
    if (event.type === "ack") {
      if (event.replyTo && data.type === "media.ready") {
        clearTimeout(this.readyRequests.get(event.replyTo));
        this.readyRequests.delete(event.replyTo);
      }
      return;
    }
    if (event.type === "error") {
      if (
        event.replyTo === this.joinedRequest ||
        event.replyTo === this.negotiation?.request ||
        (event.replyTo && this.readyRequests.has(event.replyTo))
      )
        this.fail(
          "Медиасервис отклонил подключение. Проверьте доступ к конференции и повторите.",
        );
      return;
    }
    if (event.type === "media.joined") {
      if (!this.started || this.pc || event.replyTo !== this.joinedRequest)
        return;
      const list = tracks(data.tracks);
      if (
        typeof data.mediaPeerId !== "string" ||
        !uuid.test(data.mediaPeerId) ||
        typeof data.workerId !== "string" ||
        data.workerId.length > 128 ||
        !Number.isInteger(data.maxPeers) ||
        Number(data.maxPeers) < 1 ||
        Number(data.maxPeers) > 32 ||
        !Array.isArray(data.iceServers) ||
        data.iceServers.length > 16 ||
        !list
      )
        throw new Error("invalid_join");
      const capture = videoCapture(data.videoCapture);
      this.update({
        mediaPeerId: data.mediaPeerId,
        workerId: data.workerId,
        status: "Согласуем медиасвязь…",
      });
      this.catalog = list;
      const video = this.local
        ?.getTracks()
        .find((track) => track.kind === "video");
      if (!video) throw new Error("video_track_unavailable");
      await video.applyConstraints({
        width: { ideal: capture.maxWidth, max: capture.maxWidth },
        height: { ideal: capture.maxHeight, max: capture.maxHeight },
        frameRate: { ideal: capture.maxFrameRate, max: capture.maxFrameRate },
      });
      // Applying browser constraints is async. A stop/reconnect while pending
      // must not resurrect a PeerConnection or publish the captured stream.
      if (this.disposed) return;
      this.createPeer(data.iceServers as RTCIceServer[], Number(data.maxPeers));
      await this.offer();
      return;
    }
    if (!this.pc || data.mediaPeerId !== this.view.mediaPeerId) return;
    if (event.type === "media.answer") {
      if (
        data.negotiationId !== this.negotiation?.id ||
        typeof data.sdp !== "string" ||
        data.sdp.length > 262144
      )
        return;
      const offer = this.negotiation!;
      await this.pc.setRemoteDescription({ type: "answer", sdp: data.sdp });
      if (this.disposed) return;
      this.remoteDescriptionReady = true;
      for (const candidate of this.remoteICE.splice(0))
        await this.pc.addIceCandidate(candidate || undefined);
      if (this.disposed) return;
      // RTP for a newly negotiated sender must not precede the browser applying
      // its answer. WS command serialization places ready before the next offer.
      if (this.readyRequests.size >= 32) throw new Error("ready_limit");
      const readyRequest = this.send("media.ready", {
        mediaPeerId: this.view.mediaPeerId,
        negotiationId: offer.id,
      });
      this.readyRequests.set(
        readyRequest,
        setTimeout(() => {
          this.fail(
            "Media-worker не подтвердил готовность медиасвязи. Подключите камеру и микрофон снова.",
          );
        }, 15000),
      );
      this.answeredRevision = offer.revision;
      this.negotiation = null;
      this.update({
        negotiationState: this.pc.signalingState,
        status:
          this.pc.connectionState === "connected"
            ? "Медиасвязь подключена"
            : "Устанавливаем медиасвязь…",
      });
      if (this.pc.connectionState !== "connected")
        this.deadline(
          "ICE-соединение не установилось. Проверьте доступность медиа-портов и STUN/TURN.",
          25000,
        );
      else clearTimeout(this.timer);
      if (this.wantedRevision > this.answeredRevision) await this.offer();
    } else if (event.type === "media.renegotiate") {
      if (Number.isInteger(data.revision) && Number(data.revision) >= 0) {
        this.wantedRevision = Math.max(
          this.wantedRevision,
          Number(data.revision),
        );
        if (!this.negotiation && this.wantedRevision > this.answeredRevision)
          await this.offer();
      }
    } else if (event.type === "media.ice") {
      if (data.candidate !== null && !record(data.candidate))
        throw new Error("invalid_ice");
      const candidate = data.candidate as RTCIceCandidateInit | null;
      if (this.remoteDescriptionReady)
        await this.pc.addIceCandidate(candidate || undefined);
      else if (this.remoteICE.length < 128) this.remoteICE.push(candidate);
      else throw new Error("ice_limit");
    } else if (event.type === "media.tracks") {
      const list = tracks(data.tracks);
      if (
        !list ||
        !Number.isInteger(data.revision) ||
        Number(data.revision) < 0
      )
        throw new Error("invalid_tracks");
      if (Number(data.revision) < this.catalogRevision) return;
      this.catalogRevision = Number(data.revision);
      const ids = new Set(list.map((entry) => entry.id));
      for (const previous of this.catalog) {
        if (ids.has(previous.id)) continue;
        for (const [key, received] of this.received) {
          if (
            (key === previous.id ||
              (received.track.kind === previous.kind &&
                received.streamIds.includes(previous.streamId))) &&
            !list.some(
              (entry) =>
                entry.kind === received.track.kind &&
                received.streamIds.includes(entry.streamId),
            )
          )
            this.received.delete(key);
        }
      }
      this.catalog = list;
      this.syncRemoteStreams();
    } else if (event.type === "media.state") {
      if (data.state === "failed" || data.state === "closed")
        this.fail(
          "Media-worker отключил медиасвязь. Повторите подключение камеры и микрофона.",
        );
    } else if (event.type === "media.left") {
      this.stop();
    } else if (event.type === "media.published") {
      const list = tracks([data.track]);
      if (!list || list[0].mediaPeerId !== this.view.mediaPeerId)
        throw new Error("invalid_published_track");
      this.ownPublished.set(list[0].id, list[0]);
      const localTrack = this.local
        ?.getTracks()
        .find((track) => track.kind === list[0].kind);
      if (localTrack?.readyState === "ended")
        this.send("media.unpublish", {
          mediaPeerId: this.view.mediaPeerId,
          trackId: list[0].id,
        });
    } else if (
      event.type === "media.unpublished" &&
      typeof data.trackId === "string"
    ) {
      this.ownPublished.delete(data.trackId);
    }
  }

  private createPeer(iceServers: RTCIceServer[], maxPeers: number) {
    const pc = new RTCPeerConnection({ iceServers });
    this.pc = pc;
    for (const track of this.local!.getTracks()) {
      pc.addTrack(track, this.local!);
      track.onended = () => {
        if (this.disposed) return;
        const published = [...this.ownPublished.values()].find(
          (item) => item.kind === track.kind,
        );
        if (published) {
          try {
            this.send("media.unpublish", {
              mediaPeerId: this.view.mediaPeerId,
              trackId: published.id,
            });
          } catch {
            this.fail("Realtime-связь потеряна. Подключите медиасвязь снова.");
          }
        }
      };
    }
    // The browser owns every offer. Stable receive slots prevent worker-driven
    // glare and make late publishers subscribable without server-side offers.
    for (let i = 1; i < maxPeers; i++) {
      pc.addTransceiver("audio", { direction: "recvonly" });
      pc.addTransceiver("video", { direction: "recvonly" });
    }
    for (const transceiver of pc.getTransceivers()) {
      const kind = transceiver.receiver.track.kind;
      const codecs = RTCRtpReceiver.getCapabilities?.(kind)?.codecs.filter(
        (codec) =>
          codec.mimeType.toLowerCase() ===
          (kind === "audio" ? "audio/opus" : "video/vp8"),
      );
      if (
        codecs?.length &&
        typeof transceiver.setCodecPreferences === "function"
      )
        transceiver.setCodecPreferences(codecs);
    }
    pc.onicecandidate = (event) => {
      if (this.disposed) return;
      const candidate = event.candidate?.toJSON() || null;
      if (!this.hasSentOffer) {
        if (this.localICE.length < 128) this.localICE.push(candidate);
        else
          this.fail(
            "Слишком много ICE-кандидатов. Проверьте сетевую конфигурацию.",
          );
      } else this.sendICE(candidate);
    };
    pc.ontrack = (event) => {
      if (this.disposed) return;
      if (!this.received.has(event.track.id) && this.received.size >= 128) {
        this.fail("Превышен лимит удалённых медиа-треков.");
        return;
      }
      this.received.set(event.track.id, {
        track: event.track,
        streamIds: event.streams?.map((stream) => stream.id) || [],
      });
      event.track.onended = () => {
        this.received.delete(event.track.id);
        this.syncRemoteStreams();
      };
      this.syncRemoteStreams();
    };
    pc.onconnectionstatechange = () => {
      if (this.disposed) return;
      const state = pc.connectionState;
      this.update({ connectionState: state });
      if (state === "connected") {
        if (!this.negotiation) clearTimeout(this.timer);
        clearTimeout(this.disconnectedTimer);
        this.update({ status: "Медиасвязь подключена" });
      } else if (state === "failed" || state === "closed") {
        this.fail(
          "Медиасвязь прервана. Подключите камеру и микрофон снова; может требоваться TURN.",
        );
      } else if (state === "disconnected") {
        this.update({ status: "Медиасвязь потеряна…" });
        clearTimeout(this.disconnectedTimer);
        this.disconnectedTimer = setTimeout(
          () =>
            this.fail(
              "Медиасвязь не восстановилась. Подключите камеру и микрофон снова.",
            ),
          8000,
        );
      }
    };
    pc.oniceconnectionstatechange = () => {
      if (!this.disposed) this.update({ iceState: pc.iceConnectionState });
    };
    pc.onsignalingstatechange = () => {
      if (!this.disposed) this.update({ negotiationState: pc.signalingState });
    };
  }
  private sendICE(candidate: RTCIceCandidateInit | null) {
    try {
      this.send("media.ice", { mediaPeerId: this.view.mediaPeerId, candidate });
    } catch {
      this.fail("Realtime-связь потеряна. Подключите медиасвязь снова.");
    }
  }
  private async offer() {
    const pc = this.pc;
    if (
      this.disposed ||
      !pc ||
      this.negotiation ||
      pc.signalingState !== "stable"
    )
      return;
    const id = crypto.randomUUID();
    const negotiation = { id, request: "", revision: this.wantedRevision };
    this.negotiation = negotiation;
    await pc.setLocalDescription(await pc.createOffer());
    if (this.disposed || this.pc !== pc) return;
    negotiation.request = this.send("media.offer", {
      mediaPeerId: this.view.mediaPeerId,
      negotiationId: id,
      sdp: pc.localDescription!.sdp,
    });
    this.hasSentOffer = true;
    for (const candidate of this.localICE.splice(0)) this.sendICE(candidate);
    this.deadline(
      "Ответ на медиа offer не получен. Повторите подключение камеры и микрофона.",
    );
  }
  private syncRemoteStreams() {
    if (this.disposed) return;
    const grouped = new Map<
      string,
      { participantId: string; tracks: MediaStreamTrack[] }
    >();
    for (const entry of this.catalog) {
      if (entry.mediaPeerId === this.view.mediaPeerId) continue;
      const track =
        this.received.get(entry.id)?.track ||
        [...this.received.values()].find(
          (received) =>
            received.track.kind === entry.kind &&
            received.streamIds.includes(entry.streamId),
        )?.track;
      if (!track || track.readyState === "ended") continue;
      const group = grouped.get(entry.mediaPeerId) || {
        participantId: entry.participantId,
        tracks: [],
      };
      group.tracks.push(track);
      grouped.set(entry.mediaPeerId, group);
    }
    for (const [id, stream] of this.streams) {
      const active = grouped.get(id)?.tracks || [];
      stream.getTracks().forEach((track) => {
        if (!active.includes(track)) stream.removeTrack(track);
      });
      if (!active.length) this.streams.delete(id);
    }
    const remoteStreams: RemoteMedia[] = [];
    for (const [id, group] of grouped) {
      let stream = this.streams.get(id);
      if (!stream) {
        stream = new MediaStream();
        this.streams.set(id, stream);
      }
      for (const track of group.tracks)
        if (!stream.getTracks().includes(track)) stream.addTrack(track);
      remoteStreams.push({
        mediaPeerId: id,
        participantId: group.participantId,
        stream,
        kinds: group.tracks.map((track) => track.kind),
      });
    }
    this.update({ remoteStreams });
  }
}

export function emptyMediaView() {
  return initial();
}
