import { lazy, Suspense, useId } from "react";
import { Link, useParams, useSearchParams } from "react-router";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { ArrowLeft, Clock3, Play, Users } from "lucide-react";
import { api } from "../api";
import { isAdmitted } from "../collaboration";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { useMembership } from "../queries";
import { formatDate } from "../utils";
import { useCapabilities } from "../useCapabilities";
import "./history-notifications.css";
import type { ConferenceHistory } from "../types";
import { playableRecordings } from "../recordingPresentation";
import { RecordingCard } from "./RecordingsPage";

const AnalyticsPanel = lazy(() =>
  import("../components/AnalyticsPanel").then((module) => ({
    default: module.AnalyticsPanel,
  })),
);
const RecordingPanel = lazy(() =>
  import("../components/RecordingPanel").then((module) => ({
    default: module.RecordingPanel,
  })),
);

const sections = [
  ["overview", "Обзор"],
  ["recording", "Запись"],
  ["transcript", "Расшифровка"],
  ["summary", "Итоги ИИ"],
  ["analytics", "Аналитика"],
] as const;
type Section = (typeof sections)[number][0];

/** Материалы завершённой встречи загружаются только при открытии нужной вкладки. */
export function HistoryDetailPage() {
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const tabId = useId();
  const capabilities = useCapabilities();
  const requested = params.get("section");
  const history = useQuery({
    queryKey: ["history", id],
    queryFn: ({ signal }) => api.history(id, signal),
    enabled: !!id,
    retry: false,
  });
  const membership = useMembership(id, false);
  const features =
    capabilities.isSuccess && !capabilities.isError
      ? capabilities.data.capabilities
      : undefined;
  const visibleSections = sections.filter(([value]) => {
    if (value === "transcript") return features?.transcription === true;
    if (value === "summary") return features?.aiSummary === true;
    if (value === "analytics") return features?.meetingAnalytics === true;
    return true;
  });
  const section: Section = visibleSections.some(
    ([value]) => value === requested,
  )
    ? (requested as Section)
    : "overview";

  /** Переключает раздел без потери выбранной записи и временной метки.
   * @args nextSection — доступная вкладка истории.
   */
  function openSection(nextSection: Section) {
    const next = new URLSearchParams(params);
    next.set("section", nextSection);
    if (nextSection === "summary" || nextSection === "transcript")
      next.set("tab", nextSection);
    setParams(next, { replace: true });
  }

  if (history.isPending || membership.isPending) return <Loading />;
  if (history.isError || membership.isError || !history.data)
    return (
      <section className="content-card">
        <h1>История встречи недоступна</h1>
        <ErrorNotice error={history.error || membership.error} />
        <Link className="text-link" to="/history">
          Вернуться к истории
        </Link>
      </section>
    );
  const item = history.data.item;
  if (!isAdmitted(membership.data))
    return (
      <section className="content-card">
        <h1>История встречи недоступна</h1>
        <p>Для просмотра материалов требуется доступ участника.</p>
        <Link className="text-link" to="/history">
          Вернуться к истории
        </Link>
      </section>
    );

  return (
    <>
      <Link className="back-link" to="/history">
        <ArrowLeft size={17} />
        История встреч
      </Link>
      <section className="page-heading">
        <div>
          <h1>{item.conference.title}</h1>
          <p className="history-heading-meta">
            <span>
              {formatDate(
                item.conference.finishedAt || item.conference.createdAt,
              )}
            </span>
            {item.durationSec !== null && (
              <span>
                <Clock3 size={15} aria-hidden="true" />
                {Math.max(1, Math.round(item.durationSec / 60))} мин
              </span>
            )}
            <span>
              <Users size={15} aria-hidden="true" />
              {item.participantCount} участников
            </span>
          </p>
        </div>
        {item.chatAvailable && (
          <Link className="button button-secondary" to={`/conferences/${id}`}>
            Открыть чат в конференции
          </Link>
        )}
      </section>
      <p className="history-secondary-context">
        Материалы завершённой встречи · чат только для чтения
      </p>
      <div
        className="insight-tabs history-tabs"
        role="tablist"
        aria-label="Материалы встречи"
      >
        {visibleSections.map(([value, label], index) => (
          <button
            key={value}
            type="button"
            role="tab"
            id={`${tabId}-${value}`}
            aria-controls={`${tabId}-panel`}
            aria-selected={section === value}
            tabIndex={section === value ? 0 : -1}
            onClick={() => openSection(value)}
            onKeyDown={(event) => {
              if (
                !["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)
              )
                return;
              event.preventDefault();
              const target =
                event.key === "Home"
                  ? 0
                  : event.key === "End"
                    ? visibleSections.length - 1
                    : (index +
                        (event.key === "ArrowRight" ? 1 : -1) +
                        visibleSections.length) %
                      visibleSections.length;
              const next = visibleSections[target][0];
              openSection(next);
              document.getElementById(`${tabId}-${next}`)?.focus();
            }}
          >
            {label}
          </button>
        ))}
      </div>
      <ErrorNotice error={capabilities.error} />
      {requested &&
        requested !== section &&
        sections.some(([value]) => value === requested) &&
        !capabilities.isPending && (
          <p role="status" className="field-hint">
            Этот раздел сейчас недоступен. Показан обзор встречи.
          </p>
        )}
      <div
        className="history-material"
        role="tabpanel"
        id={`${tabId}-panel`}
        aria-labelledby={`${tabId}-${section}`}
      >
        <Suspense fallback={<Loading />}>
          {section === "overview" && (
            <div className="history-overview-grid">
              <section
                className="content-card history-summary"
                aria-label="Обзор встречи"
              >
                <h2>Обзор встречи</h2>
                <dl>
                  <div>
                    <dt>Организатор</dt>
                    <dd>{item.owner.displayName || "Организатор встречи"}</dd>
                  </div>
                  <div>
                    <dt>Длительность</dt>
                    <dd>
                      {item.durationSec === null
                        ? "—"
                        : `${Math.max(1, Math.round(item.durationSec / 60))} мин`}
                    </dd>
                  </div>
                  <div>
                    <dt>Участников</dt>
                    <dd>{item.participantCount}</dd>
                  </div>
                  <div>
                    <dt>Записи</dt>
                    <dd>
                      {item.recordings.ready} готово ·{" "}
                      {item.recordings.processing} обрабатывается
                    </dd>
                  </div>
                </dl>
                <Button onClick={() => openSection("recording")}>
                  <Play size={16} aria-hidden="true" /> Открыть записи встречи
                </Button>
              </section>
              {item.participants.length > 0 && (
                <section className="content-card">
                  <h2>Участники</h2>
                  <ul className="history-participants">
                    {item.participants.map((person) => (
                      <li key={person.id}>
                        <span className="avatar" aria-hidden="true">
                          {person.displayName
                            .trim()
                            .split(/\s+/)
                            .slice(0, 2)
                            .map((part) => part[0])
                            .join("")}
                        </span>
                        {person.displayName}
                      </li>
                    ))}
                  </ul>
                  {item.participantsTruncated && (
                    <p className="field-hint">Показана часть участников.</p>
                  )}
                </section>
              )}
            </div>
          )}
          {section === "recording" && <HistoryRecordingList history={item} />}
          {["transcript", "summary"].includes(section) && (
            <RecordingPanel
              conference={item.conference}
              membership={membership.data || undefined}
              showInsights
              showHistory={false}
              materialOnly
            />
          )}
          {section === "analytics" && (
            <AnalyticsPanel conferenceId={id} active={false} />
          )}
        </Suspense>
      </div>
    </>
  );
}

/** Показывает готовые записи из реальных страниц API в композиции библиотеки.
 * @args history — подтверждённая история встречи с организатором и участниками.
 * @return Карточки доступных файлов и загрузка следующих записей без статусов обработки.
 */
function HistoryRecordingList({ history }: { history: ConferenceHistory }) {
  const query = useInfiniteQuery({
    queryKey: ["history-recording-library", history.conference.id],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      api.recordings(history.conference.id, signal, {
        limit: 20,
        offset: pageParam,
      }),
    getNextPageParam: (last, _pages, offset) =>
      last.items.length === 20 ? offset + 20 : undefined,
    retry: false,
  });
  const items = playableRecordings(
    query.data?.pages.flatMap((page) => page.items) || [],
  );
  return (
    <section aria-label="Записи встречи">
      <ErrorNotice error={query.error} />
      {query.isPending ? (
        <Loading />
      ) : (
        <div className="recordings-list">
          {items.map((record) => (
            <RecordingCard
              key={record.uuid}
              record={record}
              conference={history.conference}
              history={history}
            />
          ))}
        </div>
      )}
      {!query.isPending && !query.isError && !items.length && (
        <div className="recordings-empty">
          <Play size={32} aria-hidden="true" />
          <h2>Доступных записей пока нет</h2>
          <p>Материалы появятся здесь, если во время встречи велась запись.</p>
        </div>
      )}
      {query.isError && (
        <Button variant="outline" onClick={() => void query.refetch()}>
          Попробовать снова
        </Button>
      )}
      {query.hasNextPage && (
        <div className="recordings-pagination">
          <Button
            variant="outline"
            busy={query.isFetchingNextPage}
            onClick={() => void query.fetchNextPage()}
          >
            Ещё записи
          </Button>
        </div>
      )}
    </section>
  );
}
