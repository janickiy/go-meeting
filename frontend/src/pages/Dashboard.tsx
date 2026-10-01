import { useState } from "react";
import { Link } from "react-router";
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

export function Dashboard({
  all = false,
  create = false,
}: {
  all?: boolean;
  create?: boolean;
}) {
  const { user } = useAuth();
  const query = useConferences();
  const [tab, setTab] = useState("created");
  const [search, setSearch] = useState("");
  const [joining, setJoining] = useState(false);
  const conferences = query.data?.pages.flatMap((page) => page.items) || [];
  const filtered = conferences.filter(
    (item) =>
      (tab === "finished"
        ? item.status === "finished" || item.status === "cancelled"
        : item.status === tab) &&
      item.title
        .toLocaleLowerCase("ru")
        .includes(search.toLocaleLowerCase("ru")),
  );
  const visible = all ? filtered : filtered.slice(0, 3);
  return (
    <>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ВАШ ЛИЧНЫЙ КАБИНЕТ</span>
          <h1>
            {all
              ? "Мои конференции"
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
              { id: "created", label: "Предстоящие" },
              { id: "active", label: "Активные" },
              { id: "finished", label: "Завершённые" },
            ].map((item) => (
              <button
                key={item.id}
                id={`tab-${item.id}`}
                className={tab === item.id ? "tab tab-active" : "tab"}
                role="tab"
                aria-selected={tab === item.id}
                aria-controls="conference-list"
                onClick={() => setTab(item.id)}
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
                placeholder="Найти встречу"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </label>
          )}
        </div>
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
                      {item.status === "created"
                        ? "Создана "
                        : item.status === "active"
                          ? "Начало: "
                          : "Завершение: "}
                      {formatDate(
                        item.finishedAt || item.startedAt || item.createdAt,
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
                    : tab === "created"
                      ? "Самое время для первой встречи"
                      : tab === "active"
                        ? "Сейчас нет активных встреч"
                        : "Здесь будет история встреч"}
                </h3>
                <p>
                  {search
                    ? "Попробуйте другое название."
                    : tab === "created"
                      ? "Создайте конференцию и пригласите коллег по ссылке."
                      : "Конференции появятся здесь, когда изменится их статус."}
                </p>
                {tab === "created" && !search && (
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
