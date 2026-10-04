import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { Circle, Download, Square } from "lucide-react";
import { api } from "../api";
import type {
  Conference,
  ConferenceRecording,
  Items,
  Participant,
  RecordingMode,
} from "../types";
import { Button, ErrorNotice } from "./ui";
import { formatDate } from "../utils";
import { isAdmitted } from "../collaboration";
import { RecordingInsights } from "./RecordingInsights";
import { useCapabilities } from "../useCapabilities";

const labels = {
  starting: "Запись запускается",
  recording: "Идёт запись",
  degraded: "Запись продолжается с ограничениями",
  stopping: "Запись останавливается",
  processing: "Обрабатываем запись",
  ready: "Запись готова",
  failed: "Не удалось завершить запись",
  cancelled: "Запись отменена",
};
/**
 * RecordingPanel показывает состояние записи и разрешённые действия запуска, остановки и чтения артефактов.
 *
 * @args
 *   - conference — текущая конференция; membership — членство и права текущего пользователя.
 *   - showHistory — показывать список прошлых записей и их файлов; в диалоге управления отключается.
 *   - showInsights — показывать доступные результаты обработки записи.
 *   - onStarted — уведомить родительский интерфейс только после успешного запроса запуска записи.
 *
 * @return JSX-представление состояния записи и разрешённых действий.
 */
export function RecordingPanel({
  conference,
  membership,
  showHistory = true,
  showInsights = false,
  onStarted,
}: {
  conference: Conference;
  membership?: Participant;
  showHistory?: boolean;
  showInsights?: boolean;
  onStarted?: () => void;
}) {
  const client = useQueryClient();
  const capabilities = useCapabilities();
  const recordingModes = capabilities.data?.capabilities.recordingModes ?? [];
  const [mode, setMode] = useState<RecordingMode>("composite");
  const commandPending = useRef(false);
  const selectedMode = recordingModes.includes(mode) ? mode : recordingModes[0];
  const query = useQuery({
    queryKey: ["recordings", conference.id],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @args
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.recordings(conference.id, signal).
     */
    queryFn: ({ signal }) => api.recordings(conference.id, signal),
    enabled: isAdmitted(membership),
    refetchInterval: conference.status === "active" ? 3000 : false,
  });
  const items = query.data?.items || [];
  const current = items.find(
    /**
     * Обработчик items.find проверяет условие поиска элемента или соответствия элементов набора.
     *
     * @args
     *   - item — элемент списка, который обрабатывает текущий шаг.
     *
     * @returns логический признак соответствия элемента условию.
     */ (item) =>
      item.conferenceId === conference.id &&
      ["starting", "recording", "degraded", "stopping", "processing"].includes(
        item.status,
      ),
  );
  // Нельзя считать отсутствие загруженных данных подтверждением, что запись свободна.
  const canStart =
    query.isSuccess &&
    !query.isFetching &&
    !query.isError &&
    !current &&
    Boolean(selectedMode) &&
    conference.status === "active";
  const recording =
    current &&
    ["starting", "recording", "degraded", "stopping"].includes(current.status);
  const mutation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @args
     *   - stopId (string | null) — идентификатор останавливаемой записи.
     *
     * @return Подтверждённая карточка записи либо отказ запуска до проверки её состояния.
     */
    mutationFn: (stopId: string | null) => {
      if (stopId) return api.stopRecording(conference.id, stopId);
      if (!canStart || !selectedMode)
        return Promise.reject(new Error("Запуск записи сейчас недоступен"));
      return api.startRecording(conference.id, selectedMode);
    },
    /**
     * Уведомляет родителя об успешном запуске, не закрывая интерфейс при остановке записи или ошибке.
     * @args response — подтверждение сервера; stopId — null для запуска либо UUID останавливаемой записи.
     * @return Значение не возвращается; родитель при необходимости закрывает окно управления записью.
     */
    onSuccess: (response, stopId) => {
      client.setQueryData<Items<ConferenceRecording>>(
        ["recordings", conference.id],
        /**
         * Сразу сохраняет ответ сервера, чтобы повторный запуск и верхняя панель учитывали новую запись.
         * @args cached — ранее загруженный список записей.
         * @return Список с подтверждённой карточкой запуска или остановки.
         */
        (cached) => ({
          status: "success",
          ...cached,
          items: [
            response.item,
            ...(cached?.items ?? []).filter(
              (item) => item.uuid !== response.item.uuid,
            ),
          ],
        }),
      );
      if (stopId === null) onStarted?.();
    },
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      commandPending.current = false;
      void client.invalidateQueries({
        queryKey: ["recordings", conference.id],
      });
    },
  });
  /**
   * Блокирует повторный щелчок сразу, до обновления асинхронного состояния запроса.
   * @args stopId — UUID для остановки либо null для запуска единственной записи.
   * @return Значение не возвращается; запрещённый или уже отправленный запрос игнорируется.
   */
  function runCommand(stopId: string | null) {
    if (commandPending.current || mutation.isPending || (!stopId && !canStart))
      return;
    commandPending.current = true;
    mutation.mutate(stopId);
  }
  const owner = membership?.role === "owner" && membership.status === "joined";
  if (!membership || !isAdmitted(membership)) return null;
  return (
    <section
      className="content-card recording-panel"
      aria-label="Записи конференции"
    >
      <div className="section-heading">
        <h2>Записи конференции</h2>
        {current && (
          <span
            role="status"
            data-testid="recording-indicator"
            className={recording ? "recording-indicator" : "participant-status"}
          >
            {recording && <Circle size={10} fill="currentColor" />}{" "}
            {labels[current.status]}
          </span>
        )}
      </div>
      <ErrorNotice error={query.error || mutation.error} />
      {owner && conference.status === "active" && (
        <div className="meeting-actions">
          {!current && recordingModes.length > 0 && (
            <label>
              Режим записи
              <select
                value={selectedMode}
                disabled={mutation.isPending || !canStart}
                onChange={(event) =>
                  setMode(event.target.value as RecordingMode)
                }
              >
                {recordingModes.includes("composite") && (
                  <option value="composite">Общая видеозапись</option>
                )}
                {recordingModes.includes("audio_only") && (
                  <option value="audio_only">Только аудио</option>
                )}
                {recordingModes.includes("individual_tracks") && (
                  <option value="individual_tracks">
                    Отдельные дорожки + аудиомикс
                  </option>
                )}
                {recordingModes.includes("screen_focus") && (
                  <option value="screen_focus">Фокус на экране</option>
                )}
              </select>
            </label>
          )}
          {!current && recordingModes.length === 0 && (
            <p className="field-hint" role="status">
              {capabilities.isPending
                ? "Проверяем доступные режимы записи…"
                : "Режимы записи сейчас недоступны."}
            </p>
          )}
          {!current && recordingModes.length > 0 ? (
            <Button
              onClick={
                /**
                 * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                 *
                 *
                 * @return Значение не возвращается; запрос отправляется с защитой от повтора.
                 */ () => runCommand(null)
              }
              busy={mutation.isPending}
              disabled={!canStart}
            >
              <Circle size={16} />
              Начать запись
            </Button>
          ) : (
            recording && (
              <Button
                variant="danger"
                onClick={
                  /**
                   * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                   *
                   *
                   * @return Значение не возвращается; запрос остановки отправляется один раз.
                   */ () => runCommand(current.uuid)
                }
                busy={mutation.isPending}
                disabled={current.status === "stopping"}
              >
                <Square size={16} />
                Остановить запись
              </Button>
            )
          )}
        </div>
      )}
      {recording && (
        <p className="field-hint">
          {current.status === "starting"
            ? "Запись запускается. Для начала нужен медиапоток участника."
            : "Звук, камеры и демонстрация экрана записываются."}{" "}
          Все участники видят этот индикатор.
        </p>
      )}
      {query.isPending && (
        <p className="field-hint" role="status">
          Проверяем состояние записи…
        </p>
      )}
      {showHistory && !items.length && query.isSuccess && !query.isError && (
        <p className="muted">
          Записей пока нет. Владелец может начать запись во время встречи.
        </p>
      )}
      {showHistory && (
        <div className="recording-list">
          {items.map(
            /**
             * Обработчик items.map преобразует один элемент набора в представление или данные следующего шага.
             *
             * @args
             *   - item — элемент списка, который обрабатывает текущий шаг.
             *
             * @returns преобразованное значение текущего элемента для результирующего набора.
             */ (item) => {
              const file = item.files?.find(
                /**
                 * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
                 *
                 * @args
                 *   - f — метаданные одного файла записи.
                 *
                 * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                 */ (f) =>
                  f.fileType === "final_mp4" || f.fileType === "final_audio",
              );
              const preview = item.files?.find(
                /**
                 * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
                 *
                 * @args
                 *   - f — метаданные одного файла записи.
                 *
                 * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                 */ (f) => f.fileType === "preview_jpg",
              );
              return (
                <article
                  key={item.uuid}
                  className="recording-row"
                  data-testid={`recording-${item.uuid}`}
                >
                  {preview?.url && (
                    <a href={preview.url} target="_blank" rel="noreferrer">
                      Посмотреть превью
                    </a>
                  )}
                  <div>
                    <strong>{labels[item.status] || item.status}</strong>
                    <p className="field-hint">
                      {formatDate(item.createdAt)}
                      {item.durationSec ? ` · ${item.durationSec} с` : ""}
                    </p>
                  </div>
                  {file?.url && item.status === "ready" && (
                    <a
                      className="text-link"
                      href={file.url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      <Download size={16} />
                      {file.fileType === "final_audio"
                        ? "Скачать аудио"
                        : "Скачать MP4"}
                    </a>
                  )}
                  {item.status === "ready" &&
                    item.files
                      .filter((f) => f.fileType === "tracks_archive" && f.url)
                      .map((f) => (
                        <a
                          key={f.fileType}
                          className="text-link"
                          href={f.url}
                          target="_blank"
                          rel="noreferrer"
                        >
                          Скачать дорожки и манифест (ZIP)
                        </a>
                      ))}
                </article>
              );
            },
          )}
        </div>
      )}
      {showInsights && (
        <RecordingInsights
          conferenceId={conference.id}
          membership={membership}
          recordings={items}
        />
      )}
    </section>
  );
}
