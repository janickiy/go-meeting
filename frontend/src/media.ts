import type { ClientRealtimeType } from "./realtime";
import type { RealtimeEvent } from "./types";

export type MediaSource =
  "microphone" | "camera" | "video/screen" | "audio/screen";
export interface MediaPolicy {
  version?: number;
  microphoneBlocked?: boolean;
  cameraBlocked?: boolean;
  screenBlocked?: boolean;
}
export interface MediaTrack {
  id: string;
  streamId: string;
  mediaPeerId: string;
  participantId: string;
  kind: "audio" | "video";
  source: MediaSource;
}
export interface RemoteMedia {
  id: string;
  mediaPeerId: string;
  participantId: string;
  stream: MediaStream;
  kinds: string[];
  screen: boolean;
}
export interface MediaView {
  active: boolean;
  status: string;
  localStream: MediaStream | null;
  localScreen: MediaStream | null;
  microphoneEnabled: boolean;
  cameraEnabled: boolean;
  screenSharing: boolean;
  controlBusy: boolean;
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
  localScreen: null,
  microphoneEnabled: false,
  cameraEnabled: false,
  screenSharing: false,
  controlBusy: false,
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
      t.streamId !==
        (String(t.source).endsWith("/screen")
          ? `${t.mediaPeerId}-screen`
          : t.mediaPeerId) ||
      typeof t.participantId !== "string" ||
      !uuid.test(t.participantId) ||
      !(
        (t.kind === "audio" &&
          (t.source === "microphone" || t.source === "audio/screen")) ||
        (t.kind === "video" &&
          (t.source === "camera" || t.source === "video/screen"))
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
  private screen: MediaStream | null = null;
  private policy: MediaPolicy = {};
  private capture = videoCapture(undefined);
  private sources = new Map<
    MediaSource,
    {
      sender: RTCRtpSender;
      transceiver: RTCRtpTransceiver;
      track: MediaStreamTrack | null;
    }
  >();
  private localRevision = 0;
  private preferredInputs = new Map<MediaSource, string>();
  private answeredLocalRevision = -1;
  private joinedRequest = "";
  private negotiation: {
    id: string;
    request: string;
    revision: number;
    localRevision: number;
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
    for (const track of this.screen?.getTracks() || []) track.stop();
    this.local = null;
    this.screen = null;
    this.sources.clear();
    this.received.clear();
    this.ownPublished.clear();
    this.streams.clear();
    this.remoteICE = [];
    this.localICE = [];
    this.update({
      active: false,
      localStream: null,
      localScreen: null,
      microphoneEnabled: false,
      cameraEnabled: false,
      screenSharing: false,
      controlBusy: false,
      remoteStreams: [],
      status: "Камера и микрофон выключены",
      connectionState: "closed",
      iceState: "closed",
    });
  }

  async start(captureDevices = true) {
    if (this.started || this.disposed) return;
    this.started = true;
    this.update({
      active: true,
      status: captureDevices
        ? "Запрашиваем доступ к камере и микрофону…"
        : "Подключаем медиасвязь без устройств…",
      error: null,
    });
    try {
      if (captureDevices && !navigator.mediaDevices?.getUserMedia)
        throw new Error("secure_context");
      const stream =
        !captureDevices ||
        (this.policy.microphoneBlocked && this.policy.cameraBlocked)
          ? new MediaStream()
          : await navigator.mediaDevices.getUserMedia({
              audio: this.policy.microphoneBlocked
                ? false
                : {
                    echoCancellation: true,
                    noiseSuppression: true,
                    autoGainControl: true,
                  },
              video: this.policy.cameraBlocked
                ? false
                : {
                    width: { ideal: 1280, max: 1280 },
                    height: { ideal: 720, max: 720 },
                    frameRate: { ideal: 30, max: 30 },
                  },
            });
      if (this.disposed) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      for (const track of stream.getTracks()) {
        if (
          (track.kind === "audio" && this.policy.microphoneBlocked) ||
          (track.kind === "video" && this.policy.cameraBlocked)
        ) {
          track.stop();
          stream.removeTrack(track);
        }
      }
      this.local = stream;
      this.update({
        localStream: stream.getTracks().length ? stream : null,
        microphoneEnabled: stream.getTracks().some((t) => t.kind === "audio"),
        cameraEnabled: stream.getTracks().some((t) => t.kind === "video"),
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

  private watchTrack(source: MediaSource, track: MediaStreamTrack) {
    track.onended = () => {
      if (this.disposed || this.sources.get(source)?.track !== track) return;
      for (const publication of this.ownPublished.values()) {
        if (publication.source === source)
          this.send("media.unpublish", {
            mediaPeerId: this.view.mediaPeerId,
            trackId: publication.id,
          });
      }
      if (source === "video/screen") void this.stopScreen();
      else
        void this.mutate(async () => {
          if (this.sources.get(source)?.track === track)
            await this.replaceSource(source, null);
        });
    };
  }
  private refreshLocal() {
    this.update({
      localStream: this.local
        ?.getTracks()
        .some((track) => track.readyState === "live")
        ? this.local
        : null,
      localScreen: this.screen,
      microphoneEnabled:
        this.sources.get("microphone")?.track?.readyState === "live",
      cameraEnabled: this.sources.get("camera")?.track?.readyState === "live",
      screenSharing:
        this.sources.get("video/screen")?.track?.readyState === "live",
    });
  }
  private mutate(action: () => Promise<void>) {
    const next = this.queue.then(async () => {
      if (this.disposed) return;
      await action();
      if (this.disposed) return;
      this.refreshLocal();
      this.localRevision++;
      await this.offer();
    });
    this.queue = next.catch(() => {
      if (!this.disposed)
        this.update({
          error:
            "Не удалось изменить медиапоток. Повторите действие или переподключитесь.",
        });
    });
    return this.queue;
  }
  private async replaceSource(
    source: MediaSource,
    track: MediaStreamTrack | null,
  ) {
    const slot = this.sources.get(source);
    if (!slot || this.disposed) {
      track?.stop();
      return;
    }
    const previous = slot.track;
    await slot.sender.replaceTrack(track);
    if (this.disposed) {
      track?.stop();
      return;
    }
    slot.track = track;
    slot.transceiver.direction = track ? "sendrecv" : "recvonly";
    const stream = source.endsWith("/screen") ? this.screen : this.local;
    if (previous) {
      previous.onended = null;
      stream?.removeTrack(previous);
      previous.stop();
    }
    if (track) {
      if (!stream?.getTracks().includes(track)) stream?.addTrack(track);
      this.watchTrack(source, track);
    }
  }
  setPolicy(policy: MediaPolicy) {
    // Delayed roster fetches must not undo a newer realtime restriction.
    if ((policy.version ?? 0) < (this.policy.version ?? 0)) return;
    this.policy = { ...policy };
    if (!this.pc || this.disposed) return;
    const blocked: MediaSource[] = [];
    if (policy.microphoneBlocked) blocked.push("microphone", "audio/screen");
    if (policy.cameraBlocked) blocked.push("camera");
    if (policy.screenBlocked || policy.cameraBlocked)
      blocked.push("video/screen", "audio/screen");
    if (blocked.some((source) => this.sources.get(source)?.track))
      void this.mutate(async () => {
        for (const source of blocked) await this.replaceSource(source, null);
        if (policy.screenBlocked || policy.cameraBlocked) this.screen = null;
      });
  }
  async changeSource(
    source: "microphone" | "camera",
    enabled: boolean,
    deviceId?: string,
  ) {
    if (this.disposed || !this.pc || this.view.controlBusy) return;
    deviceId = deviceId || this.preferredInputs.get(source);
    if (
      enabled &&
      (source === "microphone"
        ? this.policy.microphoneBlocked
        : this.policy.cameraBlocked)
    ) {
      this.update({
        error: "Организатор запретил включение этого устройства.",
      });
      return;
    }
    this.update({ controlBusy: true, error: null });
    let captured: MediaStream | null = null;
    try {
      if (enabled)
        captured = await navigator.mediaDevices.getUserMedia({
          audio:
            source === "microphone"
              ? {
                  echoCancellation: true,
                  noiseSuppression: true,
                  ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
                }
              : false,
          video:
            source === "camera"
              ? {
                  width: {
                    ideal: this.capture.maxWidth,
                    max: this.capture.maxWidth,
                  },
                  height: {
                    ideal: this.capture.maxHeight,
                    max: this.capture.maxHeight,
                  },
                  frameRate: {
                    ideal: this.capture.maxFrameRate,
                    max: this.capture.maxFrameRate,
                  },
                  ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
                }
              : false,
        });
      if (
        this.disposed ||
        (enabled &&
          (source === "microphone"
            ? this.policy.microphoneBlocked
            : this.policy.cameraBlocked))
      ) {
        captured?.getTracks().forEach((track) => track.stop());
        return;
      }
      const track =
        captured
          ?.getTracks()
          .find(
            (t) => t.kind === (source === "microphone" ? "audio" : "video"),
          ) || null;
      if (enabled && !track) throw new Error("missing_track");
      await this.mutate(async () => {
        if (
          enabled &&
          (source === "microphone"
            ? this.policy.microphoneBlocked
            : this.policy.cameraBlocked)
        ) {
          track?.stop();
          return;
        }
        await this.replaceSource(source, track);
        if (track && deviceId) this.preferredInputs.set(source, deviceId);
      });
    } catch {
      captured?.getTracks().forEach((track) => track.stop());
      if (!this.disposed)
        this.update({
          error:
            "Не удалось открыть устройство. Проверьте разрешения браузера и выбор камеры или микрофона.",
        });
    } finally {
      for (const track of captured?.getTracks() || [])
        if (this.sources.get(source)?.track !== track) track.stop();
      if (!this.disposed) this.update({ controlBusy: false });
    }
  }
  async startScreen() {
    if (
      this.disposed ||
      !this.pc ||
      this.view.controlBusy ||
      this.view.screenSharing
    )
      return;
    if (this.policy.screenBlocked || this.policy.cameraBlocked) {
      this.update({
        error:
          "Организатор остановил демонстрацию экрана. Дождитесь разрешения.",
      });
      return;
    }
    this.update({ controlBusy: true, error: null });
    let stream: MediaStream | null = null;
    try {
      // Called directly by the button so transient user activation is retained.
      stream = await navigator.mediaDevices.getDisplayMedia({
        video: { frameRate: { ideal: 15, max: 30 } },
        audio: !this.policy.microphoneBlocked,
      });
      if (
        this.disposed ||
        this.policy.screenBlocked ||
        this.policy.cameraBlocked
      ) {
        stream.getTracks().forEach((t) => t.stop());
        return;
      }
      const captured = stream;
      await this.mutate(async () => {
        if (this.policy.screenBlocked || this.policy.cameraBlocked) {
          captured.getTracks().forEach((t) => t.stop());
          return;
        }
        try {
          this.screen = new MediaStream();
          const video = captured.getTracks().find((t) => t.kind === "video");
          if (!video) throw new Error("missing_screen");
          await this.replaceSource("video/screen", video);
          await this.replaceSource(
            "audio/screen",
            this.policy.microphoneBlocked
              ? null
              : captured.getTracks().find((t) => t.kind === "audio") || null,
          );
        } catch (error) {
          // A second sender may reject after the first has accepted capture.
          // Stop physical capture even if rollback itself encounters an error.
          captured.getTracks().forEach((track) => track.stop());
          for (const source of [
            "video/screen",
            "audio/screen",
          ] as MediaSource[]) {
            const slot = this.sources.get(source);
            if (!slot) continue;
            slot.track = null;
            slot.transceiver.direction = "recvonly";
            await slot.sender.replaceTrack(null).catch(() => {});
          }
          this.screen = null;
          this.refreshLocal();
          this.localRevision++;
          await this.offer();
          throw error;
        }
      });
      if (this.disposed) captured.getTracks().forEach((t) => t.stop());
    } catch {
      stream?.getTracks().forEach((t) => t.stop());
      if (!this.disposed)
        this.update({
          error:
            "Демонстрация не началась. Выберите окно или экран и разрешите доступ.",
        });
    } finally {
      for (const track of stream?.getTracks() || [])
        if (![...this.sources.values()].some((slot) => slot.track === track))
          track.stop();
      if (!this.disposed) this.update({ controlBusy: false });
    }
  }
  stopScreen() {
    return this.mutate(async () => {
      await this.replaceSource("video/screen", null);
      await this.replaceSource("audio/screen", null);
      this.screen = null;
    });
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
        data.code === "media_policy_blocked" &&
        event.replyTo === this.negotiation?.request &&
        this.pc
      ) {
        // A policy can win a race with an offer already sent by this tab.
        // Keep receiving the conference while discarding rejected capture.
        await this.pc.setLocalDescription({ type: "rollback" });
        this.negotiation = null;
        for (const source of this.sources.keys())
          await this.replaceSource(source, null);
        this.screen = null;
        this.refreshLocal();
        this.localRevision++;
        this.update({
          error:
            "Организатор изменил разрешения. Передача устройств остановлена; приём конференции продолжается.",
        });
        await this.offer();
        return;
      }
      if (
        event.replyTo === this.negotiation?.request &&
        data.code === "screen_sharing_conflict" &&
        this.pc
      ) {
        await this.pc.setLocalDescription({ type: "rollback" });
        this.negotiation = null;
        await this.replaceSource("video/screen", null);
        await this.replaceSource("audio/screen", null);
        this.screen = null;
        this.refreshLocal();
        this.localRevision++;
        this.update({
          error:
            "Экран уже показывает другой участник. Дождитесь окончания его демонстрации.",
        });
        await this.offer();
        return;
      }
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
      const joinedPolicy = record(data.policy);
      if (joinedPolicy) this.setPolicy(joinedPolicy as MediaPolicy);
      for (const track of this.local?.getTracks() || []) {
        if (
          (track.kind === "audio" && this.policy.microphoneBlocked) ||
          (track.kind === "video" && this.policy.cameraBlocked)
        ) {
          track.stop();
          this.local?.removeTrack(track);
        }
      }
      const capture = videoCapture(data.videoCapture);
      this.capture = capture;
      this.update({
        mediaPeerId: data.mediaPeerId,
        workerId: data.workerId,
        status: "Согласуем медиасвязь…",
      });
      this.catalog = list;
      const video = this.local
        ?.getTracks()
        .find((track) => track.kind === "video");
      if (video)
        await video.applyConstraints({
          width: { ideal: capture.maxWidth, max: capture.maxWidth },
          height: { ideal: capture.maxHeight, max: capture.maxHeight },
          frameRate: { ideal: capture.maxFrameRate, max: capture.maxFrameRate },
        });
      // Applying browser constraints is async. A stop/reconnect while pending
      // must not resurrect a PeerConnection or publish the captured stream.
      if (this.disposed) return;
      this.createPeer(data.iceServers as RTCIceServer[], Number(data.maxPeers));
      this.refreshLocal();
      await this.offer();
      return;
    }
    if (!this.pc || data.mediaPeerId !== this.view.mediaPeerId) return;
    if (event.type === "media.policy") {
      const policy = record(data.policy);
      if (policy) this.setPolicy(policy as MediaPolicy);
      return;
    } else if (event.type === "media.answer") {
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
      this.answeredLocalRevision = offer.localRevision;
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
      if (
        this.wantedRevision > this.answeredRevision ||
        this.localRevision > this.answeredLocalRevision
      )
        await this.offer();
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
      const localTrack = this.sources.get(list[0].source)?.track;
      if (!localTrack || localTrack.readyState === "ended")
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
    // SFU always negotiates BUNDLE. Share one ICE transport even before the
    // first answer: gathering per receive slot can otherwise exceed the
    // signaling rate limit as the room's preallocated transceiver count grows.
    const pc = new RTCPeerConnection({
      iceServers,
      bundlePolicy: "max-bundle",
    });
    this.pc = pc;
    for (const source of [
      "microphone",
      "camera",
      "video/screen",
      "audio/screen",
    ] as MediaSource[]) {
      const kind =
        source === "microphone" || source === "audio/screen"
          ? "audio"
          : "video";
      const screen = source.endsWith("/screen");
      const track = screen
        ? null
        : this.local!.getTracks().find((t) => t.kind === kind) || null;
      const transceiver = pc.addTransceiver(track || kind, {
        direction: track ? "sendrecv" : "recvonly",
        ...(track ? { streams: [this.local!] } : {}),
      });
      this.sources.set(source, {
        sender: transceiver.sender,
        transceiver,
        track,
      });
      if (track) this.watchTrack(source, track);
    }
    // The browser owns every offer. Stable receive slots prevent worker-driven
    // glare and make late publishers subscribable without server-side offers.
    for (let i = 0; i < 2 * (maxPeers - 1); i++) {
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
      const streamIds = event.streams?.map((stream) => stream.id) || [];
      for (const [id, previous] of this.received) {
        if (
          previous.track.kind === event.track.kind &&
          previous.streamIds.some((id) => streamIds.includes(id))
        )
          this.received.delete(id);
      }
      if (!this.received.has(event.track.id) && this.received.size >= 128) {
        this.fail("Превышен лимит удалённых медиа-треков.");
        return;
      }
      this.received.set(event.track.id, {
        track: event.track,
        streamIds,
      });
      event.track.onended = () => {
        if (this.received.get(event.track.id)?.track !== event.track) return;
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
    const negotiation = {
      id,
      request: "",
      revision: this.wantedRevision,
      localRevision: this.localRevision,
    };
    this.negotiation = negotiation;
    await pc.setLocalDescription(await pc.createOffer());
    if (this.disposed || this.pc !== pc) return;
    negotiation.request = this.send("media.offer", {
      mediaPeerId: this.view.mediaPeerId,
      negotiationId: id,
      sdp: pc.localDescription!.sdp,
      publications: [...this.sources]
        .filter(
          ([, slot]) =>
            slot.track?.readyState === "live" && slot.transceiver.mid !== null,
        )
        .map(([source, slot]) => ({
          mid: slot.transceiver.mid!,
          source,
          trackId: slot.track!.id,
        })),
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
      {
        mediaPeerId: string;
        participantId: string;
        screen: boolean;
        tracks: MediaStreamTrack[];
      }
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
      const group = grouped.get(entry.streamId) || {
        mediaPeerId: entry.mediaPeerId,
        participantId: entry.participantId,
        screen: entry.source.endsWith("/screen"),
        tracks: [],
      };
      group.tracks.push(track);
      grouped.set(entry.streamId, group);
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
        id,
        mediaPeerId: group.mediaPeerId,
        participantId: group.participantId,
        stream,
        kinds: group.tracks.map((track) => track.kind),
        screen: group.screen,
      });
    }
    this.update({ remoteStreams });
  }
}

export function emptyMediaView() {
  return initial();
}
