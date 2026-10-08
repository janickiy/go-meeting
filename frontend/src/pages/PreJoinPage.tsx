import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Camera,
  CameraOff,
  Headphones,
  Mic,
  MicOff,
  ShieldCheck,
  Settings,
  X,
  Pencil,
} from "lucide-react";
import { api, ApiError } from "../api";
import type { Invite } from "../types";
import { useAuth } from "../auth";
import { isAdmitted } from "../collaboration";
import { useMembership } from "../queries";
import {
  availableDeviceId,
  captureErrorMessage,
  readDevicePreferences,
  saveDevicePreferences,
  queueMediaEntry,
} from "../prejoinDevices";
import type { DevicePreferences } from "../prejoinDevices";
import {
  Brand,
  Button,
  ErrorNotice,
  Loading,
  StatusBadge,
  Modal,
} from "../components/ui";
import { initials } from "../utils";
import "./prejoin.css";

type InputKind = "audio" | "video";

/** Снимок личности перед запросом допуска отделяет старый ответ от новой сессии. */
interface JoinIdentity {
  owner: string;
  generation: number;
  guestEntry: boolean;
}

/** Локальный захват служит только предпросмотру. Клиент SFU создаётся после допуска. */
export function PreJoinPage({
  invitation,
}: { invitation?: { code: string; meeting: Invite } } = {}) {
  const { id: routeId = "" } = useParams();
  const id = invitation?.meeting.id || routeId;
  const [params] = useSearchParams();
  const rawInviteCode = invitation?.code || params.get("invite");
  const inviteCode =
    rawInviteCode && /^[A-Za-z0-9_-]{32}$/.test(rawInviteCode)
      ? rawInviteCode
      : "";
  const {
    user,
    loading: authLoading,
    startupError,
    retry,
    enterGuest,
  } = useAuth();
  const guestMode = !user || !!user.guestConferenceId;
  const needsMembership =
    !!user && (!user.guestConferenceId || user.guestConferenceId === id);
  const [guestName, setGuestName] = useState(
    user?.guestConferenceId ? user.displayName || "Гость" : "Гость",
  );
  const [settingsOpen, setSettingsOpen] = useState(false);
  useEffect(() => {
    if (user?.guestConferenceId) setGuestName(user.displayName || "Гость");
  }, [user?.id]);
  const navigate = useNavigate();
  const client = useQueryClient();
  const conference = useQuery({
    queryKey: ["prejoin-conference", user?.id, id, inviteCode],
    queryFn: ({ signal }) =>
      inviteCode ? api.invite(inviteCode, signal) : api.conference(id, signal),
    enabled: !!id && (!rawInviteCode || !!inviteCode),
    refetchInterval: 10000,
    initialData: invitation
      ? { status: "success", item: invitation.meeting }
      : undefined,
  });
  const self = useMembership(id, 3000, needsMembership);
  const preferencesOwner = authLoading ? "" : user?.id || "guest";
  const [ownedPreferences, setOwnedPreferences] = useState(() => ({
    owner: preferencesOwner,
    value: readDevicePreferences(preferencesOwner),
  }));
  const preferences = ownedPreferences.value;
  const preferencesReady =
    !!preferencesOwner && ownedPreferences.owner === preferencesOwner;
  const preferencesOwnerRef = useRef(preferencesOwner);
  preferencesOwnerRef.current = preferencesOwner;
  const identityGeneration = useRef(0);

  // Обновление от старого пользователя не может изменить настройки новой сессии.
  const setPreferences = useCallback(
    (update: (current: DevicePreferences) => DevicePreferences) => {
      setOwnedPreferences((current) =>
        current.owner && current.owner === preferencesOwnerRef.current
          ? { ...current, value: update(current.value) }
          : current,
      );
    },
    [],
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
  const autoPreviewRequested = useRef("");

  const supported =
    typeof navigator !== "undefined" && !!navigator.mediaDevices?.getUserMedia;
  const secure = typeof window !== "undefined" && window.isSecureContext;
  const outputSupported =
    typeof HTMLMediaElement !== "undefined" &&
    "setSinkId" in HTMLMediaElement.prototype;

  const refreshDevices = useCallback(async () => {
    const owner = preferencesOwnerRef.current;
    const expectedIdentity = identityGeneration.current;
    if (!owner) return;
    if (!navigator.mediaDevices?.enumerateDevices) return;
    try {
      const list = await navigator.mediaDevices.enumerateDevices();
      if (
        !mounted.current ||
        preferencesOwnerRef.current !== owner ||
        identityGeneration.current !== expectedIdentity
      )
        return;
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
      // Названия устройств могут быть недоступны до первого предоставления разрешения.
    }
  }, [setPreferences]);

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

  useLayoutEffect(() => {
    // Смена личности отменяет захват до показа новых элементов управления.
    identityGeneration.current += 1;
    stopInput("audio");
    stopInput("video");
    setBusy({ audio: false, video: false });
    setDeviceError("");
    setNotice("");
    setPlaybackBlocked(false);
    setDevices([]);
    autoPreviewRequested.current = "";
    if (preferencesOwner)
      setOwnedPreferences((current) =>
        current.owner === preferencesOwner
          ? current
          : {
              owner: preferencesOwner,
              value: readDevicePreferences(preferencesOwner),
            },
      );
  }, [preferencesOwner, stopInput]);

  useEffect(() => {
    mounted.current = true;
    navigator.mediaDevices?.addEventListener?.("devicechange", refreshDevices);
    return () => {
      mounted.current = false;
      navigator.mediaDevices?.removeEventListener?.(
        "devicechange",
        refreshDevices,
      );
      // При размонтировании страницы останавливаем устройства без обновления состояния React.
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
    if (preferencesReady && !startupError)
      saveDevicePreferences(preferencesOwner, preferences);
  }, [preferencesOwner, preferencesReady, preferences, startupError]);

  useEffect(() => {
    if (preferencesReady) void refreshDevices();
  }, [preferencesOwner, preferencesReady, refreshDevices]);

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
    if (!preferencesReady || startupError) return;
    const owner = preferencesOwner;
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
              noiseSuppression: preferences.noiseSuppression,
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
        if (
          !mounted.current ||
          preferencesOwnerRef.current !== owner ||
          generation.current[kind] !== request
        )
          return;
        stream = await navigator.mediaDevices.getUserMedia(constraints(""));
        if (
          mounted.current &&
          preferencesOwnerRef.current === owner &&
          generation.current[kind] === request
        ) {
          setPreferences((current) => ({
            ...current,
            [kind === "audio" ? "audioInputId" : "videoInputId"]: "",
          }));
          setNotice(
            "Сохранённое устройство недоступно. Используется системное устройство.",
          );
        }
      }
      if (
        !mounted.current ||
        preferencesOwnerRef.current !== owner ||
        generation.current[kind] !== request
      ) {
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
        if (
          !mounted.current ||
          preferencesOwnerRef.current !== owner ||
          tracks.current[kind] !== track
        )
          return;
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
      if (
        mounted.current &&
        preferencesOwnerRef.current === owner &&
        generation.current[kind] === request
      ) {
        setDeviceError(captureErrorMessage(error, kind));
        setPreferences((current) => ({
          ...current,
          [kind === "audio" ? "microphoneEnabled" : "cameraEnabled"]: false,
        }));
      }
    } finally {
      if (
        mounted.current &&
        preferencesOwnerRef.current === owner &&
        generation.current[kind] === request
      )
        setBusy((current) => ({ ...current, [kind]: false }));
    }
  }

  // Предпросмотр использует только загруженные настройки текущей личности.
  // Отказ браузера в разрешении не мешает подключиться без устройств.
  useEffect(() => {
    if (
      !invitation ||
      authLoading ||
      !preferencesReady ||
      startupError ||
      conference.isPending ||
      (needsMembership && self.isPending) ||
      conference.data?.item.id !== id ||
      !["created", "active"].includes(conference.data?.item.status || "")
    )
      return;
    // Отложенный запуск исключает повторные запросы в React StrictMode.
    const timer = window.setTimeout(() => {
      const entry = `${preferencesOwner}:${id}`;
      if (autoPreviewRequested.current === entry) return;
      autoPreviewRequested.current = entry;
      if (preferences.microphoneEnabled)
        void capture("audio", true, preferences.audioInputId);
      if (preferences.cameraEnabled)
        void capture("video", true, preferences.videoInputId);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [
    id,
    authLoading,
    preferencesOwner,
    preferencesReady,
    preferences,
    startupError,
    conference.isPending,
    conference.data?.item.status,
    needsMembership,
    self.isPending,
  ]);

  /**
   * Проверяет принадлежность ответа текущей сессии. Только гостевой вход
   * допускает одну ожидаемую смену личности на созданного сервером гостя.
   *
   * @args identity — снимок владельца и поколения при отправке запроса.
   * @args userId — пользователь, получивший допуск к встрече.
   * @return true, если ответ ещё можно применить без изменения чужой сессии.
   */
  function joinIdentityIsCurrent(identity: JoinIdentity, userId: string) {
    const owner = preferencesOwnerRef.current;
    const currentGeneration = identityGeneration.current;
    return (
      (owner === identity.owner && currentGeneration === identity.generation) ||
      (identity.guestEntry &&
        owner === userId &&
        currentGeneration ===
          identity.generation + Number(identity.owner !== userId))
    );
  }

  const join = useMutation({
    mutationFn: async () => {
      if (!preferencesReady || startupError)
        throw new Error("prejoin_preferences_not_ready");
      if (!conference.data || conference.data.item.id !== id)
        throw new Error("invite_conference_mismatch");
      const identity: JoinIdentity = {
        owner: preferencesOwner,
        generation: identityGeneration.current,
        guestEntry: guestMode,
      };
      // Гостевая авторизация меняет личность до onSuccess. Снимок сохраняет
      // ручной выбор устройств и их фактическое состояние перед подключением.
      const entryPreferences = invitation
        ? {
            ...preferences,
            microphoneEnabled: microphoneOn,
            cameraEnabled: cameraOn,
          }
        : preferences;
      if (guestMode) {
        if (!inviteCode)
          throw new ApiError(
            403,
            "Откройте ссылку-приглашение, чтобы войти гостем.",
          );
        const session = await enterGuest(inviteCode, guestName.trim());
        if (!joinIdentityIsCurrent(identity, session.user.id))
          throw new ApiError(409, "Сессия изменилась. Повторите подключение.");
        return {
          membership: session.item,
          userId: session.user.id,
          entryPreferences,
          identity,
        };
      }
      const current = self.data;
      const membership =
        current && current.status === "joined" && isAdmitted(current)
          ? current
          : inviteCode
            ? (await api.joinInvite(inviteCode)).item
            : (await api.membership(id, "join")).item;
      if (!joinIdentityIsCurrent(identity, identity.owner))
        throw new ApiError(409, "Сессия изменилась. Повторите подключение.");
      return {
        membership,
        userId: identity.owner,
        entryPreferences,
        identity,
      };
    },
    onSuccess: ({ membership, userId, entryPreferences, identity }) => {
      if (!joinIdentityIsCurrent(identity, userId)) return;
      saveDevicePreferences(userId, entryPreferences);
      setOwnedPreferences({ owner: userId, value: entryPreferences });
      client.setQueryData(["membership", userId, id], membership);
      void client.invalidateQueries({ queryKey: ["membership"] });
      void client.invalidateQueries({ queryKey: ["participants"] });
      void client.invalidateQueries({ queryKey: ["conferences"] });
      if (invitation && conference.data?.item.status !== "scheduled")
        queueMediaEntry(id);
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
  if (
    authLoading ||
    !preferencesReady ||
    conference.isPending ||
    (needsMembership && self.isPending)
  )
    return <Loading />;
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

  if (invitation) {
    const name = guestMode
      ? guestName
      : user?.displayName || user?.email || "Участник";
    const disabled =
      !canJoin ||
      self.isError ||
      busy.audio ||
      busy.video ||
      join.isPending ||
      !!startupError ||
      (guestMode && !guestName.trim());
    return (
      <main className="invite-entry-page">
        <form
          className="invite-entry"
          aria-label="Проверка перед входом"
          onSubmit={(event) => {
            event.preventDefault();
            if (!disabled) join.mutate();
          }}
        >
          <div className="invite-entry-preview">
            {cameraOn ? (
              <video
                ref={video}
                autoPlay
                muted
                playsInline
                aria-label="Предпросмотр камеры"
              />
            ) : (
              <div className="invite-entry-placeholder">
                <span>{initials(name || "Гость")}</span>
                <CameraOff size={28} aria-hidden="true" />
                <p>Камера выключена</p>
              </div>
            )}
          </div>
          <header className="invite-entry-heading">
            <h1>{meeting.title}</h1>
            <StatusBadge status={meeting.status} />
          </header>
          <Link
            className="invite-entry-close"
            to={user && !user.guestConferenceId ? "/app" : "/"}
            aria-label="Закрыть подключение"
          >
            <X size={30} />
          </Link>
          <div className="invite-entry-bottom">
            <div className="invite-entry-identity">
              {guestMode ? (
                <label className="invite-entry-name">
                  <span className="sr-only">Имя на встрече</span>
                  <input
                    aria-label="Имя на встрече"
                    value={guestName}
                    maxLength={100}
                    required
                    autoComplete="off"
                    onChange={(event) => setGuestName(event.target.value)}
                    disabled={join.isPending}
                  />
                  <Pencil size={19} aria-hidden="true" />
                </label>
              ) : (
                <h2>{name}</h2>
              )}
              <p>
                {guestMode
                  ? "Укажите имя, которое увидят участники"
                  : user?.email}
              </p>
            </div>
            <div className="invite-entry-notices">
              {meeting.status === "scheduled" && (
                <p>
                  Подключение будет доступно, когда организатор начнёт встречу.
                </p>
              )}
              {meeting.status === "scheduled" && !guestMode && (
                <Button
                  type="button"
                  variant="secondary"
                  busy={join.isPending}
                  disabled={!!startupError || self.isError}
                  onClick={() => join.mutate()}
                >
                  Добавить в мои встречи
                </Button>
              )}
              {closed && <p>Встреча завершена.</p>}
              {restricted && <p>Организатор ограничил повторный вход.</p>}
              {!secure && (
                <p>
                  Для камеры и микрофона откройте защищённую HTTPS-страницу.
                </p>
              )}
              <ErrorNotice error={self.error || join.error}>
                {deviceError || null}
              </ErrorNotice>
              {startupError && (
                <p role="alert">
                  Не удалось проверить сессию.{" "}
                  <button type="button" onClick={retry}>
                    Повторить
                  </button>
                </p>
              )}
              {notice && <p role="status">{notice}</p>}
              {playbackBlocked && (
                <button
                  type="button"
                  onClick={() =>
                    void video.current
                      ?.play()
                      .then(() => setPlaybackBlocked(false))
                  }
                >
                  Запустить предпросмотр
                </button>
              )}
            </div>
            <div className="invite-entry-controls">
              <div className="invite-entry-inputs">
                <button
                  type="button"
                  className={`invite-entry-control ${microphoneOn ? "is-on" : ""}`}
                  aria-label={
                    microphoneOn ? "Выключить микрофон" : "Включить микрофон"
                  }
                  aria-pressed={microphoneOn}
                  disabled={busy.audio || join.isPending}
                  onClick={() =>
                    void capture(
                      "audio",
                      !microphoneOn,
                      preferences.audioInputId,
                    )
                  }
                >
                  {microphoneOn ? <Mic /> : <MicOff />}
                </button>
                <button
                  type="button"
                  className={`invite-entry-control ${cameraOn ? "is-on" : ""}`}
                  aria-label={cameraOn ? "Выключить камеру" : "Включить камеру"}
                  aria-pressed={cameraOn}
                  disabled={busy.video || join.isPending}
                  onClick={() =>
                    void capture("video", !cameraOn, preferences.videoInputId)
                  }
                >
                  {cameraOn ? <Camera /> : <CameraOff />}
                </button>
              </div>
              <Button
                type="submit"
                className="invite-entry-connect"
                busy={join.isPending}
                disabled={disabled}
              >
                Подключиться
              </Button>
              <button
                type="button"
                className="invite-entry-control invite-entry-settings"
                aria-label="Настройки устройств"
                aria-haspopup="dialog"
                disabled={join.isPending}
                onClick={() => setSettingsOpen(true)}
              >
                <Settings />
              </button>
            </div>
          </div>
        </form>
        {settingsOpen && (
          <Modal
            title="Настройки устройств"
            onClose={() => setSettingsOpen(false)}
          >
            <div className="prejoin-settings invite-device-settings">
              <label className="field">
                Микрофон
                <select
                  value={preferences.audioInputId}
                  disabled={!supported || busy.audio || join.isPending}
                  onChange={(event) => {
                    const deviceId = event.target.value;
                    setPreferences((current) => ({
                      ...current,
                      audioInputId: deviceId,
                    }));
                    if (microphoneOn) void capture("audio", true, deviceId);
                  }}
                >
                  {inputOptions("audioinput", preferences.audioInputId)}
                </select>
              </label>
              <label className="field">
                Камера
                <select
                  value={preferences.videoInputId}
                  disabled={!supported || busy.video || join.isPending}
                  onChange={(event) => {
                    const deviceId = event.target.value;
                    setPreferences((current) => ({
                      ...current,
                      videoInputId: deviceId,
                    }));
                    if (cameraOn) void capture("video", true, deviceId);
                  }}
                >
                  {inputOptions("videoinput", preferences.videoInputId)}
                </select>
              </label>
              {outputSupported && (
                <label className="field">
                  Вывод звука
                  <select
                    value={preferences.audioOutputId}
                    disabled={join.isPending}
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
              <label className="prejoin-level">
                Уровень микрофона
                <meter min={0} max={1} value={microphoneOn ? level : 0} />
              </label>
              <p className="field-hint">
                До подключения камера и микрофон доступны только вам.
              </p>
            </div>
          </Modal>
        )}
      </main>
    );
  }

  return (
    <section className="prejoin-page" aria-label="Проверка перед входом">
      <div className="prejoin-brand">
        <Brand to="/app" />
      </div>
      <Link className="text-link" to={`/conferences/${id}`}>
        ← К встрече
      </Link>
      <div className="prejoin-heading">
        <div>
          <span className="eyebrow">НАСТРОЙКИ ПЕРЕД ВХОДОМ</span>
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
                <span className="prejoin-initials">
                  {initials(user?.displayName || "Участник")}
                </span>
                <CameraOff size={22} aria-hidden="true" />
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
              disabled={!supported || busy.audio || join.isPending}
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
              disabled={!supported || busy.video || join.isPending}
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
                disabled={join.isPending}
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
          {meeting.waitingRoomEnabled && !inviteCode && (
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
            <ShieldCheck size={15} aria-hidden="true" /> Предпросмотр
            остановится при входе. После допуска включите медиа в комнате
            выбранными устройствами.
          </p>
        </div>
      </div>
    </section>
  );
}
