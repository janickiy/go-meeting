import { useState, type KeyboardEvent } from "react";
import { Link, useSearchParams } from "react-router";
import {
  ArrowRight,
  ArrowUpRight,
  CalendarDays,
  FolderOpen,
  History,
  Link as LinkIcon,
  Plus,
  Search,
  SlidersHorizontal,
  Video,
} from "lucide-react";
import { useAuth } from "../auth";
import { useConferences } from "../queries";
import { formatDate } from "../utils";
import { Button, ErrorNotice, StatusBadge } from "../components/ui";
import { CreateConference, JoinByLink } from "../components/ConferenceModals";
import { ConferenceChatActions } from "../components/ConferenceChatActions";
import { MeetingSkeleton } from "../components/MeetingSkeleton";
import type { Conference, ConferenceFilters } from "../types";
import { localDayEnd, localSchedule } from "../collaboration";
import "./dashboard.css";

const views: { id: NonNullable<ConferenceFilters["view"]>; label: string }[] = [
  { id: "upcoming", label: "Предстоящие" },
  { id: "active", label: "Активные" },
  { id: "past", label: "Завершённые" },
];

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
  const { user } = useAuth();
  const past = ["finished", "cancelled"].includes(conference.status);
  const timestamp =
    conference.finishedAt ||
    conference.startedAt ||
    conference.scheduledAt ||
    conference.createdAt;
  const date = new Date(timestamp);
  const target = past
    ? `/history/${conference.id}`
    : `/meetings/${conference.id}`;
  return (
    <article
      className={`conference-row dashboard-meeting-row ${conference.status === "active" ? "dashboard-meeting-active" : ""}`}
    >
      <span className="dashboard-meeting-time" aria-hidden="true">
        {conference.status !== "created" && Number.isFinite(date.getTime())
          ? date.toLocaleTimeString("ru-RU", {
              hour: "2-digit",
              minute: "2-digit",
            })
          : "—"}
        <small>
          {conference.status === "created"
            ? "Без даты"
            : Number.isFinite(date.getTime())
              ? date.toLocaleDateString("ru-RU", {
                  day: "numeric",
                  month: "short",
                })
              : ""}
        </small>
      </span>
      <div className="conference-row-copy">
        <Link to={target}>
          <h3>{conference.title}</h3>
        </Link>
        <p>
          {conference.status === "created" ? "Создана: " : ""}
          {formatDate(timestamp)}
          {conference.plannedDurationMin
            ? ` · ${conference.plannedDurationMin} мин`
            : ""}
          {conference.ownerId === user?.id
            ? " · Вы организатор"
            : " · Вы участник"}
        </p>
      </div>
      <StatusBadge status={conference.status} />
      <div className="dashboard-meeting-action">
        <Link className="button button-outline" to={target}>
          {past ? (
            <FolderOpen size={16} aria-hidden="true" />
          ) : (
            <ArrowRight size={16} aria-hidden="true" />
          )}
          {past ? "Материалы" : "Открыть"}
        </Link>
        {actions && <ConferenceChatActions conference={conference} />}
      </div>
    </article>
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
    query.data?.pages.flatMap((page) => page.items).slice(0, 2) || [];
  return (
    <section
      className="dashboard-recent"
      aria-labelledby="recent-meetings-heading"
    >
      <div className="section-heading">
        <div>
          <h2 id="recent-meetings-heading">Недавние встречи</h2>
        </div>
        <Link className="text-link" to="/meetings?view=past">
          Все <ArrowUpRight size={15} aria-hidden="true" />
        </Link>
      </div>
      <ErrorNotice error={query.error} />
      {query.isError && (
        <Button variant="outline" onClick={() => void query.refetch()}>
          Повторить загрузку истории
        </Button>
      )}
      {query.isPending ? (
        <MeetingSkeleton rows={2} />
      ) : meetings.length ? (
        <div className="recent-meeting-grid">
          {meetings.map((meeting) => (
            <Link
              key={meeting.id}
              className="recent-meeting-card"
              to={`/history/${meeting.id}`}
            >
              <div className="recent-meeting-art" aria-hidden="true">
                <Video size={20} />
              </div>
              <div className="recent-meeting-copy">
                <h3>{meeting.title}</h3>
                <p>{formatDate(meeting.finishedAt || meeting.createdAt)}</p>
                <span>
                  Открыть материалы{" "}
                  <ArrowUpRight size={15} aria-hidden="true" />
                </span>
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
  const [filtersOpen, setFiltersOpen] = useState(false);

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
  const visible = all ? filtered : filtered.slice(0, 3);
  const featured = conferences.find((meeting) =>
    ["scheduled", "created", "active"].includes(meeting.status),
  );
  return (
    <div
      className={`dashboard-page ${all ? "dashboard-all" : "dashboard-home"}`}
    >
      <section className="page-heading dashboard-heading">
        <div>
          {!all && (
            <span className="eyebrow">
              {new Date().toLocaleDateString("ru-RU", {
                weekday: "long",
                day: "numeric",
                month: "long",
              })}
            </span>
          )}
          <h1>
            {all
              ? "Встречи"
              : `Добро пожаловать${user?.displayName ? `, ${user.displayName}` : ""}.`}
          </h1>
          {all && <p>Ваши разговоры, планы и общие результаты.</p>}
        </div>
        {all ? (
          <Link className="button button-primary" to="/conferences/new">
            <Plus size={18} aria-hidden="true" />
            Новая встреча
          </Link>
        ) : (
          <Link className="dashboard-date" to="/calendar">
            <CalendarDays size={17} aria-hidden="true" />
            Календарь
          </Link>
        )}
      </section>
      {!all && (
        <section
          className="dashboard-welcome"
          aria-label="Начните рабочий день"
        >
          <div className="dashboard-welcome-copy">
            <span className="dashboard-welcome-kicker">
              <span aria-hidden="true" /> БЛИЖЕ К КОМАНДЕ
            </span>
            <h2>
              Большие идеи.
              <br />
              <em>Живые разговоры.</em>
            </h2>
            <p>
              Одна встреча может многое изменить.
              <br />
              Соберите нужных людей — остальное здесь.
            </p>
          </div>
          {featured ? (
            <Link
              className="dashboard-featured-meeting"
              to={`/meetings/${featured.id}`}
              aria-label="Открыть встречу из выбранного списка"
            >
              <span className="dashboard-featured-label">
                {tab === "active"
                  ? "Встреча сейчас"
                  : "Из ваших предстоящих встреч"}
                <ArrowUpRight size={17} aria-hidden="true" />
              </span>
              <div className="dashboard-featured-content">
                <span className="dashboard-featured-icon">
                  <Video size={25} aria-hidden="true" />
                </span>
                <div>
                  <strong>{featured.title}</strong>
                  <span>
                    {featured.scheduledAt
                      ? formatDate(featured.scheduledAt)
                      : featured.status === "active"
                        ? "Встреча в эфире"
                        : "Готова к началу"}
                    {featured.plannedDurationMin
                      ? ` · ${featured.plannedDurationMin} мин`
                      : ""}
                  </span>
                </div>
              </div>
              <div className="dashboard-featured-footer">
                <span>
                  {featured.ownerId === user?.id
                    ? "Вы организатор"
                    : "Вы участник"}
                </span>
                <span>
                  Открыть встречу <ArrowRight size={16} aria-hidden="true" />
                </span>
              </div>
            </Link>
          ) : (
            <div className="dashboard-featured-meeting dashboard-featured-empty">
              <span className="dashboard-featured-icon">
                <Video size={25} aria-hidden="true" />
              </span>
              <strong>
                Хороший разговор
                <br />
                начинается с приглашения.
              </strong>
              <span>Для коллег рядом и далеко.</span>
            </div>
          )}
        </section>
      )}
      {!all && (
        <div className="dashboard-actions">
          <Link
            to="/conferences/new"
            className="button button-primary dashboard-quick-card"
            aria-label="Новая встреча"
          >
            <span className="dashboard-quick-icon">
              <Plus size={22} aria-hidden="true" />
            </span>
            <span>
              <strong>Новая встреча</strong>
              {!all && <small>Начните разговор сейчас</small>}
            </span>
            {!all && <ArrowUpRight size={18} aria-hidden="true" />}
          </Link>
          <Link
            to="/meetings/new?scheduled=1"
            className="button button-outline dashboard-quick-card"
            aria-label="Запланировать"
          >
            <span className="dashboard-quick-icon">
              <CalendarDays size={22} aria-hidden="true" />
            </span>
            <span>
              <strong>Запланировать</strong>
              {!all && <small>Найдите время для команды</small>}
            </span>
            {!all && <ArrowUpRight size={18} aria-hidden="true" />}
          </Link>
          <Button
            variant="secondary"
            className="dashboard-quick-card"
            aria-label="Присоединиться по ссылке"
            onClick={() => setJoining(true)}
          >
            <span className="dashboard-quick-icon">
              <LinkIcon size={22} aria-hidden="true" />
            </span>
            <span>
              <strong>
                {all ? "Присоединиться по ссылке" : "По приглашению"}
              </strong>
              {!all && <small>Войдите по ссылке на встречу</small>}
            </span>
            {!all && <ArrowUpRight size={18} aria-hidden="true" />}
          </Button>
        </div>
      )}
      <div className="dashboard-columns">
        <section className="conferences-panel">
          {!all && (
            <div className="section-heading">
              <h2>Ваши встречи</h2>
              {!all && (
                <Link to="/conferences" className="text-link">
                  Все встречи <ArrowUpRight size={15} aria-hidden="true" />
                </Link>
              )}
            </div>
          )}
          <div className="list-toolbar">
            <div
              className="tabs"
              role="tablist"
              aria-label="Статус конференций"
            >
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
              <div className="dashboard-search-row">
                <label className="search-field">
                  <Search size={17} aria-hidden="true" />
                  <input
                    aria-label="Поиск конференции по названию"
                    placeholder="Найти среди загруженных"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                  />
                </label>
                <Button
                  variant="outline"
                  className="dashboard-mobile-filter"
                  aria-label="Фильтры встреч"
                  aria-expanded={filtersOpen}
                  onClick={() => setFiltersOpen(!filtersOpen)}
                >
                  <SlidersHorizontal size={18} />
                  Фильтры
                </Button>
              </div>
            )}
          </div>
          {all && (
            <div
              className={`conference-filters ${filtersOpen ? "conference-filters-open" : ""}`}
            >
              <label className="field">
                Моё участие
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
          {all && (
            <div className="dashboard-list-caption">
              <span>
                {tab === "active"
                  ? "Встречи в эфире"
                  : tab === "past"
                    ? "Завершённые и отменённые"
                    : "Предстоящие и без даты"}
              </span>
              <span>
                Локальное время ·{" "}
                {Intl.DateTimeFormat().resolvedOptions().timeZone}
              </span>
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
              <MeetingSkeleton rows={all ? 4 : 3} />
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
          <aside className="dashboard-side">
            <RecentMeetings />
            <div className="dashboard-tip">
              <span className="tip-icon">
                <LinkIcon size={21} aria-hidden="true" />
              </span>
              <div>
                <strong>Ссылка вместо лишних шагов</strong>
                <p>
                  Приглашайте коллег без аккаунта. Достаточно поделиться
                  ссылкой.
                </p>
                <button className="text-link" onClick={() => setJoining(true)}>
                  Открыть приглашение{" "}
                  <ArrowRight size={16} aria-hidden="true" />
                </button>
              </div>
            </div>
          </aside>
        )}
      </div>
      {create && <CreateConference />}
      {joining && <JoinByLink onClose={() => setJoining(false)} />}
    </div>
  );
}
