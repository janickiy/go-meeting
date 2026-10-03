import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { emptyMediaView } from "../media";
import type { useMedia } from "../useMedia";
import type { useRealtime } from "../realtime";
import type { Participant } from "../types";
import { RealtimePanel } from "./RealtimePanel";

const mediaRef = vi.hoisted(() => ({
  current: null as unknown as ReturnType<typeof useMedia>,
}));
vi.mock("../useMedia", () => ({ useMedia: () => mediaRef.current }));
vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "self" } }) }));
vi.mock("../useCapabilities", () => ({
  useCapabilities: () => ({ data: { buildVersion: "1.9.0+stage9" } }),
}));

const onlineDescriptor = Object.getOwnPropertyDescriptor(navigator, "onLine");
const clipboardDescriptor = Object.getOwnPropertyDescriptor(
  navigator,
  "clipboard",
);

beforeEach(() => {
  mediaRef.current = {
    view: { ...emptyMediaView(), mediaPeerId: "peer", status: "Подключено" },
    running: true,
    start: vi.fn(),
    stop: vi.fn(),
    diagnostics: vi.fn().mockResolvedValue(null),
    microphone: vi.fn(),
    camera: vi.fn(),
    startScreen: vi.fn(),
    stopScreen: vi.fn(),
  } as unknown as ReturnType<typeof useMedia>;
});

afterEach(() => {
  if (onlineDescriptor)
    Object.defineProperty(navigator, "onLine", onlineDescriptor);
  if (clipboardDescriptor)
    Object.defineProperty(navigator, "clipboard", clipboardDescriptor);
  else Reflect.deleteProperty(navigator, "clipboard");
  vi.clearAllMocks();
});

function panel(member?: Partial<Participant>) {
  const membership = {
    id: "self",
    role: "participant",
    status: "joined",
    ...member,
  } as Participant;
  const live = {
    state: { connectionId: "connection", participants: [] },
    status: "Подключено",
    error: null,
    reconnect: vi.fn(),
  } as unknown as ReturnType<typeof useRealtime>;
  return {
    membership,
    live,
    ...render(
      <RealtimePanel conferenceId="room" membership={membership} live={live} />,
    ),
  };
}

it("uses the same mic and camera controls for M/V, but ignores typing and moderation blocks", () => {
  const view = panel();
  fireEvent.keyDown(window, { key: "m" });
  expect(mediaRef.current.microphone).toHaveBeenCalledExactlyOnceWith(true);
  const input = document.createElement("input");
  document.body.append(input);
  fireEvent.keyDown(input, { key: "v" });
  expect(mediaRef.current.camera).not.toHaveBeenCalled();
  input.remove();
  fireEvent.keyDown(window, { key: "v" });
  expect(mediaRef.current.camera).toHaveBeenCalledExactlyOnceWith(true);
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={{ ...view.membership, cameraBlocked: true }}
      live={view.live}
    />,
  );
  fireEvent.keyDown(window, { key: "v" });
  expect(mediaRef.current.camera).toHaveBeenCalledTimes(1);
});

it("explains offline state and offers a reconnect when the network returns", () => {
  const view = panel();
  Object.defineProperty(navigator, "onLine", {
    configurable: true,
    value: false,
  });
  fireEvent(window, new Event("offline"));
  expect(screen.getByText("Нет подключения к сети")).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Переподключиться" }),
  ).toBeDisabled();
  Object.defineProperty(navigator, "onLine", {
    configurable: true,
    value: true,
  });
  fireEvent(window, new Event("online"));
  expect(screen.queryByText("Нет подключения к сети")).toBeNull();
  expect(view.live.reconnect).not.toHaveBeenCalled();
});

it("shows measured stats and copies only a redacted report", async () => {
  vi.mocked(mediaRef.current.diagnostics).mockResolvedValue({
    roundTripTimeMs: 42,
    route: "relay",
    sdp: "private-sdp",
  } as never);
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  panel();
  const details = screen.getByText("Состояние медиасвязи").closest("details")!;
  details.open = true;
  fireEvent(details, new Event("toggle"));
  expect(await screen.findByText("42 мс")).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "Скопировать диагностический отчёт" }),
  );
  await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1));
  const report = writeText.mock.calls[0][0] as string;
  expect(JSON.parse(report)).toMatchObject({
    buildVersion: "1.9.0+stage9",
    rtc: { roundTripTimeMs: 42, route: "relay" },
  });
  expect(report).not.toContain("private-sdp");
});

it("показывает настоящих участников без потоков и не выдаёт их за подключённое видео", () => {
  const view = panel({ displayName: "Алиса", role: "owner" });
  const colleague = {
    id: "colleague",
    displayName: "Борис Волков",
    status: "joined",
    admissionState: "admitted",
    role: "participant",
    microphoneEnabled: false,
  } as Participant;
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={view.membership}
      live={view.live}
      participants={[view.membership, colleague]}
      raisedHands={[colleague.id]}
    />,
  );
  expect(screen.getByText("Борис Волков")).toBeInTheDocument();
  expect(screen.getByText("Б")).toBeInTheDocument();
  expect(screen.getByLabelText("Рука поднята")).toBeInTheDocument();
  expect(screen.getByLabelText("Организатор")).toBeInTheDocument();
  expect(screen.getAllByTestId("participant-placeholder")).toHaveLength(2);
  expect(screen.queryByTestId("remote-media")).toBeNull();
});

it("не перепривязывает тот же поток при обновлении панелей и очищает srcObject после выхода", async () => {
  const play = vi
    .spyOn(HTMLMediaElement.prototype, "play")
    .mockResolvedValue(undefined);
  const stream = {} as MediaStream;
  const colleague = {
    id: "colleague",
    displayName: "Борис Волков",
    status: "joined",
    role: "participant",
  } as Participant;
  mediaRef.current.view.remoteStreams = [
    {
      id: "remote",
      mediaPeerId: "remote-peer",
      participantId: colleague.id,
      stream,
      kinds: ["video", "audio"],
      screen: false,
    },
  ];
  const view = panel();
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={view.membership}
      live={view.live}
      participants={[colleague]}
    />,
  );
  const video = screen.getByTestId("remote-media").querySelector("video")!;
  expect(video.srcObject).toBe(stream);
  const calls = play.mock.calls.length;
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={view.membership}
      live={view.live}
      participants={[{ ...colleague }]}
      controls={<button>Чат</button>}
    />,
  );
  await waitFor(() => expect(play).toHaveBeenCalledTimes(calls));
  view.unmount();
  expect(video.srcObject).toBeNull();
  play.mockRestore();
});
