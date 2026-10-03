import type { ConferenceRecording } from "./types";
import { formatDate } from "./utils";

/** RecordingFile описывает реальный артефакт, разрешённый сервером для чтения.
 * @params fileType — вид материала; url — подписанная ссылка; sizeBytes — известный размер.
 */
export type RecordingFile = ConferenceRecording["files"][number];

/** safeRecordingUrl проверяет ссылку из ответа backend перед показом медиа или скачиванием.
 * Не создаёт ссылку из ключа хранилища и отвергает исполняемые схемы, credentials
 * и неоднозначные адреса; исходная подпись URL остаётся без изменений.
 * @args value — подписанная ссылка, полученная из API.
 * @return безопасный HTTP(S)-адрес или локальный путь; undefined для недопустимой ссылки.
 */
export function safeRecordingUrl(value?: string): string | undefined {
  if (
    !value ||
    value !== value.trim() ||
    /[\u0000-\u0020\u007f\\]/u.test(value)
  )
    return undefined;
  if (value.startsWith("/") && !value.startsWith("//")) return value;
  if (!/^https?:\/\//i.test(value)) return undefined;
  try {
    const url = new URL(value);
    if (
      !["https:", "http:"].includes(url.protocol) ||
      url.username ||
      url.password
    )
      return undefined;
    return value;
  } catch {
    return undefined;
  }
}

/** recordingMediaFile выбирает доступный основной видео- или аудиофайл.
 * @args record — запись с файлами из защищённого API конференции.
 * @return видео при его наличии, иначе аудио; undefined, если безопасного файла нет.
 */
export function recordingMediaFile(
  record: ConferenceRecording,
): RecordingFile | undefined {
  const files = record.files || [];
  return ["final_mp4", "final_audio"].flatMap((type) =>
    files.filter(
      (file) => file.fileType === type && safeRecordingUrl(file.url),
    ),
  )[0];
}

/** recordingPreviewFile выбирает настоящее превью, не подставляя чужие фотографии.
 * @args record — запись с доступными артефактами.
 * @return разрешённое JPEG-превью или undefined, если сервер не создал изображение.
 */
export function recordingPreviewFile(
  record: ConferenceRecording,
): RecordingFile | undefined {
  return record.files?.find(
    (file) => file.fileType === "preview_jpg" && safeRecordingUrl(file.url),
  );
}

/** recordingDate форматирует фактическое начало записи с резервной датой её создания.
 * @args record — запись с серверными временными отметками.
 * @return дата и время на русском языке либо понятная подпись при отсутствии даты.
 */
export function recordingDate(record: ConferenceRecording): string {
  for (const value of [record.startedAt, record.createdAt])
    if (value && Number.isFinite(Date.parse(value))) return formatDate(value);
  return "Дата не указана";
}

/** formatRecordingDuration показывает длительность без округления до целых минут.
 * @args seconds — измеренная сервером длительность в секундах, если она известна.
 * @return M:SS или H:MM:SS; прочерк, когда сервер не сообщил корректное значение.
 */
export function formatRecordingDuration(seconds?: number | null): string {
  if (
    seconds === undefined ||
    seconds === null ||
    !Number.isFinite(seconds) ||
    seconds < 0
  )
    return "—";
  const total = Math.floor(seconds);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const rest = String(total % 60).padStart(2, "0");
  return hours
    ? `${hours}:${String(minutes).padStart(2, "0")}:${rest}`
    : `${minutes}:${rest}`;
}

/** formatRecordingSize форматирует известный размер без вымышленных чисел.
 * @args bytes — размер артефакта в байтах, возвращённый backend.
 * @return размер в Б/КБ/МБ/ГБ/ТБ или прочерк при неизвестном значении.
 */
export function formatRecordingSize(bytes?: number | null): string {
  if (
    bytes === undefined ||
    bytes === null ||
    !Number.isFinite(bytes) ||
    bytes < 0
  )
    return "—";
  const units = ["Б", "КБ", "МБ", "ГБ", "ТБ"];
  const exponent = bytes
    ? Math.max(0, Math.min(4, Math.floor(Math.log(bytes) / Math.log(1024))))
    : 0;
  const value = bytes / 1024 ** exponent;
  return `${new Intl.NumberFormat("ru-RU", { maximumFractionDigits: value < 10 ? 1 : 0 }).format(value)} ${units[exponent]}`;
}

/** recordingDetailPath связывает UUID записи с конференцией для существующего API.
 * @args conferenceId — конференция, проверяемая backend; recordingId — UUID записи.
 * @return локальная ссылка на отдельный просмотр записи с сохранённым контекстом.
 */
export function recordingDetailPath(
  conferenceId: string,
  recordingId: string,
): string {
  return `/recordings/${encodeURIComponent(recordingId)}?${new URLSearchParams({ conference: conferenceId })}`;
}

/** playableRecordings скрывает технические состояния и повторные строки соседних страниц.
 * @args records — уже загруженные записи только выбранной конференции.
 * @return уникальные готовые записи с реальным безопасным видео- или аудиофайлом.
 */
export function playableRecordings(
  records: readonly ConferenceRecording[],
): ConferenceRecording[] {
  const unique = new Map<string, ConferenceRecording>();
  for (const record of records) unique.set(record.uuid, record);
  return [...unique.values()].filter(
    (record) => record.status === "ready" && recordingMediaFile(record),
  );
}
