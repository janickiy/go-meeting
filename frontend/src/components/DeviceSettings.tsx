import { useEffect, useState } from "react";
import { useAuth } from "../auth";
import {
  availableDeviceId,
  readDevicePreferences,
  saveDevicePreferences,
} from "../prejoinDevices";
import type { DevicePreferences } from "../prejoinDevices";

/** Локальные предпочтения устройств применяются на экране предварительного входа с проверкой доступности. */
export function DeviceSettings() {
  const { user } = useAuth();
  const userId = user?.id || "";
  const [preferences, setPreferences] = useState<DevicePreferences>(() =>
    readDevicePreferences(userId),
  );
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [error, setError] = useState("");
  const mediaAvailable =
    typeof navigator !== "undefined" &&
    !!navigator.mediaDevices?.enumerateDevices;
  const outputSupported =
    typeof HTMLMediaElement !== "undefined" &&
    "setSinkId" in HTMLMediaElement.prototype;

  useEffect(() => {
    setPreferences(readDevicePreferences(userId));
  }, [userId]);
  useEffect(() => {
    if (!mediaAvailable) return;
    let active = true;
    const refresh = () => {
      void navigator.mediaDevices
        .enumerateDevices()
        .then((found) => {
          if (active) {
            setDevices(found);
            setError("");
          }
        })
        .catch(() => {
          if (active)
            setError(
              "Не удалось получить список устройств. Проверьте разрешения браузера.",
            );
        });
    };
    refresh();
    navigator.mediaDevices.addEventListener?.("devicechange", refresh);
    return () => {
      active = false;
      navigator.mediaDevices.removeEventListener?.("devicechange", refresh);
    };
  }, [mediaAvailable]);

  function update(patch: Partial<DevicePreferences>) {
    const next = { ...preferences, ...patch };
    setPreferences(next);
    saveDevicePreferences(userId, next);
  }

  function selector(
    kind: MediaDeviceKind,
    label: string,
    field: keyof Pick<
      DevicePreferences,
      "audioInputId" | "videoInputId" | "audioOutputId"
    >,
  ) {
    const available = devices.filter((device) => device.kind === kind);
    const selected = availableDeviceId(devices, kind, preferences[field]);
    return (
      <label className="field" key={field}>
        <span>{label}</span>
        <select
          value={selected}
          onChange={(event) => update({ [field]: event.target.value })}
        >
          <option value="">Системное устройство</option>
          {available.map((device, index) => (
            <option
              key={device.deviceId || `${kind}-${index}`}
              value={device.deviceId}
            >
              {device.label || `${label} ${index + 1}`}
            </option>
          ))}
        </select>
      </label>
    );
  }

  return (
    <section
      className="content-card"
      id="audio-video"
      aria-labelledby="audio-video-title"
    >
      <h2 id="audio-video-title">Аудио и видео</h2>
      <p className="field-hint">
        Выбор сохраняется только в этом браузере. Перед каждой встречей
        доступность устройств проверяется заново.
      </p>
      {!mediaAvailable && (
        <p role="status">
          Настройки устройств доступны через HTTPS или localhost в
          поддерживаемом браузере.
        </p>
      )}
      {error && <p role="alert">{error}</p>}
      {mediaAvailable && (
        <>
          {selector("audioinput", "Микрофон", "audioInputId")}
          {selector("videoinput", "Камера", "videoInputId")}
          {outputSupported &&
            selector("audiooutput", "Динамики", "audioOutputId")}
          {!outputSupported && (
            <p className="field-hint">
              Выбор динамиков не поддерживается этим браузером.
            </p>
          )}
          <label className="checkbox-field">
            <input
              type="checkbox"
              checked={preferences.microphoneEnabled}
              onChange={(event) =>
                update({ microphoneEnabled: event.target.checked })
              }
            />
            <span>Включать микрофон при подключении медиасвязи</span>
          </label>
          <label className="checkbox-field">
            <input
              type="checkbox"
              checked={preferences.cameraEnabled}
              onChange={(event) =>
                update({ cameraEnabled: event.target.checked })
              }
            />
            <span>Включать камеру при подключении медиасвязи</span>
          </label>
        </>
      )}
    </section>
  );
}
