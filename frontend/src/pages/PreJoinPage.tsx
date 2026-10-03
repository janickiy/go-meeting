import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Camera, CameraOff, Headphones, Mic, MicOff } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { isAdmitted } from "../collaboration";
import { useMembership } from "../queries";
import {
  availableDeviceId,
  captureErrorMessage,
  readDevicePreferences,
  saveDevicePreferences,
} from "../prejoinDevices";
import type { DevicePreferences } from "../prejoinDevices";
import { Button, ErrorNotice, Loading, StatusBadge } from "../components/ui";
import "./prejoin.css";

type InputKind = "audio" | "video";

/** Local capture is only a preview. The SFU client is mounted after admission. */
export function PreJoinPage() {
  const { id = "" } = useParams();
  const [params] = useSearchParams();
  const rawInviteCode = params.get("invite");
  const inviteCode =
    rawInviteCode && /^[A-Za-z0-9_-]{32}$/.test(rawInviteCode)
      ? rawInviteCode
      : "";
  const { user } = useAuth();
  const navigate = useNavigate();
  const client = useQueryClient();
  const conference = useQuery({
    queryKey: ["prejoin-conference", user?.id, id, inviteCode],
    queryFn: ({ signal }) =>
      inviteCode ? api.invite(inviteCode, signal) : api.conference(id, signal),
    enabled: !!id && (!rawInviteCode || !!inviteCode),
    refetchInterval: 10000,
  });
  const self = useMembership(id);
  const [preferences, setPreferences] = useState<DevicePreferences>(() =>
    readDevicePreferences(user?.id || ""),
  );
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [videoStream, setVideoStream] = useState<MediaStream | null>(null);
  const [audioTrack, setAudioTrack] = useState<MediaStreamTrack | null>(null);
  const [cameraOn, setCameraOn] = useState(false);
  const [microphoneOn, setMicrophoneOn] = useState(false);
  const [busy, setBusy] = useState({ audio: false, video: false });
  const [deviceError, setDeviceError] = useState("");
  const [notice, setNotice] = useState("");
  const [playbackBlocked, setPlaybackBlocked] = useState(false);
  const [level, setLevel] = useState(0);
  const video = useRef<HTMLVideoElement>(null);
  const tracks = useRef<{
    audio: MediaStreamTrack | null;
    video: MediaStreamTrack | null;
  }>({ audio: null, video: null });
  const generation = useRef({ audio: 0, video: 0 });
  const mounted = useRef(false);

  const supported =
    typeof navigator !== "undefined" && !!navigator.mediaDevices?.getUserMedia;
  const secure = typeof window !== "undefined" && window.isSecureContext;
  const outputSupported =
    typeof HTMLMediaElement !== "undefined" &&
    "setSinkId" in HTMLMediaElement.prototype;

  const refreshDevices = useCallback(async () => {
    if (!navigator.mediaDevices?.enumerateDevices) return;
    try {
      const list = await navigator.mediaDevices.enumerateDevices();
      if (!mounted.current) return;
      setDevices(list);
      setPreferences((current) => {
        const next = {
          ...current,
          audioInputId: availableDeviceId(
            list,
            "audioinput",
            current.audioInputId,
          ),
          videoInputId: availableDeviceId(
            list,
            "videoinput",
            current.videoInputId,
          ),
          audioOutputId: availableDeviceId(
            list,
            "audiooutput",
            current.audioOutputId,
          ),
        };
        if (
          next.audioInputId !== current.audioInputId ||
          next.videoInputId !== current.videoInputId ||
          next.audioOutputId !== current.audioOutputId
        )
          setNotice(
            "Сохранённое устройство отключено. Выбрано системное устройство.",
          );
        return next;
      });
    } catch {
      // Device labels can remain unavailable until the first permission grant.
    }
  }, []);

  const stopInput = useCallback((kind: InputKind) => {
    generation.current[kind] += 1;
    const track = tracks.current[kind];
    tracks.current[kind] = null;
    if (track) {
      track.onended = null;
      track.stop();
    }
    if (kind === "video") {
      setVideoStream(null);
      setCameraOn(false);
    } else {
      setAudioTrack(null);
      setMicrophoneOn(false);
      setLevel(0);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void refreshDevices();
    navigator.mediaDevices?.addEventListener?.("devicechange", refreshDevices);
    return () => {
      mounted.current = false;
      navigator.mediaDevices?.removeEventListener?.(
        "devicechange",
        refreshDevices,
      );
      // Stop without React state updates while the page is unmounting.
      for (const kind of ["audio", "video"] as const) {
        generation.current[kind] += 1;
        const track = tracks.current[kind];
        tracks.current[kind] = null;
        if (track) {
          track.onended = null;
          track.stop();
        }
      }
    };
  }, [refreshDevices]);

  useEffect(() => {
    if (user?.id) saveDevicePreferences(user.id, preferences);
  }, [user?.id, preferences]);

  useEffect(() => {
    const element = video.current;
    if (!element) return;
    element.srcObject = videoStream;
    if (videoStream)
      void element.play().then(
        () => setPlaybackBlocked(false),
        () => setPlaybackBlocked(true),
      );
    return () => {
      element.srcObject = null;
    };
  }, [videoStream]);

  useEffect(() => {
    if (!audioTrack || typeof AudioContext === "undefined") return;
    let context: AudioContext;
    try {
      context = new AudioContext();
    } catch {
      return;
    }
    let source: MediaStreamAudioSourceNode;
    let analyser: AnalyserNode;
    try {
      source = context.createMediaStreamSource(new MediaStream([audioTrack]));
      analyser = context.createAnalyser();
    } catch {
      void context.close().catch(() => {});
      return;
    }
    analyser.fftSize = 512;
    source.connect(analyser);
    const samples = new Uint8Array(analyser.fftSize);
    let frame = 0;
    let lastUpdate = 0;
    const sample = (at: number) => {
      analyser.getByteTimeDomainData(samples);
      if (at - lastUpdate >= 100) {
        let square = 0;
        for (const value of samples) square += ((value - 128) / 128) ** 2;
        setLevel(Math.min(1, Math.sqrt(square / samples.length) * 4));
        lastUpdate = at;
      }
      frame = requestAnimationFrame(sample);
    };
    void context.resume().catch(() => {});
    frame = requestAnimationFrame(sample);
    return () => {
      cancelAnimationFrame(frame);
      source.disconnect();
      analyser.disconnect();
      void context.close().catch(() => {});
    };
  }, [audioTrack]);

  async function capture(
    kind: InputKind,
    enabled: boolean,
    preferredId: string,
  ) {
    stopInput(kind);
    setDeviceError("");
    if (!enabled) {
      setPreferences((current) => ({
        ...current,
        [kind === "audio" ? "microphoneEnabled" : "cameraEnabled"]: false,
      }));
      return;
    }
    if (!secure || !supported) {
      setPreferences((current) => ({
        ...current,
        [kind === "audio" ? "microphoneEnabled" : "cameraEnabled"]: false,
      }));
      setDeviceError(
        !secure
          ? "Камера и микрофон доступны только на HTTPS или localhost. Откройте защищённую страницу."
          : "Этот браузер не поддерживает доступ к камере и микрофону.",
      );
      return;
    }
    const request = generation.current[kind];
    setBusy((current) => ({ ...current, [kind]: true }));
    const constraints = (deviceId: string): MediaStreamConstraints =>
      kind === "audio"
        ? {
            audio: {
              echoCancellation: true,
              noiseSuppression: true,
              ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
            },
            video: false,
          }
        : {
            audio: false,
            video: {
              width: { ideal: 1280 },
              height: { ideal: 720 },
              ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
            },
          };
    try {
      let stream: MediaStream;
      try {
        stream = await navigator.mediaDevices.getUserMedia(
          constraints(preferredId),
        );
      } catch (error) {
        const name =
          error &&
          typeof error === "object" &&
          "name" in error &&
          typeof error.name === "string"
            ? error.name
            : "";
        if (
          !preferredId ||
          (name !== "NotFoundError" && name !== "OverconstrainedError")
        )
          throw error;
        stream = await navigator.mediaDevices.getUserMedia(constraints(""));
        if (mounted.current && generation.current[kind] === request) {
          setPreferences((current) => ({
            ...current,
            [kind === "audio" ? "audioInputId" : "videoInputId"]: "",
          }));
          setNotice(
            "Сохранённое устройство недоступно. Используется системное устройство.",
          );
        }
      }
      if (!mounted.current || generation.current[kind] !== request) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      const track =
        kind === "audio"
          ? stream.getAudioTracks()[0]
          : stream.getVideoTracks()[0];
      if (!track) {
        stream.getTracks().forEach((item) => item.stop());
        throw new DOMException("Missing capture track", "NotFoundError");
      }
      stream
        .getTracks()
        .filter((item) => item !== track)
        .forEach((item) => item.stop());
      tracks.current[kind] = track;
      track.onended = () => {
        if (!mounted.current || tracks.current[kind] !== track) return;
        tracks.current[kind] = null;
        generation.current[kind] += 1;
        if (kind === "video") {
          setVideoStream(null);
          setCameraOn(false);
        } else {
          setAudioTrack(null);
          setMicrophoneOn(false);
          setLevel(0);
        }
        setPreferences((current) => ({
          ...current,
          [kind === "audio" ? "microphoneEnabled" : "cameraEnabled"]: false,
        }));
        setDeviceError(
          "Устройство отключено. Выберите другое или войдите без него.",
        );
        void refreshDevices();
      };
      if (kind === "video") {
        setVideoStream(new MediaStream([track]));
        setCameraOn(true);
      } else {
        setAudioTrack(track);
        setMicrophoneOn(true);
      }
      setPreferences((current) => ({
        ...current,
        [kind === "audio" ? "microphoneEnabled" : "cameraEnabled"]: true,
      }));
      void refreshDevices();
    } catch (error) {
      if (mounted.current && generation.current[kind] === request) {
        setDeviceError(captureErrorMessage(error, kind));
        setPreferences((current) => ({
          ...current,
          [kind === "audio" ? "microphoneEnabled" : "cameraEnabled"]: false,
        }));
      }
    } finally {
      if (mounted.current && generation.current[kind] === request)
        setBusy((current) => ({ ...current, [kind]: false }));
    }
  }

  const join = useMutation({
    mutationFn: async () => {
      if (!conference.data || conference.data.item.id !== id)
        throw new Error("invite_conference_mismatch");
      const current = self.data;
      if (
        current &&
        (current.status === "waiting" ||
          (current.status === "joined" && isAdmitted(current)))
      )
        return current;
      return inviteCode
        ? (await api.joinInvite(inviteCode)).item
        : (await api.membership(id, "join")).item;
    },
    onSuccess: (membership) => {
      if (user?.id) saveDevicePreferences(user.id, preferences);
      if (user?.id)
        client.setQueryData(["membership", user.id, id], membership);
      void client.invalidateQueries({ queryKey: ["membership"] });
      void client.invalidateQueries({ queryKey: ["participants"] });
      void client.invalidateQueries({ queryKey: ["conferences"] });
      stopInput("audio");
      stopInput("video");
      navigate(`/conferences/${id}`, { replace: true });
    },
  });

  if (rawInviteCode && !inviteCode)
    return (
      <section className="content-card">
        <h1>Приглашение недоступно</h1>
        <p>Проверьте ссылку приглашения.</p>
      </section>
    );
  if (conference.isPending || self.isPending) return <Loading />;
  if (conference.isError || !conference.data)
    return (
      <section className="content-card">
        <h1>Встреча недоступна</h1>
        <ErrorNotice error={conference.error} />
        <Link to="/conferences">К моим встречам</Link>
      </section>
    );

  const meeting = conference.data.item;
  if (meeting.id !== id)
    return (
      <section className="content-card">
        <h1>Приглашение недоступно</h1>
        <p>Ссылка относится к другой встрече.</p>
      </section>
    );
  const closed =
    meeting.status === "finished" || meeting.status === "cancelled";
  const restricted =
    self.data?.status === "rejected" || self.data?.status === "kicked";
  const canJoin = !closed && meeting.status !== "scheduled" && !restricted;
  const inputOptions = (kind: MediaDeviceKind, selected: string) => {
    const options = devices.filter(
      (device) => device.kind === kind && device.deviceId,
    );
    return (
      <>
        <option value="">Системное устройство</option>
        {selected &&
          !options.some((device) => device.deviceId === selected) && (
            <option value={selected}>Сохранённое устройство</option>
          )}
        {options
          .filter((device) => device.deviceId !== "default")
          .map((device, index) => (
            <option key={device.deviceId} value={device.deviceId}>
              {device.label || `Устройство ${index + 1}`}
            </option>
          ))}
      </>
    );
  };

  return (
    <section className="prejoin-page" aria-label="Проверка перед входом">
      <Link className="text-link" to={`/conferences/${id}`}>
        ← К встрече
      </Link>
      <div className="prejoin-heading">
        <div>
          <span className="eyebrow">ПЕРЕД ВХОДОМ</span>
          <h1>{meeting.title}</h1>
          <p>
            Проверьте устройства. Камера и микрофон пока доступны только вам.
          </p>
        </div>
        <StatusBadge status={meeting.status} />
      </div>
      <div className="prejoin-grid">
        <div className="content-card prejoin-preview">
          <div className="prejoin-video-frame">
            {cameraOn ? (
              <video
                ref={video}
                autoPlay
                muted
                playsInline
                aria-label="Предпросмотр камеры"
              />
            ) : (
              <div className="prejoin-camera-off">
                <CameraOff size={42} aria-hidden="true" />
                <span>Камера выключена</span>
              </div>
            )}
          </div>
          {playbackBlocked && (
            <Button
              variant="outline"
              onClick={() =>
                void video.current?.play().then(() => setPlaybackBlocked(false))
              }
            >
              Запустить предпросмотр
            </Button>
          )}
          <div className="prejoin-toggles">
            <Button
              variant={microphoneOn ? "secondary" : "outline"}
              aria-pressed={microphoneOn}
              busy={busy.audio}
              disabled={join.isPending}
              onClick={() =>
                void capture("audio", !microphoneOn, preferences.audioInputId)
              }
            >
              {microphoneOn ? <Mic size={18} /> : <MicOff size={18} />}
              {microphoneOn ? "Выключить микрофон" : "Проверить микрофон"}
            </Button>
            <Button
              variant={cameraOn ? "secondary" : "outline"}
              aria-pressed={cameraOn}
              busy={busy.video}
              disabled={join.isPending}
              onClick={() =>
                void capture("video", !cameraOn, preferences.videoInputId)
              }
            >
              {cameraOn ? <Camera size={18} /> : <CameraOff size={18} />}
              {cameraOn ? "Выключить камеру" : "Проверить камеру"}
            </Button>
          </div>
          <label className="prejoin-level">
            Уровень микрофона
            <meter min={0} max={1} value={microphoneOn ? level : 0} />
          </label>
          <ErrorNotice>{deviceError || null}</ErrorNotice>
          {notice && (
            <p role="status" className="field-hint">
              {notice}
            </p>
          )}
          {!secure && (
            <p className="field-hint">
              Для доступа к устройствам откройте встречу через HTTPS или
              localhost.
            </p>
          )}
        </div>
        <div className="content-card prejoin-settings">
          <h2>Устройства и вход</h2>
          <label className="field">
            Имя на встрече
            <input value={user?.displayName || user?.email || ""} readOnly />
          </label>
          <p className="field-hint">
            Имя берётся из вашего аккаунта.{" "}
            <Link className="text-link" to="/settings">
              Изменить имя в настройках
            </Link>
          </p>
          <label className="field">
            <span className="prejoin-device-label">
              <Mic size={16} aria-hidden="true" /> Микрофон
            </span>
            <select
              value={preferences.audioInputId}
              disabled={!supported || busy.audio}
              onChange={(event) => {
                const id = event.target.value;
                setPreferences((current) => ({ ...current, audioInputId: id }));
                if (microphoneOn) void capture("audio", true, id);
              }}
            >
              {inputOptions("audioinput", preferences.audioInputId)}
            </select>
          </label>
          <label className="field">
            <span className="prejoin-device-label">
              <Camera size={16} aria-hidden="true" /> Камера
            </span>
            <select
              value={preferences.videoInputId}
              disabled={!supported || busy.video}
              onChange={(event) => {
                const id = event.target.value;
                setPreferences((current) => ({ ...current, videoInputId: id }));
                if (cameraOn) void capture("video", true, id);
              }}
            >
              {inputOptions("videoinput", preferences.videoInputId)}
            </select>
          </label>
          {outputSupported && (
            <label className="field">
              <span className="prejoin-device-label">
                <Headphones size={16} aria-hidden="true" /> Вывод звука
              </span>
              <select
                value={preferences.audioOutputId}
                onChange={(event) =>
                  setPreferences((current) => ({
                    ...current,
                    audioOutputId: event.target.value,
                  }))
                }
              >
                {inputOptions("audiooutput", preferences.audioOutputId)}
              </select>
            </label>
          )}
          <p className="field-hint">
            Выбранные устройства запоминаются только в этом браузере. Если
            устройство исчезнет, будет использовано системное.
          </p>
          {meeting.waitingRoomEnabled && (
            <p className="prejoin-waiting-note">
              После входа организатор может направить вас в зал ожидания. Медиа
              подключается только после допуска.
            </p>
          )}
          {meeting.status === "scheduled" && (
            <p className="field-hint">Войти можно после начала встречи.</p>
          )}
          {closed && <p className="field-hint">Встреча завершена.</p>}
          {restricted && (
            <p className="field-hint">Организатор ограничил повторный вход.</p>
          )}
          <ErrorNotice error={self.error || join.error} />
          <Button
            className="prejoin-join"
            busy={join.isPending}
            disabled={!canJoin || self.isError || busy.audio || busy.video}
            onClick={() => join.mutate()}
          >
            Войти во встречу
          </Button>
          <p className="field-hint">
            Предпросмотр остановится при входе. После допуска включите медиа в
            комнате выбранными устройствами.
          </p>
        </div>
      </div>
    </section>
  );
}
