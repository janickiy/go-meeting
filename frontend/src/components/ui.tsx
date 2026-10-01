import { useEffect, useId, useRef, useState } from "react";
import type {
  ButtonHTMLAttributes,
  InputHTMLAttributes,
  ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Link } from "react-router";
import {
  Check,
  CircleAlert,
  Copy,
  Eye,
  EyeOff,
  LoaderCircle,
  Video,
  X,
} from "lucide-react";
import { errorMessage, statusLabels } from "../api";
import type { ConferenceStatus } from "../types";

export function Brand({ to = "/" }: { to?: string }) {
  return (
    <Link to={to} className="brand" aria-label="Meet — главная">
      <span className="brand-mark">
        <Video size={21} fill="currentColor" strokeWidth={2.3} />
      </span>
      <span>Meet</span>
    </Link>
  );
}
export function Button({
  children,
  busy,
  variant = "primary",
  className = "",
  disabled,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  busy?: boolean;
  variant?: "primary" | "secondary" | "danger" | "outline";
}) {
  return (
    <button
      {...props}
      disabled={disabled || busy}
      aria-busy={busy || undefined}
      className={`button button-${variant} ${className}`}
    >
      {busy && <LoaderCircle size={18} className="spin" />}
      {children}
    </button>
  );
}
export function PasswordInput(props: InputHTMLAttributes<HTMLInputElement>) {
  const [visible, setVisible] = useState(false);
  return (
    <div className="password-input">
      <input {...props} type={visible ? "text" : "password"} />
      <button
        type="button"
        className="eye-toggle"
        aria-label={visible ? "Скрыть пароль" : "Показать пароль"}
        aria-pressed={visible}
        onClick={() => setVisible(!visible)}
      >
        {visible ? <EyeOff size={18} /> : <Eye size={18} />}
      </button>
    </div>
  );
}
export function ErrorNotice({
  error,
  children,
}: {
  error?: unknown;
  children?: ReactNode;
}) {
  if (!error && !children) return null;
  return (
    <div className="error-notice" role="alert">
      <CircleAlert size={18} />
      <span>{children || errorMessage(error)}</span>
    </div>
  );
}
export function Loading() {
  return (
    <div className="loading" role="status">
      <LoaderCircle className="spin" size={24} />
      <span>Загружаем…</span>
    </div>
  );
}
export function StatusBadge({ status }: { status: ConferenceStatus }) {
  return (
    <span className={`status-badge status-${status}`}>
      {statusLabels[status]}
    </span>
  );
}
export function SuccessMark() {
  return (
    <span className="success-mark" aria-hidden="true">
      <Check size={37} strokeWidth={2.4} />
    </span>
  );
}
export function Modal({
  title,
  children,
  onClose,
  wide = false,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
}) {
  const id = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const focusable = () =>
      Array.from(
        dialog.current?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), input:not(:disabled), a[href], textarea:not(:disabled), [tabindex="0"]',
        ) || [],
      );
    (
      dialog.current?.querySelector<HTMLElement>("[data-autofocus]") ||
      focusable()[0] ||
      dialog.current
    )?.focus();
    function handle(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        close.current();
      }
      if (event.key === "Tab") {
        const items = focusable();
        const first = items[0];
        const last = items.at(-1);
        if (!first) {
          event.preventDefault();
          dialog.current?.focus();
        } else if (
          event.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === dialog.current)
        ) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    }
    document.addEventListener("keydown", handle);
    return () => {
      document.body.style.overflow = overflow;
      document.removeEventListener("keydown", handle);
      if (previous?.isConnected) previous.focus();
    };
  }, []);
  return createPortal(
    <div
      className="modal-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) close.current();
      }}
    >
      <div
        ref={dialog}
        className={`modal ${wide ? "modal-wide" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={id}
        tabIndex={-1}
      >
        <button
          className="icon-button modal-close"
          aria-label="Закрыть окно"
          onClick={onClose}
        >
          <X size={21} />
        </button>
        <h2 id={id}>{title}</h2>
        {children}
      </div>
    </div>,
    document.body,
  );
}
export function CopyLink({ value }: { value: string }) {
  const [state, setState] = useState<"idle" | "copied" | "error">("idle");
  useEffect(() => {
    if (state !== "copied") return;
    const timer = window.setTimeout(() => setState("idle"), 2500);
    return () => window.clearTimeout(timer);
  }, [state]);
  return (
    <>
      <div className="copy-link">
        <input
          aria-label="Ссылка-приглашение"
          value={value}
          readOnly
          onFocus={(event) => event.target.select()}
        />
        <Button
          variant="outline"
          onClick={() => {
            navigator.clipboard
              ?.writeText(value)
              .then(() => setState("copied"))
              .catch(() => setState("error"));
            if (!navigator.clipboard) setState("error");
          }}
        >
          {state === "copied" ? <Check size={17} /> : <Copy size={17} />}
          {state === "copied" ? "Скопировано" : "Копировать"}
        </Button>
      </div>
      <span className="copy-feedback" role="status">
        {state === "error"
          ? "Не удалось скопировать. Выделите ссылку и скопируйте вручную."
          : state === "copied"
            ? "Ссылка скопирована."
            : ""}
      </span>
    </>
  );
}
