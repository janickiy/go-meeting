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
  useCapabilities: () => ({ data: { buildVersion: "test-presence" } }),
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
  id: "colleague",
  userId: "user-colleague",
  displayName: "Борис Волков",
  role: "participant",
  status: "joined",
  admissionState: "admitted",
} as Participant;

/** Дополняет членство состоянием физического подключения, не создавая настоящее медиа.
 * @args person — участник; changes — отличия текущего серверного присутствия.
 * @return Подтверждённый онлайн-снимок с одним подключением по умолчанию.
 */
function present(
  person: Participant,
  changes: Partial<PresenceParticipant> = {},
): PresenceParticipant {
  return {
    ...person,
    online: true,
    connections: 1,
    connectionIds: [`connection-${person.id}`],
    ...changes,
  };
}

/** Собирает минимальное состояние единственного сигнального подключения панели.
 * @args participants — авторитетный снимок или null до его получения либо при потере связи.
 * @return Управляемое состояние для повторной отрисовки без сети и настоящего WebSocket.
 */
function live(
  participants: readonly PresenceParticipant[] | null,
): ReturnType<typeof useRealtime> {
  return {
    state: participants
      ? {
          connectionId: "connection-self",
          participantId: self.id,
          status: "active",
          participants: [...participants],
        }
      : null,
    status: participants ? "Подключено" : "Связь потеряна",
    error: null,
    reconnect: vi.fn(),
  } as unknown as ReturnType<typeof useRealtime>;
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

it.each(["camera", "audio", "screen"] as const)(
  "убирает отключённый поток %s вместе с DOM-привязкой и строкой присутствия",
  (kind) => {
    const stream = {} as MediaStream;
    mediaRef.current.view.remoteStreams = [
      {
        id: `remote-${kind}`,
        participantId: colleague.id,
        mediaPeerId: "peer-colleague",
        stream,
        kinds: kind === "audio" ? ["audio"] : ["video", "audio"],
        screen: kind === "screen",
      },
    ];
    const view = render(
      <RealtimePanel
        conferenceId="room"
        membership={self}
        live={live([present(self), present(colleague)])}
        participants={[self, colleague]}
      />,
    );
    const tile = screen.getByTestId("remote-media");
    const element = tile.querySelector("video, audio") as HTMLMediaElement;
    expect(element.srcObject).toBe(stream);
    if (kind === "screen")
      expect(
        view.container.querySelector(".media-grid-sharing"),
      ).not.toBeNull();
    view.rerender(
      <RealtimePanel
        conferenceId="room"
        membership={self}
        live={live([
          present(self),
          present(colleague, {
            online: false,
            connections: 0,
            connectionIds: [],
          }),
        ])}
        participants={[self, colleague]}
      />,
    );
    expect(screen.queryByTestId("remote-media")).toBeNull();
    expect(screen.queryByTestId("presence-user-colleague")).toBeNull();
    expect(screen.queryByText("Борис Волков")).toBeNull();
    expect(view.container.querySelector(".media-grid-sharing")).toBeNull();
    expect(element.srcObject).toBeNull();
    expect(mediaRef.current.stop).not.toHaveBeenCalled();
  },
);

it.each([null, []] as const)(
  "не добавляет себя, собственный экран и устаревшие удалённые медиа без подтверждённого присутствия (%s)",
  (snapshot) => {
    mediaRef.current.view.localStream = {} as MediaStream;
    mediaRef.current.view.localScreen = {} as MediaStream;
    mediaRef.current.view.remoteStreams = [
      {
        id: "remote-screen",
        participantId: colleague.id,
        mediaPeerId: "peer-colleague",
        stream: {} as MediaStream,
        kinds: ["video"],
        screen: true,
      },
    ];
    const view = render(
      <RealtimePanel
        conferenceId="room"
        membership={self}
        live={live(snapshot)}
        participants={[self, colleague]}
      />,
    );
    expect(screen.queryByTestId("local-media")).toBeNull();
    expect(screen.queryByTestId("remote-media")).toBeNull();
    expect(screen.queryByTestId("participant-placeholder")).toBeNull();
    expect(view.container.querySelector(".media-grid-sharing")).toBeNull();
    expect(screen.getByTestId("media-empty")).toBeInTheDocument();
  },
);

it("возвращает подключившегося заново участника и экран только после нового серверного снимка", () => {
  mediaRef.current.view.remoteStreams = [
    {
      id: "remote-screen",
      participantId: colleague.id,
      mediaPeerId: "peer-colleague",
      stream: {} as MediaStream,
      kinds: ["video"],
      screen: true,
    },
  ];
  const view = render(
    <RealtimePanel
      conferenceId="room"
      membership={self}
      live={live(null)}
      participants={[self, colleague]}
    />,
  );
  expect(screen.queryByTestId("remote-media")).toBeNull();
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={self}
      live={live([present(self), present(colleague)])}
      participants={[self]}
    />,
  );
  expect(screen.getByTestId("remote-media")).toHaveClass("media-tile-screen");
  expect(view.container.querySelector(".media-grid-sharing")).not.toBeNull();
  expect(screen.getByTestId("presence-user-colleague")).toBeInTheDocument();
});

it("не убирает медиа и не перепривязывает поток при отключении одной из двух вкладок", () => {
  const stream = {} as MediaStream;
  mediaRef.current.view.remoteStreams = [
    {
      id: "remote-camera",
      participantId: colleague.id,
      mediaPeerId: "peer-colleague",
      stream,
      kinds: ["video", "audio"],
      screen: false,
    },
  ];
  const view = render(
    <RealtimePanel
      conferenceId="room"
      membership={self}
      live={live([
        present(self),
        present(colleague, { connections: 2, connectionIds: ["one", "two"] }),
      ])}
      participants={[self, colleague]}
    />,
  );
  const element = screen.getByTestId("remote-media").querySelector("video")!;
  const plays = vi.mocked(HTMLMediaElement.prototype.play).mock.calls.length;
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={self}
      live={live([
        present(self),
        present(colleague, { connections: 1, connectionIds: ["two"] }),
      ])}
      participants={[self, colleague]}
    />,
  );
  expect(screen.getAllByTestId("remote-media")).toHaveLength(1);
  expect(screen.getByTestId("remote-media").querySelector("video")).toBe(
    element,
  );
  expect(element.srcObject).toBe(stream);
  expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(plays);
});
