import type { Caption } from "./types";

/** mergeCaptions заменяет реплику по версии, сохраняя final при перестановке сетевых событий.
 * @args current — видимые реплики; incoming — проверенные события одной встречи.
 * @return Последние 200 реплик в порядке времени, без накапливания промежуточных редакций.
 */
export function mergeCaptions(
  current: Caption[],
  incoming: Caption[],
): Caption[] {
  const entries = new Map(current.map((item) => [item.id, item]));
  let changed = false;
  for (const next of incoming) {
    if (
      !next ||
      typeof next.id !== "string" ||
      typeof next.text !== "string" ||
      next.text.length > 16000 ||
      !Number.isSafeInteger(next.sequence) ||
      !Number.isSafeInteger(next.revision) ||
      !Number.isFinite(next.startMs) ||
      !Number.isFinite(next.endMs) ||
      next.startMs < 0 ||
      next.endMs < next.startMs
    )
      continue;
    const old = entries.get(next.id);
    if (
      old &&
      (next.sequence < old.sequence ||
        next.revision < old.revision ||
        (old.final && !next.final) ||
        (next.revision === old.revision && (old.final || !next.final)))
    )
      continue;
    entries.set(next.id, next);
    changed = true;
  }
  if (!changed) return current;
  return [...entries.values()]
    .sort((a, b) => a.startMs - b.startMs || a.id.localeCompare(b.id))
    .slice(-200);
}
