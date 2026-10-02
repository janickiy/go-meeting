import type { Notification, SearchResult } from "./types";

/**
 * Форматирует смещение записи, не принимая отрицательные и бесконечные значения.
 * @parameters milliseconds — смещение в миллисекундах.
 * @return Подпись времени для кнопки перехода.
 */
export function recordingTime(milliseconds: number): string {
  const seconds = Math.floor(
    Math.max(0, Number.isFinite(milliseconds) ? milliseconds : 0) / 1000,
  );
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours ? `${hours}:` : ""}${String(minutes).padStart(2, "0")}:${String(seconds % 60).padStart(2, "0")}`;
}

/**
 * Строит внутреннюю ссылку без вставки текста результата в адрес или HTML.
 * @parameters result — разрешённый сервером результат поиска.
 * @return Маршрут встречи с выбранной записью и проверенным временем.
 */
export function searchResultLink(result: SearchResult): string {
  const params = new URLSearchParams();
  if (result.recordingId) {
    params.set("recording", result.recordingId);
    params.set("tab", result.type === "summary" ? "summary" : "transcript");
    if (
      typeof result.startMs === "number" &&
      Number.isFinite(result.startMs) &&
      result.startMs >= 0
    )
      params.set("t", String(result.startMs));
    if (result.segmentId) params.set("segment", result.segmentId);
  }
  return `/conferences/${encodeURIComponent(result.conferenceId)}${params.size ? `?${params}` : ""}`;
}

/**
 * Открывает нужный материал по уведомлению, не выдавая дополнительных прав.
 * @parameters notification — личное уведомление со ссылочными идентификаторами.
 * @return Безопасный внутренний маршрут.
 */
export function notificationLink(notification: Notification): string {
  return searchResultLink({
    type:
      notification.type.includes("summary") || notification.payload.summaryId
        ? "summary"
        : "transcript",
    conferenceId: notification.payload.conferenceId,
    conferenceTitle: "",
    recordingId: notification.payload.recordingId,
    snippet: "",
    rank: 0,
  });
}
