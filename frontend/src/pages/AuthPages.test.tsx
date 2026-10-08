import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthPage } from "./AuthPages";

const login = vi.hoisted(() => vi.fn());
vi.mock("../auth", () => ({
  useAuth: () => ({ user: null, loading: false, expired: false, login }),
}));
beforeEach(() => {
  login.mockReset();
  login.mockResolvedValue(undefined);
});

/** Показывает фактический адрес после отправки формы входа.
 * @return Текущий локальный путь и параметры перехода.
 */
function Destination() {
  const location = useLocation();
  return (
    <output data-testid="destination">
      {location.pathname + location.search}
    </output>
  );
}

describe("вход и будущие провайдеры Meetrix", () => {
  it("показывает явно недоступные заглушки без ссылок и входа", () => {
    render(
      <MemoryRouter>
        <AuthPage />
      </MemoryRouter>,
    );
    expect(
      screen.getByRole("button", { name: /Продолжить с Google/ }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: /Продолжить с Microsoft/ }),
    ).toBeDisabled();
    expect(
      screen.getByText(/Эти способы входа появятся позже/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Google|Microsoft/ })).toBeNull();
    expect(login).not.toHaveBeenCalled();
  });

  it("сохраняет настоящую авторизацию по email и паролю", async () => {
    render(
      <MemoryRouter>
        <AuthPage />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "member@example.test" },
    });
    fireEvent.change(screen.getByLabelText("Пароль", { exact: true }), {
      target: { value: "valid-password" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^Войти$/ }));
    await waitFor(() =>
      expect(login).toHaveBeenCalledWith(
        "member@example.test",
        "valid-password",
      ),
    );
  });

  it.each([
    "/calendar?date=2026-10-09",
    "/history/45144e4e-c3d7-4eed-863e-2edc7ceec5b4",
    "/meetings/45144e4e-c3d7-4eed-863e-2edc7ceec5b4/join",
    "/settings",
    "/notifications",
    "/folders/45144e4e-c3d7-4eed-863e-2edc7ceec5b4",
  ])("возвращает в нужный раздел после успешного входа: %s", async (next) => {
    render(
      <MemoryRouter
        initialEntries={[`/login?next=${encodeURIComponent(next)}`]}
      >
        <AuthPage />
        <Destination />
      </MemoryRouter>,
    );
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "member@example.test" },
    });
    fireEvent.change(screen.getByLabelText("Пароль", { exact: true }), {
      target: { value: "valid-password" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^Войти$/ }));
    await waitFor(() =>
      expect(screen.getByTestId("destination").textContent).toBe(next),
    );
  });
});
