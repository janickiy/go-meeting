import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Modal, PasswordInput } from "./ui";

describe("accessible UI controls", /**
 * Проверка: accessible UI controls выполняет тестовый сценарий «accessible UI controls» и проверяет ожидаемые результаты.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("shows and hides the password without submitting the form", /**
   * Проверка: shows and hides the password without submitting the form выполняет тестовый сценарий «shows and hides the password without submitting the form» и проверяет ожидаемые результаты.
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
   * Проверка: traps keyboard focus, handles escape, and restores scroll выполняет тестовый сценарий «traps keyboard focus, handles escape, and restores scroll» и проверяет ожидаемые результаты.
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
