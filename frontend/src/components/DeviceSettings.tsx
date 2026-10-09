import { useCallback, useEffect, useRef, useState } from "react";
import { Mic, Play, Square, VideoOff } from "lucide-react";
import { useAuth } from "../auth";
import {
  availableDeviceId,
  captureErrorMessage,
  readDevicePreferences,
  saveDevicePreferences,
  subscribeDevicePreferences,
} from "../prejoinDevices";
import type { DevicePreferences } from "../prejoinDevices";
import { playDeviceTone } from "../deviceSound";
import "./device-settings.css";

function useSettings() {
  const { user } = useAuth();
  const userId = user?.id || "guest";
  const [preferences, setPreferences] = useState(() =>
    readDevicePreferences(userId),
  );
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [deviceError, setDeviceError] = useState("");
  const mounted = useRef(false);
  const available = !!navigator.mediaDevices?.getUserMedia;
  const outputSupported = "setSinkId" in HTMLMediaElement.prototype;
  const refresh = useCallback(async () => {
    try {
      const found = await navigator.mediaDevices?.enumerateDevices();
      if (mounted.current && found) {
        setDevices(found);
        setDeviceError("");
      }
    } catch {
      if (mounted.current)
        setDeviceError(
          "Не удалось получить список устройств. Проверьте разрешения браузера.",
        );
    }
  }, []);
  useEffect(() => {
    setPreferences(readDevicePreferences(userId));
    return subscribeDevicePreferences(userId, setPreferences);
  }, [userId]);
  useEffect(() => {
    mounted.current = true;
    void refresh();
    navigator.mediaDevices?.addEventListener?.("devicechange", refresh);
    return () => {
      mounted.current = false;
      navigator.mediaDevices?.removeEventListener?.("devicechange", refresh);
    };
  }, [refresh]);
  function update(patch: Partial<DevicePreferences>) {
    const next = { ...preferences, ...patch };
    setPreferences(next);
    saveDevicePreferences(userId, next);
  }
  return {
    preferences,
    devices,
    available,
    outputSupported,
    deviceError,
    refresh,
    update,
  };
}

/** Панель владеет предпросмотром: запоздавшие ответы разрешений останавливаются после закрытия или смены вкладки. */
function usePreview(
  kind: "audio" | "video",
  enabled: boolean,
  deviceId: string,
  noiseSuppression: boolean,
  refresh: () => Promise<void>,
) {
  const [stream, setStream] = useState<MediaStream | null>(null);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const [loading, setLoading] = useState(false);
  useEffect(() => {
    let active = true;
    let owned: MediaStream | null = null;
    setStream(null);
    setError("");
    setLoading(enabled);
    if (enabled && navigator.mediaDevices?.getUserMedia) {
      const constraints = {
        ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
        ...(kind === "audio"
          ? { echoCancellation: true, noiseSuppression }
          : {}),
      };
      void navigator.mediaDevices
        .getUserMedia({
          audio: kind === "audio" ? constraints : false,
          video: kind === "video" ? constraints : false,
        })
        .then((next) => {
          if (!active) {
            next.getTracks().forEach((track) => track.stop());
            return;
          }
          owned = next;
          setStream(next);
          setLoading(false);
          void refresh();
        })
        .catch((reason: unknown) => {
          if (active) {
            setError(captureErrorMessage(reason, kind));
            setLoading(false);
          }
        });
    } else setLoading(false);
    return () => {
      active = false;
      owned?.getTracks().forEach((track) => track.stop());
    };
  }, [kind, enabled, deviceId, noiseSuppression, refresh, retry]);
  return {
    stream,
    error,
    loading,
    retry: () => setRetry((value) => value + 1),
  };
}

function LevelMeter({
  stream,
  playing = false,
  label,
}: {
  stream?: MediaStream | null;
  playing?: boolean;
  label: string;
}) {
  const [level, setLevel] = useState(0);
  useEffect(() => {
    setLevel(0);
    if (!stream || typeof AudioContext === "undefined") return;
    const context = new AudioContext();
    const source = context.createMediaStreamSource(stream);
    const analyser = context.createAnalyser();
    analyser.fftSize = 256;
    source.connect(analyser);
    const data = new Uint8Array(analyser.fftSize);
    void context.resume().catch(() => {});
    const timer = window.setInterval(() => {
      analyser.getByteTimeDomainData(data);
      const rms = Math.sqrt(
        data.reduce((sum, value) => sum + ((value - 128) / 128) ** 2, 0) /
          data.length,
      );
      setLevel(Math.min(8, Math.ceil(rms * 40)));
    }, 80);
    return () => {
      clearInterval(timer);
      source.disconnect();
      analyser.disconnect();
      void context.close().catch(() => {});
    };
  }, [stream]);
  return (
    <span
      className={`device-level ${playing ? "device-level-playing" : ""}`}
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={8}
      aria-valuenow={playing ? 8 : level}
    >
      {Array.from({ length: 8 }, (_, index) => (
        <i key={index} className={index < level ? "lit" : ""} />
      ))}
    </span>
  );
}

function Toggle({
  label,
  hint,
  checked,
  onChange,
  disabled = false,
}: {
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <label className="device-toggle-row">
      <span>
        {label}
        {hint && <small>{hint}</small>}
      </span>
      <input
        type="checkbox"
        role="switch"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className="device-switch" aria-hidden="true" />
    </label>
  );
}

function DeviceSelect({
  devices,
  kind,
  value,
  onChange,
  label,
  disabled,
}: {
  devices: MediaDeviceInfo[];
  kind: MediaDeviceKind;
  value: string;
  onChange: (id: string) => void;
  label: string;
  disabled?: boolean;
}) {
  const options = devices.filter(
    (device) =>
      device.kind === kind &&
      device.deviceId &&
      !["default", "communications"].includes(device.deviceId),
  );
  const defaultLabel = devices
    .find((device) => device.kind === kind && device.deviceId === "default")
    ?.label.replace(/^(default|по умолчанию)\s*[-–—]?\s*/i, "");
  const selected = availableDeviceId(devices, kind, value);
  return (
    <select
      className="device-select"
      aria-label={label}
      value={selected}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
    >
      <option value="">
        По умолчанию{defaultLabel ? ` — ${defaultLabel}` : ""}
      </option>
      {selected && !options.some((item) => item.deviceId === selected) && (
        <option value={selected}>Выбранное устройство</option>
      )}
      {options.map((device, index) => (
        <option key={device.deviceId} value={device.deviceId}>
          {device.label || `${label} ${index + 1}`}
        </option>
      ))}
    </select>
  );
}

function SpeakerTest({
  sinkId,
  label,
  onPlaying,
}: {
  sinkId: string;
  label: string;
  onPlaying?: (value: boolean) => void;
}) {
  const [playing, setPlaying] = useState(false);
  const [error, setError] = useState("");
  const cancel = useRef<(() => void) | null>(null);
  useEffect(
    () => () => {
      cancel.current?.();
      onPlaying?.(false);
    },
    [sinkId, onPlaying],
  );
  function test() {
    if (cancel.current) {
      cancel.current();
      cancel.current = null;
      return;
    }
    setError("");
    setPlaying(true);
    onPlaying?.(true);
    cancel.current = playDeviceTone(sinkId, (reason) => {
      cancel.current = null;
      setPlaying(false);
      onPlaying?.(false);
      if (reason)
        setError(
          "Не удалось воспроизвести звук. Проверьте выбранный динамик и разрешения браузера.",
        );
    });
  }
  return (
    <>
      <button
        type="button"
        className="device-test"
        aria-label={playing ? `Остановить: ${label}` : `Проверить: ${label}`}
        onClick={test}
      >
        {playing ? <Square size={16} /> : <Play size={16} />}{" "}
        {playing ? "Остановить" : "Проверить"}
      </button>
      {error && (
        <p className="device-error" role="alert">
          {error}
        </p>
      )}
    </>
  );
}

export function AudioSettings() {
  const settings = useSettings();
  const { preferences, devices, update, available, outputSupported } = settings;
  const [checking, setChecking] = useState(false);
  const [speakerPlaying, setSpeakerPlaying] = useState(false);
  const preview = usePreview(
    "audio",
    checking,
    availableDeviceId(devices, "audioinput", preferences.audioInputId),
    preferences.noiseSuppression,
    settings.refresh,
  );
  const noiseSupported =
    !!navigator.mediaDevices?.getSupportedConstraints?.().noiseSuppression;
  async function chooseOutput(field: "audioOutputId" | "notificationOutputId") {
    const media = navigator.mediaDevices as MediaDevices & {
      selectAudioOutput?: (options?: {
        deviceId: string;
      }) => Promise<MediaDeviceInfo>;
    };
    if (!media.selectAudioOutput) return;
    try {
      const chosen = await media.selectAudioOutput({
        deviceId: preferences[field],
      });
      update({ [field]: chosen.deviceId });
      await settings.refresh();
    } catch {
      /* Отмена выбора устройства в браузере сохраняет прежнее значение. */
    }
  }
  const canChooseOutput = !!(
    navigator.mediaDevices as MediaDevices & { selectAudioOutput?: unknown }
  )?.selectAudioOutput;
  return (
    <div className="device-settings" id="audio-settings">
      <div className="account-settings-section-heading">
        <h2>Аудио</h2>
        <p>Подготовьте звук к следующей встрече</p>
      </div>
      {!available && (
        <p role="status">
          Разрешите доступ к устройствам в браузере. Настройки доступны через
          HTTPS.
        </p>
      )}
      {settings.deviceError && <p role="alert">{settings.deviceError}</p>}
      <section className="device-card" aria-labelledby="microphone-title">
        <header>
          <h2 id="microphone-title">Микрофон</h2>
          <LevelMeter stream={preview.stream} label="Уровень микрофона" />
        </header>
        <div className="device-select-row">
          <DeviceSelect
            devices={devices}
            kind="audioinput"
            label="Микрофон"
            value={preferences.audioInputId}
            onChange={(id) => update({ audioInputId: id })}
            disabled={!available}
          />
          <button
            type="button"
            className="device-test"
            disabled={!available}
            onClick={() => {
              if (preview.error) preview.retry();
              else setChecking(!checking);
            }}
            aria-label={
              checking && !preview.error
                ? "Остановить проверку микрофона"
                : "Проверить микрофон"
            }
          >
            {checking && !preview.error ? (
              <Square size={16} />
            ) : (
              <Mic size={16} />
            )}
            {checking && !preview.error ? "Остановить" : "Проверить"}
          </button>
        </div>
        <p className="device-hint" role="status">
          {preview.loading
            ? "Ожидаем разрешение на доступ к микрофону…"
            : preview.stream
              ? "Говорите — индикатор показывает уровень вашего голоса."
              : "Проверьте микрофон, чтобы увидеть уровень звука и названия устройств."}
        </p>
        {preview.error && (
          <p className="device-error" role="alert">
            {preview.error}
          </p>
        )}
        <Toggle
          label="Подключаться с выключенным микрофоном"
          checked={!preferences.microphoneEnabled}
          onChange={(muted) => update({ microphoneEnabled: !muted })}
        />
        <Toggle
          label="Шумоподавление"
          checked={preferences.noiseSuppression}
          disabled={!noiseSupported}
          onChange={(noiseSuppression) => update({ noiseSuppression })}
          hint={!noiseSupported ? "Недоступно в этом браузере" : undefined}
        />
      </section>
      <section className="device-card" aria-labelledby="speaker-title">
        <header>
          <h2 id="speaker-title">Динамик</h2>
          <LevelMeter label="Тестовый звук" playing={speakerPlaying} />
        </header>
        <div className="device-select-row">
          <DeviceSelect
            devices={devices}
            kind="audiooutput"
            label="Динамик"
            value={preferences.audioOutputId}
            onChange={(id) => update({ audioOutputId: id })}
            disabled={!outputSupported}
          />
          <SpeakerTest
            sinkId={preferences.audioOutputId}
            label="динамик"
            onPlaying={setSpeakerPlaying}
          />
        </div>
        {canChooseOutput && (
          <button
            type="button"
            className="device-output-picker"
            onClick={() => void chooseOutput("audioOutputId")}
          >
            Выбрать другое устройство
          </button>
        )}
        {!outputSupported && (
          <p className="device-hint">
            Динамик выбирается в настройках системы или браузера.
          </p>
        )}
      </section>
      <section className="device-card" aria-labelledby="ringtone-title">
        <header>
          <h2 id="ringtone-title">Звук приглашения</h2>
        </header>
        <p className="device-hint">
          Уведомления о приглашениях и скором начале встречи.
        </p>
        <label className="device-output-label">Источник звука</label>
        <div className="device-select-row">
          <DeviceSelect
            devices={devices}
            kind="audiooutput"
            label="Источник звука уведомлений"
            value={preferences.notificationOutputId}
            onChange={(id) => update({ notificationOutputId: id })}
            disabled={!outputSupported}
          />
          <SpeakerTest
            sinkId={preferences.notificationOutputId}
            label="звук приглашения"
          />
        </div>
        {canChooseOutput && (
          <button
            type="button"
            className="device-output-picker"
            onClick={() => void chooseOutput("notificationOutputId")}
          >
            Выбрать другое устройство
          </button>
        )}
      </section>
      <p className="device-saved-hint">
        Настройки сохраняются автоматически в этом браузере.
      </p>
    </div>
  );
}

export function VideoSettings() {
  const settings = useSettings();
  const { preferences, devices, update, available } = settings;
  const preview = usePreview(
    "video",
    available,
    availableDeviceId(devices, "videoinput", preferences.videoInputId),
    true,
    settings.refresh,
  );
  const video = useRef<HTMLVideoElement>(null);
  useEffect(() => {
    const element = video.current;
    if (!element) return;
    element.srcObject = preview.stream;
    if (preview.stream) void element.play().catch(() => {});
    return () => {
      element.srcObject = null;
    };
  }, [preview.stream]);
  return (
    <div className="device-settings" id="video-settings">
      <section className="device-card" aria-labelledby="camera-title">
        <header>
          <div className="account-settings-section-heading">
            <h2 id="camera-title">Видео</h2>
            <p>Предпросмотр доступен только вам</p>
          </div>
        </header>
        <div className="device-camera-preview">
          <video
            ref={video}
            autoPlay
            muted
            playsInline
            aria-label="Предпросмотр камеры"
          />
          {!preview.stream && (
            <div className="device-camera-empty">
              <VideoOff size={32} />
              <span>
                {preview.loading ? "Подключаем камеру…" : "Предпросмотр камеры"}
              </span>
            </div>
          )}
        </div>
        {preview.error && (
          <div className="device-error" role="alert">
            {preview.error}
            <button
              type="button"
              className="device-test"
              onClick={preview.retry}
            >
              Повторить
            </button>
          </div>
        )}
        {!available && (
          <p role="status">
            Камера недоступна. Проверьте разрешения браузера и
            HTTPS-подключение.
          </p>
        )}
        {settings.deviceError && <p role="alert">{settings.deviceError}</p>}
        <div className="device-camera-select">
          <span>Камера</span>
          <DeviceSelect
            devices={devices}
            kind="videoinput"
            label="Камера"
            value={preferences.videoInputId}
            onChange={(id) => update({ videoInputId: id })}
            disabled={!available}
          />
        </div>
        <Toggle
          label="Подключаться с выключенной камерой"
          checked={!preferences.cameraEnabled}
          onChange={(muted) => update({ cameraEnabled: !muted })}
        />
        <Toggle
          label="Видеть себя на звонке"
          hint="Ваше видео или аватар"
          checked={preferences.showSelf}
          onChange={(showSelf) => update({ showSelf })}
        />
        <Toggle
          label="Скрыть видео участников"
          hint="Снижает нагрузку на сеть. Звук сохраняется; видео и экраны скрываются."
          checked={preferences.hideParticipantVideo}
          onChange={(hideParticipantVideo) => update({ hideParticipantVideo })}
        />
      </section>
      <p className="device-saved-hint">
        Настройки сохраняются автоматически в этом браузере.
      </p>
    </div>
  );
}
