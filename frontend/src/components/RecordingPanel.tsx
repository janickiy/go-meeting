import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Circle, Download, Square } from "lucide-react";
import { api } from "../api";
import type { Conference, Participant } from "../types";
import { Button, ErrorNotice } from "./ui";
import { formatDate } from "../utils";
import { isAdmitted } from "../collaboration";

const labels = {
  starting: "Запись запускается",
  recording: "Идёт запись",
  stopping: "Запись останавливается",
  processing: "Обрабатываем запись",
  ready: "Запись готова",
  failed: "Не удалось завершить запись",
  cancelled: "Запись отменена",
};
/**
 * RecordingPanel показывает состояние записи и разрешённые действия запуска, остановки и чтения артефактов.
 *
 * @parameters:
 *   - объект параметров: conference — свойство текущего компонента; membership — свойство текущего компонента.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function RecordingPanel({
  conference,
  membership,
}: {
  conference: Conference;
  membership?: Participant;
}) {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: ["recordings", conference.id],
    /**
     * queryFn загружает данные запроса с его сигналом отмены для кеша React Query.
     *
     * @parameters:
     *   - объект параметров: signal — сигнал отмены запроса или потока.
     *
     * @returns вычисленное значение: api.recordings(conference.id, signal).
     */
    queryFn: ({ signal }) => api.recordings(conference.id, signal),
    enabled: isAdmitted(membership),
    refetchInterval: 3000,
  });
  const items = query.data?.items || [];
  const current = items.find(
    /**
     * Обработчик items.find проверяет условие поиска элемента или соответствия элементов набора.
     *
     * @parameters:
     *   - item — элемент списка, который обрабатывает текущий шаг.
     *
     * @returns логический признак соответствия элемента условию.
     */ (item) =>
      ["starting", "recording", "stopping", "processing"].includes(item.status),
  );
  const recording =
    current && ["starting", "recording", "stopping"].includes(current.status);
  const mutation = useMutation({
    /**
     * mutationFn выполняет изменяющий запрос по переданным параметрам действия.
     *
     * @parameters:
     *   - stopId (string | null) — идентификатор останавливаемой записи.
     *
     * @returns вычисленное значение: stopId ? api.stopRecording(conference.id, stopId) : api.startRecording(conference.id).
     */
    mutationFn: (stopId: string | null) =>
      stopId
        ? api.stopRecording(conference.id, stopId)
        : api.startRecording(conference.id),
    /**
     * onSettled обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
     *
     *
     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    onSettled: () => {
      void client.invalidateQueries({
        queryKey: ["recordings", conference.id],
      });
    },
  });
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
          {!current ? (
            <Button
              onClick={
                /**
                 * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                 *
                 *
                 * @returns вычисленное значение: mutation.mutate(null).
                 */ () => mutation.mutate(null)
              }
              busy={mutation.isPending}
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
                   * @returns вычисленное значение: mutation.mutate(current.uuid).
                   */ () => mutation.mutate(current.uuid)
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
      {!items.length && !query.isError && (
        <p className="muted">
          Записей пока нет. Владелец может начать запись во время встречи.
        </p>
      )}
      <div className="recording-list">
        {items.map(
          /**
           * Обработчик items.map преобразует один элемент набора в представление или данные следующего шага.
           *
           * @parameters:
           *   - item — элемент списка, который обрабатывает текущий шаг.
           *
           * @returns преобразованное значение текущего элемента для результирующего набора.
           */ (item) => {
            const file = item.files?.find(
              /**
               * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
               *
               * @parameters:
               *   - f — метаданные одного файла записи.
               *
               * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
               */ (f) => f.fileType === "final_mp4",
            );
            const preview = item.files?.find(
              /**
               * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
               *
               * @parameters:
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
                    Скачать MP4
                  </a>
                )}
              </article>
            );
          },
        )}
      </div>
    </section>
  );
}
