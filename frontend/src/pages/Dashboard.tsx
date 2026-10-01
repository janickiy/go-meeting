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

/**
 * Dashboard показывает серверный список встреч с вкладками, фильтрами, пагинацией и действиями создания или входа.
 *
 * @parameters:
 *   - объект параметров: all — свойство текущего компонента; create — свойство текущего компонента.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
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
  const conferences =
    query.data?.pages.flatMap(
      /**
       * Обработчик flatMap преобразует текущий элемент в данные или представление результирующего списка.
       *
       * @parameters:
       *   - page — изолированная страница Playwright.
       *
       * @returns преобразованное значение текущего элемента для результирующего набора.
       */ (page) => page.items,
    ) || [];
  const filtered = conferences.filter(
    /**
     * Обработчик conferences.filter проверяет, должен ли элемент войти в отфильтрованный набор.
     *
     * @parameters:
     *   - item — элемент списка, который обрабатывает текущий шаг.
     *
     * @returns логический признак соответствия элемента условию.
     */ (item) =>
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
        <Button
          variant="secondary"
          onClick={
            /**
             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: setJoining(true).
             */ () => setJoining(true)
          }
        >
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
            ].map(
              /**
               * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
               *
               * @parameters:
               *   - item — элемент списка, который обрабатывает текущий шаг.
               *
               * @returns преобразованное значение текущего элемента для результирующего набора.
               */ (item) => (
                <button
                  key={item.id}
                  id={`tab-${item.id}`}
                  className={tab === item.id ? "tab tab-active" : "tab"}
                  role="tab"
                  aria-selected={tab === item.id}
                  aria-controls="conference-list"
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
                     */ () => setParams({ view: item.id })
                  }
                >
                  {item.label}
                </button>
              ),
            )}
          </div>
          {all && (
            <label className="search-field">
              <Search size={17} />
              <input
                aria-label="Поиск конференции по названию"
                placeholder="Найти среди загруженных"
                value={search}
                onChange={
                  /**
                   * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   * @parameters:
                   *   - event — проверенный конверт события комнаты.
                   *
                   * @returns вычисленное значение: setSearch(event.target.value).
                   */ (event) => setSearch(event.target.value)
                }
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
                onChange={
                  /**
                   * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   * @parameters:
                   *   - event — проверенный конверт события комнаты.
                   *
                   * @returns вычисленное значение: setScope( event.target.value as NonNullable< ConferenceFilters["scope"] >, ).
                   */ (event) =>
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
                onChange={
                  /**
                   * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   * @parameters:
                   *   - event — проверенный конверт события комнаты.
                   *
                   * @returns вычисленное значение: setFrom(event.target.value).
                   */ (event) => setFrom(event.target.value)
                }
              />
            </label>
            <label className="field">
              По дату
              <input
                type="date"
                aria-label="Встречи по дату"
                value={to}
                min={from || undefined}
                onChange={
                  /**
                   * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   * @parameters:
                   *   - event — проверенный конверт события комнаты.
                   *
                   * @returns вычисленное значение: setTo(event.target.value).
                   */ (event) => setTo(event.target.value)
                }
              />
            </label>
          </div>
        )}
        <ErrorNotice error={query.error} />
        {query.isError && (
          <Button
            variant="outline"
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                void query.refetch();
              }
            }
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
              {visible.map(
                /**
                 * Обработчик visible.map преобразует один элемент набора в представление или данные следующего шага.
                 *
                 * @parameters:
                 *   - item — элемент списка, который обрабатывает текущий шаг.
                 *
                 * @returns преобразованное значение текущего элемента для результирующего набора.
                 */ (item) => (
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
                ),
              )}
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
            onClick={
              /**
               * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                void query.fetchNextPage();
              }
            }
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
      {joining && (
        <JoinByLink
          onClose={
            /**
             * onClose обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns вычисленное значение: setJoining(false).
             */ () => setJoining(false)
          }
        />
      )}
    </>
  );
}
