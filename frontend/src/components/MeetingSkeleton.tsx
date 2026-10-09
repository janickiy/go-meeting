import "./meeting-skeleton.css";

/**
 * MeetingSkeleton сохраняет геометрию содержимого, пока сервер загружает встречи.
 * @args rows — количество декоративных строк; label — доступное описание ожидания.
 * @return Состояние загрузки без подстановки вымышленных встреч или участников.
 */
export function MeetingSkeleton({
  rows = 3,
  label = "Загрузка встреч",
}: {
  rows?: number;
  label?: string;
}) {
  return (
    <div
      className="meetings-skeleton"
      role="status"
      aria-label={label}
      aria-busy="true"
    >
      {Array.from({ length: rows }, (_, row) => (
        <div className="meetings-skeleton-card" key={row} aria-hidden="true">
          <span className="meetings-skeleton-block" />
          <div>
            <span />
            <span />
            <span />
          </div>
        </div>
      ))}
    </div>
  );
}
