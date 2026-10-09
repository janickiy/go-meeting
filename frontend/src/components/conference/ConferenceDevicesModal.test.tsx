import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ConferenceDevicesModal } from "./ConferenceDevicesModal";
import {
  readDevicePreferences,
  saveDevicePreferences,
} from "../../prejoinDevices";
import { playDeviceTone } from "../../deviceSound";

const identity = vi.hoisted(() => ({ user: { id: "device-user" } }));
vi.mock("../../auth", () => ({ useAuth: () => identity }));
vi.mock("../../deviceSound", () => ({ playDeviceTone: vi.fn() }));
const capture = vi.fn();
const mediaDevices = new EventTarget();
const enumerate = vi.fn();
beforeEach(() => {
  localStorage.clear();
  identity.user = { id: "device-user" };
  capture.mockReset();
  enumerate.mockResolvedValue([
    { kind: "audioinput", deviceId: "mic-usb", label: "USB-микрофон" },
    { kind: "videoinput", deviceId: "cam-usb", label: "USB-камера" },
  ]);
  Object.assign(mediaDevices, {
    enumerateDevices: enumerate,
    getUserMedia: capture,
  });
  vi.stubGlobal("navigator", { mediaDevices });
});
afterEach(() => {
  cleanup();
  expect(capture).not.toHaveBeenCalled();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("меняет общие настройки без повторного захвата и без включения выключенных устройств", async () => {
  saveDevicePreferences("device-user", {
    ...readDevicePreferences("device-user"),
    microphoneEnabled: false,
    cameraEnabled: false,
  });
  const onClose = vi.fn();
  render(<ConferenceDevicesModal onClose={onClose} />);
  await screen.findByRole("option", { name: "USB-микрофон" });
  fireEvent.change(screen.getByRole("combobox", { name: "Микрофон" }), {
    target: { value: "mic-usb" },
  });
  fireEvent.change(screen.getByRole("combobox", { name: "Камера" }), {
    target: { value: "cam-usb" },
  });
  fireEvent.click(screen.getByRole("checkbox", { name: "Шумоподавление" }));
  expect(readDevicePreferences("device-user")).toMatchObject({
    audioInputId: "mic-usb",
    videoInputId: "cam-usb",
    microphoneEnabled: false,
    cameraEnabled: false,
    noiseSuppression: false,
  });
  fireEvent.click(screen.getByRole("button", { name: "Готово" }));
  expect(onClose).toHaveBeenCalledOnce();
});

it("сохраняет выбранное отключённое устройство и обновляет список после подключения", async () => {
  saveDevicePreferences("device-user", {
    ...readDevicePreferences("device-user"),
    audioInputId: "missing",
  });
  render(<ConferenceDevicesModal onClose={() => {}} />);
  expect(
    await screen.findByRole("option", {
      name: "Выбранное устройство недоступно",
    }),
  ).toHaveValue("missing");
  enumerate.mockResolvedValue([
    { kind: "audioinput", deviceId: "missing", label: "Подключённый микрофон" },
  ]);
  act(() => mediaDevices.dispatchEvent(new Event("devicechange")));
  await screen.findByRole("option", { name: "Подключённый микрофон" });
  expect(screen.getByRole("combobox", { name: "Микрофон" })).toHaveValue(
    "missing",
  );
});

it("останавливает тестовый звук при закрытии и безопасно игнорирует поздний callback", async () => {
  let done: ((error?: unknown) => void) | undefined;
  const stop = vi.fn(() => done?.());
  vi.mocked(playDeviceTone).mockImplementation((_sink, callback) => {
    done = callback;
    return stop;
  });
  const view = render(<ConferenceDevicesModal onClose={() => {}} />);
  await waitFor(() => expect(enumerate).toHaveBeenCalled());
  fireEvent.click(screen.getByRole("button", { name: "Проверить звук" }));
  expect(screen.getByRole("button", { name: "Воспроизводим…" })).toBeDisabled();
  view.unmount();
  expect(stop).toHaveBeenCalledOnce();
  act(() => done?.(new Error("late")));
});

it("показывает отказ перечисления устройств и восстанавливается при изменении", async () => {
  enumerate.mockRejectedValueOnce(new Error("denied"));
  render(<ConferenceDevicesModal onClose={() => {}} />);
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Не удалось получить устройства",
  );
  act(() => mediaDevices.dispatchEvent(new Event("devicechange")));
  await screen.findByRole("option", { name: "USB-микрофон" });
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
