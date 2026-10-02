import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import { recordingTime } from "../intelligence";
import { ErrorNotice } from "./ui";

/** AnalyticsPanel показывает технические агрегаты в алфавитном порядке без рейтинга участников.
 * @args conferenceId — встреча с подтверждённым доступом; active — обновлять ли текущие агрегаты.
 * @return Длительность, присутствие, приблизительная речь и счётчики взаимодействий.
 */
export function AnalyticsPanel({
  conferenceId,
  active,
}: {
  conferenceId: string;
  active: boolean;
}) {
  const query = useQuery({
    queryKey: ["analytics", conferenceId],
    queryFn: ({ signal }) => api.analytics(conferenceId, signal),
    refetchInterval: active ? 30000 : false,
    retry: false,
  });
  const value = query.data?.item;
  return (
    <section className="content-card" aria-label="Аналитика встречи">
      <h2>Аналитика встречи</h2>
      <ErrorNotice error={query.error} />
      {!value?.enabled ? (
        <p className="muted">
          Агрегаты ещё не готовы или сбор аналитики отключён.
        </p>
      ) : (
        <>
          <p>
            Длительность: {recordingTime(value.durationMs)} · Участников:{" "}
            {value.participantCount}
          </p>
          <p className="field-hint">
            Речевая активность — приблизительная оценка по уровню аудио. Шум и
            музыка могут учитываться как речь; пропуски наблюдения не считаются
            молчанием. Это не оценка продуктивности.
          </p>
          <p>
            Запись: {value.recordingAvailable ? "доступна" : "нет"} ·
            Расшифровка: {value.transcriptAvailable ? "доступна" : "нет"}
          </p>
          <div className="analytics-scroll">
            <table>
              <caption>Технические показатели участников</caption>
              <thead>
                <tr>
                  <th>Участник</th>
                  <th>Присутствие</th>
                  <th>Речь ≈</th>
                  <th>Наблюдалось аудио</th>
                  <th>Экран</th>
                  <th>Сообщения</th>
                  <th>Руки</th>
                </tr>
              </thead>
              <tbody>
                {value.participants.map((p) => (
                  <tr key={p.participantId}>
                    <th scope="row">{p.displayName || "Участник"}</th>
                    <td>{recordingTime(p.participationMs)}</td>
                    <td>{recordingTime(p.speakingMs)}</td>
                    <td>{recordingTime(p.observedAudioMs)}</td>
                    <td>{recordingTime(p.screenMs)}</td>
                    <td>{p.messageCount}</td>
                    <td>{p.handRaises}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <details>
            <summary>Участники во времени</summary>
            <ul className="analytics-timeline">
              {value.timeline.map((p) => (
                <li key={p.atMs}>
                  {recordingTime(p.atMs)} — {p.count}
                </li>
              ))}
            </ul>
          </details>
        </>
      )}
    </section>
  );
}
