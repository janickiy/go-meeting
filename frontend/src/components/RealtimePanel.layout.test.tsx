import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { emptyMediaView } from "../media";
import type { Participant, PresenceParticipant } from "../types";
import type { useMedia } from "../useMedia";
import type { useRealtime } from "../realtime";
import { RealtimePanel } from "./RealtimePanel";

const mediaRef = vi.hoisted(() => ({
  current: null as unknown as ReturnType<typeof useMedia>,
}));
vi.mock("../useMedia", () => ({ useMedia: () => mediaRef.current }));
vi.mock("../useSpeakingParticipants", () => ({
  useSpeakingParticipants: () => new Map<string, number>(),
}));
vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "user-self" } }) }));
vi.mock("../useCapabilities", () => ({
  useCapabilities: () => ({ data: { buildVersion: "test-layout" } }),
}));

const self = {
  id: "self",
  userId: "user-self",
  displayName: "Алиса",
  role: "owner",
  status: "joined",
  admissionState: "admitted",
} as Participant;
const colleague = {
  ...self,
  id: "colleague",
  userId: "user-colleague",
  role: "participant",
  displayName: "Борис",
} as Participant;
const third = {
  ...colleague,
  id: "third",
  userId: "user-third",
  displayName: "Вера",
} as Participant;

/** Создаёт только авторитетный снимок присутствия для проверки раскладки без сети.
 * @args people — онлайн-участники текущего снимка.
 * @return Параметры панели с подтверждённым присутствием переданных участников.
 */
function panel(people: Participant[]) {
  const participants: PresenceParticipant[] = people.map((person) => ({
    ...person,
    online: true,
    connections: 1,
    connectionIds: [`connection-${person.id}`],
  }));
  const live = {
    state: {
      connectionId: "connection-self",
      participantId: self.id,
      status: "active",
      participants,
    },
    status: "Подключено",
    error: null,
    reconnect: vi.fn(),
  } as unknown as ReturnType<typeof useRealtime>;
  return (
    <RealtimePanel
      conferenceId="room"
      membership={self}
      live={live}
      participants={people}
    />
  );
}

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  mediaRef.current = {
    view: {
      ...emptyMediaView(),
      mediaPeerId: "peer-self",
      status: "Подключено",
    },
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
afterEach(() => vi.restoreAllMocks());

it("выбирает парную раскладку только для двух видимых плиток и позволяет прокрутку с клавиатуры", () => {
  const view = render(panel([self]));
  expect(view.container.querySelector(".media-grid-pair")).toBeNull();
  view.rerender(panel([self, colleague]));
  const grid = screen.getByRole("region", { name: "Видео участников" });
  expect(grid).toHaveClass("media-grid-pair");
  expect(grid).toHaveAttribute("tabindex", "0");
  expect(grid.querySelectorAll(".media-tile")).toHaveLength(2);
  const firstTile = grid.querySelector(".media-tile");
  view.rerender(panel([self, colleague, third]));
  expect(view.container.querySelector(".media-grid-pair")).toBeNull();
  expect(view.container.querySelector(".media-tile")).toBe(firstTile);
  view.rerender(panel([self, colleague]));
  expect(view.container.querySelector(".media-grid-pair")).not.toBeNull();
});

it.each([true, false])(
  "считает локальную камеру и удалённый поток без дублирования заглушек (видео: %s)",
  (video) => {
    mediaRef.current.view.localStream = {} as MediaStream;
    mediaRef.current.view.remoteStreams = [
      {
        id: "remote-camera",
        participantId: colleague.id,
        mediaPeerId: "peer-colleague",
        stream: {} as MediaStream,
        screen: false,
        kinds: video ? ["audio", "video"] : ["audio"],
      },
    ];
    const view = render(panel([self, colleague]));
    expect(view.container.querySelector(".media-grid-pair")).not.toBeNull();
    expect(view.container.querySelectorAll(".media-tile")).toHaveLength(2);
    expect(screen.queryByTestId("participant-placeholder")).toBeNull();
    mediaRef.current.view.remoteStreams.push({
      id: "offline-camera",
      participantId: third.id,
      mediaPeerId: "peer-third",
      stream: {} as MediaStream,
      screen: false,
      kinds: ["video"],
    });
    view.rerender(panel([self, colleague]));
    expect(view.container.querySelector(".media-grid-pair")).not.toBeNull();
    expect(view.container.querySelectorAll(".media-tile")).toHaveLength(2);
  },
);

it.each(["local", "remote"])(
  "сохраняет полноразмерный экран и возвращает парную раскладку после его остановки (%s)",
  (source) => {
    mediaRef.current.view.localStream = {} as MediaStream;
    mediaRef.current.view.remoteStreams = [
      {
        id: "remote-camera",
        participantId: colleague.id,
        mediaPeerId: "peer-colleague",
        stream: {} as MediaStream,
        screen: false,
        kinds: ["audio", "video"],
      },
    ];
    const view = render(panel([self, colleague]));
    const localTile = screen.getByTestId("local-media");
    const remoteTile = screen.getByTestId("remote-media");
    if (source === "local")
      mediaRef.current.view.localScreen = {} as MediaStream;
    else
      mediaRef.current.view.remoteStreams.push({
        id: "remote-screen",
        participantId: colleague.id,
        mediaPeerId: "peer-colleague",
        stream: {} as MediaStream,
        screen: true,
        kinds: ["video"],
      });
    view.rerender(panel([self, colleague]));
    expect(view.container.querySelector(".media-grid-pair")).toBeNull();
    expect(view.container.querySelector(".media-grid-sharing")).not.toBeNull();
    expect(localTile).toHaveAttribute("hidden");
    expect(remoteTile).toHaveAttribute("hidden");
    mediaRef.current.view.localScreen = null;
    mediaRef.current.view.remoteStreams =
      mediaRef.current.view.remoteStreams.filter((remote) => !remote.screen);
    view.rerender(panel([self, colleague]));
    expect(view.container.querySelector(".media-grid-pair")).not.toBeNull();
    expect(screen.getByTestId("local-media")).toBe(localTile);
    expect(screen.getByTestId("remote-media")).toBe(remoteTile);
  },
);
