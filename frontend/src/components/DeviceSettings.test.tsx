import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AudioSettings, VideoSettings } from "./DeviceSettings";
import { readDevicePreferences } from "../prejoinDevices";

vi.mock("../auth", () => ({
  useAuth: () => ({ user: { id: "settings-user" } }),
}));
const capture = vi.fn();
const devices = [
  { kind: "audioinput", deviceId: "mic-1", label: "USB-микрофон" },
  { kind: "audiooutput", deviceId: "speaker-1", label: "Наушники" },
  { kind: "videoinput", deviceId: "camera-1", label: "USB-камера" },
];
beforeEach(() => {
  localStorage.clear();
  capture.mockReset();
  vi.stubGlobal("navigator", {
    mediaDevices: {
      getUserMedia: capture,
      enumerateDevices: async () => devices,
      getSupportedConstraints: () => ({ noiseSuppression: true }),
    },
  });
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("saves inverted join controls and independent video preferences", async () => {
  const stop = vi.fn();
  capture.mockResolvedValue({ getTracks: () => [{ stop }] });
  const view = render(<AudioSettings />);
  fireEvent.click(
    screen.getByRole("switch", {
      name: "Подключаться с выключенным микрофоном",
    }),
  );
  fireEvent.click(screen.getByRole("switch", { name: "Шумоподавление" }));
  expect(readDevicePreferences("settings-user")).toMatchObject({
    microphoneEnabled: false,
    cameraEnabled: true,
    noiseSuppression: false,
  });
  view.unmount();
  const video = render(<VideoSettings />);
  await waitFor(() => expect(capture).toHaveBeenCalled());
  fireEvent.click(screen.getByRole("switch", { name: /Видеть себя/ }));
  fireEvent.click(screen.getByRole("switch", { name: /Скрыть видео/ }));
  expect(readDevicePreferences("settings-user")).toMatchObject({
    microphoneEnabled: false,
    showSelf: false,
    hideParticipantVideo: true,
  });
  video.unmount();
  expect(stop).toHaveBeenCalledOnce();
});

it("releases camera access granted after closing the video section", async () => {
  let grant!: (stream: unknown) => void;
  capture.mockImplementation(
    () =>
      new Promise((resolve) => {
        grant = resolve;
      }),
  );
  const stop = vi.fn();
  const view = render(<VideoSettings />);
  view.unmount();
  await act(async () => grant({ getTracks: () => [{ stop }] }));
  expect(stop).toHaveBeenCalledOnce();
});

it("reports denied microphone permission and permits a retry", async () => {
  capture.mockRejectedValue(new DOMException("denied", "NotAllowedError"));
  render(<AudioSettings />);
  fireEvent.click(screen.getByRole("button", { name: "Проверить микрофон" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Доступ к устройству запрещён",
  );
  fireEvent.click(screen.getByRole("button", { name: "Проверить микрофон" }));
  await waitFor(() => expect(capture).toHaveBeenCalledTimes(2));
  expect(capture.mock.calls[0][0]).toMatchObject({
    audio: { noiseSuppression: true },
    video: false,
  });
});
