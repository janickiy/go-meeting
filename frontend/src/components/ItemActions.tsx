import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { MoreHorizontal } from "lucide-react";
import "./folders.css";

export interface ItemAction {
  label: string;
  icon?: ReactNode;
  run: () => void;
  disabled?: boolean;
  danger?: boolean;
  pressed?: boolean;
}
/** Открывает доступное с клавиатуры меню действий и возвращает фокус к его кнопке. */
export function ItemActions({
  label,
  actions,
  triggerIcon,
}: {
  label: string;
  actions: ItemAction[];
  triggerIcon?: ReactNode;
}) {
  const id = useId();
  const root = useRef<HTMLDivElement>(null),
    trigger = useRef<HTMLButtonElement>(null),
    menu = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const focused = useRef(false);
  const [position, setPosition] = useState<{ left: number; top: number }>();
  useLayoutEffect(() => {
    if (!open) {
      focused.current = false;
      return;
    }
    function place() {
      const anchor = trigger.current?.getBoundingClientRect(),
        popup = menu.current?.getBoundingClientRect();
      if (!anchor || !popup) return;
      setPosition({
        left: Math.max(
          16,
          Math.min(
            anchor.right - popup.width,
            window.innerWidth - popup.width - 16,
          ),
        ),
        top:
          anchor.bottom + 8 + popup.height <= window.innerHeight - 16
            ? anchor.bottom + 8
            : Math.max(16, anchor.top - popup.height - 8),
      });
    }
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [open]);
  useLayoutEffect(() => {
    if (!open || !position || focused.current) return;
    menu.current
      ?.querySelector<HTMLElement>('[role^="menuitem"]:not(:disabled)')
      ?.focus();
    focused.current = true;
  }, [open, position]);
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, [open]);
  function key(event: KeyboardEvent<HTMLDivElement>) {
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
      return;
    }
    const items = Array.from(
      menu.current?.querySelectorAll<HTMLElement>(
        '[role^="menuitem"]:not(:disabled)',
      ) || [],
    );
    const index = items.indexOf(document.activeElement as HTMLElement);
    const next =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? items.length - 1
          : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
            items.length;
    items[next]?.focus();
  }
  return (
    <div
      className="item-actions"
      ref={root}
      onKeyDown={key}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <button
        ref={trigger}
        type="button"
        className="icon-button"
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        onClick={() => setOpen(!open)}
      >
        {triggerIcon || <MoreHorizontal size={20} aria-hidden="true" />}
      </button>
      {open && (
        <div
          ref={menu}
          id={id}
          className="item-action-menu"
          role="menu"
          aria-label={label}
          style={position || { visibility: "hidden" }}
        >
          {actions.map((action) => (
            <button
              key={action.label}
              type="button"
              role={
                action.pressed === undefined ? "menuitem" : "menuitemcheckbox"
              }
              tabIndex={-1}
              disabled={action.disabled}
              aria-checked={action.pressed}
              className={action.danger ? "item-action-danger" : undefined}
              onClick={() => {
                setOpen(false);
                trigger.current?.focus();
                action.run();
              }}
            >
              {action.icon}
              {action.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
