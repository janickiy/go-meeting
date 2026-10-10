import { useState, type KeyboardEvent } from "react";
import { Link, useSearchParams } from "react-router";
import {
  ArrowRight,
  ArrowUpRight,
  CalendarDays,
  FolderOpen,
  Plus,
  MessageCircle,
  DoorOpen,
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
import { NewDirectChatModal } from "../components/NewDirectChatModal";
import type { Conference, ConferenceFilters } from "../types";
import { localDayEnd, localSchedule } from "../collaboration";
import "./dashboard.css";
import "./dashboard-home.css";

const views: { id: NonNullable<ConferenceFilters["view"]>; label: string }[] = [
  { id: "upcoming", label: "Предстоящие" },
  { id: "active", label: "Активные" },
  { id: "past", label: "Завершённые" },
];

/**
 * MeetingRow показывает время и фактическое состояние одной встречи.
 * @args conference — запись сервера с проверенным доступом текущего пользователя.
 * actions — показывать меню действий; showOpenAction — показывать отдельный переход
 * «Открыть» для незавершённой встречи. Ссылка в названии остаётся доступной всегда.
 * @return Ссылка на комнату либо материалы завершённой встречи.
 */
function MeetingRow({
  conference,
  actions = false,
  showOpenAction = true,
}: {
  conference: Conference;
  actions?: boolean;
  showOpenAction?: boolean;
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
        {(past || showOpenAction) && (
          <Link className="button button-outline" to={target}>
            {past ? (
              <FolderOpen size={16} aria-hidden="true" />
            ) : (
              <ArrowRight size={16} aria-hidden="true" />
            )}
            {past ? "Материалы" : "Открыть"}
          </Link>
        )}
        {actions && <ConferenceChatActions conference={conference} />}
      </div>
    </article>
  );
}

/**
 * Dashboard разделяет главную с действиями и полный список встреч.
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
  return all ? <ConferenceDashboard /> : <HomeDashboard create={create} />;
}

function ConferenceDashboard() {
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
  const visible = filtered;
  return (
    <div className="dashboard-page dashboard-all">
      <section className="page-heading dashboard-heading">
        <div>
          <h1>Встречи</h1>
          <p>Ваши разговоры, планы и общие результаты.</p>
        </div>
        <Link className="button button-primary" to="/conferences/new">
          <Plus size={18} aria-hidden="true" />
          Новая встреча
        </Link>
      </section>
      <div className="dashboard-columns">
        <section className="conferences-panel">
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
          </div>
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
              <MeetingSkeleton rows={4} />
            ) : visible.length ? (
              <div className="conference-list">
                {visible.map((item) => (
                  <MeetingRow
                    key={item.id}
                    conference={item}
                    actions
                    showOpenAction={false}
                  />
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
          {query.hasNextPage && (
            <Button
              variant="outline"
              busy={query.isFetchingNextPage}
              onClick={() => void query.fetchNextPage()}
            >
              Загрузить ещё встречи
            </Button>
          )}
        </section>
      </div>
    </div>
  );
}

/** Главная содержит только лозунг и быстрые действия; списки встреч загружаются в их разделе. */
function HomeDashboard({ create = false }: { create?: boolean }) {
  const [joining, setJoining] = useState(false);
  const [newChat, setNewChat] = useState(false);
  return (
    <div className="dashboard-page dashboard-home">
      <section className="home-launchpad" aria-labelledby="home-slogan">
        <div className="home-intro">
          <h1 id="home-slogan">
            Большие идеи.
            <br />
            <em>Живые разговоры.</em>
          </h1>
          <p>
            Одна встреча может многое изменить.
            <br />
            Соберите нужных людей — остальное здесь.
          </p>
        </div>
        <div className="home-action-grid" aria-label="Быстрые действия">
          <Link className="home-action-card" to="/conferences/new">
            <Video size={30} strokeWidth={1.7} aria-hidden="true" />
            <span>
              <strong>Новая встреча</strong>
              <small>Соберите команду и начните разговор</small>
            </span>
            <ArrowUpRight size={20} aria-hidden="true" />
          </Link>
          <button
            type="button"
            className="home-action-card"
            onClick={() => setNewChat(true)}
          >
            <MessageCircle size={30} strokeWidth={1.7} aria-hidden="true" />
            <span>
              <strong>Новый чат</strong>
              <small>Напишите коллеге личное сообщение</small>
            </span>
            <ArrowUpRight size={20} aria-hidden="true" />
          </button>
          <button
            type="button"
            className="home-action-card"
            onClick={() => setJoining(true)}
          >
            <DoorOpen size={30} strokeWidth={1.7} aria-hidden="true" />
            <span>
              <strong>Подключиться по ссылке</strong>
              <small>Присоединитесь к встрече по приглашению</small>
            </span>
            <ArrowUpRight size={20} aria-hidden="true" />
          </button>
          <Link className="home-action-card" to="/meetings/new?scheduled=1">
            <CalendarDays size={30} strokeWidth={1.7} aria-hidden="true" />
            <span>
              <strong>Запланировать встречу</strong>
              <small>Выберите удобное время для всех</small>
            </span>
            <ArrowUpRight size={20} aria-hidden="true" />
          </Link>
        </div>
      </section>
      {create && <CreateConference />}
      {joining && <JoinByLink onClose={() => setJoining(false)} />}
      {newChat && <NewDirectChatModal onClose={() => setNewChat(false)} />}
    </div>
  );
}
