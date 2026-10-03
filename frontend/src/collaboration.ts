import type { ChatMessage, Participant } from "./types";

/**
 * isAdmitted проверяет допуск участника; старые записи без admissionState доступны только при неограниченном членстве.
 *
 * @args
 *   - member (Participant | null) — членство участника; отсутствие значения означает отсутствие допуска (необязательный параметр).
 *
 * @returns boolean — true, если членство допускает доступ; false для отсутствующего или ограниченного членства.
 */
export function isAdmitted(member?: Participant | null): boolean {
  return (
    !!member &&
    (member.admissionState
      ? member.admissionState === "admitted"
      : ["joined", "left"].includes(member.status))
  );
}

/**
 * mergeChatPages объединяет страницы по UUID, выбирает новую версию и сортирует по номеру BigInt без потери точности.
 *
 * @args
 *   - pages ({ items: ChatMessage[] }[]) — загруженные страницы чата, которые могут содержать разные версии одного сообщения.
 *
 * @returns ChatMessage[] — список без повторов UUID с последней версией каждого сообщения, упорядоченный по sequence и ID.
 */
export function mergeChatPages(
  pages: { items: ChatMessage[] }[],
): ChatMessage[] {
  const items = new Map<string, ChatMessage>();
  for (const page of pages)
    for (const item of page.items) {
      if (!items.has(item.id) || items.get(item.id)!.version < item.version)
        items.set(item.id, item);
    }
  return [...items.values()].sort(
    /**
     * Обработчик sort сравнивает два элемента, определяя их порядок в итоговом списке.
     *
     * @args
     *   - a — первый сравниваемый элемент.
     *   - b — второй сравниваемый элемент.
     *
     * @returns отрицательное число для первого элемента перед вторым, ноль для равного порядка или положительное число для обратного порядка.
     */ (a, b) => {
      const left = BigInt(a.sequence),
        right = BigInt(b.sequence);
      return left < right ? -1 : left > right ? 1 : a.id.localeCompare(b.id);
    },
  );
}

/**
 * localSchedule преобразует локальное время формы в UTC ISO и отвергает несуществующее время перехода часового пояса.
 *
 * @args
 *   - value (string) — локальная дата и время из поля формы в формате YYYY-MM-DDTHH:mm.
 *
 * @returns string | null — UTC-время в формате ISO либо null для неверного или несуществующего локального времени.
 */
export function localSchedule(value: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return null;
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return null;
  // Несуществующее местное время при переводе часов отклоняется, а не сдвигается автоматически.
  if (toLocalInput(date.toISOString()) !== value) return null;
  return date.toISOString();
}
/**
 * toLocalInput преобразует однозначную временную отметку в локальный формат datetime-local.
 *
 * @args
 *   - value (string) — временная отметка ISO, преобразуемая в местное время пользователя.
 *
 * @returns string — локальная дата и время для поля datetime-local либо пустая строка при ошибке разбора.
 */
export function toLocalInput(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "";
  /**
   * pad добавляет ведущий ноль к числу для формата даты.
   *
   * @args
   *   - number (number) — числовая часть даты, дополняемая ведущим нулём.
   *
   * @returns строка числа длиной не менее двух символов с ведущим нулём при необходимости.
   */
  const pad = (number: number) => String(number).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
/**
 * localDayEnd возвращает конец выбранного локального дня с точностью до миллисекунды для верхней границы фильтра.
 *
 * @args
 *   - value (string) — локальная дата выбранного дня в формате YYYY-MM-DD.
 *
 * @returns string | null — UTC-время последней миллисекунды выбранного локального дня либо null при неверной дате.
 */
export function localDayEnd(value: string): string | null {
  const start = localSchedule(`${value}T00:00`);
  if (!start) return null;
  const date = new Date(start);
  date.setHours(23, 59, 59, 999);
  return date.toISOString();
}
/**
 * formatBytes переводит размер файла в короткую подпись КБ или МБ для интерфейса.
 *
 * @args
 *   - value (number) — размер файла в байтах.
 *
 * @returns string — округлённая подпись размера в КБ или МБ.
 */
export function formatBytes(value: number): string {
  return value < 1024 * 1024
    ? `${Math.ceil(value / 1024)} КБ`
    : `${(value / 1024 / 1024).toFixed(1)} МБ`;
}
