import { localSchedule } from "../collaboration";

export function ScheduleFields({
  value,
  onChange,
  duration,
  onDuration,
  disabled = false,
}: {
  value: string;
  onChange: (value: string) => void;
  duration: string;
  onDuration: (value: string) => void;
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
          onChange={(event) => onChange(event.target.value)}
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
          onChange={(event) => onDuration(event.target.value)}
          disabled={disabled}
          placeholder="60"
        />
      </label>
    </div>
  );
}
