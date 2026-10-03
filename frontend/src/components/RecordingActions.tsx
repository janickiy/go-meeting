import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { Copy, Download, MoreVertical } from "lucide-react";
import { safeRecordingUrl } from "../recordingPresentation";

/** RecordingActions показывает только поддерживаемые действия одной записи.
 * Скопированная ссылка ведёт на защищённую страницу, а не предоставляет доступ
 * посторонним; скачивание использует неизменённый подписанный адрес из backend.
 * @args title — название встречи; path — локальная страница записи; downloadUrl — разрешённый файл.
 * @return доступное с клавиатуры меню скачивания и копирования ссылки.
 */
export function RecordingActions({
  title,
  path,
  downloadUrl,
}: {
  title: string;
  path: string;
  downloadUrl?: string;
}) {
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const [manualLink, setManualLink] = useState("");
  const url = safeRecordingUrl(downloadUrl);
  useEffect(() => {
    if (!open) return;
    root.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    /** closeOutside закрывает меню без перехвата действия вне карточки.
     * @args event — нажатие указателя на странице.
     */
    const closeOutside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, [open]);

  /** copyLink копирует защищённую страницу или предлагает ручное копирование.
   * @return завершение работы с буфером обмена; отказ не раскрывает технические ошибки.
   */
  async function copyLink() {
    const link = new URL(path, window.location.origin).href;
    setOpen(false);
    trigger.current?.focus();
    try {
      await navigator.clipboard.writeText(link);
      setCopied(true);
      setManualLink("");
    } catch {
      setCopied(false);
      setManualLink(link);
    }
  }

  /** navigateMenu поддерживает стрелки, Home/End и закрытие через Escape.
   * @args event — клавиатурное событие внутри меню действий.
   */
  function navigateMenu(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      trigger.current?.focus();
      return;
    }
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    if (!open) {
      setOpen(true);
      setCopied(false);
      setManualLink("");
      return;
    }
    const items = [
      ...(root.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ||
        []),
    ];
    const current = items.indexOf(document.activeElement as HTMLElement);
    const next =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? items.length - 1
          : (current + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
            items.length;
    items[next]?.focus();
  }

  return (
    <div
      className="recordings-menu"
      ref={root}
      onKeyDown={navigateMenu}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <button
        ref={trigger}
        type="button"
        className="icon-button recordings-menu-trigger"
        aria-label={`Действия с записью: ${title}`}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        onClick={() => {
          setOpen(!open);
          setCopied(false);
          setManualLink("");
        }}
      >
        <MoreVertical size={19} aria-hidden="true" />
      </button>
      {open && (
        <div
          id={id}
          role="menu"
          aria-label="Действия с записью"
          className="recordings-menu-popup"
        >
          {url && (
            <a
              role="menuitem"
              tabIndex={-1}
              href={url}
              download
              target="_blank"
              rel="noreferrer noopener"
              onClick={() => {
                setOpen(false);
                trigger.current?.focus();
              }}
            >
              <Download size={16} aria-hidden="true" /> Скачать
            </a>
          )}
          <button
            type="button"
            role="menuitem"
            tabIndex={-1}
            onClick={() => void copyLink()}
          >
            <Copy size={16} aria-hidden="true" /> Копировать ссылку
          </button>
          <p>Для участников с доступом к встрече.</p>
        </div>
      )}
      {copied && (
        <span role="status" className="recordings-copy-notice">
          Ссылка скопирована
        </span>
      )}
      {manualLink && (
        <label className="recordings-copy-fallback">
          Скопируйте ссылку вручную
          <input
            aria-label="Ссылка на запись"
            value={manualLink}
            readOnly
            onFocus={(event) => event.currentTarget.select()}
          />
        </label>
      )}
    </div>
  );
}
