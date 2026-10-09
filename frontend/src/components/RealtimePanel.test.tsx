import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { emptyMediaView } from "../media";
import type { useMedia } from "../useMedia";
import type { useRealtime } from "../realtime";
import type { Participant, PresenceParticipant } from "../types";
import { MediaTile, RealtimePanel } from "./RealtimePanel";
import { AccountSettingsContext } from "./AccountSettingsContext";

const mediaRef = vi.hoisted(() => ({
  current: null as unknown as ReturnType<typeof useMedia>,
  speaking: new Map<string, number>(),
}));
vi.mock("../useMedia", () => ({ useMedia: () => mediaRef.current }));
vi.mock("../useSpeakingParticipants", () => ({
  useSpeakingParticipants: () => mediaRef.speaking,
}));
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
  mediaRef.speaking = new Map();
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

it("подсвечивает видеоплитку и микрофон говорящего, но не заглушённого участника или экран", async () => {
  const play = vi
    .spyOn(HTMLMediaElement.prototype, "play")
    .mockResolvedValue(undefined);
  mediaRef.speaking.set("colleague", 0.6);
  const colleague = {
    id: "colleague",
    displayName: "Борис",
    status: "joined",
    role: "participant",
    microphoneEnabled: true,
  } as Participant;
  mediaRef.current.view.remoteStreams = [
    {
      id: "voice",
      mediaPeerId: "remote",
      participantId: "colleague",
      stream: {} as MediaStream,
      kinds: ["audio", "video"],
      screen: false,
    },
    {
      id: "screen",
      mediaPeerId: "remote",
      participantId: "colleague",
      stream: {} as MediaStream,
      kinds: ["video"],
      screen: true,
    },
  ];
  const view = panel(undefined, [colleague]);
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={view.membership}
      live={view.live}
      participants={[colleague]}
    />,
  );
  const [personTile, screenTile] = screen.getAllByTestId("remote-media");
  expect(personTile).toHaveClass("media-tile-speaking");
  expect(screen.getByLabelText("Говорит: Борис")).toHaveClass(
    "media-microphone-speaking",
  );
  expect(screenTile).not.toHaveClass("media-tile-speaking");
  view.live.state!.participants = onlinePresence([
    view.membership,
    { ...colleague, microphoneEnabled: false },
  ]);
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={view.membership}
      live={view.live}
      participants={[{ ...colleague, microphoneEnabled: false }]}
    />,
  );
  expect(personTile).not.toHaveClass("media-tile-speaking");
  expect(screen.queryByLabelText("Говорит: Борис")).toBeNull();
  expect(
    personTile.querySelector('[aria-label="Микрофон выключен"]'),
  ).not.toHaveClass("media-microphone-speaking");
  view.unmount();
  play.mockRestore();
});

it("подсвечивает аудиоплитку с выключенной камерой и свой значок микрофона, гася их при потере связи", () => {
  const play = vi
    .spyOn(HTMLMediaElement.prototype, "play")
    .mockResolvedValue(undefined);
  mediaRef.speaking.set("self", 0.4);
  mediaRef.current.view.localStream = {} as MediaStream;
  mediaRef.current.view.microphoneEnabled = true;
  mediaRef.current.view.cameraEnabled = false;
  const view = panel({ displayName: "Алиса" });
  const tile = screen.getByTestId("local-media");
  expect(tile.querySelector("audio")).toBeInTheDocument();
  expect(tile).toHaveClass("media-tile-speaking");
  expect(screen.getByLabelText("Говорит: Алиса")).toHaveClass(
    "media-microphone-speaking",
  );
  expect(
    screen
      .getByRole("button", { name: "Выключить микрофон" })
      .querySelector(".media-control-microphone"),
  ).toHaveClass("media-microphone-speaking");
  mediaRef.current.view.connectionState = "disconnected";
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={view.membership}
      live={view.live}
    />,
  );
  expect(tile).not.toHaveClass("media-tile-speaking");
  view.unmount();
  play.mockRestore();
});

it("меняет уровень рамки и микрофона без переподключения видеопотока", () => {
  const play = vi
    .spyOn(HTMLMediaElement.prototype, "play")
    .mockResolvedValue(undefined);
  const stream = {} as MediaStream;
  const view = render(
    <MediaTile
      name="Борис"
      stream={stream}
      microphoneEnabled
      audioLevel={0.2}
    />,
  );
  const tile = screen.getByTestId("remote-media");
  const video = tile.querySelector("video")!;
  expect(tile).toHaveAttribute("data-audio-level", "0.2");
  expect(tile.style.getPropertyValue("--audio-level")).toBe("0.2");
  view.rerender(
    <MediaTile
      name="Борис"
      stream={stream}
      microphoneEnabled
      audioLevel={0.8}
    />,
  );
  expect(tile).toHaveAttribute("data-audio-level", "0.8");
  expect(tile.style.getPropertyValue("--audio-level")).toBe("0.8");
  expect(tile.querySelector("video")).toBe(video);
  expect(video.srcObject).toBe(stream);
  expect(play).toHaveBeenCalledTimes(1);
  view.rerender(
    <MediaTile name="Борис" stream={stream} microphoneEnabled audioLevel={0} />,
  );
  expect(tile).toHaveAttribute("data-speaking", "false");
  expect(tile).toHaveAttribute("data-audio-level", "0");
  expect(screen.getByLabelText("Микрофон включён")).not.toHaveClass(
    "media-microphone-speaking",
  );
  view.unmount();
  play.mockRestore();
});

it("ограничивает уровень и гасит некорректные замеры, выключенный микрофон, экран и потерю связи", () => {
  const view = render(
    <MediaTile name="Борис" microphoneEnabled audioLevel={2} />,
  );
  const tile = screen.getByTestId("participant-placeholder");
  expect(tile).toHaveAttribute("data-audio-level", "1");
  for (const props of [
    { audioLevel: -1 },
    { audioLevel: NaN },
    { audioLevel: Infinity },
    { audioLevel: 0.7, microphoneEnabled: false },
    { audioLevel: 0.7, screen: true },
    { audioLevel: 0.7, reconnecting: true },
  ]) {
    view.rerender(<MediaTile name="Борис" microphoneEnabled {...props} />);
    expect(tile).toHaveAttribute("data-audio-level", "0");
    expect(tile).toHaveAttribute("data-speaking", "false");
  }
});

afterEach(() => {
  if (onlineDescriptor)
    Object.defineProperty(navigator, "onLine", onlineDescriptor);
  if (clipboardDescriptor)
    Object.defineProperty(navigator, "clipboard", clipboardDescriptor);
  else Reflect.deleteProperty(navigator, "clipboard");
  vi.clearAllMocks();
});

function onlinePresence(participants: Participant[]): PresenceParticipant[] {
  return participants.map((person) => ({
    ...person,
    online: true,
    connections: 1,
    connectionIds: [`connection-${person.id}`],
  }));
}

function panel(member?: Partial<Participant>, others: Participant[] = []) {
  const membership = {
    id: "self",
    role: "participant",
    status: "joined",
    ...member,
  } as Participant;
  const live = {
    state: {
      connectionId: "connection",
      participants: onlinePresence([membership, ...others]),
    },
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

it.each([true, false])(
  "кнопка настроек передаёт свой элемент общему диалогу без изменения медиасвязи (медиа запущено: %s)",
  (running) => {
    mediaRef.current.running = running;
    const openSettings = vi.fn();
    const view = panel();
    view.rerender(
      <AccountSettingsContext.Provider value={openSettings}>
        <RealtimePanel
          conferenceId="room"
          membership={view.membership}
          live={view.live}
        />
      </AccountSettingsContext.Provider>,
    );
    const settings = screen.getByRole("button", {
      name: /^Настройки$/,
    });
    expect(settings).toBeEnabled();
    expect(settings).toHaveAttribute("aria-haspopup", "dialog");
    expect(settings.querySelector(".lucide-settings")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /^Устройства$/ }),
    ).not.toBeInTheDocument();
    fireEvent.click(settings);
    expect(openSettings).toHaveBeenCalledExactlyOnceWith(settings);
    for (const command of [
      mediaRef.current.start,
      mediaRef.current.stop,
      mediaRef.current.microphone,
      mediaRef.current.camera,
      mediaRef.current.startScreen,
      mediaRef.current.stopScreen,
      view.live.reconnect,
    ])
      expect(command).not.toHaveBeenCalled();
    view.unmount();
  },
);

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

it("заменяет меню одной кнопкой переподключения в панели и останавливает медиа до обновления связи", () => {
  const view = panel();
  const reconnect = screen.getByRole("button", { name: "Переподключиться" });
  expect(reconnect.closest(".conference-control-bar")).not.toBeNull();
  expect(reconnect).toHaveClass("room-reconnect-control");
  expect(reconnect).toHaveAttribute("title", "Переподключиться");
  expect(reconnect).not.toHaveAttribute("aria-haspopup");
  expect(reconnect.querySelector(".lucide-refresh-cw")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Ещё" })).not.toBeInTheDocument();
  expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  fireEvent.click(reconnect);
  expect(mediaRef.current.stop).toHaveBeenCalledOnce();
  expect(view.live.reconnect).toHaveBeenCalledOnce();
  expect(
    vi.mocked(mediaRef.current.stop).mock.invocationCallOrder[0],
  ).toBeLessThan(vi.mocked(view.live.reconnect).mock.invocationCallOrder[0]);
  expect(mediaRef.current.start).not.toHaveBeenCalled();
  view.unmount();
});

it("renders one reconnect action in the header slot and stops media before reconnecting", () => {
  const host = document.createElement("div");
  document.body.append(host);
  const view = panel();
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={view.membership}
      live={view.live}
      reconnectTarget={host}
    />,
  );
  const button = screen.getByRole("button", { name: "Переподключиться" });
  expect(host).toContainElement(button);
  expect(
    screen.getAllByRole("button", { name: "Переподключиться" }),
  ).toHaveLength(1);
  expect(button.closest(".conference-control-bar")).toBeNull();
  expect(button).toHaveClass("room-header-reconnect");
  expect(button.querySelector(".sr-only")).toHaveTextContent(
    "Переподключиться",
  );
  expect(screen.queryByText("Состояние медиасвязи")).toBeNull();
  expect(mediaRef.current.diagnostics).not.toHaveBeenCalled();
  fireEvent.click(button);
  expect(mediaRef.current.stop).toHaveBeenCalledOnce();
  expect(view.live.reconnect).toHaveBeenCalledOnce();
  expect(
    vi.mocked(mediaRef.current.stop).mock.invocationCallOrder[0],
  ).toBeLessThan(vi.mocked(view.live.reconnect).mock.invocationCallOrder[0]);
  view.unmount();
  host.remove();
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
  view.live.state!.participants = onlinePresence([view.membership, colleague]);
  view.rerender(
    <RealtimePanel
      conferenceId="room"
      membership={view.membership}
      live={view.live}
      participants={[view.membership, colleague]}
    />,
  );
  expect(
    within(screen.getAllByTestId("participant-placeholder")[1]).getByText(
      "Борис Волков",
    ),
  ).toBeInTheDocument();
  expect(screen.getByText("БВ")).toBeInTheDocument();
  expect(screen.queryByLabelText("Рука поднята")).not.toBeInTheDocument();
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
  const view = panel(undefined, [colleague]);
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

it.each(["local", "remote"])(
  "экран %s скрывает все плитки, не перепривязывая медиа, и возвращает сетку после остановки",
  (source) => {
    const play = vi
      .spyOn(HTMLMediaElement.prototype, "play")
      .mockResolvedValue(undefined);
    const localStream = {} as MediaStream;
    const remoteStream = {} as MediaStream;
    const screenStream = {} as MediaStream;
    const colleague = {
      id: "colleague",
      displayName: "Борис",
      status: "joined",
      microphoneEnabled: true,
    } as Participant;
    const waiting = {
      id: "waiting",
      displayName: "Вера",
      status: "joined",
    } as Participant;
    const remote = {
      id: "remote",
      mediaPeerId: "peer-boris",
      participantId: colleague.id,
      stream: remoteStream,
      kinds: ["video", "audio"],
      screen: false,
    };
    mediaRef.current.view.localStream = localStream;
    mediaRef.current.view.remoteStreams = [remote];
    const view = panel({ displayName: "Алиса" }, [colleague, waiting]);
    const rerender = () =>
      view.rerender(
        <RealtimePanel
          conferenceId="room"
          membership={view.membership}
          live={view.live}
          participants={[view.membership, colleague, waiting]}
        />,
      );
    rerender();
    const localTile = screen.getByTestId("local-media");
    const remoteTile = screen.getByTestId("remote-media");
    const audio = localTile.querySelector("audio")!;
    const video = remoteTile.querySelector("video")!;
    if (source === "local") {
      mediaRef.current.view.localScreen = screenStream;
    } else {
      mediaRef.current.view.remoteStreams = [
        remote,
        {
          ...remote,
          id: "screen",
          stream: screenStream,
          kinds: ["video"],
          screen: true,
        },
      ];
    }
    rerender();
    const sharing = view.container.querySelector(".media-grid-sharing")!;
    expect(sharing).toBeInTheDocument();
    expect(sharing.querySelector(".media-tile-screen")).not.toHaveAttribute(
      "hidden",
    );
    for (const tile of sharing.querySelectorAll(
      ".media-tile:not(.media-tile-screen)",
    ))
      expect(tile).toHaveAttribute("hidden");
    expect(localTile.querySelector("audio")).toBe(audio);
    expect(remoteTile.querySelector("video")).toBe(video);
    expect(audio.srcObject).toBe(localStream);
    expect(video.srcObject).toBe(remoteStream);
    const playing = play.mock.calls.length;
    mediaRef.current.view.localScreen = null;
    mediaRef.current.view.remoteStreams = [remote];
    rerender();
    expect(view.container.querySelector(".media-grid-sharing")).toBeNull();
    expect(localTile).not.toHaveAttribute("hidden");
    expect(remoteTile).not.toHaveAttribute("hidden");
    expect(screen.getByTestId("participant-placeholder")).not.toHaveAttribute(
      "hidden",
    );
    expect(play).toHaveBeenCalledTimes(playing);
    expect(mediaRef.current.stop).not.toHaveBeenCalled();
    expect(video.srcObject).toBe(remoteStream);
    view.unmount();
    play.mockRestore();
  },
);

it("оставляет доступной кнопку разрешения звука для скрытого участника", async () => {
  const play = vi
    .spyOn(HTMLMediaElement.prototype, "play")
    .mockRejectedValueOnce(new Error("autoplay blocked"))
    .mockResolvedValue(undefined);
  const stream = {} as MediaStream;
  const view = render(
    <MediaTile name="Борис" stream={stream} video={false} covered />,
  );
  const audio = view.container.querySelector("audio")!;
  const button = await screen.findByRole("button", {
    name: "Включить звук: Борис",
  });
  expect(button).toBeVisible();
  expect(screen.getByTestId("remote-media")).toHaveAttribute("hidden");
  fireEvent.click(button);
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Включить звук: Борис" }),
    ).toBeNull(),
  );
  expect(audio.srcObject).toBe(stream);
  expect(play).toHaveBeenCalledTimes(2);
  view.unmount();
  play.mockRestore();
});
