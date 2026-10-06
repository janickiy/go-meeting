import { useState, type KeyboardEvent } from "react";
import { Link, useSearchParams } from "react-router";
import {
  ArrowRight,
  CalendarDays,
  Clock3,
  History,
  Link as LinkIcon,
  Plus,
  Search,
  Video,
} from "lucide-react";
import { useAuth } from "../auth";
import { useConferences } from "../queries";
import { formatDate } from "../utils";
import { Button, ErrorNotice, StatusBadge } from "../components/ui";
import { CreateConference, JoinByLink } from "../components/ConferenceModals";
import { ConferenceChatActions } from "../components/ConferenceChatActions";
import type { Conference, ConferenceFilters } from "../types";
import { localDayEnd, localSchedule } from "../collaboration";
import "./dashboard.css";

const views: { id: NonNullable<ConferenceFilters["view"]>; label: string }[] = [
  { id: "upcoming", label: "Предстоящие" },
  { id: "active", label: "Активные" },
  { id: "past", label: "Завершённые" },
];

/**
 * MeetingSkeleton сохраняет место списка во время загрузки без фиктивных встреч.
 * @return Доступное состояние ожидания с декоративными строками.
 */
function MeetingSkeleton() {
  return (
    <div
      className="meeting-skeleton"
      role="status"
      aria-label="Загрузка встреч"
    >
      {[0, 1, 2].map((row) => (
        <div className="meeting-skeleton-row" key={row} aria-hidden="true">
          <span />
          <span />
          <span />
        </div>
      ))}
    </div>
  );
}

/**
 * MeetingRow показывает время и фактическое состояние одной встречи.
 * @args conference — запись сервера с проверенным доступом текущего пользователя.
 * @return Ссылка на комнату либо материалы завершённой встречи.
 */
function MeetingRow({
  conference,
  actions = false,
}: {
  conference: Conference;
  actions?: boolean;
}) {
  const past = ["finished", "cancelled"].includes(conference.status);
  const timestamp =
    conference.finishedAt ||
    conference.startedAt ||
    conference.scheduledAt ||
    conference.createdAt;
  const date = new Date(timestamp);
  return (
    <div className={actions ? "dashboard-meeting-with-actions" : undefined}>
      <Link
        className={`conference-row dashboard-meeting-row ${conference.status === "active" ? "dashboard-meeting-active" : ""}`}
        to={past ? `/history/${conference.id}` : `/meetings/${conference.id}`}
      >
        <span className="dashboard-meeting-time" aria-hidden="true">
          {Number.isFinite(date.getTime())
            ? date.toLocaleTimeString("ru-RU", {
                hour: "2-digit",
                minute: "2-digit",
              })
            : "—"}
          <small>
            {Number.isFinite(date.getTime())
              ? date.toLocaleDateString("ru-RU", {
                  day: "numeric",
                  month: "short",
                })
              : ""}
          </small>
        </span>
        <div className="conference-row-copy">
          <h3>{conference.title}</h3>
          <p>
            {conference.status === "created" ? "Создана: " : ""}
            {formatDate(timestamp)}
            {conference.plannedDurationMin
              ? ` · ${conference.plannedDurationMin} мин`
              : ""}
          </p>
        </div>
        <StatusBadge status={conference.status} />
        <ArrowRight className="row-arrow" size={17} aria-hidden="true" />
      </Link>
      {actions && <ConferenceChatActions conference={conference} />}
    </div>
  );
}

/**
 * RecentMeetings использует один серверный список истории, не запрашивая записи каждой встречи.
 * @return Последние завершённые встречи без выдуманных превью или количества записей.
 */
function RecentMeetings() {
  const query = useConferences({
    view: "past",
    scope: "all",
    status: "finished",
  });
  const meetings =
    query.data?.pages.flatMap((page) => page.items).slice(0, 3) || [];
  return (
    <section
      className="dashboard-recent"
      aria-labelledby="recent-meetings-heading"
    >
      <div className="section-heading">
        <div>
          <h2 id="recent-meetings-heading">Недавние встречи</h2>
          <p className="dashboard-section-note">
            Записи и материалы — в истории каждой встречи.
          </p>
        </div>
        <Link className="text-link" to="/history">
          Вся история <ArrowRight size={15} aria-hidden="true" />
        </Link>
      </div>
      <ErrorNotice error={query.error} />
      {query.isError && (
        <Button variant="outline" onClick={() => void query.refetch()}>
          Повторить загрузку истории
        </Button>
      )}
      {query.isPending ? (
        <MeetingSkeleton />
      ) : meetings.length ? (
        <div className="recent-meeting-grid">
          {meetings.map((meeting) => (
            <Link
              key={meeting.id}
              className="recent-meeting-card"
              to={`/history/${meeting.id}`}
            >
              <div className="recent-meeting-art" aria-hidden="true">
                <History size={26} />
                <span>История встречи</span>
                <ArrowRight size={19} />
              </div>
              <div className="recent-meeting-copy">
                <h3>{meeting.title}</h3>
                <p>
                  <Clock3 size={13} aria-hidden="true" />
                  {formatDate(meeting.finishedAt || meeting.createdAt)}
                </p>
              </div>
            </Link>
          ))}
        </div>
      ) : (
        !query.isError && (
          <div className="dashboard-history-empty">
            <History size={23} aria-hidden="true" />
            <div>
              <strong>История начинается с первой встречи</strong>
              <p>
                Здесь появятся завершённые встречи и доступные вам материалы.
              </p>
            </div>
          </div>
        )
      )}
    </section>
  );
}

/**
 * Dashboard объединяет реальные встречи, создание, планирование и переход в историю.
 * @args all — показывать полный список и фильтры; create — открыть форму создания поверх страницы.
 * @return Адаптивный кабинет со статусами загрузки, ошибки, пустого списка и пагинации.
 */
export function Dashboard({
  all = false,
  create = false,
}: {
  all?: boolean;
  create?: boolean;
}) {
  const { user } = useAuth();
  const [params, setParams] = useSearchParams();
  const tab: NonNullable<ConferenceFilters["view"]> =
    params.get("view") === "active"
      ? "active"
      : params.get("view") === "past"
        ? "past"
        : "upcoming";
  const [scope, setScope] =
    useState<NonNullable<ConferenceFilters["scope"]>>("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const query = useConferences({
    view: tab,
    scope,
    from: from ? localSchedule(`${from}T00:00`) || undefined : undefined,
    to: to ? localDayEnd(to) || undefined : undefined,
  });
  const [search, setSearch] = useState("");
  const [joining, setJoining] = useState(false);

  /** @args view — серверная категория встреч. Сохраняет прочие параметры маршрута. */
  function selectView(view: NonNullable<ConferenceFilters["view"]>) {
    setParams((current) => {
      const next = new URLSearchParams(current);
      next.set("view", view);
      return next;
    });
  }

  /** @args event — клавиатурное событие вкладки. Реализует стрелки, Home и End по шаблону ARIA. */
  function onTabKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    const current = views.findIndex(
      (view) => view.id === event.currentTarget.dataset.view,
    );
    let next = current;
    if (event.key === "ArrowRight") next = (current + 1) % views.length;
    else if (event.key === "ArrowLeft")
      next = (current - 1 + views.length) % views.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = views.length - 1;
    else return;
    event.preventDefault();
    selectView(views[next].id);
    document.getElementById(`tab-${views[next].id}`)?.focus();
  }

  const conferences = query.data?.pages.flatMap((page) => page.items) || [];
  const filtered = conferences.filter((item) =>
    item.title.toLocaleLowerCase("ru").includes(search.toLocaleLowerCase("ru")),
  );
  const visible = all ? filtered : filtered.slice(0, 4);
  return (
    <div className="dashboard-page">
      <section className="page-heading dashboard-heading">
        <div>
          <span className="eyebrow">
            {all ? "ВАШИ ВСТРЕЧИ" : "ВАШ РАБОЧИЙ ДЕНЬ"}
          </span>
          <h1>
            {all
              ? tab === "past"
                ? "История встреч"
                : "Мои конференции"
              : `Добро пожаловать${user?.displayName ? `, ${user.displayName}` : ""}!`}
          </h1>
          <p>
            {all
              ? "Все ваши встречи — от первого приглашения до завершения."
              : "Встречайтесь, обсуждайте идеи и сохраняйте важное."}
          </p>
        </div>
        <div className="dashboard-date" aria-label="Сегодня">
          <CalendarDays size={17} aria-hidden="true" />
          {new Date().toLocaleDateString("ru-RU", {
            day: "numeric",
            month: "long",
          })}
        </div>
      </section>
      <div className="dashboard-actions">
        <Link to="/conferences/new" className="button button-primary">
          <Plus size={19} aria-hidden="true" />
          Новая конференция
        </Link>
        <Link to="/meetings/new?scheduled=1" className="button button-outline">
          <CalendarDays size={18} aria-hidden="true" />
          Запланировать
        </Link>
        <Button variant="secondary" onClick={() => setJoining(true)}>
          <LinkIcon size={18} aria-hidden="true" />
          Присоединиться по ссылке
        </Button>
      </div>
      <section className="conferences-panel">
        <div className="section-heading">
          <h2>{all ? "Ваши встречи" : "Мои конференции"}</h2>
          {!all && (
            <Link to="/conferences" className="text-link">
              Смотреть все <ArrowRight size={15} aria-hidden="true" />
            </Link>
          )}
        </div>
        <div className="list-toolbar">
          <div className="tabs" role="tablist" aria-label="Статус конференций">
            {views.map((item) => (
              <button
                key={item.id}
                id={`tab-${item.id}`}
                data-view={item.id}
                className={tab === item.id ? "tab tab-active" : "tab"}
                role="tab"
                aria-selected={tab === item.id}
                aria-controls="conference-list"
                tabIndex={tab === item.id ? 0 : -1}
                onKeyDown={onTabKeyDown}
                onClick={() => selectView(item.id)}
              >
                {item.label}
              </button>
            ))}
          </div>
          {all && (
            <label className="search-field">
              <Search size={17} aria-hidden="true" />
              <input
                aria-label="Поиск конференции по названию"
                placeholder="Найти среди загруженных"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </label>
          )}
        </div>
        {all && (
          <div className="conference-filters">
            <label className="field">
              Показывать
              <select
                aria-label="Моё участие"
                value={scope}
                onChange={(event) =>
                  setScope(
                    event.target.value as NonNullable<
                      ConferenceFilters["scope"]
                    >,
                  )
                }
              >
                <option value="all">Все мои встречи</option>
                <option value="owned">Я организатор</option>
                <option value="participating">Я участник</option>
              </select>
            </label>
            <label className="field">
              С даты
              <input
                type="date"
                aria-label="Встречи с даты"
                value={from}
                max={to || undefined}
                onChange={(event) => setFrom(event.target.value)}
              />
            </label>
            <label className="field">
              По дату
              <input
                type="date"
                aria-label="Встречи по дату"
                value={to}
                min={from || undefined}
                onChange={(event) => setTo(event.target.value)}
              />
            </label>
          </div>
        )}
        <ErrorNotice error={query.error} />
        {query.isError && (
          <Button variant="outline" onClick={() => void query.refetch()}>
            Попробовать снова
          </Button>
        )}
        <div
          id="conference-list"
          role="tabpanel"
          aria-labelledby={`tab-${tab}`}
          tabIndex={0}
        >
          {query.isPending ? (
            <MeetingSkeleton />
          ) : visible.length ? (
            <div className="conference-list">
              {visible.map((item) => (
                <MeetingRow key={item.id} conference={item} actions={all} />
              ))}
            </div>
          ) : (
            !query.isError && (
              <div className="empty-state">
                <span className="empty-icon">
                  <CalendarDays size={29} aria-hidden="true" />
                </span>
                <h3>
                  {search
                    ? "Встречи не найдены"
                    : tab === "upcoming"
                      ? "Самое время для первой встречи"
                      : tab === "active"
                        ? "Сейчас нет активных встреч"
                        : "Здесь будет история встреч"}
                </h3>
                <p>
                  {search
                    ? "Поиск работает по загруженным встречам. Попробуйте другое название или загрузите ещё."
                    : tab === "upcoming"
                      ? "Создайте конференцию и пригласите коллег по ссылке."
                      : "Конференции появятся здесь, когда изменится их статус."}
                </p>
                {tab === "upcoming" && !search && (
                  <Link to="/conferences/new" className="text-link">
                    <Plus size={16} aria-hidden="true" />
                    Создать конференцию
                  </Link>
                )}
              </div>
            )
          )}
        </div>
        {all && query.hasNextPage && (
          <Button
            variant="outline"
            busy={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            Загрузить ещё встречи
          </Button>
        )}
        {!all && (filtered.length > visible.length || query.hasNextPage) && (
          <Link
            className="dashboard-show-more text-link"
            to={`/meetings?view=${tab}`}
          >
            Открыть весь список <ArrowRight size={15} aria-hidden="true" />
          </Link>
        )}
      </section>
      {!all && (
        <>
          <RecentMeetings />
          <div className="dashboard-tip">
            <span className="tip-icon">
              <Video size={21} aria-hidden="true" />
            </span>
            <div>
              <strong>Всё готово к следующему разговору</strong>
              <p>
                Проверьте камеру и микрофон перед входом. Подключение к встрече
                начнётся только после вашего подтверждения.
              </p>
            </div>
            <Link className="text-link" to="/settings">
              Устройства <ArrowRight size={15} aria-hidden="true" />
            </Link>
          </div>
        </>
      )}
      {create && <CreateConference />}
      {joining && <JoinByLink onClose={() => setJoining(false)} />}
    </div>
  );
}
