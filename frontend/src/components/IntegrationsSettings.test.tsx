import { StrictMode } from "react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "../api";
import { CalendarCallback, IntegrationsSettings } from "./IntegrationsSettings";

const auth = vi.hoisted(() => ({
  user: { id: "user" } as { id: string } | null,
  loading: false,
}));
vi.mock("../auth", () => ({ useAuth: () => auth }));
const preferences = {
  invitation: true,
  reminder: true,
  recording: true,
  summary: true,
  email: false,
  push: false,
};
let clients: QueryClient[] = [];

/**
 * Создаёт независимый кеш настроек для проверки без внешних провайдеров.
 * @args callback — включение маршрута возврата OAuth.
 * @return Результат монтирования тестового дерева.
 */
function show(callback = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <MemoryRouter
          initialEntries={[
            callback
              ? "/app/settings/calendar/generic/callback"
              : "/app/settings",
          ]}
        >
          {callback ? (
            <Routes>
              <Route
                path="/app/settings/calendar/:provider/callback"
                element={<CalendarCallback />}
              />
            </Routes>
          ) : (
            <IntegrationsSettings />
          )}
        </MemoryRouter>
      </QueryClientProvider>
    </StrictMode>,
  );
}
beforeEach(() => {
  auth.user = { id: "user" };
  auth.loading = false;
  vi.spyOn(api, "notificationPreferences").mockResolvedValue(preferences);
  vi.spyOn(api, "integrationCapabilities").mockResolvedValue({
    email: "noop",
    push: "noop",
    calendar: "noop",
    calendarOAuthConfigured: false,
    mockConnectAllowed: false,
  });
  vi.spyOn(api, "calendars").mockResolvedValue({
    status: "success",
    items: [],
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients = [];
  vi.restoreAllMocks();
  window.history.replaceState(null, "", "/");
});

describe("настройки интеграций", () => {
  it("не предлагает фиктивное подключение и отключает недоступные каналы", async () => {
    show();
    expect(
      await screen.findByRole("checkbox", { name: /Email/ }),
    ).toBeDisabled();
    expect(
      screen.getByRole("checkbox", { name: /Push-уведомления/ }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Подключить календарь" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Подключить тестовый календарь" }),
    ).toBeNull();
    expect(screen.getByText(/Подключение станет доступно/)).toBeInTheDocument();
  });
  it("сохраняет явные предпочтения и подтверждает результат", async () => {
    const save = vi
      .spyOn(api, "saveNotificationPreferences")
      .mockResolvedValue({ ...preferences, reminder: false });
    show();
    fireEvent.click(
      await screen.findByRole("checkbox", { name: "Напоминания о встречах" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Сохранить настройки" }),
    );
    await screen.findByText("Настройки сохранены.");
    expect(save).toHaveBeenCalledWith({ ...preferences, reminder: false });
  });
  it("разрешает email через SMTP и сохраняет выбор канала", async () => {
    vi.mocked(api.integrationCapabilities).mockResolvedValue({
      email: "smtp",
      push: "noop",
      calendar: "noop",
      calendarOAuthConfigured: false,
      mockConnectAllowed: false,
    });
    const save = vi
      .spyOn(api, "saveNotificationPreferences")
      .mockResolvedValue({ ...preferences, email: true });
    show();
    const email = await screen.findByRole("checkbox", { name: /Email/ });
    expect(email).toBeEnabled();
    expect(screen.getByText("Настроена отправка почты")).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: /Push-уведомления/ }),
    ).toBeDisabled();
    fireEvent.click(email);
    fireEvent.click(
      screen.getByRole("button", { name: "Сохранить настройки" }),
    );
    await screen.findByText("Настройки сохранены.");
    expect(save).toHaveBeenCalledWith({ ...preferences, email: true });
  });
  it("явно отличает тестовый календарь от реальной интеграции", async () => {
    vi.mocked(api.integrationCapabilities).mockResolvedValue({
      email: "mock",
      push: "mock",
      calendar: "mock",
      calendarOAuthConfigured: false,
      mockConnectAllowed: true,
    });
    const connect = vi.spyOn(api, "connectMockCalendar").mockResolvedValue({
      status: "success",
      item: {
        id: "calendar",
        provider: "mock",
        calendarId: "primary",
        status: "connected",
        createdAt: "now",
        updatedAt: "now",
      },
    });
    show();
    fireEvent.click(
      await screen.findByRole("button", {
        name: "Подключить тестовый календарь",
      }),
    );
    await waitFor(() => expect(connect).toHaveBeenCalledTimes(1));
    expect(
      screen.getByText(/не создаёт события во внешнем сервисе/),
    ).toBeInTheDocument();
  });
  it("удаляет OAuth code/state из URL и не повторяет callback при StrictMode", async () => {
    window.history.replaceState(
      null,
      "",
      "/app/settings/calendar/generic/callback?code=authorization-code&state=nonce",
    );
    const callback = vi.spyOn(api, "calendarCallback").mockResolvedValue({
      status: "success",
      item: {
        id: "calendar",
        provider: "generic",
        calendarId: "primary",
        status: "connected",
        createdAt: "now",
        updatedAt: "now",
      },
    });
    show(true);
    await screen.findByText("Календарь подключён.");
    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith(
      "generic",
      "authorization-code",
      "nonce",
    );
    expect(window.location.search).toBe("");
  });
  it("при истёкшей сессии очищает код и просит повторить подключение", async () => {
    auth.user = null;
    window.history.replaceState(
      null,
      "",
      "/app/settings/calendar/generic/callback?code=secret&state=nonce",
    );
    const callback = vi.spyOn(api, "calendarCallback");
    show(true);
    await screen.findByText(/Войдите в аккаунт и начните подключение заново/);
    expect(callback).not.toHaveBeenCalled();
    expect(window.location.search).toBe("");
  });
});
