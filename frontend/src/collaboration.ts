import type { ChatMessage, Participant } from "./types";

// Legacy rows are admitted only when their existing membership is not restricted.
export function isAdmitted(member?: Participant | null): boolean {
  return (
    !!member &&
    (member.admissionState
      ? member.admissionState === "admitted"
      : ["joined", "left"].includes(member.status))
  );
}

export function mergeChatPages(
  pages: { items: ChatMessage[] }[],
): ChatMessage[] {
  const items = new Map<string, ChatMessage>();
  for (const page of pages)
    for (const item of page.items) {
      if (!items.has(item.id) || items.get(item.id)!.version < item.version)
        items.set(item.id, item);
    }
  return [...items.values()].sort((a, b) => {
    const left = BigInt(a.sequence),
      right = BigInt(b.sequence);
    return left < right ? -1 : left > right ? 1 : a.id.localeCompare(b.id);
  });
}

export function localSchedule(value: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return null;
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return null;
  // Reject nonexistent local wall times instead of silently moving them across DST.
  if (toLocalInput(date.toISOString()) !== value) return null;
  return date.toISOString();
}
export function toLocalInput(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return "";
  const pad = (number: number) => String(number).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
export function localDayEnd(value: string): string | null {
  const start = localSchedule(`${value}T00:00`);
  if (!start) return null;
  const date = new Date(start);
  date.setHours(23, 59, 59, 999);
  return date.toISOString();
}
export function formatBytes(value: number): string {
  return value < 1024 * 1024
    ? `${Math.ceil(value / 1024)} КБ`
    : `${(value / 1024 / 1024).toFixed(1)} МБ`;
}
