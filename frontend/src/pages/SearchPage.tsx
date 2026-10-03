import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { localDayEnd, toLocalInput } from "../collaboration";
import { useConferences } from "../queries";
import { recordingTime, searchResultLink } from "../intelligence";
import type { SearchFilters, SearchSource } from "../types";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { useCapabilities } from "../useCapabilities";

const labels: Record<SearchSource, string> = {
  all: "Все материалы",
  conference: "Названия встреч",
  transcript: "Расшифровки",
  summary: "Итоги ИИ",
};

/**
 * Ищет только среди разрешённых сервером встреч и материалов с пагинацией.
 * @return Форма фильтров, безопасные текстовые фрагменты и ссылки на запись.
 */
export function SearchPage() {
  const { user } = useAuth();
  const capabilities = useCapabilities();
  const semanticAvailable =
    capabilities.data?.capabilities.semanticSearch === true;
  const [params, setParams] = useSearchParams();
  const initialSource = params.get("source") as SearchSource;
  const [query, setQuery] = useState(params.get("q") || "");
  const [mode, setMode] = useState<SearchFilters["mode"]>("keyword");
  const [membership, setMembership] =
    useState<SearchFilters["membership"]>("all");
  const [participantId, setParticipantId] = useState("");
  const [source, setSource] = useState<SearchSource>(
    initialSource in labels ? initialSource : "all",
  );
  const [conferenceId, setConferenceId] = useState(
    params.get("conferenceId") || "",
  );
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [validation, setValidation] = useState<string>();
  const filters = useMemo<SearchFilters | null>(() => {
    const text = (params.get("q") || "").trim().slice(0, 500);
    const kind = params.get("source") as SearchSource;
    const nextSource = kind in labels ? kind : "all";
    const start = params.get("from"),
      end = params.get("to");
    const startDate =
      start && Number.isFinite(Date.parse(start)) ? start : undefined;
    const endDate = end && Number.isFinite(Date.parse(end)) ? end : undefined;
    return text.length >= 2
      ? {
          q: text,
          source: nextSource,
          mode:
            semanticAvailable &&
            ["semantic", "hybrid"].includes(params.get("mode") || "")
              ? (params.get("mode") as SearchFilters["mode"])
              : "keyword",
          membership: ["all", "owned", "participating"].includes(
            params.get("membership") || "",
          )
            ? (params.get("membership") as SearchFilters["membership"])
            : "all",
          participantId: params.get("participantId") || undefined,
          conferenceId: params.get("conferenceId") || undefined,
          from: startDate,
          to: endDate,
        }
      : null;
  }, [params, semanticAvailable]);
  useEffect(() => {
    setQuery(filters?.q || "");
    setSource(filters?.source || "all");
    setConferenceId(filters?.conferenceId || "");
    setMode(filters?.mode || "keyword");
    setMembership(filters?.membership || "all");
    setParticipantId(filters?.participantId || "");
    setFrom(filters?.from ? toLocalInput(filters.from).slice(0, 10) : "");
    setTo(filters?.to ? toLocalInput(filters.to).slice(0, 10) : "");
  }, [filters]);
  const conferences = useConferences();
  const rows = conferences.data?.pages.flatMap((page) => page.items) || [];
  const participants = useInfiniteQuery({
    queryKey: ["search-participants", user?.id, conferenceId],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      api.participants(conferenceId, pageParam, signal),
    enabled: !!conferenceId,
    getNextPageParam: (page, _pages, lastOffset) =>
      page.items.length === 100 ? lastOffset + 100 : undefined,
    retry: false,
  });
  const people = participants.data?.pages.flatMap((page) => page.items) || [];
  const result = useInfiniteQuery({
    queryKey: ["content-search", user?.id, filters],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => api.search(filters!, pageParam, signal),
    enabled: !!filters && !capabilities.isPending,
    getNextPageParam: (page) =>
      page.items.length &&
      page.offset + page.items.length < page.total &&
      page.offset + page.items.length <= 10000
        ? page.offset + page.items.length
        : undefined,
    retry: false,
  });
  const matches = result.data?.pages.flatMap((page) => page.items) || [];
  return (
    <>
      <section className="page-heading">
        <div>
          <span className="eyebrow">ВАШИ ВСТРЕЧИ</span>
          <h1>Поиск по материалам</h1>
          <p>Найдите встречу, фразу в записи или договорённость.</p>
        </div>
      </section>
      <section className="content-card">
        <form
          className="search-form"
          onSubmit={(event) => {
            event.preventDefault();
            const text = query.trim();
            if (text.length < 2) {
              setValidation("Введите не менее двух символов.");
              return;
            }
            if (from && to && from > to) {
              setValidation("Дата начала должна быть не позже даты окончания.");
              return;
            }
            const next: SearchFilters = {
              q: text,
              source,
              conferenceId: conferenceId || undefined,
              from: from
                ? new Date(`${from}T00:00:00`).toISOString()
                : undefined,
              to: to ? localDayEnd(to) || undefined : undefined,
            };
            const url = new URLSearchParams({ q: text, source });
            url.set("mode", semanticAvailable ? mode || "keyword" : "keyword");
            url.set("membership", membership || "all");
            if (participantId) url.set("participantId", participantId);
            if (conferenceId) url.set("conferenceId", conferenceId);
            if (next.from) url.set("from", next.from);
            if (next.to) url.set("to", next.to);
            setValidation(undefined);
            setParams(url, { replace: true });
          }}
        >
          <label>
            Поисковый запрос
            <input
              type="search"
              value={query}
              maxLength={500}
              required
              placeholder="Например, план запуска"
              onChange={(event) => setQuery(event.target.value)}
            />
          </label>
          <div className="search-filters">
            <label>
              Способ поиска
              <select
                value={mode}
                onChange={(event) =>
                  setMode(event.target.value as SearchFilters["mode"])
                }
              >
                <option value="keyword">По ключевым словам</option>
                {semanticAvailable && (
                  <option value="semantic">По смыслу</option>
                )}
                {semanticAvailable && (
                  <option value="hybrid">Слова и смысл</option>
                )}
              </select>
            </label>
            {capabilities.isPending && (
              <p className="field-hint">Проверяем доступные способы поиска…</p>
            )}
            {!capabilities.isPending && !semanticAvailable && (
              <p className="field-hint">
                Поиск по смыслу сейчас недоступен. Используется поиск по
                ключевым словам.
              </p>
            )}
            <label>
              Моё участие
              <select
                value={membership}
                onChange={(event) =>
                  setMembership(
                    event.target.value as SearchFilters["membership"],
                  )
                }
              >
                <option value="all">Все доступные встречи</option>
                <option value="owned">Я организатор</option>
                <option value="participating">Я участник</option>
              </select>
            </label>
            <label>
              Где искать
              <select
                value={source}
                onChange={(event) =>
                  setSource(event.target.value as SearchSource)
                }
              >
                {Object.entries(labels).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Встреча
              <select
                value={conferenceId}
                onChange={(event) => {
                  setConferenceId(event.target.value);
                  setParticipantId("");
                }}
              >
                <option value="">Все доступные</option>
                {conferenceId &&
                  !rows.some((row) => row.id === conferenceId) && (
                    <option value={conferenceId}>Выбранная встреча</option>
                  )}
                {rows.map((row) => (
                  <option key={row.id} value={row.id}>
                    {row.title}
                  </option>
                ))}
              </select>
            </label>
            {conferenceId && (
              <label>
                Автор реплики
                <select
                  value={participantId}
                  onChange={(event) => setParticipantId(event.target.value)}
                >
                  <option value="">Все участники</option>
                  {participantId &&
                    !people.some((p) => p.id === participantId) && (
                      <option value={participantId}>Выбранный участник</option>
                    )}
                  {people.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.displayName || "Участник"}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              С даты
              <input
                type="date"
                value={from}
                onChange={(event) => setFrom(event.target.value)}
              />
            </label>
            <label>
              По дату
              <input
                type="date"
                value={to}
                onChange={(event) => setTo(event.target.value)}
              />
            </label>
          </div>
          {conferences.hasNextPage && (
            <Button
              type="button"
              variant="outline"
              busy={conferences.isFetchingNextPage}
              onClick={() => void conferences.fetchNextPage()}
            >
              Загрузить ещё встречи в фильтр
            </Button>
          )}
          {participants.hasNextPage && (
            <Button
              type="button"
              variant="outline"
              busy={participants.isFetchingNextPage}
              onClick={() => void participants.fetchNextPage()}
            >
              Загрузить ещё участников
            </Button>
          )}
          <ErrorNotice error={participants.error} />
          <p className="field-hint">
            Даты — в часовом поясе{" "}
            {Intl.DateTimeFormat().resolvedOptions().timeZone}. Учитываются
            только материалы, к которым у вас есть доступ.
          </p>
          {validation && <ErrorNotice>{validation}</ErrorNotice>}
          <Button
            type="submit"
            busy={result.isFetching && !result.isFetchingNextPage}
          >
            <Search size={18} />
            Найти
          </Button>
        </form>
      </section>
      <section className="content-card" aria-label="Результаты поиска">
        <h2>
          Результаты{result.data ? ` · ${result.data.pages[0].total}` : ""}
        </h2>
        <ErrorNotice error={result.error} />
        {result.data?.pages[0].fallbackReason && (
          <p role="status">
            Поиск по смыслу сейчас недоступен для выбранных материалов. Показаны
            результаты по ключевым словам.
          </p>
        )}
        {!filters ? (
          <p className="muted">Введите запрос, чтобы начать поиск.</p>
        ) : result.isPending ? (
          <Loading />
        ) : (
          !result.isError && (
            <>
              {!matches.length && (
                <p className="muted">
                  Ничего не найдено. Попробуйте другую фразу или измените
                  фильтры.
                </p>
              )}
              <ul className="search-results">
                {matches.map((item, index) => (
                  <li
                    key={`${item.type}:${item.conferenceId}:${item.segmentId || index}`}
                  >
                    <Link to={searchResultLink(item)}>
                      <strong>{item.conferenceTitle}</strong>
                    </Link>
                    <p className="field-hint">
                      {labels[item.type]}
                      {item.speaker ? ` · ${item.speaker}` : ""}
                      {typeof item.startMs === "number"
                        ? ` · ${recordingTime(item.startMs)}`
                        : ""}
                    </p>
                    <p>{item.snippet}</p>
                    {item.recordingId && (
                      <Link className="text-link" to={searchResultLink(item)}>
                        {typeof item.startMs === "number"
                          ? "Открыть фрагмент записи"
                          : "Открыть материалы"}
                      </Link>
                    )}
                  </li>
                ))}
              </ul>
              {result.hasNextPage && (
                <Button
                  variant="outline"
                  busy={result.isFetchingNextPage}
                  onClick={() => void result.fetchNextPage()}
                >
                  Ещё результаты
                </Button>
              )}
            </>
          )
        )}
      </section>
    </>
  );
}
