import { fireEvent, render, screen, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { CHAT_EMOJIS } from "../emoji";
import { EmojiPicker } from "./EmojiPicker";

/**
 * openPicker отображает компонент и открывает палитру для проверки.
 * @return обработчик выбора, кнопка открытия, палитра и методы рендера.
 */
function openPicker() {
  const onSelect = vi.fn();
  const view = render(<EmojiPicker onSelect={onSelect} />);
  const trigger = screen.getByRole("button", { name: "Добавить смайлик" });
  fireEvent.click(trigger);
  const dialog = screen.getByRole("dialog", { name: "Смайлики" });
  const choices = within(
    within(dialog).getByRole("group", { name: "Выберите смайлик" }),
  ).getAllByRole("button");
  return { ...view, onSelect, trigger, dialog, choices };
}

it("предлагает 64 уникальных смайлика с русскими подписями", () => {
  const { trigger, dialog, choices } = openPicker();
  expect(choices).toHaveLength(64);
  expect(choices.length).toBeGreaterThanOrEqual(40);
  expect(new Set(CHAT_EMOJIS.map((item) => item.emoji)).size).toBe(64);
  for (const choice of choices)
    expect(choice.getAttribute("aria-label")).toMatch(/[А-Яа-яЁё]/);
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  expect(trigger).toHaveAttribute("aria-controls", dialog.id);
  expect(choices[0]).toHaveFocus();
  expect(choices.filter((choice) => choice.tabIndex === 0)).toHaveLength(1);
});

it("возвращает выбранный смайлик, закрывает палитру и восстанавливает фокус", () => {
  const { onSelect, trigger } = openPicker();
  fireEvent.click(screen.getByRole("button", { name: "Улыбающийся кот" }));
  expect(onSelect).toHaveBeenCalledExactlyOnceWith("😺");
  expect(screen.queryByRole("dialog", { name: "Смайлики" })).toBeNull();
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  expect(trigger).toHaveFocus();
});

it("не открывает палитру при заблокированном вводе", () => {
  const onSelect = vi.fn();
  render(<EmojiPicker onSelect={onSelect} disabled />);
  const trigger = screen.getByRole("button", { name: "Добавить смайлик" });
  expect(trigger).toBeDisabled();
  fireEvent.click(trigger);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(onSelect).not.toHaveBeenCalled();
});

it("закрывает уже открытую палитру при блокировке ввода", () => {
  const { onSelect, rerender } = openPicker();
  rerender(<EmojiPicker onSelect={onSelect} disabled />);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(onSelect).not.toHaveBeenCalled();
});

it("закрывает палитру клавишей Escape без выбора смайлика", () => {
  const { choices, onSelect, trigger } = openPicker();
  fireEvent.keyDown(choices[0], { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(trigger).toHaveFocus();
  expect(onSelect).not.toHaveBeenCalled();
});

it("закрывает палитру при клике вне неё", () => {
  const { onSelect, trigger } = openPicker();
  fireEvent.pointerDown(document.body);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(trigger).toHaveFocus();
  expect(onSelect).not.toHaveBeenCalled();
});

it("обрабатывает обычный клик вне палитры и кнопку закрытия", () => {
  const { trigger } = openPicker();
  fireEvent.click(document.body);
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(trigger);
  fireEvent.click(
    screen.getByRole("button", { name: "Закрыть выбор смайликов" }),
  );
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(trigger).toHaveFocus();
});

it("перемещает фокус стрелками и клавишами Home и End по сетке", () => {
  const { choices } = openPicker();
  fireEvent.keyDown(choices[0], { key: "ArrowRight" });
  expect(choices[1]).toHaveFocus();
  fireEvent.keyDown(choices[1], { key: "ArrowDown" });
  expect(choices[7]).toHaveFocus();
  fireEvent.keyDown(choices[7], { key: "End" });
  expect(choices[11]).toHaveFocus();
  fireEvent.keyDown(choices[11], { key: "Home" });
  expect(choices[6]).toHaveFocus();
  fireEvent.keyDown(choices[6], { key: "ArrowUp" });
  expect(choices[0]).toHaveFocus();
  fireEvent.keyDown(choices[0], { key: "ArrowLeft" });
  expect(choices[63]).toHaveFocus();
  fireEvent.keyDown(choices[63], { key: "ArrowRight" });
  expect(choices[0]).toHaveFocus();
  fireEvent.keyDown(choices[0], { key: "End", ctrlKey: true });
  expect(choices[63]).toHaveFocus();
  fireEvent.keyDown(choices[63], { key: "Home", ctrlKey: true });
  expect(choices[0]).toHaveFocus();
  expect(choices.filter((choice) => choice.tabIndex === 0)).toHaveLength(1);
});
