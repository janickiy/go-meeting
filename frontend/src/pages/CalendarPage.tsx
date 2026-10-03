import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRight,
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Plus,
  Video,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { useConferences } from "../queries";
import {
  Button,
  ErrorNotice,
  Loading,
  Modal,
  StatusBadge,
} from "../components/ui";
import { EditSchedule } from "../components/ConferenceModals";
import { formatDate } from "../utils";
import { toLocalInput } from "../collaboration";
import type { Conference } from "../types";
import "./calendar.css";

type CalendarView = "week" | "month";
const weekdays = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
const hours = Array.from({ length: 24 }, (_, hour) => hour);

/**
 * calendarDayKey возвращает местную дату, не сдвигая день через преобразование в UTC.
 * @args date — дата в часовом поясе браузера.
 * @return Устойчивый ключ года, месяца и дня для группировки встреч.
 */
export function calendarDayKey(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

/**
 * calendarRange считает локальные границы недели или шестинедельной сетки месяца.
 * @args anchor — выбранная дата; view — неделя либо месяц.
 * @return Дни сетки и UTC-границы серверного запроса, учитывающие переходы часового пояса.
 */
export function calendarRange(anchor: Date, view: CalendarView) {
  const start = new Date(
    anchor.getFullYear(),
    anchor.getMonth(),
    view === "month" ? 1 : anchor.getDate(),
  );
  start.setDate(start.getDate() - ((start.getDay() + 6) % 7));
  const count = view === "month" ? 42 : 7;
  const days = Array.from(
    { length: count },
    (_, offset) =>
      new Date(start.getFullYear(), start.getMonth(), start.getDate() + offset),
  );
  const end = new Date(
    start.getFullYear(),
    start.getMonth(),
    start.getDate() + count,
  );
  end.setMilliseconds(-1);
  return { days, from: start.toISOString(), to: end.toISOString() };
}

/**
 * scheduledMeetingDate отделяет встречи с настоящим расписанием от созданных без даты.
 * @args conference — строка серверного списка, которая может не иметь scheduledAt.
 * @return Корректная дата запланированного начала либо null без выдуманной замены на createdAt.
 */
export function scheduledMeetingDate(conference: Conference) {
  if (!conference.scheduledAt) return null;
  const date = new Date(conference.scheduledAt);
  return Number.isFinite(date.getTime()) ? date : null;
}

/**
 * CalendarMeeting показывает в сетке время и статус реальной встречи.
 * @args meeting — встреча сервера; onSelect — открытие её карточки.
 * @return Клавиатурно доступная кнопка с названием и полным временем встречи.
 */
function CalendarMeeting({
  meeting,
  onSelect,
}: {
  meeting: Conference;
  onSelect: (meeting: Conference) => void;
}) {
  const date = scheduledMeetingDate(meeting)!;
  const time = date.toLocaleTimeString("ru-RU", {
    hour: "2-digit",
    minute: "2-digit",
  });
  return (
    <button
      type="button"
      className={`calendar-event calendar-event-${meeting.status}`}
      onClick={() => onSelect(meeting)}
      aria-label={`${meeting.title}, ${formatDate(date.toISOString())}`}
    >
      <strong>{meeting.title}</strong>
      <span>
        {time}
        {meeting.plannedDurationMin
          ? ` · ${meeting.plannedDurationMin} мин`
          : ""}
        {meeting.status === "cancelled"
          ? " · Отменена"
          : meeting.status === "active"
            ? " · Идёт сейчас"
            : ""}
      </span>
    </button>
  );
}

/**
 * CalendarMeetingDetails даёт владельцу реальные операции переноса и подтверждённой отмены.
 * @args meeting — выбранная встреча; onClose — закрытие карточки; onEdit — открытие существующей формы расписания.
 * @return Доступный диалог с серверной проверкой операции и без оптимистического успеха.
 */
function CalendarMeetingDetails({
  meeting,
  onClose,
  onEdit,
}: {
  meeting: Conference;
  onClose: () => void;
  onEdit: (meeting: Conference) => void;
}) {
  const { user } = useAuth();
  const client = useQueryClient();
  const [confirmCancel, setConfirmCancel] = useState(false);
  const canChange =
    meeting.ownerId === user?.id &&
    ["created", "scheduled"].includes(meeting.status);
  const cancellation = useMutation({
    mutationFn: () => api.transition(meeting.id, "cancel"),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["conferences"] });
      void client.invalidateQueries({ queryKey: ["conference"] });
      onClose();
    },
  });
  const past = meeting.status === "finished" || meeting.status === "cancelled";
  return (
    <Modal
      title={confirmCancel ? "Отменить встречу?" : meeting.title}
      onClose={() => {
        if (!cancellation.isPending) onClose();
      }}
    >
      <div className="calendar-meeting-details">
        <StatusBadge status={meeting.status} />
        <p>
          <CalendarDays size={18} aria-hidden="true" />
          {formatDate(meeting.scheduledAt!)}
        </p>
        {meeting.plannedDurationMin && (
          <p>
            <Clock3 size={18} aria-hidden="true" />
            {meeting.plannedDurationMin} мин
          </p>
        )}
        <p className="field-hint">
          Часовой пояс: {Intl.DateTimeFormat().resolvedOptions().timeZone}.
          Запланированная встреча не начинается автоматически.
        </p>
        {confirmCancel ? (
          <>
            <p>
              Встреча «{meeting.title}» будет отменена для всех участников. Это
              действие нельзя отменить.
            </p>
            <ErrorNotice error={cancellation.error} />
            <Button
              variant="danger"
              busy={cancellation.isPending}
              onClick={() => cancellation.mutate()}
            >
              Да, отменить встречу
            </Button>
            <Button
              variant="secondary"
              disabled={cancellation.isPending}
              onClick={() => setConfirmCancel(false)}
            >
              Оставить встречу
            </Button>
          </>
        ) : (
          <>
            <Link
              className="button button-primary"
              to={past ? `/history/${meeting.id}` : `/meetings/${meeting.id}`}
            >
              <Video size={17} aria-hidden="true" />
              {past ? "Открыть историю" : "Открыть встречу"}
            </Link>
            {canChange && (
              <div className="calendar-meeting-actions">
                <Button variant="outline" onClick={() => onEdit(meeting)}>
                  Изменить расписание
                </Button>
                <Button
                  variant="secondary"
                  onClick={() => setConfirmCancel(true)}
                >
                  Отменить встречу
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}

/**
 * CalendarPage показывает недельное расписание и месяц из того же API, что список встреч.
 * @return Адаптивный календарь с явной пагинацией, местным временем, созданием и управлением встречами.
 */
export function CalendarPage() {
  const [anchor, setAnchor] = useState(() => new Date());
  const [view, setView] = useState<CalendarView>("week");
  const [selected, setSelected] = useState<Conference | null>(null);
  const [editing, setEditing] = useState<Conference | null>(null);
  const scroll = useRef<HTMLDivElement>(null);
  const range = useMemo(() => calendarRange(anchor, view), [anchor, view]);
  const query = useConferences({
    view: "upcoming",
    scope: "all",
    from: range.from,
    to: range.to,
  });
  const meetings = useMemo(() => {
    const unique = new Map<string, Conference>();
    for (const meeting of query.data?.pages.flatMap((page) => page.items) ||
      []) {
      const date = scheduledMeetingDate(meeting);
      if (
        date &&
        date.getTime() >= new Date(range.from).getTime() &&
        date.getTime() <= new Date(range.to).getTime()
      )
        unique.set(meeting.id, meeting);
    }
    return [...unique.values()].sort(
      (left, right) =>
        scheduledMeetingDate(left)!.getTime() -
        scheduledMeetingDate(right)!.getTime(),
    );
  }, [query.data, range.from, range.to]);
  const byDay = useMemo(() => {
    const grouped = new Map<string, Conference[]>();
    for (const meeting of meetings) {
      const key = calendarDayKey(scheduledMeetingDate(meeting)!);
      grouped.set(key, [...(grouped.get(key) || []), meeting]);
    }
    return grouped;
  }, [meetings]);
  useEffect(() => {
    if (view === "week" && scroll.current) scroll.current.scrollTop = 52 * 8;
  }, [range.from, view, query.isPending]);

  /** @args direction — предыдущий или следующий период. Сохраняет локальные границы при смене месяца и DST. */
  function move(direction: number) {
    setAnchor((date) =>
      view === "week"
        ? new Date(
            date.getFullYear(),
            date.getMonth(),
            date.getDate() + direction * 7,
          )
        : new Date(date.getFullYear(), date.getMonth() + direction, 1),
    );
  }

  const label =
    view === "month"
      ? anchor.toLocaleDateString("ru-RU", { month: "long", year: "numeric" })
      : `${range.days[0].toLocaleDateString("ru-RU", { day: "numeric", month: "short" })} — ${range.days[6].toLocaleDateString("ru-RU", { day: "numeric", month: "short", year: "numeric" })}`;
  const today = calendarDayKey(new Date());
  return (
    <div className="calendar-page">
      <section className="page-heading">
        <div>
          <span className="eyebrow">ПЛАНИРУЙТЕ ВАЖНОЕ</span>
          <h1>Календарь</h1>
          <p>Ваши встречи — в удобном ритме.</p>
        </div>
        <Link
          className="button button-primary"
          to="/meetings/new?scheduled=1&returnTo=calendar"
        >
          <Plus size={18} aria-hidden="true" />
          Новая встреча
        </Link>
      </section>
      <section className="calendar-card" aria-label="Календарь встреч">
        <div className="calendar-toolbar">
          <div className="calendar-navigation">
            <Button variant="outline" onClick={() => setAnchor(new Date())}>
              Сегодня
            </Button>
            <button
              type="button"
              className="icon-button"
              aria-label="Предыдущий период"
              onClick={() => move(-1)}
            >
              <ChevronLeft size={18} />
            </button>
            <button
              type="button"
              className="icon-button"
              aria-label="Следующий период"
              onClick={() => move(1)}
            >
              <ChevronRight size={18} />
            </button>
          </div>
          <h2 className="calendar-period" aria-live="polite">
            {label}
          </h2>
          <div
            className="calendar-view-switch"
            role="group"
            aria-label="Вид календаря"
          >
            <button
              type="button"
              aria-pressed={view === "week"}
              onClick={() => setView("week")}
            >
              Неделя
            </button>
            <button
              type="button"
              aria-pressed={view === "month"}
              onClick={() => setView("month")}
            >
              Месяц
            </button>
          </div>
        </div>
        <p className="calendar-zone">
          Часовой пояс: {Intl.DateTimeFormat().resolvedOptions().timeZone}. В
          календаре — предстоящие встречи с указанной датой.
        </p>
        <ErrorNotice error={query.error} />
        {query.isError && (
          <Button variant="outline" onClick={() => void query.refetch()}>
            Повторить загрузку календаря
          </Button>
        )}
        {query.isPending ? (
          <Loading />
        ) : (
          <>
            <div
              className={`calendar-grid-scroll ${view === "week" ? "calendar-week-scroll" : ""}`}
              ref={scroll}
              tabIndex={0}
              role="region"
              aria-label={
                view === "week"
                  ? "Расписание на неделю, прокрутка по времени"
                  : "Сетка месяца"
              }
            >
              {view === "week" ? (
                <table className="calendar-week-table">
                  <caption className="sr-only">
                    Неделя {label}. Встречи сгруппированы по часу начала.
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col">
                        <span className="sr-only">Время</span>
                      </th>
                      {range.days.map((date, index) => (
                        <th
                          scope="col"
                          key={calendarDayKey(date)}
                          className={
                            calendarDayKey(date) === today
                              ? "calendar-today"
                              : ""
                          }
                        >
                          <span>{weekdays[index]}</span>
                          <strong>{date.getDate()}</strong>
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {hours.map((hour) => (
                      <tr key={hour}>
                        <th scope="row">{String(hour).padStart(2, "0")}:00</th>
                        {range.days.map((date) => (
                          <td key={calendarDayKey(date)}>
                            {(byDay.get(calendarDayKey(date)) || [])
                              .filter(
                                (meeting) =>
                                  scheduledMeetingDate(meeting)!.getHours() ===
                                  hour,
                              )
                              .map((meeting) => (
                                <CalendarMeeting
                                  key={meeting.id}
                                  meeting={meeting}
                                  onSelect={setSelected}
                                />
                              ))}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <div
                  className="calendar-month-grid"
                  aria-label={`Месяц ${label}`}
                >
                  {weekdays.map((day) => (
                    <div key={day} className="calendar-weekday">
                      {day}
                    </div>
                  ))}
                  {range.days.map((date) => {
                    const key = calendarDayKey(date);
                    const at = toLocalInput(
                      new Date(
                        date.getFullYear(),
                        date.getMonth(),
                        date.getDate(),
                        9,
                      ).toISOString(),
                    );
                    return (
                      <section
                        key={key}
                        className={`calendar-month-day ${date.getMonth() !== anchor.getMonth() ? "calendar-outside-month" : ""} ${key === today ? "calendar-today" : ""}`}
                        aria-label={formatDate(date.toISOString())}
                      >
                        <div className="calendar-day-heading">
                          <time dateTime={key}>{date.getDate()}</time>
                          <Link
                            className="calendar-add-day"
                            aria-label={`Запланировать на ${date.toLocaleDateString("ru-RU")}`}
                            to={`/meetings/new?scheduled=1&returnTo=calendar&at=${encodeURIComponent(at)}`}
                          >
                            <Plus size={14} aria-hidden="true" />
                          </Link>
                        </div>
                        {(byDay.get(key) || []).map((meeting) => (
                          <CalendarMeeting
                            key={meeting.id}
                            meeting={meeting}
                            onSelect={setSelected}
                          />
                        ))}
                      </section>
                    );
                  })}
                </div>
              )}
            </div>
            <div
              className="calendar-mobile-agenda"
              aria-label="Встречи выбранного периода списком"
            >
              {range.days
                .filter((day) => byDay.has(calendarDayKey(day)))
                .map((date) => (
                  <section key={calendarDayKey(date)}>
                    <h3>
                      {date.toLocaleDateString("ru-RU", {
                        weekday: "long",
                        day: "numeric",
                        month: "long",
                      })}
                    </h3>
                    {byDay.get(calendarDayKey(date))!.map((meeting) => (
                      <CalendarMeeting
                        key={meeting.id}
                        meeting={meeting}
                        onSelect={setSelected}
                      />
                    ))}
                  </section>
                ))}
            </div>
            {!meetings.length && !query.isError && (
              <p className="calendar-empty">
                <CalendarDays size={20} aria-hidden="true" />
                На этот период встреч пока нет.
              </p>
            )}
            {query.hasNextPage && (
              <div className="calendar-pagination">
                <p>
                  Показана часть встреч периода. Загрузите продолжение, чтобы
                  увидеть остальные.
                </p>
                <Button
                  variant="outline"
                  busy={query.isFetchingNextPage}
                  onClick={() => void query.fetchNextPage()}
                >
                  Загрузить ещё встречи
                </Button>
              </div>
            )}
          </>
        )}
      </section>
      <p className="calendar-footer-note">
        Встречи без даты — в{" "}
        <Link className="text-link" to="/meetings">
          общем списке <ArrowRight size={13} aria-hidden="true" />
        </Link>
        . Уже начавшиеся — в{" "}
        <Link className="text-link" to="/meetings?view=active">
          активных встречах
        </Link>
        , завершённые — в{" "}
        <Link className="text-link" to="/history">
          истории
        </Link>
        .
      </p>
      {selected && (
        <CalendarMeetingDetails
          key={selected.id}
          meeting={selected}
          onClose={() => setSelected(null)}
          onEdit={(meeting) => {
            setSelected(null);
            setEditing(meeting);
          }}
        />
      )}
      {editing && (
        <EditSchedule conference={editing} onClose={() => setEditing(null)} />
      )}
    </div>
  );
}
