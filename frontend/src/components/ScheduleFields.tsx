import { localSchedule } from "../collaboration";

/**
 * ScheduleFields показывает управляемые поля локального времени и плановой длительности встречи.
 *
 * @parameters:
 *   - объект параметров: value — значение для проверки, преобразования или отображения; onChange — обработчик изменения управляемого значения; duration — свойство текущего компонента; onDuration — свойство текущего компонента; disabled — запрещает действие в текущем состоянии.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function ScheduleFields({
  value,
  onChange,
  duration,
  onDuration,
  disabled = false,
}: {
  value: string;
  onChange: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в конференциях, расписании и истории.
   *
   * @parameters:
   *   - value (string) — значение для проверки, преобразования или отображения.
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ (value: string) => void;
  duration: string;
  onDuration: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в конференциях, расписании и истории.
   *
   * @parameters:
   *   - value (string) — значение для проверки, преобразования или отображения.
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ (value: string) => void;
  disabled?: boolean;
}) {
  const utc = localSchedule(value);
  return (
    <div className="schedule-fields">
      <label className="field" htmlFor="scheduled-at">
        Дата и время
        <input
          id="scheduled-at"
          type="datetime-local"
          value={value}
          onChange={
            /**
             * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             * @parameters:
             *   - event — проверенный конверт события комнаты.
             *
             * @returns вычисленное значение: onChange(event.target.value).
             */ (event) => onChange(event.target.value)
          }
          disabled={disabled}
        />
      </label>
      <p className="field-hint">
        Часовой пояс: {Intl.DateTimeFormat().resolvedOptions().timeZone}.{" "}
        {utc
          ? `Время UTC: ${utc.replace("T", " ").replace(".000Z", " UTC")}.`
          : "Укажите дату и время встречи."}
      </p>
      <label className="field" htmlFor="planned-duration">
        Плановая длительность, минуты{" "}
        <span className="muted">(необязательно)</span>
        <input
          id="planned-duration"
          type="number"
          min={1}
          max={1440}
          value={duration}
          onChange={
            /**
             * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             * @parameters:
             *   - event — проверенный конверт события комнаты.
             *
             * @returns вычисленное значение: onDuration(event.target.value).
             */ (event) => onDuration(event.target.value)
          }
          disabled={disabled}
          placeholder="60"
        />
      </label>
    </div>
  );
}
