import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { api, ApiError, configureAuth } from "../api";
import { AuthProvider, useAuth } from "../auth";
import {
  readDevicePreferences,
  saveDevicePreferences,
  consumeMediaEntry,
} from "../prejoinDevices";
import type { DevicePreferences } from "../prejoinDevices";
import type { Invite, Item, LoginResponse, Participant, User } from "../types";
import { PreJoinPage } from "./PreJoinPage";

vi.mock("../queries", () => ({
  useMembership: () => ({
    data: null,
    isPending: false,
    isError: false,
    error: null,
  }),
}));

const invitation = {
  code: "a".repeat(32),
  meeting: {
    id: "meeting-1",
    title: "Проверка настроек",
    status: "active",
  } satisfies Invite,
};
const alice: User = {
  id: "alice",
  email: "alice@example.test",
  displayName: "Алиса",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};
const bob: User = { ...alice, id: "bob", displayName: "Борис" };
const guest: User = {
  ...alice,
  id: "guest-session",
  email: "",
  displayName: "Гость",
  guestConferenceId: invitation.meeting.id,
};
const membership: Participant = {
  id: "alice-participant",
  conferenceId: invitation.meeting.id,
  userId: alice.id,
  displayName: alice.displayName || "Участник",
  role: "participant",
  status: "joined",
  admissionState: "admitted",
  joinedAt: "2026-01-01T00:00:00Z",
  leftAt: null,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};
const originalMediaDevices = Object.getOwnPropertyDescriptor(
  navigator,
  "mediaDevices",
);

function session(user: User): LoginResponse {
  return {
    user,
    accessToken: `token-${user.id}`,
    tokenType: "Bearer",
    expiresIn: 3600,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function saved(owner: string, patch: Partial<DevicePreferences> = {}) {
  const preferences = {
    ...readDevicePreferences(owner),
    microphoneEnabled: false,
    cameraEnabled: false,
    ...patch,
  };
  saveDevicePreferences(owner, preferences);
  return preferences;
}

function media() {
  const getUserMedia = vi
    .fn()
    .mockRejectedValue(
      new DOMException("Permission denied", "NotAllowedError"),
    );
  const enumerateDevices = vi.fn().mockResolvedValue([]);
  Object.defineProperty(navigator, "mediaDevices", {
    configurable: true,
    value: {
      getUserMedia,
      enumerateDevices,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    },
  });
  return { getUserMedia, enumerateDevices };
}

function stream(kind: "audio" | "video") {
  const track = { kind, stop: vi.fn(), onended: null };
  const value = {
    getTracks: () => [track],
    getAudioTracks: () => (kind === "audio" ? [track] : []),
    getVideoTracks: () => (kind === "video" ? [track] : []),
  } as unknown as MediaStream;
  return { track, value };
}

function AuthControls() {
  const auth = useAuth();
  return (
    <>
      <span data-testid="auth-owner">{auth.user?.id || "guest"}</span>
      <span data-testid="auth-loading">{String(auth.loading)}</span>
      <button onClick={() => void auth.login(bob.email, "test-password")}>
        Сменить аккаунт
      </button>
      <button onClick={() => void auth.logout()}>Выйти из аккаунта</button>
      <button onClick={() => void auth.updateProfile("Новое имя")}>
        Изменить профиль
      </button>
    </>
  );
}

function page() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <AuthControls />
          <MemoryRouter initialEntries={["/conferences/meeting-1/join"]}>
            <Routes>
              <Route
                path="/conferences/:id/join"
                element={<PreJoinPage invitation={invitation} />}
              />
              <Route path="/conferences/:id" element={<p>Комната встречи</p>} />
            </Routes>
          </MemoryRouter>
        </AuthProvider>
      </QueryClientProvider>,
    ),
  };
}

async function ready(owner: string) {
  await waitFor(() => {
    expect(screen.getByTestId("auth-owner")).toHaveTextContent(owner);
    expect(screen.getByTestId("auth-loading")).toHaveTextContent("false");
    expect(
      screen.getByRole("button", { name: "Подключиться" }),
    ).toBeInTheDocument();
  });
}

beforeEach(() => {
  localStorage.clear();
  consumeMediaEntry(invitation.meeting.id);
  vi.stubGlobal("isSecureContext", true);
  vi.stubGlobal(
    "MediaStream",
    class {
      constructor(public tracks: unknown[]) {}
      getTracks() {
        return this.tracks;
      }
    },
  );
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(api, "refreshSession").mockResolvedValue(session(alice));
  vi.spyOn(api, "invite").mockResolvedValue({
    status: "success",
    item: invitation.meeting,
  });
  vi.spyOn(api, "login").mockResolvedValue(session(bob));
  vi.spyOn(api, "logout").mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  configureAuth(null, () => false);
  localStorage.clear();
  consumeMediaEntry(invitation.meeting.id);
  vi.unstubAllGlobals();
  if (originalMediaDevices)
    Object.defineProperty(navigator, "mediaDevices", originalMediaDevices);
  else Reflect.deleteProperty(navigator, "mediaDevices");
});

describe("Настройки предпросмотра и восстановление личности", () => {
  it("не сохраняет гостевые настройки и не захватывает устройства до восстановления cookie-сессии", async () => {
    const restored = deferred<LoginResponse>();
    vi.mocked(api.refreshSession).mockReturnValue(restored.promise);
    const accountPreferences = saved(alice.id, {
      audioInputId: "saved-mic",
      videoInputId: "saved-camera",
      audioOutputId: "saved-output",
      notificationOutputId: "saved-notification",
      noiseSuppression: false,
      showSelf: false,
      hideParticipantVideo: true,
    });
    const guestPreferences = saved("guest", {
      microphoneEnabled: true,
      cameraEnabled: true,
    });
    const devices = media();
    page();
    await act(async () => {
      await Promise.resolve();
    });
    expect(devices.getUserMedia).not.toHaveBeenCalled();
    expect(devices.enumerateDevices).not.toHaveBeenCalled();
    expect(readDevicePreferences(alice.id)).toEqual(accountPreferences);
    expect(readDevicePreferences("guest")).toEqual(guestPreferences);

    await act(async () => restored.resolve(session(alice)));
    await ready(alice.id);
    fireEvent.click(
      screen.getByRole("button", { name: "Настройки устройств" }),
    );
    expect(screen.getByRole("combobox", { name: "Микрофон" })).toHaveValue(
      "saved-mic",
    );
    expect(screen.getByRole("combobox", { name: "Камера" })).toHaveValue(
      "saved-camera",
    );
    expect(readDevicePreferences(alice.id)).toEqual(accountPreferences);
    expect(readDevicePreferences("guest")).toEqual(guestPreferences);
    expect(devices.getUserMedia).not.toHaveBeenCalled();
  });

  it("использует выбранный пользователем микрофон и не включает сохранённую выключенную камеру", async () => {
    saved(alice.id, {
      microphoneEnabled: true,
      audioInputId: "own-mic",
      noiseSuppression: false,
    });
    saved("guest", { microphoneEnabled: true, cameraEnabled: true });
    const devices = media();
    const microphone = stream("audio");
    devices.getUserMedia.mockResolvedValue(microphone.value);
    page();
    await ready(alice.id);
    await waitFor(() => expect(devices.getUserMedia).toHaveBeenCalledOnce());
    expect(devices.getUserMedia).toHaveBeenCalledWith({
      audio: {
        echoCancellation: true,
        noiseSuppression: false,
        deviceId: { exact: "own-mic" },
      },
      video: false,
    });
    await screen.findByRole("button", { name: "Выключить микрофон" });
    expect(readDevicePreferences(alice.id).cameraEnabled).toBe(false);
  });

  it.each([
    { action: "Сменить аккаунт", nextOwner: bob.id },
    { action: "Выйти из аккаунта", nextOwner: "guest" },
  ])(
    "останавливает запоздалый захват при действии «$action»",
    async ({ action, nextOwner }) => {
      saved(alice.id, { microphoneEnabled: true, audioInputId: "alice-mic" });
      const nextPreferences = saved(nextOwner, { audioInputId: "next-mic" });
      const devices = media();
      const pending = deferred<MediaStream>();
      devices.getUserMedia.mockReturnValue(pending.promise);
      page();
      await ready(alice.id);
      await waitFor(() => expect(devices.getUserMedia).toHaveBeenCalledOnce());
      fireEvent.click(screen.getByRole("button", { name: action }));
      await ready(nextOwner);
      const microphone = stream("audio");
      await act(async () => pending.resolve(microphone.value));
      await waitFor(() => expect(microphone.track.stop).toHaveBeenCalledOnce());
      expect(readDevicePreferences(nextOwner)).toEqual(nextPreferences);
      expect(devices.getUserMedia).toHaveBeenCalledOnce();
      expect(
        screen.getByRole("button", { name: "Включить микрофон" }),
      ).toHaveAttribute("aria-pressed", "false");
    },
  );

  it("не применяет запоздалый список устройств предыдущего аккаунта", async () => {
    saved(alice.id, { audioInputId: "alice-mic" });
    const bobPreferences = saved(bob.id, { audioInputId: "bob-mic" });
    const devices = media();
    const firstDevices = deferred<MediaDeviceInfo[]>();
    devices.enumerateDevices.mockReturnValueOnce(firstDevices.promise);
    page();
    await ready(alice.id);
    await waitFor(() =>
      expect(devices.enumerateDevices).toHaveBeenCalledOnce(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Сменить аккаунт" }));
    await ready(bob.id);
    await act(async () =>
      firstDevices.resolve([
        { kind: "audioinput", deviceId: "alice-mic" } as MediaDeviceInfo,
      ]),
    );
    expect(readDevicePreferences(bob.id)).toEqual(bobPreferences);
    expect(screen.queryByText(/Сохранённое устройство отключено/)).toBeNull();
  });

  it("останавливает уже работающий предпросмотр при смене аккаунта", async () => {
    saved(alice.id, { cameraEnabled: true });
    const bobPreferences = saved(bob.id, { videoInputId: "bob-camera" });
    const devices = media();
    const camera = stream("video");
    devices.getUserMedia.mockResolvedValue(camera.value);
    page();
    await ready(alice.id);
    await screen.findByRole("button", { name: "Выключить камеру" });
    expect(camera.track.stop).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Сменить аккаунт" }));
    await ready(bob.id);
    expect(camera.track.stop).toHaveBeenCalledOnce();
    expect(camera.track.onended).toBeNull();
    expect(readDevicePreferences(bob.id)).toEqual(bobPreferences);
    expect(
      screen.getByRole("button", { name: "Включить камеру" }),
    ).toHaveAttribute("aria-pressed", "false");
    expect(devices.getUserMedia).toHaveBeenCalledOnce();
  });

  it.each([
    { action: "Сменить аккаунт", nextOwner: bob.id },
    { action: "Выйти из аккаунта", nextOwner: "guest" },
  ])(
    "не применяет допуск старого аккаунта после действия «$action»",
    async ({ action, nextOwner }) => {
      const alicePreferences = saved(alice.id, { audioInputId: "alice-mic" });
      const nextPreferences = saved(nextOwner, { audioInputId: "next-mic" });
      media();
      const pending = deferred<Item<Participant>>();
      vi.spyOn(api, "joinInvite").mockReturnValue(pending.promise);
      const view = page();
      await ready(alice.id);
      fireEvent.click(screen.getByRole("button", { name: "Подключиться" }));
      await waitFor(() => expect(api.joinInvite).toHaveBeenCalledOnce());
      fireEvent.click(screen.getByRole("button", { name: action }));
      await ready(nextOwner);
      await act(async () =>
        pending.resolve({ status: "success", item: membership }),
      );
      await screen.findByText("Сессия изменилась. Повторите подключение.");
      expect(screen.queryByText("Комната встречи")).toBeNull();
      expect(readDevicePreferences(alice.id)).toEqual(alicePreferences);
      expect(readDevicePreferences(nextOwner)).toEqual(nextPreferences);
      expect(
        view.queryClient.getQueryData([
          "membership",
          alice.id,
          invitation.meeting.id,
        ]),
      ).toBeUndefined();
      expect(
        view.queryClient.getQueryData([
          "membership",
          nextOwner,
          invitation.meeting.id,
        ]),
      ).toBeUndefined();
      expect(consumeMediaEntry(invitation.meeting.id)).toBe(false);
    },
  );

  it("блокирует настройки устройств до завершения запроса подключения", async () => {
    const preferences = saved(alice.id, {
      audioInputId: "chosen-mic",
      videoInputId: "chosen-camera",
    });
    media();
    const pending = deferred<Item<Participant>>();
    vi.spyOn(api, "joinInvite").mockReturnValue(pending.promise);
    page();
    await ready(alice.id);
    fireEvent.click(
      screen.getByRole("button", { name: "Настройки устройств" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Подключиться" }));
    await waitFor(() => expect(api.joinInvite).toHaveBeenCalledOnce());
    expect(
      screen.getByRole("button", { name: "Настройки устройств" }),
    ).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Микрофон" })).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Камера" })).toBeDisabled();
    await act(async () =>
      pending.resolve({ status: "success", item: membership }),
    );
    await screen.findByText("Комната встречи");
    expect(readDevicePreferences(alice.id)).toEqual(preferences);
  });

  it("сохраняет ручной выбор при обновлении профиля с той же личностью", async () => {
    saved(alice.id, { audioInputId: "initial-mic" });
    vi.spyOn(api, "updateProfile").mockResolvedValue({
      status: "success",
      user: { ...alice, displayName: "Новое имя" },
    });
    const devices = media();
    devices.enumerateDevices.mockResolvedValue([
      { kind: "audioinput", deviceId: "initial-mic", label: "Первый" },
      { kind: "audioinput", deviceId: "manual-mic", label: "Второй" },
    ]);
    page();
    await ready(alice.id);
    fireEvent.click(
      screen.getByRole("button", { name: "Настройки устройств" }),
    );
    await screen.findByRole("option", { name: "Второй" });
    fireEvent.change(screen.getByRole("combobox", { name: "Микрофон" }), {
      target: { value: "manual-mic" },
    });
    await waitFor(() =>
      expect(readDevicePreferences(alice.id).audioInputId).toBe("manual-mic"),
    );
    fireEvent.click(screen.getByRole("button", { name: "Изменить профиль" }));
    await screen.findByRole("heading", { name: "Новое имя" });
    expect(screen.getByRole("combobox", { name: "Микрофон" })).toHaveValue(
      "manual-mic",
    );
    expect(readDevicePreferences(alice.id).audioInputId).toBe("manual-mic");
  });

  it("переносит выбранные гостем устройства в новую гостевую сессию после подключения", async () => {
    vi.mocked(api.refreshSession).mockRejectedValue(
      new ApiError(401, "Нет сессии"),
    );
    saved("guest", { audioInputId: "initial-mic", noiseSuppression: false });
    const devices = media();
    devices.enumerateDevices.mockResolvedValue([
      { kind: "audioinput", deviceId: "initial-mic", label: "Первый" },
      { kind: "audioinput", deviceId: "manual-mic", label: "Второй" },
    ]);
    vi.spyOn(api, "joinGuest").mockResolvedValue({
      ...session(guest),
      status: "success",
      item: {
        id: "guest-participant",
        conferenceId: invitation.meeting.id,
        userId: guest.id,
        displayName: guest.displayName || "Гость",
        role: "participant",
        status: "joined",
        admissionState: "admitted",
        joinedAt: "2026-01-01T00:00:00Z",
        leftAt: null,
        createdAt: "2026-01-01T00:00:00Z",
        updatedAt: "2026-01-01T00:00:00Z",
      },
    });
    page();
    await ready("guest");
    fireEvent.click(
      screen.getByRole("button", { name: "Настройки устройств" }),
    );
    await screen.findByRole("option", { name: "Второй" });
    fireEvent.change(screen.getByRole("combobox", { name: "Микрофон" }), {
      target: { value: "manual-mic" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Закрыть окно" }));
    fireEvent.click(screen.getByRole("button", { name: "Подключиться" }));
    await screen.findByText("Комната встречи");
    expect(readDevicePreferences(guest.id)).toMatchObject({
      audioInputId: "manual-mic",
      microphoneEnabled: false,
      cameraEnabled: false,
      noiseSuppression: false,
    });
    expect(devices.getUserMedia).not.toHaveBeenCalled();
  });
});
