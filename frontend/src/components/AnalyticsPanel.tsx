import { useQuery } from "@tanstack/react-query";
import { Clock3, Hand, MessageCircle, Users } from "lucide-react";
import { api } from "../api";
import { recordingTime } from "../intelligence";
import { useCapabilities } from "../useCapabilities";
import { ErrorNotice, Loading } from "./ui";
import "../pages/analytics.css";

/** Показывает объективные агрегаты одной встречи без рейтинга участников.
 * @args conferenceId — встреча с серверной проверкой доступа; active — периодическое обновление текущей встречи.
 * @return Реальные показатели встречи, временная шкала и алфавитная таблица участников.
 */
export function AnalyticsPanel({
  conferenceId,
  active,
}: {
  conferenceId: string;
  active: boolean;
}) {
  const capabilities = useCapabilities();
  const enabled =
    capabilities.isSuccess &&
    !capabilities.isError &&
    capabilities.data.capabilities.meetingAnalytics === true;
  const query = useQuery({
    queryKey: ["analytics", conferenceId],
    queryFn: ({ signal }) => api.analytics(conferenceId, signal),
    enabled: !!conferenceId && enabled,
    refetchInterval: enabled && active ? 30000 : false,
    retry: false,
  });
  const value = query.isError ? undefined : query.data?.item;
  const people = [...(value?.participants || [])].sort((a, b) =>
    (a.displayName || "Участник").localeCompare(
      b.displayName || "Участник",
      "ru",
    ),
  );
  const timeline = [...(value?.timeline || [])].sort((a, b) => a.atMs - b.atMs);
  const timelineMax = Math.max(1, ...timeline.map((point) => point.count));
  const timelineEnd = Math.max(
    1,
    value?.durationMs || 0,
    timeline.at(-1)?.atMs || 0,
  );
  return (
    <section className="analytics-panel" aria-label="Аналитика встречи">
      <div className="section-heading">
        <h2>Аналитика встречи</h2>
        {enabled && active && (
          <span className="field-hint">Обновляется каждые 30 секунд</span>
        )}
      </div>
      <ErrorNotice error={capabilities.error || query.error} />
      {capabilities.isPending || (enabled && query.isPending) ? (
        <Loading />
      ) : !enabled ? (
        <p className="content-card muted">
          Аналитика недоступна или выключена администратором.
        </p>
      ) : !query.isError && !value?.enabled ? (
        <p className="content-card muted">
          Агрегаты ещё не готовы или сбор аналитики отключён.
        </p>
      ) : value?.enabled ? (
        <>
          <dl className="analytics-metrics">
            <div>
              <dt>
                <Clock3 size={18} aria-hidden="true" /> Длительность
              </dt>
              <dd>{recordingTime(value.durationMs)}</dd>
            </div>
            <div>
              <dt>
                <Users size={18} aria-hidden="true" /> Участников
              </dt>
              <dd>{value.participantCount}</dd>
            </div>
            <div>
              <dt>
                <MessageCircle size={18} aria-hidden="true" /> Сообщений
              </dt>
              <dd>
                {people.reduce(
                  (total, person) => total + person.messageCount,
                  0,
                )}
              </dd>
            </div>
            <div>
              <dt>
                <Hand size={18} aria-hidden="true" /> Поднятий руки
              </dt>
              <dd>
                {people.reduce((total, person) => total + person.handRaises, 0)}
              </dd>
            </div>
          </dl>
          <div className="analytics-chart-grid">
            <section
              className="content-card analytics-chart-card"
              aria-labelledby={`timeline-title-${conferenceId}`}
            >
              <h3 id={`timeline-title-${conferenceId}`}>
                Участники во времени
              </h3>
              {timeline.length ? (
                <>
                  <svg
                    className="analytics-timeline-chart"
                    viewBox="0 0 600 160"
                    preserveAspectRatio="none"
                    aria-hidden="true"
                  >
                    {timeline.map((point, index) => {
                      const height = Math.max(
                        0,
                        (point.count / timelineMax) * 160,
                      );
                      const end = timeline[index + 1]?.atMs ?? timelineEnd;
                      return (
                        <rect
                          key={point.atMs}
                          x={(point.atMs / timelineEnd) * 600}
                          y={160 - height}
                          width={Math.max(
                            0,
                            ((end - point.atMs) / timelineEnd) * 600,
                          )}
                          height={height}
                        >
                          <title>
                            {recordingTime(point.atMs)} — {point.count}{" "}
                            участников
                          </title>
                        </rect>
                      );
                    })}
                  </svg>
                  <p className="analytics-chart-axis">
                    <span>00:00</span>
                    <span>{recordingTime(timelineEnd)}</span>
                  </p>
                  <details>
                    <summary>Показать значения графика</summary>
                    <ul className="analytics-timeline">
                      {timeline.map((point) => (
                        <li key={point.atMs}>
                          {recordingTime(point.atMs)} — {point.count}
                        </li>
                      ))}
                    </ul>
                  </details>
                </>
              ) : (
                <p className="muted">
                  Для этой встречи временная шкала пока не собрана.
                </p>
              )}
            </section>
            <section className="content-card analytics-context">
              <h3>Материалы и точность</h3>
              <dl>
                <div>
                  <dt>Запись</dt>
                  <dd>{value.recordingAvailable ? "Доступна" : "Нет"}</dd>
                </div>
                <div>
                  <dt>Расшифровка</dt>
                  <dd>{value.transcriptAvailable ? "Доступна" : "Нет"}</dd>
                </div>
              </dl>
              <p className="field-hint">
                Речевая активность — приблизительная оценка по уровню аудио. Шум
                и музыка могут учитываться как речь; пропуски наблюдения не
                считаются молчанием. Это не оценка продуктивности.
              </p>
            </section>
          </div>
          <div
            className="content-card analytics-scroll"
            tabIndex={0}
            role="region"
            aria-label="Таблица показателей участников"
          >
            <table>
              <caption>Технические показатели участников</caption>
              <thead>
                <tr>
                  <th scope="col">Участник</th>
                  <th scope="col">Присутствие</th>
                  <th scope="col">Речь ≈</th>
                  <th scope="col">Наблюдалось аудио</th>
                  <th scope="col">Экран</th>
                  <th scope="col">Сообщения</th>
                  <th scope="col">Руки</th>
                </tr>
              </thead>
              <tbody>
                {people.map((person) => (
                  <tr key={person.participantId}>
                    <th scope="row">{person.displayName || "Участник"}</th>
                    <td>{recordingTime(person.participationMs)}</td>
                    <td>{recordingTime(person.speakingMs)}</td>
                    <td>{recordingTime(person.observedAudioMs)}</td>
                    <td>{recordingTime(person.screenMs)}</td>
                    <td>{person.messageCount}</td>
                    <td>{person.handRaises}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {!people.length && (
              <p className="muted">Показатели участников ещё не собраны.</p>
            )}
          </div>
        </>
      ) : null}
    </section>
  );
}
