export interface DevicePreferences {
  audioInputId: string;
  videoInputId: string;
  audioOutputId: string;
  microphoneEnabled: boolean;
  cameraEnabled: boolean;
}

const emptyPreferences: DevicePreferences = {
  audioInputId: "",
  videoInputId: "",
  audioOutputId: "",
  microphoneEnabled: false,
  cameraEnabled: false,
};

const key = (userId: string) => `meet.devices.v1:${userId}`;
const safeId = (value: unknown) =>
  typeof value === "string" && value.length <= 512 ? value : "";

/** ID устройств используются только как предпочтения и не определяют личность участника. */
export function readDevicePreferences(userId: string): DevicePreferences {
  if (!userId) return { ...emptyPreferences };
  try {
    const data: unknown = JSON.parse(
      localStorage.getItem(key(userId)) || "null",
    );
    if (!data || typeof data !== "object") return { ...emptyPreferences };
    const value = data as Record<string, unknown>;
    return {
      audioInputId: safeId(value.audioInputId),
      videoInputId: safeId(value.videoInputId),
      audioOutputId: safeId(value.audioOutputId),
      microphoneEnabled: value.microphoneEnabled === true,
      cameraEnabled: value.cameraEnabled === true,
    };
  } catch {
    return { ...emptyPreferences };
  }
}

export function hasDevicePreferences(userId: string) {
  if (!userId) return false;
  try {
    return localStorage.getItem(key(userId)) !== null;
  } catch {
    return false;
  }
}

export function saveDevicePreferences(
  userId: string,
  preferences: DevicePreferences,
) {
  if (!userId) return;
  try {
    localStorage.setItem(key(userId), JSON.stringify(preferences));
  } catch {
    // Хранилище может быть отключено; встреча продолжает работать с устройствами по умолчанию.
  }
}

/** Сохраняет непрозрачное предпочтение, пока браузер не предоставит реальные ID устройств. */
export function availableDeviceId(
  devices: MediaDeviceInfo[],
  kind: MediaDeviceKind,
  preferredId: string,
) {
  if (!preferredId) return "";
  const known = devices.filter(
    (device) =>
      device.kind === kind &&
      device.deviceId &&
      device.deviceId !== "default" &&
      device.deviceId !== "communications",
  );
  return known.length &&
    !known.some((device) => device.deviceId === preferredId)
    ? ""
    : preferredId;
}

export function captureErrorMessage(error: unknown, kind: "audio" | "video") {
  const name =
    error &&
    typeof error === "object" &&
    "name" in error &&
    typeof error.name === "string"
      ? error.name
      : "";
  const label = kind === "audio" ? "микрофон" : "камеру";
  if (name === "NotAllowedError" || name === "PermissionDeniedError")
    return `Доступ к устройству запрещён. Разрешите ${label} для этого сайта в настройках браузера и повторите.`;
  if (name === "NotFoundError" || name === "DevicesNotFoundError")
    return `Устройство не найдено. Подключите ${label} или войдите без него.`;
  if (name === "NotReadableError" || name === "TrackStartError")
    return `Не удалось открыть ${label}. Возможно, устройство занято другим приложением.`;
  if (name === "OverconstrainedError")
    return `Выбранное устройство недоступно. Выберите другое или системное устройство.`;
  if (name === "SecurityError")
    return "Браузер запретил доступ к устройствам. Откройте страницу по HTTPS или через localhost.";
  return `Не удалось включить ${label}. Проверьте разрешения и попробуйте снова.`;
}
