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
  DeviceSettings: () => <div>Настройки устройств</div>,
}));
vi.mock("../components/IntegrationsSettings", () => ({
  IntegrationsSettings: () => <div>Настройки уведомлений</div>,
}));

beforeEach(() => {
  updateProfile.mockReset();
  updateProfile.mockResolvedValue({ displayName: "Новое имя" });
});

describe("профиль в настройках", () => {
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
