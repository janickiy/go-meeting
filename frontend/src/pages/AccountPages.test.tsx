import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SettingsPage } from "./AccountPages";

const updateProfile = vi.hoisted(() => vi.fn());
vi.mock("../auth", () => ({
  useAuth: () => ({
    user: {
      id: "user",
      email: "member@example.test",
      displayName: "Старое имя",
      createdAt: "2026-10-01T10:00:00Z",
    },
    updateProfile,
  }),
}));
vi.mock("../components/DeviceSettings", () => ({
  AudioSettings: () => <div>Настройки звука</div>,
  VideoSettings: () => <div>Настройки видео</div>,
}));
vi.mock("../components/IntegrationsSettings", () => ({
  IntegrationsSettings: () => <div>Настройки уведомлений</div>,
}));

beforeEach(() => {
  updateProfile.mockReset();
  updateProfile.mockResolvedValue({ displayName: "Новое имя" });
});

describe("профиль в настройках", () => {
  it("показывает только поддерживаемые вкладки, не изменяя адрес при переключении", () => {
    render(<SettingsPage />);
    const before = window.location.href;
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "Профиль",
      "Аудио",
      "Видео",
      "Уведомления",
      "Оформление",
    ]);
    expect(screen.queryByText("Интеграции")).toBeNull();
    expect(screen.queryByText("Безопасность")).toBeNull();
    fireEvent.click(screen.getByRole("tab", { name: "Аудио" }));
    expect(screen.getByText("Настройки звука")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Email" })).toBeNull();
    fireEvent.click(screen.getByRole("tab", { name: "Уведомления" }));
    expect(screen.getByText("Настройки уведомлений")).toBeInTheDocument();
    expect(window.location.href).toBe(before);
  });

  it("показывает только две темы и процентный размер текста в оформлении", () => {
    render(<SettingsPage />);
    fireEvent.click(screen.getByRole("tab", { name: "Оформление" }));
    expect(screen.getByRole("radio", { name: "Светлая тема" })).toBeChecked();
    expect(
      screen.getByRole("radio", { name: "Тёмная тема" }),
    ).not.toBeChecked();
    expect(screen.getAllByRole("radio")).toHaveLength(2);
    const size = screen.getByRole("combobox", { name: "Размер текста" });
    expect(size).toHaveValue("100");
    expect(
      screen
        .getAllByRole("option")
        .map((option) => (option as HTMLOptionElement).value),
    ).toEqual(["75", "90", "100", "110", "125", "150", "200"]);
    expect(
      screen.queryByRole("combobox", { name: /Масштаб интерфейса|Плотность/ }),
    ).toBeNull();
  });

  it("переключает вкладки с клавиатуры и оставляет только выбранную в обычном Tab-порядке", () => {
    render(<SettingsPage />);
    const profile = screen.getByRole("tab", { name: "Профиль" });
    profile.focus();
    fireEvent.keyDown(profile, { key: "End" });
    const appearance = screen.getByRole("tab", { name: "Оформление" });
    expect(appearance).toHaveFocus();
    expect(appearance).toHaveAttribute("aria-selected", "true");
    expect(profile).toHaveAttribute("tabindex", "-1");
    expect(screen.getAllByRole("tabpanel")).toHaveLength(1);
    fireEvent.keyDown(appearance, { key: "Home" });
    expect(profile).toHaveFocus();
    expect(profile).toHaveAttribute("tabindex", "0");
  });

  it("показывает новую вкладку с начала, не перенося прокрутку предыдущей", () => {
    render(<SettingsPage />);
    const panel = screen.getByRole("tabpanel");
    panel.scrollTop = 240;
    fireEvent.click(screen.getByRole("tab", { name: "Уведомления" }));
    expect(screen.getByRole("tabpanel").scrollTop).toBe(0);
    panel.scrollTop = 100;
    fireEvent.keyDown(screen.getByRole("tab", { name: "Уведомления" }), {
      key: "Home",
    });
    expect(screen.getByRole("tabpanel").scrollTop).toBe(0);
  });

  it("показывает реальный email только для чтения без вымышленных полей профиля", () => {
    render(<SettingsPage />);
    expect(screen.getByRole("textbox", { name: "Email" })).toHaveValue(
      "member@example.test",
    );
    expect(screen.getByRole("textbox", { name: "Email" })).toHaveAttribute(
      "readonly",
    );
    expect(screen.queryByRole("textbox", { name: "Должность" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Изменить фото" })).toBeNull();
    expect(
      screen.getByRole("tablist", { name: "Разделы настроек" }),
    ).toBeInTheDocument();
  });
  it("сохраняет собственное имя без повторной авторизации", async () => {
    render(<SettingsPage />);
    fireEvent.change(screen.getByRole("textbox", { name: /Имя для встреч/ }), {
      target: { value: "  Новое имя  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить имя" }));
    await waitFor(() =>
      expect(updateProfile).toHaveBeenCalledWith("Новое имя"),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "Имя сохранено",
    );
  });

  it("отклоняет управляющие символы до отправки", async () => {
    render(<SettingsPage />);
    fireEvent.change(screen.getByRole("textbox", { name: /Имя для встреч/ }), {
      target: { value: "Имя\u0007" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить имя" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "управляющих знаков",
    );
    expect(updateProfile).not.toHaveBeenCalled();
  });
});
