import { useEffect, useId, useRef, useState } from "react";
import { useSearchParams } from "react-router";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api";
import { isAdmitted } from "../collaboration";
import { recordingTime } from "../intelligence";
import type {
  ConferenceRecording,
  Participant,
  TranscriptSegment,
} from "../types";
import { Button, ErrorNotice, Loading } from "./ui";

const processing = new Set(["queued", "processing"]);
const contentLabels = {
  queued: "В очереди на обработку",
  processing: "Обрабатываем материал",
  ready: "Готово",
  failed: "Обработка не завершена",
};

/**
 * Выбирает готовую запись по UUID, сохраняя глубокую ссылку из поиска.
 * @parameters conferenceId, membership — встреча и актуальный допуск; recordings — доступные записи.
 * @return Приватный проигрыватель и вкладки материалов либо пустое представление.
 */
export function RecordingInsights({
  conferenceId,
  membership,
  recordings,
}: {
  conferenceId: string;
  membership: Participant;
  recordings: ConferenceRecording[];
}) {
  const [params, setParams] = useSearchParams();
  const ready = recordings.filter((recording) => recording.status === "ready");
  const selected = params.get("recording") || ready[0]?.uuid;
  if (!isAdmitted(membership) || !selected) return null;
  return (
    <div className="recording-insights">
      <div className="section-heading">
        <h3>Просмотр и материалы</h3>
        <label>
          Запись{" "}
          <select
            aria-label="Выбрать запись"
            value={selected}
            onChange={(event) => {
              const next = new URLSearchParams(params);
              next.set("recording", event.target.value);
              next.delete("t");
              next.delete("segment");
              setParams(next, { replace: true });
            }}
          >
            {!ready.some((recording) => recording.uuid === selected) && (
              <option value={selected}>Запись по ссылке</option>
            )}
            {ready.map((recording, index) => (
              <option key={recording.uuid} value={recording.uuid}>
                Запись {index + 1} ·{" "}
                {new Date(recording.createdAt).toLocaleString("ru-RU")}
              </option>
            ))}
          </select>
        </label>
      </div>
      <RecordingMaterial
        key={`${membership.userId}:${conferenceId}:${selected}`}
        conferenceId={conferenceId}
        recordingId={selected}
        membership={membership}
      />
    </div>
  );
}

/**
 * Стабильно воспроизводит приватный MP4 и загружает авторизованные материалы.
 * @parameters conferenceId, recordingId — идентификаторы; membership — текущие права участника.
 * @return Проигрыватель, состояния обработки и разрешённые действия организаторов.
 */
function RecordingMaterial({
  conferenceId,
  recordingId,
  membership,
}: {
  conferenceId: string;
  recordingId: string;
  membership: Participant;
}) {
  const [params, setParams] = useSearchParams();
  const client = useQueryClient();
  const key = [membership.userId, conferenceId, recordingId];
  const video = useRef<HTMLVideoElement>(null);
  const pendingSeek = useRef<number | null>(null);
  const [source, setSource] = useState<string>();
  const [playbackError, setPlaybackError] = useState(false);
  const tab = params.get("tab") === "summary" ? "summary" : "transcript";
  const seekOffset = params.get("t");
  const tabsId = useId();
  const record = useQuery({
    queryKey: ["recording-detail", ...key],
    queryFn: ({ signal }) => api.recording(conferenceId, recordingId, signal),
    staleTime: 60_000,
    retry: false,
  });
  const available = record.data?.item.status === "ready" && !record.isError;
  const transcript = useQuery({
    queryKey: ["transcript", ...key],
    queryFn: ({ signal }) => api.transcript(conferenceId, recordingId, signal),
    enabled: available,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.item && processing.has(query.state.data.item.status)
        ? 3000
        : false,
  });
  const summary = useQuery({
    queryKey: ["summary", ...key],
    queryFn: ({ signal }) => api.summary(conferenceId, recordingId, signal),
    enabled: available,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.item && processing.has(query.state.data.item.status)
        ? 3000
        : false,
  });
  const segments = useInfiniteQuery({
    queryKey: [
      "transcript-segments",
      ...key,
      transcript.data?.item?.id,
      transcript.data?.item?.updatedAt,
    ],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      api.transcriptSegments(conferenceId, recordingId, pageParam, signal),
    enabled:
      available &&
      transcript.data?.item?.status === "ready" &&
      !transcript.isError,
    getNextPageParam: (page) =>
      page.offset + page.items.length < page.total && page.items.length
        ? page.offset + page.items.length
        : undefined,
    retry: false,
  });
  const retry = useMutation({
    mutationFn: () => api.retryTranscript(conferenceId, recordingId),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ["transcript", ...key] });
      void client.invalidateQueries({ queryKey: ["summary", ...key] });
    },
  });
  const regenerate = useMutation({
    mutationFn: () => api.regenerateSummary(conferenceId, recordingId),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: ["summary", ...key] }),
  });
  const canManage =
    membership.role === "owner" || membership.role === "co_host";
  const rows = segments.data?.pages.flatMap((page) => page.items) || [];

  /**
   * Применяет отложенное смещение только после получения длительности MP4.
   * @return Ничего; изменяет позицию проигрывателя, не включая автозапуск.
   */
  function applySeek() {
    if (
      !video.current ||
      pendingSeek.current === null ||
      video.current.readyState < 1
    )
      return;
    const duration = video.current.duration;
    const seconds = pendingSeek.current / 1000;
    video.current.currentTime = Number.isFinite(duration)
      ? Math.min(seconds, Math.max(0, duration))
      : seconds;
    pendingSeek.current = null;
  }

  /**
   * Переводит выбранный сегмент в позицию записи и воспроизводимую ссылку.
   * @parameters segment — сегмент с проверенным сервером временем.
   * @return Ничего; обновляет позицию и адрес вкладки.
   */
  function seek(segment: TranscriptSegment) {
    if (!Number.isFinite(segment.startMs) || segment.startMs < 0) return;
    pendingSeek.current = segment.startMs;
    applySeek();
    const next = new URLSearchParams(params);
    next.set("recording", recordingId);
    next.set("tab", "transcript");
    next.set("t", String(segment.startMs));
    next.set("segment", segment.id);
    setParams(next, { replace: true });
  }
  useEffect(() => {
    const url = record.data?.item.files?.find(
      (file) => file.fileType === "final_mp4",
    )?.url;
    if (available && url) setSource((current) => current || url);
  }, [record.data, available]);
  useEffect(() => {
    const milliseconds = seekOffset === null ? NaN : Number(seekOffset);
    if (Number.isFinite(milliseconds) && milliseconds >= 0) {
      pendingSeek.current = milliseconds;
      applySeek();
    }
  }, [seekOffset]);

  if (record.isPending) return <Loading />;
  if (record.isError)
    return (
      <>
        <ErrorNotice error={record.error} />
        <Button
          variant="outline"
          busy={record.isFetching}
          onClick={() => void record.refetch()}
        >
          Проверить доступность записи
        </Button>
      </>
    );
  if (!available)
    return (
      <p className="field-hint">
        Просмотр и материалы доступны после готовности записи.
      </p>
    );
  const currentTranscript = transcript.isError ? null : transcript.data?.item;
  const currentSummary = summary.isError ? null : summary.data?.item;
  return (
    <>
      {source ? (
        <video
          ref={video}
          className="recording-video"
          aria-label="Запись встречи"
          controls
          preload="metadata"
          src={source}
          onLoadedMetadata={applySeek}
          onError={() => setPlaybackError(true)}
        />
      ) : (
        <p className="field-hint">Файл записи пока недоступен.</p>
      )}
      {playbackError && (
        <div role="alert">
          <p>Не удалось воспроизвести запись. Возможно, ссылка устарела.</p>
          <Button
            variant="outline"
            busy={record.isFetching}
            onClick={async () => {
              pendingSeek.current =
                Math.max(0, video.current?.currentTime || 0) * 1000;
              const result = await record.refetch();
              if (result.data && !result.error) {
                setSource(
                  result.data.item.files.find(
                    (file) => file.fileType === "final_mp4",
                  )?.url,
                );
                setPlaybackError(false);
              }
            }}
          >
            Обновить ссылку
          </Button>
        </div>
      )}
      <div
        className="insight-tabs"
        role="tablist"
        aria-label="Материалы записи"
      >
        {[
          ["transcript", "Расшифровка"],
          ["summary", "Итоги ИИ"],
        ].map(([value, label]) => (
          <button
            key={value}
            type="button"
            role="tab"
            id={`${tabsId}-${value}`}
            aria-controls={`${tabsId}-panel`}
            tabIndex={tab === value ? 0 : -1}
            aria-selected={tab === value}
            onKeyDown={(event) => {
              if (
                !["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)
              )
                return;
              event.preventDefault();
              const nextValue =
                event.key === "Home"
                  ? "transcript"
                  : event.key === "End"
                    ? "summary"
                    : value === "transcript"
                      ? "summary"
                      : "transcript";
              const next = new URLSearchParams(params);
              next.set("recording", recordingId);
              next.set("tab", nextValue);
              setParams(next, { replace: true });
              document.getElementById(`${tabsId}-${nextValue}`)?.focus();
            }}
            onClick={() => {
              const next = new URLSearchParams(params);
              next.set("recording", recordingId);
              next.set("tab", value);
              setParams(next, { replace: true });
            }}
          >
            {label}
          </button>
        ))}
      </div>
      <ErrorNotice error={retry.error || regenerate.error} />
      {tab === "transcript" ? (
        <div
          role="tabpanel"
          id={`${tabsId}-panel`}
          aria-labelledby={`${tabsId}-transcript`}
        >
          <ErrorNotice error={transcript.error || segments.error} />
          {transcript.isPending ? (
            <Loading />
          ) : (
            !transcript.isError && (
              <>
                {!transcript.data?.enabled && (
                  <p className="field-hint">
                    Расшифровка отключена администратором. Просмотр записи
                    доступен.
                  </p>
                )}
                {transcript.data?.providerMode === "mock" && (
                  <p className="demo-notice">
                    Тестовые данные: это демонстрационная расшифровка, а не
                    распознанная речь.
                  </p>
                )}
                {!currentTranscript && transcript.data?.enabled && (
                  <p className="muted">Расшифровка ещё не создана.</p>
                )}
                {currentTranscript && currentTranscript.status !== "ready" && (
                  <p role="status">
                    {contentLabels[currentTranscript.status]}
                    {currentTranscript.status === "failed"
                      ? ". Организатор или соорганизатор может повторить попытку, если это разрешено сервером."
                      : ". Можно закрыть страницу и вернуться позже."}
                  </p>
                )}
                {canManage &&
                  transcript.data?.enabled &&
                  transcript.data.canRetry && (
                    <Button
                      variant="outline"
                      busy={retry.isPending}
                      onClick={() => retry.mutate()}
                    >
                      {currentTranscript
                        ? "Повторить расшифровку"
                        : "Создать расшифровку"}
                    </Button>
                  )}
                {currentTranscript?.status === "ready" && (
                  <>
                    <p className="field-hint">
                      Нажмите время, чтобы перейти к фрагменту. Обозначения
                      говорящих не подтверждают личность участника.
                    </p>
                    {segments.isPending && <Loading />}
                    <ol className="transcript-list">
                      {rows.map((segment) => (
                        <li
                          key={segment.id}
                          className={
                            params.get("segment") === segment.id
                              ? "segment-selected"
                              : undefined
                          }
                        >
                          <button
                            type="button"
                            className="text-link timestamp"
                            onClick={() => seek(segment)}
                            aria-label={`Перейти к ${recordingTime(segment.startMs)}`}
                          >
                            {recordingTime(segment.startMs)}
                          </button>
                          <div>
                            {segment.speakerLabel && (
                              <strong>{segment.speakerLabel}</strong>
                            )}
                            <p>{segment.text}</p>
                          </div>
                        </li>
                      ))}
                    </ol>
                    {!segments.isPending &&
                      !rows.length &&
                      !segments.isError && (
                        <p className="muted">Речь не найдена.</p>
                      )}
                    {segments.hasNextPage && (
                      <Button
                        variant="outline"
                        busy={segments.isFetchingNextPage}
                        onClick={() => void segments.fetchNextPage()}
                      >
                        Ещё фрагменты
                      </Button>
                    )}
                  </>
                )}
              </>
            )
          )}
        </div>
      ) : (
        <div
          role="tabpanel"
          id={`${tabsId}-panel`}
          aria-labelledby={`${tabsId}-summary`}
        >
          <ErrorNotice error={summary.error} />
          {summary.isPending ? (
            <Loading />
          ) : (
            !summary.isError && (
              <>
                {!summary.data?.enabled && (
                  <p className="field-hint">
                    Итоги ИИ отключены администратором.
                  </p>
                )}
                {summary.data?.providerMode === "mock" && (
                  <p className="demo-notice">
                    Тестовые данные: демонстрационные итоги, не анализ вашей
                    встречи.
                  </p>
                )}
                {!currentSummary && summary.data?.enabled && (
                  <p className="muted">
                    Итоги появятся после готовности расшифровки.
                  </p>
                )}
                {currentSummary && currentSummary.status !== "ready" && (
                  <p role="status">
                    {contentLabels[currentSummary.status]}
                    {currentSummary.status === "failed"
                      ? ". Организатор или соорганизатор может повторить попытку, если это разрешено сервером."
                      : ". Можно закрыть страницу и вернуться позже."}
                  </p>
                )}
                {canManage &&
                  summary.data?.enabled &&
                  summary.data.canRegenerate && (
                    <Button
                      variant="outline"
                      busy={regenerate.isPending}
                      onClick={() => regenerate.mutate()}
                    >
                      {currentSummary ? "Пересоздать итоги" : "Создать итоги"}
                    </Button>
                  )}
                {currentSummary?.status === "ready" && (
                  <div className="summary-content">
                    <p className="field-hint">
                      Автоматические итоги могут содержать ошибки. Проверяйте
                      важные договорённости по записи.
                    </p>
                    <h4>Краткое содержание</h4>
                    <p>{currentSummary.summary}</p>
                    <h4>Ключевые моменты</h4>
                    <ul>
                      {currentSummary.keyPoints.map((point, index) => (
                        <li key={index}>{point}</li>
                      ))}
                    </ul>
                    <h4>Договорённости и задачи</h4>
                    {currentSummary.actionItems.length ? (
                      <ul>
                        {currentSummary.actionItems.map((action, index) => (
                          <li key={index}>
                            <p>{action.text}</p>
                            <p className="field-hint">
                              Ответственный: {action.assignee || "не указан"} ·
                              Срок: {action.dueDate || "не указан"}
                            </p>
                            {action.sourceSegmentIds.map((id) => {
                              const segment = rows.find((row) => row.id === id);
                              return segment ? (
                                <button
                                  key={id}
                                  type="button"
                                  className="text-link"
                                  onClick={() => seek(segment)}
                                >
                                  Источник {recordingTime(segment.startMs)}
                                </button>
                              ) : null;
                            })}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <p className="muted">Явные задачи не найдены.</p>
                    )}
                    <h4>Темы</h4>
                    <ul className="topic-list">
                      {currentSummary.topics.map((topic, index) => (
                        <li key={index}>{topic}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </>
            )
          )}
        </div>
      )}
    </>
  );
}
