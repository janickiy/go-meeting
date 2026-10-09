import "./messaging-states.css";

/**
 * Сохраняет структуру списка, пока сервер загружает переписки, участников или папки.
 * @args rows — число декоративных строк, не являющихся реальными данными.
 * @return Доступный индикатор загрузки с нейтральными заполнителями вместо имён и сообщений.
 */
export function MessagingSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="messaging-skeleton" role="status">
      <span className="sr-only">Загружаем…</span>
      {Array.from({ length: rows }, (_, index) => (
        <div className="messaging-skeleton-row" key={index} aria-hidden="true">
          <i />
          <span>
            <b />
            <b />
          </span>
        </div>
      ))}
    </div>
  );
}
