const chatTextLimit = 4000;

/**
 * chatDayKey возвращает локальную календарную дату для группировки сообщений.
 *
 * @args
 *   - value (string) — серверная временная отметка сообщения.
 * @return string — ключ YYYY-MM-DD либо пустая строка для неверной даты.
 */
export function chatDayKey(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "";
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

/**
 * chatDayLabel подписывает локальный день без предположения, что сутки длятся 24 часа.
 *
 * @args
 *   - value (string) — серверная временная отметка сообщения.
 *   - now (Date) — текущая дата для подписей «Сегодня», «Вчера» и показа года.
 * @return string — русская подпись дня либо пустая строка для неверной даты.
 */
export function chatDayLabel(value: string, now: Date = new Date()): string {
  const date = new Date(value);
  const key = chatDayKey(value);
  if (!key) return "";
  if (Number.isFinite(now.getTime())) {
    if (key === chatDayKey(now.toISOString())) return "Сегодня";
    const yesterday = new Date(now);
    yesterday.setDate(yesterday.getDate() - 1);
    if (key === chatDayKey(yesterday.toISOString())) return "Вчера";
  }
  return date.toLocaleDateString("ru-RU", {
    day: "numeric",
    month: "long",
    ...(date.getFullYear() !== now.getFullYear()
      ? { year: "numeric" as const }
      : {}),
  });
}

/**
 * chatTime показывает локальное время сообщения в 24-часовом формате.
 *
 * @args
 *   - value (string) — серверная временная отметка сообщения.
 * @return string — HH:mm либо пустая строка для неверной даты.
 */
export function chatTime(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "";
  return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

/**
 * selectionPosition ограничивает позицию индексами UTF-16 строки.
 *
 * @args
 *   - position (number) — запрошенная позиция курсора или границы выделения.
 *   - length (number) — длина строки в кодовых единицах UTF-16.
 * @return number — целая позиция внутри строки; NaN трактуется как её начало.
 */
function selectionPosition(position: number, length: number): number {
  return Number.isNaN(position)
    ? 0
    : Math.max(0, Math.min(length, Math.trunc(position)));
}

/**
 * splitsSurrogatePair проверяет, проходит ли граница посередине Unicode-символа.
 *
 * @args
 *   - text (string) — исходный текст сообщения.
 *   - position (number) — безопасная позиция в кодовых единицах UTF-16.
 * @return boolean — true, когда слева старший, а справа младший суррогат.
 */
function splitsSurrogatePair(text: string, position: number): boolean {
  const before = text.charCodeAt(position - 1);
  const after = text.charCodeAt(position);
  return (
    before >= 0xd800 && before <= 0xdbff && after >= 0xdc00 && after <= 0xdfff
  );
}

/**
 * insertChatEmoji вставляет смайлик вместо выделения, сохраняя лимит Unicode сервера.
 *
 * @args
 *   - text (string) — исходный текст сообщения.
 *   - emoji (string) — выбранный смайлик, в том числе последовательность с ZWJ.
 *   - selectionStart (number) — начало выделения в кодовых единицах UTF-16.
 *   - selectionEnd (number) — конец выделения в кодовых единицах UTF-16.
 * @return объект с текстом и UTF-16 позицией после смайлика либо null при превышении 4000 Unicode-символов.
 */
export function insertChatEmoji(
  text: string,
  emoji: string,
  selectionStart: number,
  selectionEnd: number,
): { text: string; caret: number } | null {
  const first = selectionPosition(selectionStart, text.length);
  const last = selectionPosition(selectionEnd, text.length);
  let start = Math.min(first, last);
  let end = Math.max(first, last);
  if (start === end) {
    if (splitsSurrogatePair(text, start)) start = end = start - 1;
  } else {
    if (splitsSurrogatePair(text, start)) start -= 1;
    if (splitsSurrogatePair(text, end)) end += 1;
  }
  const candidate = text.slice(0, start) + emoji + text.slice(end);
  if (Array.from(candidate.trim()).length > chatTextLimit) return null;
  return { text: candidate, caret: start + emoji.length };
}
