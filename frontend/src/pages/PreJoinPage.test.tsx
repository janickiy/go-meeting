import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { api } from "../api";
import type { Participant } from "../types";
import { PreJoinPage } from "./PreJoinPage";

vi.mock("../auth", () => ({
  useAuth: () => ({
    user: {
      id: "user-1",
      email: "alice@example.test",
      displayName: "Алиса",
    },
  }),
}));
vi.mock("../queries", () => ({
  useMembership: () => ({
    data: null,
    isPending: false,
    isError: false,
    error: null,
  }),
}));

const code = "a".repeat(32);
const participant = {
  id: "participant-1",
  conferenceId: "meeting-1",
  userId: "user-1",
  displayName: "Алиса",
  role: "participant",
  status: "waiting",
  admissionState: "waiting",
} as Participant;

function page() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter
        initialEntries={[`/conferences/meeting-1/join?invite=${code}`]}
      >
        <Routes>
          <Route path="/conferences/:id/join" element={<PreJoinPage />} />
          <Route path="/conferences/:id" element={<p>Комната встречи</p>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.spyOn(api, "invite").mockResolvedValue({
    status: "ok",
    item: {
      id: "meeting-1",
      title: "Проверка проекта",
      status: "active",
      waitingRoomEnabled: true,
    },
  });
  vi.spyOn(api, "joinInvite").mockResolvedValue({
    status: "ok",
    item: participant,
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("PreJoinPage", () => {
  it("shows invite details without joining until the user presses Join", async () => {
    page();
    expect(
      await screen.findByRole("heading", { name: "Проверка проекта" }),
    ).toBeInTheDocument();
    expect(api.joinInvite).not.toHaveBeenCalled();
    expect(screen.queryByText(/зал ожидания/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Войти во встречу" }));
    expect(await screen.findByText("Комната встречи")).toBeInTheDocument();
    expect(api.joinInvite).toHaveBeenCalledExactlyOnceWith(code);
  });

  it("blocks an invite whose conference ID differs from the route", async () => {
    vi.mocked(api.invite).mockResolvedValue({
      status: "ok",
      item: { id: "other-meeting", title: "Чужая встреча", status: "active" },
    });
    page();
    expect(
      await screen.findByText("Ссылка относится к другой встрече."),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Войти во встречу" }),
    ).toBeNull();
    expect(api.joinInvite).not.toHaveBeenCalled();
  });

  it("stops local preview tracks before navigating to a waiting room", async () => {
    const track = { kind: "video", stop: vi.fn(), onended: null };
    const stream = {
      getTracks: () => [track],
      getVideoTracks: () => [track],
      getAudioTracks: () => [],
    } as unknown as MediaStream;
    const capture = vi.fn().mockResolvedValue(stream);
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: {
        getUserMedia: capture,
        enumerateDevices: vi.fn().mockResolvedValue([]),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      },
    });
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
    page();
    await screen.findByRole("heading", { name: "Проверка проекта" });
    fireEvent.click(screen.getByRole("button", { name: "Включить камеру" }));
    await waitFor(() => expect(capture).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Выключить камеру" }),
      ).toHaveAttribute("aria-pressed", "true"),
    );
    expect(api.joinInvite).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Войти во встречу" }));
    expect(await screen.findByText("Комната встречи")).toBeInTheDocument();
    expect(track.stop).toHaveBeenCalled();
  });
});
