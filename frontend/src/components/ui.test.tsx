import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Modal, PasswordInput } from "./ui";

describe("accessible UI controls", /**
 * Проверяет доступность элементов управления интерфейса.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("shows and hides the password without submitting the form", /**
   * Проверяет показ и скрытие пароля без отправки формы.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const user = userEvent.setup();
    render(
      <label>
        Пароль
        <PasswordInput aria-label="Пароль" defaultValue="test" />
      </label>,
    );
    expect(screen.getByLabelText("Пароль")).toHaveAttribute("type", "password");
    await user.click(screen.getByRole("button", { name: "Показать пароль" }));
    expect(screen.getByLabelText("Пароль")).toHaveAttribute("type", "text");
    await user.click(screen.getByRole("button", { name: "Скрыть пароль" }));
    expect(screen.getByLabelText("Пароль")).toHaveAttribute("type", "password");
  });
  it("traps keyboard focus, handles escape, and restores scroll", /**
   * Проверяет удержание клавиатурного фокуса, обработку Escape и восстановление прокрутки.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    const close = vi.fn();
    const { unmount } = render(
      <Modal title="Новая конференция" onClose={close}>
        <input aria-label="Название" data-autofocus />
        <button>Последняя кнопка</button>
      </Modal>,
    );
    expect(screen.getByRole("dialog")).toHaveAccessibleName(
      "Новая конференция",
    );
    expect(screen.getByLabelText("Название")).toHaveFocus();
    screen.getByRole("button", { name: "Последняя кнопка" }).focus();
    fireEvent.keyDown(document, { key: "Tab" });
    expect(screen.getByRole("button", { name: "Закрыть окно" })).toHaveFocus();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(close).toHaveBeenCalledOnce();
    unmount();
    expect(document.body.style.overflow).toBe("");
  });
});
