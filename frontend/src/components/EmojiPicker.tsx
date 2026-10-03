import { useEffect, useId, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import { Smile, X } from "lucide-react";
import { CHAT_EMOJIS } from "../emoji";
import "./emoji-picker.css";

const GRID_COLUMNS = 6;

/**
 * EmojiPicker открывает локальную палитру с клавиатурным выбором смайликов.
 * @args onSelect — передаёт выбранный Unicode-смайлик; disabled — запрещает выбор; className — дополнительные классы.
 * @return кнопка и раскрывающаяся палитра; вставку в текст выполняет родитель.
 */
export function EmojiPicker({
  onSelect,
  disabled = false,
  className = "",
}: {
  onSelect: (emoji: string) => void;
  disabled?: boolean;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const popupId = useId();
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const buttons = useRef<Array<HTMLButtonElement | null>>([]);

  /**
   * closePicker закрывает палитру и возвращает фокус к открывшей её кнопке.
   * @return значение не возвращается.
   */
  const closePicker = () => {
    setOpen(false);
    trigger.current?.focus();
  };

  useEffect(() => {
    if (!open) return;
    if (disabled) {
      setOpen(false);
      return;
    }
    buttons.current[0]?.focus();

    /**
     * closeOutside закрывает палитру при взаимодействии за её пределами.
     * @args event — событие указателя или мыши.
     * @return значение не возвращается.
     */
    const closeOutside = (event: Event) => {
      if (event.target instanceof Node && !root.current?.contains(event.target))
        closePicker();
    };

    /**
     * closeOnEscape закрывает палитру независимо от положения фокуса.
     * @args event — нажатие клавиши в документе.
     * @return значение не возвращается.
     */
    const closeOnEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      closePicker();
    };

    document.addEventListener("pointerdown", closeOutside);
    document.addEventListener("click", closeOutside);
    document.addEventListener("keydown", closeOnEscape, true);
    return () => {
      document.removeEventListener("pointerdown", closeOutside);
      document.removeEventListener("click", closeOutside);
      document.removeEventListener("keydown", closeOnEscape, true);
    };
  }, [open, disabled]);

  /**
   * navigateGrid перемещает фокус по строкам палитры, не прокручивая страницу.
   * @args event — клавиатурное событие; index — индекс выбранной кнопки.
   * @return значение не возвращается.
   */
  const navigateGrid = (
    event: KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) => {
    let next = index;
    const last = CHAT_EMOJIS.length - 1;
    const rowStart = Math.floor(index / GRID_COLUMNS) * GRID_COLUMNS;
    switch (event.key) {
      case "ArrowRight":
        next = (index + 1) % CHAT_EMOJIS.length;
        break;
      case "ArrowLeft":
        next = (index - 1 + CHAT_EMOJIS.length) % CHAT_EMOJIS.length;
        break;
      case "ArrowDown":
        next = Math.min(last, index + GRID_COLUMNS);
        break;
      case "ArrowUp":
        next = Math.max(0, index - GRID_COLUMNS);
        break;
      case "Home":
        next = event.ctrlKey || event.metaKey ? 0 : rowStart;
        break;
      case "End":
        next =
          event.ctrlKey || event.metaKey
            ? last
            : Math.min(last, rowStart + GRID_COLUMNS - 1);
        break;
      default:
        return;
    }
    event.preventDefault();
    setActiveIndex(next);
    buttons.current[next]?.focus();
  };

  return (
    <div className={`emoji-picker ${className}`.trim()} ref={root}>
      <button
        type="button"
        className="emoji-picker-trigger"
        ref={trigger}
        aria-label="Добавить смайлик"
        title="Добавить смайлик"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? popupId : undefined}
        disabled={disabled}
        onClick={() => {
          if (open) closePicker();
          else {
            setActiveIndex(0);
            setOpen(true);
          }
        }}
      >
        <Smile aria-hidden="true" />
      </button>
      {open && !disabled && (
        <div
          id={popupId}
          className="emoji-picker-popover"
          role="dialog"
          aria-label="Смайлики"
        >
          <div className="emoji-picker-heading">
            <span>Смайлики</span>
            <span className="emoji-picker-count" aria-hidden="true">
              {CHAT_EMOJIS.length}
            </span>
            <button
              type="button"
              className="emoji-picker-close"
              aria-label="Закрыть выбор смайликов"
              onClick={closePicker}
            >
              <X aria-hidden="true" />
            </button>
          </div>
          <div
            className="emoji-picker-grid"
            role="group"
            aria-label="Выберите смайлик"
          >
            {CHAT_EMOJIS.map(({ emoji, label }, index) => (
              <button
                key={emoji}
                type="button"
                className="emoji-picker-option"
                aria-label={label}
                title={label}
                tabIndex={activeIndex === index ? 0 : -1}
                ref={(button) => {
                  buttons.current[index] = button;
                }}
                onFocus={() => setActiveIndex(index)}
                onKeyDown={(event) => navigateGrid(event, index)}
                onClick={() => {
                  closePicker();
                  onSelect(emoji);
                }}
              >
                <span aria-hidden="true">{emoji}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
