import { BarChart3 } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { AnalyticsPanel } from "../components/AnalyticsPanel";
import { Button, ErrorNotice, Loading } from "../components/ui";
import { useConferences } from "../queries";
import { useCapabilities } from "../useCapabilities";
import { formatDate } from "../utils";
import "./analytics.css";

/** Открывает аналитику только после подтверждения серверной возможности.
 * @return Выбор реальной встречи либо состояние недоступной функции.
 */
export function AnalyticsPage() {
  const capabilities = useCapabilities();
  return (
    <>
      <section className="page-heading">
        <div>
          <h1>Аналитика</h1>
          <p>Объективные показатели ваших встреч.</p>
        </div>
      </section>
      <ErrorNotice error={capabilities.error} />
      {capabilities.isPending ? (
        <Loading />
      ) : capabilities.isSuccess &&
        !capabilities.isError &&
        capabilities.data.capabilities.meetingAnalytics ? (
        <MeetingAnalyticsSelection />
      ) : (
        <section className="content-card analytics-empty">
          <BarChart3 size={32} aria-hidden="true" />
          <h2>Аналитика сейчас недоступна</h2>
          <p>
            Функция не включена администратором или её доступность не удалось
            проверить.
          </p>
          <Link className="text-link" to="/history">
            Вернуться к истории встреч
          </Link>
        </section>
      )}
    </>
  );
}

/** Загружает завершённые либо активные встречи с явным серверным фильтром и пагинацией.
 * @return Выбор одной встречи и её собственные показатели, а не выдуманные общие KPI.
 */
function MeetingAnalyticsSelection() {
  const [params, setParams] = useSearchParams();
  const view = params.get("view") === "active" ? "active" : "past";
  const query = useConferences({ view });
  const meetings = query.data?.pages.flatMap((page) => page.items) || [];
  const selectedId = params.get("conference") || meetings[0]?.id || "";
  const selected = meetings.find((meeting) => meeting.id === selectedId);
  return (
    <>
      <section className="content-card analytics-selector">
        <div className="analytics-selector-fields">
          <label className="field">
            <span>Статус встречи</span>
            <select
              value={view}
              onChange={(event) => {
                const next = new URLSearchParams(params);
                next.set("view", event.target.value);
                next.delete("conference");
                setParams(next, { replace: true });
              }}
            >
              <option value="past">Завершённые</option>
              <option value="active">Активные</option>
            </select>
          </label>
          <label className="field">
            <span>Встреча для анализа</span>
            <select
              value={selectedId}
              onChange={(event) => {
                const next = new URLSearchParams(params);
                next.set("conference", event.target.value);
                setParams(next, { replace: true });
              }}
              disabled={query.isPending || (!meetings.length && !selectedId)}
            >
              {!selectedId && <option value="">Выберите встречу</option>}
              {selectedId && !selected && (
                <option value={selectedId}>Встреча по ссылке</option>
              )}
              {meetings.map((meeting) => (
                <option key={meeting.id} value={meeting.id}>
                  {meeting.title} ·{" "}
                  {formatDate(
                    meeting.startedAt ||
                      meeting.scheduledAt ||
                      meeting.createdAt,
                  )}
                </option>
              ))}
            </select>
          </label>
        </div>
        <p className="field-hint">
          Показатели относятся только к выбранной встрече. Сводные показатели по
          всем встречам сервер пока не предоставляет.
        </p>
        <ErrorNotice error={query.error} />
        {query.isPending && <Loading />}
        {!query.isPending &&
          !query.isError &&
          !meetings.length &&
          !selectedId && (
            <p className="muted">
              {view === "past"
                ? "Завершённых встреч пока нет. Их показатели появятся здесь после завершения."
                : "Сейчас нет активных встреч. Выберите завершённые встречи, чтобы посмотреть их показатели."}
            </p>
          )}
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
      {selectedId && !query.isError && (
        <AnalyticsPanel
          key={selectedId}
          conferenceId={selectedId}
          active={selected?.status === "active"}
        />
      )}
    </>
  );
}
