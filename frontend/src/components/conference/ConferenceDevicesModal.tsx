import { useEffect, useRef, useState } from "react";
import { Volume2 } from "lucide-react";
import { useAuth } from "../../auth";
import { playDeviceTone } from "../../deviceSound";
import {
  readDevicePreferences,
  saveDevicePreferences,
  subscribeDevicePreferences,
  type DevicePreferences,
} from "../../prejoinDevices";
import { Button, ErrorNotice, Modal } from "../ui";

/** Настройки уже подключённых устройств: не открывают камеру и микрофон повторно.
 * @args onClose — закрывает окно; audioLevel — текущая громкость уже подключённого микрофона, от 0 до 1.
 * @return Выбор устройств, применяемый существующим медиасоединением через общие предпочтения.
 */
export function ConferenceDevicesModal({
  onClose,
  audioLevel = 0,
}: {
  onClose: () => void;
  audioLevel?: number;
}) {
  const { user } = useAuth();
  const userId = user?.id || "guest";
  const [preferences, setPreferences] = useState(() =>
    readDevicePreferences(userId),
  );
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [error, setError] = useState("");
  const [playing, setPlaying] = useState(false);
  const stopTone = useRef<(() => void) | null>(null);
  const level = Number.isFinite(audioLevel)
    ? Math.max(0, Math.min(1, audioLevel))
    : 0;
  const mounted = useRef(false);
  useEffect(() => {
    setPreferences(readDevicePreferences(userId));
    return subscribeDevicePreferences(userId, setPreferences);
  }, [userId]);
  useEffect(() => {
    mounted.current = true;
    let active = true;
    const refresh = async () => {
      try {
        const result = await navigator.mediaDevices?.enumerateDevices();
        if (active) {
          setDevices(result || []);
          setError(
            result ? "" : "Браузер не предоставляет доступ к устройствам.",
          );
        }
      } catch {
        if (active)
          setError(
            "Не удалось получить устройства. Проверьте разрешения браузера.",
          );
      }
    };
    void refresh();
    navigator.mediaDevices?.addEventListener?.("devicechange", refresh);
    return () => {
      active = false;
      mounted.current = false;
      navigator.mediaDevices?.removeEventListener?.("devicechange", refresh);
      stopTone.current?.();
    };
  }, []);
  function update(patch: Partial<DevicePreferences>) {
    const next = { ...preferences, ...patch };
    setPreferences(next);
    saveDevicePreferences(userId, next);
  }
  return (
    <Modal title="Настройки устройств" onClose={onClose}>
      <div className="conference-devices-dialog">
        <ErrorNotice>{error}</ErrorNotice>
        {(
          [
            ["audioinput", "audioInputId", "Микрофон"],
            ["audiooutput", "audioOutputId", "Динамики"],
            ["videoinput", "videoInputId", "Камера"],
          ] as const
        ).map(([kind, key, label]) => {
          const options = devices.filter(
            (item) =>
              item.kind === kind &&
              item.deviceId &&
              item.deviceId !== "default",
          );
          const selected = preferences[key];
          const outputUnsupported =
            kind === "audiooutput" &&
            !("setSinkId" in HTMLMediaElement.prototype);
          return (
            <label className="field" key={kind}>
              {label}
              <select
                aria-label={label}
                value={selected}
                disabled={outputUnsupported}
                onChange={(event) => update({ [key]: event.target.value })}
              >
                <option value="">Системное устройство</option>
                {selected &&
                  !options.some((item) => item.deviceId === selected) && (
                    <option value={selected}>
                      Выбранное устройство недоступно
                    </option>
                  )}
                {options.map((item, index) => (
                  <option key={item.deviceId} value={item.deviceId}>
                    {item.label || `${label} ${index + 1}`}
                  </option>
                ))}
              </select>
              {kind === "audioinput" && (
                <span
                  className="conference-device-level"
                  role="meter"
                  aria-label="Уровень микрофона"
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(level * 100)}
                >
                  {Array.from({ length: 10 }, (_, index) => (
                    <i
                      key={index}
                      className={
                        index < Math.ceil(level * 10) ? "active" : undefined
                      }
                    />
                  ))}
                </span>
              )}
              {outputUnsupported && (
                <span className="field-hint">
                  Выход звука выбирается в настройках системы.
                </span>
              )}
            </label>
          );
        })}
        <Button
          variant="outline"
          disabled={playing}
          onClick={() => {
            setPlaying(true);
            stopTone.current = playDeviceTone(
              preferences.audioOutputId,
              (reason) => {
                if (!mounted.current) return;
                setPlaying(false);
                if (reason) setError("Не удалось воспроизвести тестовый звук.");
              },
            );
          }}
        >
          <Volume2 size={18} />
          {playing ? "Воспроизводим…" : "Проверить звук"}
        </Button>
        <label className="conference-device-toggle">
          <input
            type="checkbox"
            checked={preferences.noiseSuppression}
            onChange={(event) =>
              update({ noiseSuppression: event.target.checked })
            }
          />
          Шумоподавление
        </label>
        <p className="field-hint">
          Выбор применяется к подключённым устройствам. Выключенные камера и
          микрофон не включаются автоматически.
        </p>
        <div className="conference-dialog-actions">
          <Button onClick={onClose}>Готово</Button>
        </div>
      </div>
    </Modal>
  );
}
