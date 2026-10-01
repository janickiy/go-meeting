import { useEffect, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { Paperclip, X } from "lucide-react";
import { api, errorMessage, uploadAttachment } from "../api";
import { formatBytes } from "../collaboration";
import type { ChatAttachment } from "../types";
import { Button, ErrorNotice } from "./ui";

export const attachmentLimit = 10 * 1024 * 1024;
export function validateAttachment(
  file: Pick<File, "name" | "size">,
): string | null {
  if (!file.size || file.size > attachmentLimit)
    return "Размер файла должен быть от 1 байта до 10 МБ.";
  if (!/\.(jpe?g|png|webp|pdf|txt|csv)$/i.test(file.name))
    return "Можно прикрепить JPG, PNG, WebP, PDF, TXT или CSV.";
  return null;
}
interface UploadRow {
  key: string;
  file: File;
  progress: number;
  state: "uploading" | "ready" | "failed";
  error?: string;
  attachmentId?: string;
}
export function AttachmentUploader({
  conferenceId,
  value,
  onChange,
  onBusy,
  disabled = false,
}: {
  conferenceId: string;
  value: ChatAttachment[];
  onChange: Dispatch<SetStateAction<ChatAttachment[]>>;
  onBusy: (busy: boolean) => void;
  disabled?: boolean;
}) {
  const [rows, setRows] = useState<UploadRow[]>([]);
  const [error, setError] = useState("");
  const controllers = useRef(new Map<string, AbortController>());
  const mounted = useRef(true);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => {
    mounted.current = true;
    const active = controllers.current;
    return () => {
      mounted.current = false;
      for (const controller of active.values()) controller.abort();
    };
  }, []);
  useEffect(() => {
    onBusy(rows.some((row) => row.state === "uploading"));
  }, [rows, onBusy]);
  const update = (key: string, fields: Partial<UploadRow>) => {
    if (mounted.current)
      setRows((current) =>
        current.map((row) => (row.key === key ? { ...row, ...fields } : row)),
      );
  };
  const start = async (row: UploadRow) => {
    const controller = new AbortController();
    controllers.current.set(row.key, controller);
    update(row.key, { state: "uploading", progress: 0, error: undefined });
    try {
      const result = await api.initAttachment(conferenceId, {
        clientRequestId: row.key,
        filename: row.file.name,
        size: row.file.size,
        mimeType: row.file.type || "application/octet-stream",
      });
      if (controller.signal.aborted) return;
      if (result.item.status === "pending")
        await uploadAttachment(
          result.uploadUrl,
          row.file,
          (progress) => update(row.key, { progress }),
          controller.signal,
        );
      if (controller.signal.aborted) return;
      const finalized = await api.finalizeAttachment(
        conferenceId,
        result.item.id,
      );
      if (!mounted.current || controller.signal.aborted) return;
      onChange((current) =>
        current.some((item) => item.id === finalized.item.id)
          ? current
          : [...current, finalized.item],
      );
      update(row.key, {
        state: "ready",
        progress: 100,
        attachmentId: finalized.item.id,
      });
    } catch (cause) {
      if (!controller.signal.aborted)
        update(row.key, { state: "failed", error: errorMessage(cause) });
    } finally {
      if (controllers.current.get(row.key) === controller)
        controllers.current.delete(row.key);
    }
  };
  return (
    <div className="chat-uploads">
      <input
        ref={input}
        type="file"
        multiple
        accept=".jpg,.jpeg,.png,.webp,.pdf,.txt,.csv"
        aria-label="Выбрать файлы для сообщения"
        hidden
        disabled={disabled || rows.length >= 5}
        onChange={(event) => {
          const files = [...(event.currentTarget.files || [])];
          event.currentTarget.value = "";
          setError("");
          if (files.length + rows.length > 5) {
            setError("В одном сообщении может быть не больше 5 файлов.");
            return;
          }
          const validation = files.map(validateAttachment).find(Boolean);
          if (validation) {
            setError(validation);
            return;
          }
          const added = files.map((file): UploadRow => ({
            key: crypto.randomUUID(),
            file,
            progress: 0,
            state: "uploading",
          }));
          setRows((current) => [...current, ...added]);
          for (const row of added) void start(row);
        }}
      />
      <Button
        type="button"
        variant="outline"
        disabled={disabled || rows.length >= 5}
        onClick={() => input.current?.click()}
      >
        <Paperclip size={16} />
        Прикрепить файл
      </Button>
      <span className="field-hint">До 5 файлов, каждый до 10 МБ.</span>
      <ErrorNotice>{error || null}</ErrorNotice>
      <ul className="upload-list">
        {rows.map((row) => (
          <li key={row.key}>
            <div>
              <span className="upload-filename" title={row.file.name}>
                {row.file.name}
              </span>
              <small>
                {formatBytes(row.file.size)} ·{" "}
                {row.state === "ready" &&
                value.some((item) => item.id === row.attachmentId)
                  ? "Готов к отправке"
                  : row.state === "uploading"
                    ? `${row.progress}%`
                    : "Не загружен"}
              </small>
              {row.state === "uploading" && (
                <progress
                  max={100}
                  value={row.progress}
                  aria-label={`Загрузка ${row.file.name}`}
                />
              )}
              {row.error && <p className="field-error">{row.error}</p>}
            </div>
            {row.state === "failed" && (
              <Button
                type="button"
                variant="outline"
                disabled={disabled}
                onClick={() => void start(row)}
              >
                Повторить
              </Button>
            )}
            <button
              type="button"
              className="icon-button"
              disabled={disabled}
              aria-label={`Убрать файл ${row.file.name}`}
              onClick={() => {
                controllers.current.get(row.key)?.abort();
                controllers.current.delete(row.key);
                setRows((current) =>
                  current.filter((item) => item.key !== row.key),
                );
                onChange((current) =>
                  current.filter((item) => item.id !== row.attachmentId),
                );
              }}
            >
              <X size={17} />
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
