import { useEffect, useRef, useState } from "react";
import type { Dispatch, ReactNode, SetStateAction } from "react";
import { createPortal } from "react-dom";
import { Paperclip, X } from "lucide-react";
import { api, errorMessage, uploadAttachment } from "../api";
import { formatBytes } from "../collaboration";
import type { ChatAttachment } from "../types";
import { Button, ErrorNotice } from "./ui";

export const attachmentLimit = 10 * 1024 * 1024;
/**
 * validateAttachment проверяет клиентские ограничения имени, типа и размера файла до обращения к серверу.
 *
 * @args
 *   - file (Pick<File, "name" | "size">) — выбранный пользователем файл для проверки или передачи.
 *
 * @returns string | null — текст найденного нарушения либо отсутствие ошибки для допустимого файла.
 */
export function validateAttachment(
  file: Pick<File, "name" | "size">,
): string | null {
  if (!file.size || file.size > attachmentLimit)
    return "Размер файла должен быть от 1 байта до 10 МБ.";
  if (!/\.(jpe?g|png|webp|pdf|txt|csv)$/i.test(file.name))
    return "Можно прикрепить JPG, PNG, WebP, PDF, TXT или CSV.";
  return null;
}
/**
 * UploadRow хранит состояние файла, прогресс, результат загрузки и ошибку.
 *
 * @params:
 *   - key — поле или операция этого контракта.
 *   - file — выбранный пользователем файл для проверки или передачи.
 *   - progress — поле или операция этого контракта.
 *   - state — новое состояние источников медиа.
 *   - error — пойманная ошибка API или сети.
 *   - attachmentId — идентификатор подготовленного или прикреплённого вложения.
 */
interface UploadRow {
  key: string;
  file: File;
  progress: number;
  state: "uploading" | "ready" | "failed";
  error?: string;
  attachmentId?: string;
}
/**
 * AttachmentUploader координирует инициализацию, передачу с прогрессом, финализацию и повтор загрузки выбранного файла.
 *
 * @args
 *   - объект параметров: conferenceId — идентификатор конференции и области данных; value — значение для проверки, преобразования или отображения; onChange — обработчик изменения управляемого значения; onBusy — свойство текущего компонента; disabled — запрещает действие в текущем состоянии.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function AttachmentUploader({
  conferenceId,
  value,
  onChange,
  onBusy,
  disabled = false,
  compact = false,
  detailsTarget,
}: {
  conferenceId: string;
  value: ChatAttachment[];
  onChange: Dispatch<SetStateAction<ChatAttachment[]>>;
  onBusy: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в чате, файлах и совместной работе.
   *
   * @args
   *   - busy (boolean) — признак выполняющейся операции.
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ (busy: boolean) => void;
  disabled?: boolean;
  /** Показывает только скрепку в компактной строке ввода, сохраняя очередь и ошибки загрузки. */
  compact?: boolean;
  /** Область прокрутки очереди над строкой ввода; сами кнопки остаются доступными внизу. */
  detailsTarget?: HTMLElement | null;
}) {
  const [rows, setRows] = useState<UploadRow[]>([]);
  const [error, setError] = useState("");
  const controllers = useRef(new Map<string, AbortController>());
  const mounted = useRef(true);
  const input = useRef<HTMLInputElement>(null);
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      mounted.current = true;
      const active = controllers.current;
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        mounted.current = false;
        for (const controller of active.values()) controller.abort();
      };
    },
    [],
  );
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      onBusy(
        rows.some(
          /**
           * Обработчик rows.some проверяет условие поиска элемента или соответствия элементов набора.
           *
           * @args
           *   - row — состояние одного файла в очереди загрузки.
           *
           * @returns логический признак соответствия элемента условию.
           */ (row) => row.state === "uploading",
        ),
      );
    },
    [rows, onBusy],
  );
  /**
   * update объединяет изменение со снимком медиа и уведомляет подписчика состояния.
   *
   * @args
   *   - key (string) — идентификатор строки загрузки.
   *   - fields (Partial<UploadRow>) — изменённые поля состояния загрузки.
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  const update = (key: string, fields: Partial<UploadRow>) => {
    if (mounted.current)
      setRows(
        /**
         * Обработчик setRows вычисляет следующее React-состояние из предыдущего значения.
         *
         * @args
         *   - current — текущее значение состояния.
         *
         * @returns следующее состояние, рассчитанное из предыдущего значения.
         */ (current) =>
          current.map(
            /**
             * Обработчик current.map преобразует один элемент набора в представление или данные следующего шага.
             *
             * @args
             *   - row — состояние одного файла в очереди загрузки.
             *
             * @returns преобразованное значение текущего элемента для результирующего набора.
             */ (row) => (row.key === key ? { ...row, ...fields } : row),
          ),
      );
  };
  /**
   * start подготавливает медиа-соединение и при явном разрешении захватывает устройства пользователя.
   *
   * @args
   *   - row (UploadRow) — состояние одного файла в очереди загрузки.
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
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
          /**
           * Обработчик uploadAttachment выполняет переданный шаг вызова uploadAttachment в чате, файлах и совместной работе.
           *
           * @args
           *   - progress — доля завершённой передачи файла.
           *
           * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
           */
          (progress) => update(row.key, { progress }),
          controller.signal,
        );
      if (controller.signal.aborted) return;
      const finalized = await api.finalizeAttachment(
        conferenceId,
        result.item.id,
      );
      if (!mounted.current || controller.signal.aborted) return;
      onChange(
        /**
         * Обработчик onChange выполняет переданный шаг вызова onChange в чате, файлах и совместной работе.
         *
         * @args
         *   - current — текущее значение состояния.
         *
         * @returns вычисленное значение: current.some( (item) => item.id === finalized.item.id, ) ? current : [...current, finalized.item].
         */ (current) =>
          current.some(
            /**
             * Обработчик current.some проверяет условие поиска элемента или соответствия элементов набора.
             *
             * @args
             *   - item — элемент списка, который обрабатывает текущий шаг.
             *
             * @returns логический признак соответствия элемента условию.
             */ (item) => item.id === finalized.item.id,
          )
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
    <div className={`chat-uploads ${compact ? "chat-uploads-compact" : ""}`}>
      <input
        ref={input}
        type="file"
        multiple
        accept=".jpg,.jpeg,.png,.webp,.pdf,.txt,.csv"
        aria-label="Выбрать файлы для сообщения"
        hidden
        disabled={disabled || rows.length >= 5}
        onChange={
          /**
           * onChange обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
           *
           * @args
           *   - event — проверенный конверт события комнаты.
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ (event) => {
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
            const added = files.map(
              /**
               * Обработчик files.map преобразует один элемент набора в представление или данные следующего шага.
               *
               * @args
               *   - file — выбранный пользователем файл для проверки или передачи.
               *
               * @returns UploadRow — преобразованное значение текущего элемента для результирующего набора.
               */ (file): UploadRow => ({
                key: crypto.randomUUID(),
                file,
                progress: 0,
                state: "uploading",
              }),
            );
            setRows(
              /**
               * Обработчик setRows вычисляет следующее React-состояние из предыдущего значения.
               *
               * @args
               *   - current — текущее значение состояния.
               *
               * @returns следующее состояние, рассчитанное из предыдущего значения.
               */ (current) => [...current, ...added],
            );
            for (const row of added) void start(row);
          }
        }
      />
      <Button
        type="button"
        variant="outline"
        className={compact ? "chat-attachment-trigger" : undefined}
        aria-label="Прикрепить файл"
        title="Прикрепить файл"
        disabled={disabled || rows.length >= 5}
        onClick={
          /**
           * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
           *
           *
           * @returns вычисленное значение: input.current?.click().
           */ () => input.current?.click()
        }
      >
        <Paperclip size={compact ? 24 : 16} />
        {!compact && "Прикрепить файл"}
      </Button>
      <span className={compact ? "sr-only" : "field-hint"}>
        До 5 файлов, каждый до 10 МБ.
      </span>
      <UploadDetails target={detailsTarget}>
        <ErrorNotice>{error || null}</ErrorNotice>
        <ul className="upload-list">
          {rows.map(
            /**
             * Обработчик rows.map преобразует один элемент набора в представление или данные следующего шага.
             *
             * @args
             *   - row — состояние одного файла в очереди загрузки.
             *
             * @returns преобразованное значение текущего элемента для результирующего набора.
             */ (row) => (
              <li key={row.key}>
                <div>
                  <span className="upload-filename" title={row.file.name}>
                    {row.file.name}
                  </span>
                  <small>
                    {formatBytes(row.file.size)} ·{" "}
                    {row.state === "ready" &&
                    value.some(
                      /**
                       * Обработчик value.some проверяет условие поиска элемента или соответствия элементов набора.
                       *
                       * @args
                       *   - item — элемент списка, который обрабатывает текущий шаг.
                       *
                       * @returns логический признак соответствия элемента условию.
                       */ (item) => item.id === row.attachmentId,
                    )
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
                    onClick={
                      /**
                       * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                       *
                       *
                       * @returns вычисленное значение: void start(row).
                       */ () => void start(row)
                    }
                  >
                    Повторить
                  </Button>
                )}
                <button
                  type="button"
                  className="icon-button"
                  disabled={disabled}
                  aria-label={`Убрать файл ${row.file.name}`}
                  onClick={
                    /**
                     * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
                     *
                     *
                     * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                     */ () => {
                      controllers.current.get(row.key)?.abort();
                      controllers.current.delete(row.key);
                      setRows(
                        /**
                         * Обработчик setRows вычисляет следующее React-состояние из предыдущего значения.
                         *
                         * @args
                         *   - current — текущее значение состояния.
                         *
                         * @returns следующее состояние, рассчитанное из предыдущего значения.
                         */ (current) =>
                          current.filter(
                            /**
                             * Обработчик current.filter проверяет, должен ли элемент войти в отфильтрованный набор.
                             *
                             * @args
                             *   - item — элемент списка, который обрабатывает текущий шаг.
                             *
                             * @returns логический признак соответствия элемента условию.
                             */ (item) => item.key !== row.key,
                          ),
                      );
                      onChange(
                        /**
                         * Обработчик onChange выполняет переданный шаг вызова onChange в чате, файлах и совместной работе.
                         *
                         * @args
                         *   - current — текущее значение состояния.
                         *
                         * @returns вычисленное значение: current.filter( (item) => item.id !== row.attachmentId, ).
                         */ (current) =>
                          current.filter(
                            /**
                             * Обработчик current.filter проверяет, должен ли элемент войти в отфильтрованный набор.
                             *
                             * @args
                             *   - item — элемент списка, который обрабатывает текущий шаг.
                             *
                             * @returns логический признак соответствия элемента условию.
                             */ (item) => item.id !== row.attachmentId,
                          ),
                      );
                    }
                  }
                >
                  <X size={17} />
                </button>
              </li>
            ),
          )}
        </ul>
      </UploadDetails>
    </div>
  );
}

/**
 * Переносит очередь и ошибки загрузки в отдельную прокручиваемую область, не пересоздавая загрузчик.
 * @args target — область над редактором либо null для обычного расположения; children — очередь и ошибки.
 * @return То же содержимое в указанной области DOM или на прежнем месте.
 */
function UploadDetails({
  target,
  children,
}: {
  target?: HTMLElement | null;
  children: ReactNode;
}) {
  return target ? createPortal(children, target) : <>{children}</>;
}
