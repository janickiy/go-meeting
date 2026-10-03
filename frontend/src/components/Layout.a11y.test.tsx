import axe from "axe-core";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  afterAll,
  afterEach,
  beforeAll,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { MemoryRouter, Route, Routes } from "react-router";
import { Layout } from "./Layout";

const adminState = vi.hoisted(() => ({ isAdmin: false }));
vi.mock("../auth", () => ({
  useAuth: () => ({
    user: {
      id: "user-1",
      displayName: "Алиса",
      email: "alice@example.test",
      isAdmin: adminState.isAdmin,
    },
    logout: vi.fn(),
  }),
}));
vi.mock("../notifications", () => ({ useNotificationStream: vi.fn() }));
vi.mock("./NotificationBell", () => ({
  NotificationBell: () => <span>Новые сообщения</span>,
}));

beforeAll(() => {
  vi.stubGlobal("matchMedia", () => ({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }));
});
afterAll(() => vi.unstubAllGlobals());
afterEach(() => {
  adminState.isAdmin = false;
});

function showLayout() {
  return render(
    <MemoryRouter initialEntries={["/app"]}>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/app" element={<h1>Главная</h1>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe("навигация приложения", () => {
  it("предлагает переход к содержимому и доступные разделы", () => {
    showLayout();
    expect(
      screen.getByRole("link", { name: "Перейти к содержимому" }),
    ).toHaveAttribute("href", "#workspace-main");
    expect(screen.getByRole("main")).toHaveAttribute("id", "workspace-main");
    expect(screen.getByRole("link", { name: "История" })).toHaveAttribute(
      "href",
      "/history",
    );
    expect(screen.getByRole("link", { name: "Уведомления" })).toHaveAttribute(
      "href",
      "/notifications",
    );
    expect(
      screen.queryByRole("link", { name: "Администрирование" }),
    ).toBeNull();
  });

  it("показывает администрирование только пользователю с правом", () => {
    adminState.isAdmin = true;
    showLayout();
    expect(
      screen.getByRole("link", { name: "Администрирование" }),
    ).toHaveAttribute("href", "/admin");
  });

  it("открывает мобильное меню с явным состоянием и закрывает по Escape", async () => {
    const user = userEvent.setup();
    showLayout();
    const button = screen.getByRole("button", { name: "Открыть меню" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    await user.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByRole("dialog", { name: "Меню Meet" }),
    ).toBeInTheDocument();
    const menuAudit = await axe.run(document.body, {
      runOnly: {
        type: "tag",
        values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
      },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(menuAudit.violations.map((violation) => violation.id)).toEqual([]);
    await user.keyboard("{Escape}");
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(button).toHaveFocus();
  });

  it("проходит автоматические правила WCAG для каркаса", async () => {
    showLayout();
    const results = await axe.run(document.body, {
      runOnly: {
        type: "tag",
        values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
      },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(results.violations.map((violation) => violation.id)).toEqual([]);
  });
});
