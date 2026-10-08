import { useEffect, useState } from "react";

/** Обновляет только длительность встречи, не перерисовывая сетку видеопотоков.
 * @args startedAt — фактическое серверное начало; @return доступный таймер или отсутствие значения.
 */
export function MeetingClock({ startedAt }: { startedAt?: string | null }) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);
  const start = Date.parse(startedAt || "");
  if (!Number.isFinite(start)) return null;
  const seconds = Math.max(0, Math.floor((now - start) / 1000));
  return (
    <span className="room-clock" aria-label="Длительность встречи">
      {[Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60]
        .map((part) => String(part).padStart(2, "0"))
        .join(":")}
    </span>
  );
}
