import { ErrorNotice, Loading } from "../ui";
import type { ConferenceViewProps } from "./types";

/**
 * Отображает сохранённые итоги завершённой встречи без подключения медиасвязи.
 * @args props — общие данные, команды и состояние интерфейса, созданные корневой страницей.
 * @return Представление раздела с прежними условиями доступа и монтирования панелей.
 */
export function ConferenceHistorySummary(props: ConferenceViewProps) {
  const { admitted, closed, history } = props.data;

  return (
    <>
      {admitted && closed && (
        <section
          className="content-card history-summary"
          aria-label="История встречи"
        >
          <h2>Итоги встречи</h2>
          <ErrorNotice error={history.error} />
          {history.isPending ? (
            <Loading />
          ) : (
            history.data && (
              <dl>
                <div>
                  <dt>Организатор</dt>
                  <dd>
                    {history.data.item.owner.displayName ||
                      "Организатор встречи"}
                  </dd>
                </div>
                <div>
                  <dt>Длительность</dt>
                  <dd>
                    {history.data.item.durationSec === null
                      ? "—"
                      : `${Math.max(1, Math.round(history.data.item.durationSec / 60))} мин`}
                  </dd>
                </div>
                <div>
                  <dt>Участников</dt>
                  <dd>{history.data.item.participantCount}</dd>
                </div>
                <div>
                  <dt>Записи</dt>
                  <dd>
                    {history.data.item.recordings.ready} готово ·{" "}
                    {history.data.item.recordings.processing} обрабатывается
                  </dd>
                </div>
              </dl>
            )
          )}
        </section>
      )}
    </>
  );
}
