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
  X,
} from "lucide-react";
import { errorMessage, statusLabels } from "../api";
import type { ConferenceStatus } from "../types";
import { PRODUCT_NAME } from "../brand";

/**
 * Brand показывает фирменный знак Meet со ссылкой на указанную страницу.
 *
 * @args
 *   - объект параметров: to — верхняя граница фильтра либо локальный путь согласно типу.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function Brand({ to = "/" }: { to?: string }) {
  return (
    <Link to={to} className="brand" aria-label={`${PRODUCT_NAME} — главная`}>
      <span className="brand-mark">
        <img src="/brand-mark.svg" width="32" height="36" alt="" />
      </span>
      <span>{PRODUCT_NAME}</span>
    </Link>
  );
}
/**
 * Button показывает единообразную кнопку и состояние ожидания действия.
 *
 * @args
 *   - объект параметров: children — вложенное содержимое компонента или диалога; busy — свойство текущего компонента; variant — свойство текущего компонента; className — дополнительное оформление элемента; disabled — запрещает действие в текущем состоянии; props — типизированные свойства компонента; передаются в отображаемый элемент.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
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
/**
 * PasswordInput показывает поле пароля с управляемым переключением видимости ввода.
 *
 * @args
 *   - props (InputHTMLAttributes<HTMLInputElement>) — типизированные свойства компонента; передаются в отображаемый элемент.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
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
        onClick={
          /**
           * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
           *
           *
           * @returns вычисленное значение: setVisible(!visible).
           */ () => setVisible(!visible)
        }
      >
        {visible ? <EyeOff size={18} /> : <Eye size={18} />}
      </button>
    </div>
  );
}
/**
 * ErrorNotice выводит доступное сообщение об ошибке действия или загрузки.
 *
 * @args
 *   - объект параметров: error — пойманная ошибка API или сети; children — вложенное содержимое компонента или диалога.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
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
/**
 * Loading показывает индикатор ожидания данных страницы.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function Loading() {
  return (
    <div className="loading" role="status">
      <LoaderCircle className="spin" size={24} />
      <span>Загружаем…</span>
    </div>
  );
}
/**
 * StatusBadge переводит серверное состояние конференции в подпись и оформление индикатора.
 *
 * @args
 *   - объект параметров: status — HTTP-статус либо состояние встречи.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function StatusBadge({ status }: { status: ConferenceStatus }) {
  return (
    <span className={`status-badge status-${status}`}>
      {statusLabels[status]}
    </span>
  );
}
/**
 * SuccessMark показывает графическое подтверждение успешного действия.
 *
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function SuccessMark() {
  return (
    <span className="success-mark" aria-hidden="true">
      <Check size={37} strokeWidth={2.4} />
    </span>
  );
}
/**
 * Modal создаёт диалог с управлением фокусом, закрытием и доступностью клавиатуры.
 *
 * @args
 *   - объект параметров: title — название встречи или диалога; children — вложенное содержимое компонента или диалога; onClose — обработчик закрытия формы или диалога; wide — свойство текущего компонента.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function Modal({
  title,
  children,
  onClose,
  wide = false,
  className = "",
  returnFocus,
}: {
  title: string;
  children: ReactNode;
  onClose: /**
   * Вложенный обработчик выполняет шаг «Вложенный обработчик» в интерфейсе Meet.
   *
   *
   * @returns void — значение не возвращается; функция выполняет описанные действия.
   */ () => void;
  wide?: boolean;
  className?: string;
  returnFocus?: () => HTMLElement | null;
}) {
  const id = useId();
  const dialog = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  close.current = onClose;
  const restoreFocus = useRef(returnFocus);
  restoreFocus.current = returnFocus;
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      const previous = document.activeElement as HTMLElement | null;
      const overflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      /**
       * focusable находит доступные элементы диалога для клавиатурного фокуса.
       *
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */
      const focusable = () =>
        Array.from(
          dialog.current?.querySelectorAll<HTMLElement>(
            'button:not(:disabled), input:not(:disabled), select:not(:disabled), a[href], textarea:not(:disabled), [tabindex="0"]',
          ) || [],
        ).filter(
          (item) => item.tabIndex >= 0 && !item.closest("[hidden], [inert]"),
        );
      (
        dialog.current?.querySelector<HTMLElement>("[data-autofocus]") ||
        focusable()[0] ||
        dialog.current
      )?.focus();
      /**
       * handle ставит входящее медиа-событие в последовательную обработку, сохраняя порядок SDP и ICE.
       *
       * @args
       *   - event (KeyboardEvent) — проверенный конверт события комнаты.
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      function handle(event: KeyboardEvent) {
        // Горячие клавиши фоновой встречи не должны реагировать на ввод в диалоге.
        event.stopPropagation();
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
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      return () => {
        document.body.style.overflow = overflow;
        document.removeEventListener("keydown", handle);
        const target = restoreFocus.current?.() || previous;
        if (target?.isConnected && !target.closest("[inert]")) target.focus();
      };
    },
    [],
  );
  return createPortal(
    <div
      className="modal-backdrop"
      onMouseDown={
        /**
         * onMouseDown обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
         *
         * @args
         *   - event — проверенный конверт события комнаты.
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */ (event) => {
          if (event.target === event.currentTarget) {
            // Не даём фону забрать фокус после восстановления ссылки открытия.
            event.preventDefault();
            close.current();
          }
        }
      }
    >
      <div
        ref={dialog}
        className={`modal ${wide ? "modal-wide" : ""} ${className}`}
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
/**
 * CopyLink показывает ссылку и копирует её в буфер обмена с индикацией результата.
 *
 * @args
 *   - объект параметров: value — значение для проверки, преобразования или отображения.
 *
 * @returns JSX-представление компонента для текущих свойств и состояния.
 */
export function CopyLink({ value }: { value: string }) {
  const [state, setState] = useState<"idle" | "copied" | "error">("idle");
  useEffect(
    /**
     * Обработчик useEffect связывает внешние ресурсы с временем жизни React-компонента и возвращает необходимую очистку.
     *
     *
     * @returns функция освобождения созданных ресурсов, если эффект её объявляет; иначе значение не возвращается.
     */ () => {
      if (state !== "copied") return;
      const timer = window.setTimeout(
        /**
         * Обработчик window.setTimeout выполняет отложенную либо периодическую часть операции.
         *
         *
         * @returns вычисленное значение: setState("idle").
         */ () => setState("idle"),
        2500,
      );
      /**
       * Освобождение ресурсов завершает ресурсы предыдущего эффекта перед повторным выполнением либо удалением компонента.
       *
       *
       * @returns вычисленное значение: window.clearTimeout(timer).
       */
      return () => window.clearTimeout(timer);
    },
    [state],
  );
  return (
    <>
      <div className="copy-link">
        <input
          aria-label="Ссылка-приглашение"
          value={value}
          readOnly
          onFocus={
            /**
             * onFocus обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             * @args
             *   - event — проверенный конверт события комнаты.
             *
             * @returns вычисленное значение: event.target.select().
             */ (event) => event.target.select()
          }
        />
        <Button
          variant="outline"
          onClick={
            /**
             * onClick обрабатывает соответствующее событие интерфейса и изменяет состояние текущего действия.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {
              navigator.clipboard
                ?.writeText(value)
                .then(
                  /**
                   * Обработчик then выполняет переданный шаг вызова then в интерфейсе Meet.
                   *
                   *
                   * @returns вычисленное значение: setState("copied").
                   */ () => setState("copied"),
                )
                .catch(
                  /**
                   * Обработчик catch выполняет переданный шаг вызова catch в интерфейсе Meet.
                   *
                   *
                   * @returns вычисленное значение: setState("error").
                   */ () => setState("error"),
                );
              if (!navigator.clipboard) setState("error");
            }
          }
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
