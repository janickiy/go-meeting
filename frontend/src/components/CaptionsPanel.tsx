import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { mergeCaptions } from "../captions";
import { recordingTime } from "../intelligence";
import type { Caption } from "../types";
import type { useRealtime } from "../realtime";
import { Button, ErrorNotice } from "./ui";

const statuses: Record<string, string> = {
  off: "Выключено",
  queued: "Подключение",
  active: "Распознавание включено",
  degraded: "Субтитры временно неполные",
  failed: "Распознавание недоступно",
  completed: "Распознавание завершено",
};

/** CaptionsPanel показывает согласие, язык и версии субтитров независимо от состояния медиасвязи.
 * @args conferenceId — разрешённая встреча; active — можно ли менять настройку; live — транспорт комнаты.
 * @return Панель с локальным скрытием текста и восстановлением сохранённых финалов.
 */
export function CaptionsPanel({
  conferenceId,
  active,
  live,
}: {
  conferenceId: string;
  active: boolean;
  live: ReturnType<typeof useRealtime>;
}) {
  const client = useQueryClient();
  const [visible, setVisible] = useState(true);
  const [rows, setRows] = useState<Caption[]>([]);
  const [recoveryError, setRecoveryError] = useState<unknown>();
  const query = useQuery({
    queryKey: ["captions", conferenceId],
    queryFn: ({ signal }) => api.captions(conferenceId, signal),
    refetchInterval: active ? 3000 : false,
    retry: false,
  });
  const state = query.data?.item;
  const change = useMutation({
    mutationFn: ({
      enabled,
      language,
    }: {
      enabled: boolean;
      language: string;
    }) => api.setCaptions(conferenceId, enabled, language),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: ["captions", conferenceId] }),
  });
  useEffect(
    () =>
      live.subscribe((event) => {
        if (event.type === "caption.status") {
          void client.invalidateQueries({
            queryKey: ["captions", conferenceId],
          });
          return;
        }
        if (event.type !== "caption.partial" && event.type !== "caption.final")
          return;
        const value = event.data as Caption;
        if (
          !value ||
          value.conferenceId !== conferenceId ||
          value.generation < (state?.generation || 0)
        )
          return;
        setRows((old) => mergeCaptions(old, [value]));
      }),
    [live.subscribe, conferenceId, client, state?.generation],
  );
  useEffect(() => {
    const controller = new AbortController();
    let cursor = 0;
    let timer: ReturnType<typeof setTimeout>;
    setRows([]);
    /** poll восстанавливает потерянные финалы с отменой при выходе/смене прав.
     * @return Завершение одной ограниченной страницы; следующий запрос планируется отдельно.
     */
    async function poll() {
      let more = false;
      try {
        const page = await api.captionFinals(
          conferenceId,
          cursor,
          controller.signal,
        );
        if (controller.signal.aborted) return;
        setRows((old) => mergeCaptions(old, page.items));
        cursor = page.nextCursor;
        more = page.hasMore;
        setRecoveryError(undefined);
      } catch (error) {
        if (!controller.signal.aborted) setRecoveryError(error);
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => void poll(), more ? 100 : 2000);
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [conferenceId]);
  return (
    <section className="content-card" aria-label="Живые субтитры">
      <div className="section-heading">
        <h2>Субтитры</h2>
        <span role="status">
          {state
            ? statuses[state.status] || "Состояние обновляется"
            : "Загрузка состояния"}
        </span>
      </div>
      <ErrorNotice error={query.error || change.error || recoveryError} />
      {state?.enabled && (
        <p className="field-hint">
          Звук участников передаётся сервису распознавания. Скрытие текста на
          этом устройстве не отключает распознавание.
        </p>
      )}
      {state && !state.available && (
        <p className="muted">Живое распознавание отключено администратором.</p>
      )}
      <label>
        <input
          type="checkbox"
          checked={visible}
          onChange={(event) => setVisible(event.target.checked)}
        />{" "}
        Показывать текст на этом устройстве
      </label>
      {active && state?.canManage && (
        <div className="meeting-actions">
          <label>
            Язык{" "}
            <select
              aria-label="Язык распознавания"
              value={state.language}
              disabled={change.isPending}
              onChange={(event) =>
                change.mutate({
                  enabled: state.enabled,
                  language: event.target.value,
                })
              }
            >
              <option value="auto">Автоматически</option>
              <option value="ru">Русский</option>
              <option value="en">English</option>
            </select>
          </label>
          <Button
            variant="outline"
            busy={change.isPending}
            onClick={() =>
              change.mutate({
                enabled: !state.enabled,
                language: state.language,
              })
            }
          >
            {state.enabled
              ? "Отключить распознавание"
              : "Включить распознавание"}
          </Button>
        </div>
      )}
      {state?.canonicalRecordingId && (
        <p className="field-hint">
          Ниже — живой черновик. Итоговая расшифровка доступна в материалах
          записи.
        </p>
      )}
      {visible && (
        <div
          className="caption-transcript"
          role="log"
          aria-live="polite"
          aria-relevant="additions text"
          aria-label="Текст субтитров"
        >
          {!rows.length && <p className="muted">Реплик пока нет.</p>}
          {rows.map((row) => (
            <p
              key={row.id}
              className={row.final ? "caption-final" : "caption-partial"}
            >
              <strong>{row.speaker || "Участник"}</strong>{" "}
              <span className="field-hint">
                {recordingTime(row.startMs)} · {row.language}
                {!row.final ? " · черновик" : ""}
              </span>
              <br />
              {row.text}
            </p>
          ))}
        </div>
      )}
    </section>
  );
}
