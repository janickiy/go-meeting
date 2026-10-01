import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import {
  ArrowRight,
  CalendarDays,
  Link as LinkIcon,
  Plus,
  Search,
  Video,
} from "lucide-react";
import { useAuth } from "../auth";
import { useConferences } from "../queries";
import { formatDate } from "../utils";
import { Button, ErrorNotice, Loading, StatusBadge } from "../components/ui";
import { CreateConference, JoinByLink } from "../components/ConferenceModals";
import type { ConferenceFilters } from "../types";
import { localDayEnd, localSchedule } from "../collaboration";

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
  const conferences = query.data?.pages.flatMap((page) => page.items) || [];
  const filtered = conferences.filter((item) =>
    item.title.toLocaleLowerCase("ru").includes(search.toLocaleLowerCase("ru")),
  );
  const visible = all ? filtered : filtered.slice(0, 3);
  return (
    <>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ВАШ ЛИЧНЫЙ КАБИНЕТ</span>
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
              : "Создайте новую встречу или присоединитесь к существующей."}
          </p>
        </div>
        <span className="heading-art">
          <Video size={32} />
        </span>
      </section>
      <div className="dashboard-actions">
        <Link to="/conferences/new" className="button button-primary">
          <Plus size={21} />
          Новая конференция
        </Link>
        <Button variant="secondary" onClick={() => setJoining(true)}>
          <LinkIcon size={19} />
          Присоединиться по ссылке
        </Button>
      </div>
      <section className="conferences-panel">
        <div className="section-heading">
          <h2>{all ? "Ваши встречи" : "Мои конференции"}</h2>
          {!all && (
            <Link to="/conferences" className="text-link">
              Смотреть все <ArrowRight size={15} />
            </Link>
          )}
        </div>
        <div className="list-toolbar">
          <div className="tabs" role="tablist" aria-label="Статус конференций">
            {[
              { id: "upcoming", label: "Предстоящие" },
              { id: "active", label: "Активные" },
              { id: "past", label: "Завершённые" },
            ].map((item) => (
              <button
                key={item.id}
                id={`tab-${item.id}`}
                className={tab === item.id ? "tab tab-active" : "tab"}
                role="tab"
                aria-selected={tab === item.id}
                aria-controls="conference-list"
                onClick={() => setParams({ view: item.id })}
              >
                {item.label}
              </button>
            ))}
          </div>
          {all && (
            <label className="search-field">
              <Search size={17} />
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
          <Button
            variant="outline"
            onClick={() => {
              void query.refetch();
            }}
          >
            Попробовать снова
          </Button>
        )}
        <div
          id="conference-list"
          role="tabpanel"
          aria-labelledby={`tab-${tab}`}
        >
          {query.isPending ? (
            <Loading />
          ) : visible.length ? (
            <div className="conference-list">
              {visible.map((item) => (
                <Link
                  className="conference-row"
                  key={item.id}
                  to={`/conferences/${item.id}`}
                >
                  <span
                    className={`meeting-icon ${item.status === "active" ? "meeting-live" : ""}`}
                  >
                    {item.status === "active" ? (
                      <Video size={21} />
                    ) : (
                      <CalendarDays size={21} />
                    )}
                  </span>
                  <div className="conference-row-copy">
                    <h3>{item.title}</h3>
                    <p>
                      {item.status === "scheduled"
                        ? "Запланировано: "
                        : item.status === "created"
                          ? "Создана "
                          : item.status === "active"
                            ? "Начало: "
                            : "Завершение: "}
                      {formatDate(
                        item.finishedAt ||
                          item.startedAt ||
                          item.scheduledAt ||
                          item.createdAt,
                      )}
                    </p>
                  </div>
                  <StatusBadge status={item.status} />
                  <ArrowRight className="row-arrow" size={18} />
                </Link>
              ))}
            </div>
          ) : (
            !query.isError && (
              <div className="empty-state">
                <span className="empty-icon">
                  <CalendarDays size={29} />
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
                    ? "Попробуйте другое название."
                    : tab === "upcoming"
                      ? "Создайте конференцию и пригласите коллег по ссылке."
                      : "Конференции появятся здесь, когда изменится их статус."}
                </p>
                {tab === "upcoming" && !search && (
                  <Link to="/conferences/new" className="text-link">
                    <Plus size={16} />
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
            onClick={() => {
              void query.fetchNextPage();
            }}
          >
            Загрузить ещё встречи
          </Button>
        )}
      </section>
      {!all && (
        <div className="dashboard-tip">
          <span className="tip-icon">
            <LinkIcon size={21} />
          </span>
          <div>
            <strong>Одна ссылка — и вы вместе</strong>
            <p>
              Скопируйте приглашение из карточки встречи и отправьте его
              коллегам.
            </p>
          </div>
        </div>
      )}
      {create && <CreateConference />}
      {joining && <JoinByLink onClose={() => setJoining(false)} />}
    </>
  );
}
