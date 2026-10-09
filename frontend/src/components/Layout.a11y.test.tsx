import axe from "axe-core";
import { render, screen, within } from "@testing-library/react";
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

const adminState = vi.hoisted(() => ({
  isAdmin: false,
  analytics: false,
  capabilityError: false,
  unread: 2,
}));
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
vi.mock("../personalRealtime", () => ({
  usePersonalRealtime: () => ({
    unread: adminState.unread,
    notice: null,
    dismiss: vi.fn(),
  }),
}));
vi.mock("../useCapabilities", () => ({
  useCapabilities: () => ({
    data: { capabilities: { meetingAnalytics: adminState.analytics } },
    isSuccess: !adminState.capabilityError,
    isError: adminState.capabilityError,
  }),
}));
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
  adminState.analytics = false;
  adminState.capabilityError = false;
  adminState.unread = 2;
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
  it("показывает оформленный слоган под логотипом и копирайт в футере", () => {
    const { container } = showLayout();
    expect(container.querySelector(".app-chrome")).not.toHaveTextContent(
      "Встречи, чаты",
    );
    expect(container.querySelector(".sidebar-tagline")).toHaveTextContent(
      "Встречи, чаты и совместная работа в одном месте.",
    );
    expect(
      container.querySelector(".sidebar-tagline strong"),
    ).toHaveTextContent("в одном месте.");
    const footer = container.querySelector(".workspace-footer") as HTMLElement;
    expect(footer).toHaveTextContent(
      "© 2026 Яницкий Александр. Все права защищены.",
    );
    expect(
      within(footer).getByRole("link", { name: "Яницкий Александр" }),
    ).toHaveAttribute("href", "https://janickiy.com/");
  });
  it("показывает общий счётчик личных и групповых сообщений в основной и мобильной навигации", () => {
    showLayout();
    for (const name of ["Основная навигация", "Быстрая навигация"]) {
      expect(
        within(screen.getByRole("navigation", { name })).getByLabelText(
          "2 непрочитанных сообщений",
        ),
      ).toHaveTextContent("2");
    }
  });
  it("не отображает нулевые счётчики сообщений в навигации", () => {
    adminState.unread = 0;
    const { container } = showLayout();
    expect(container.querySelector(".message-unread-count")).toBeNull();
  });
  it("не использует устаревшие возможности после ошибки обновления", () => {
    adminState.analytics = true;
    adminState.capabilityError = true;
    showLayout();
    expect(screen.queryByRole("link", { name: "Аналитика" })).toBeNull();
  });
  it("предлагает аналитику только после подтверждения возможности сервером", () => {
    const view = showLayout();
    expect(screen.queryByRole("link", { name: "Аналитика" })).toBeNull();
    view.unmount();
    adminState.analytics = true;
    showLayout();
    expect(screen.getByRole("link", { name: "Аналитика" })).toHaveAttribute(
      "href",
      "/analytics",
    );
  });
  it("предлагает переход к содержимому и доступные разделы", () => {
    showLayout();
    expect(
      screen.getByRole("link", { name: "Перейти к содержимому" }),
    ).toHaveAttribute("href", "#workspace-main");
    expect(screen.getAllByRole("link", { name: /Личные/ })[0]).toHaveAttribute(
      "href",
      "/personal",
    );
    expect(screen.queryByRole("link", { name: "Чаты" })).toBeNull();
    expect(
      screen.getByRole("navigation", { name: "Личное пространство" }),
    ).toHaveTextContent("Папки");
    expect(screen.getByRole("link", { name: "Папки" })).toHaveAttribute(
      "href",
      "/folders",
    );
    expect(
      screen.getByRole("navigation", { name: "Настройки приложения" }),
    ).toHaveTextContent("Настройки");
    expect(screen.getByRole("main")).toHaveAttribute("id", "workspace-main");
    expect(screen.queryByRole("link", { name: "История" })).toBeNull();
    expect(screen.getByRole("link", { name: "Записи" })).toHaveAttribute(
      "href",
      "/recordings",
    );
    expect(screen.queryByRole("link", { name: "Уведомления" })).toBeNull();
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
    const drawer = screen.getByRole("dialog", { name: "Меню MeetSpace" });
    expect(drawer).toBeInTheDocument();
    expect(within(drawer).queryByRole("link", { name: "История" })).toBeNull();
    expect(
      within(drawer).getByRole("link", { name: "Записи" }),
    ).toHaveAttribute("href", "/recordings");
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
