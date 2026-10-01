import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Circle, Download, Square } from "lucide-react";
import { api } from "../api";
import type { Conference, Participant } from "../types";
import { Button, ErrorNotice } from "./ui";
import { formatDate } from "../utils";

const labels = {
  starting: "Запись запускается",
  recording: "Идёт запись",
  stopping: "Запись останавливается",
  processing: "Обрабатываем запись",
  ready: "Запись готова",
  failed: "Не удалось завершить запись",
  cancelled: "Запись отменена",
};
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
    queryFn: ({ signal }) => api.recordings(conference.id, signal),
    enabled: !!membership && membership.status !== "kicked",
    refetchInterval: 3000,
  });
  const items = query.data?.items || [];
  const current = items.find((item) =>
    ["starting", "recording", "stopping", "processing"].includes(item.status),
  );
  const recording =
    current && ["starting", "recording", "stopping"].includes(current.status);
  const mutation = useMutation({
    mutationFn: (stopId: string | null) =>
      stopId
        ? api.stopRecording(conference.id, stopId)
        : api.startRecording(conference.id),
    onSettled: () => {
      void client.invalidateQueries({
        queryKey: ["recordings", conference.id],
      });
    },
  });
  const owner = membership?.role === "owner" && membership.status === "joined";
  if (!membership || membership.status === "kicked") return null;
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
              onClick={() => mutation.mutate(null)}
              busy={mutation.isPending}
            >
              <Circle size={16} />
              Начать запись
            </Button>
          ) : (
            recording && (
              <Button
                variant="danger"
                onClick={() => mutation.mutate(current.uuid)}
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
        {items.map((item) => {
          const file = item.files?.find((f) => f.fileType === "final_mp4");
          const preview = item.files?.find((f) => f.fileType === "preview_jpg");
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
        })}
      </div>
    </section>
  );
}
