import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ConferenceRecording, Participant, RealtimeEvent } from "../types";
import { RecordingNotice, type RecordingNoticeProps } from "./RecordingNotice";

const sound = vi.hoisted(() => ({ play: vi.fn(() => vi.fn()) }));
vi.mock("../recordingAnnouncement", () => ({
  playRecordingAnnouncement: sound.play,
}));

const stamp = "2026-10-04T12:00:00Z";
const self = {
  id: "self-member",
  userId: "self",
  conferenceId: "room",
  displayName: "Анна",
  role: "participant",
  status: "joined",
  admissionState: "admitted",
} as Participant;
const owner = {
  ...self,
  id: "owner-member",
  userId: "owner",
  displayName: "Сергей",
  role: "owner",
} as Participant;
const started = {
  recordingId: "record-a",
  conferenceId: "room",
  status: "recording",
  requestedBy: "owner",
  mode: "composite",
};

beforeEach(() => {
  sound.play.mockClear();
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.reject(new Error("Сетевые запросы запрещены"))),
  );
  vi.stubGlobal(
    "WebSocket",
    vi.fn(() => {
      throw new Error("Новые соединения запрещены");
    }),
  );
});
afterEach(() => {
  cleanup();
  expect(globalThis.fetch).not.toHaveBeenCalled();
  expect(globalThis.WebSocket).not.toHaveBeenCalled();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

/**
 * Создаёт серверное событие без настоящего транспорта.
 * @args changes — изменяемые поля конверта, включая недоверенную нагрузку.
 * @return Событие текущей комнаты с управляемой нагрузкой.
 */
function event(changes: Partial<RealtimeEvent> = {}): RealtimeEvent {
  return {
    version: 1,
    id: "delivery-1",
    type: "recording.started",
    conferenceId: "room",
    timestamp: stamp,
    data: started,
    ...changes,
  };
}

/**
 * Создаёт подтверждённую карточку для имитации успешного опроса.
 * @args status — серверное состояние; uuid — идентификатор записи; conferenceId — область карточки.
 * @return Карточка без файлов и приватных ссылок.
 */
function row(
  status: ConferenceRecording["status"] = "recording",
  uuid = "record-a",
  conferenceId = "room",
): ConferenceRecording {
  return {
    uuid,
    conferenceId,
    status,
    mode: "composite",
    createdAt: stamp,
    files: [],
  };
}

/**
 * Монтирует плашку с управляемыми свойствами и существующей тестовой подпиской.
 * @args initial — отличия от допущенного участника текущей комнаты.
 * @return Управление событиями, свойствами, подписками и временем жизни компонента.
 */
function notice(initial: Partial<RecordingNoticeProps> = {}) {
  const listeners = new Set<(incoming: RealtimeEvent) => void>();
  const unsubscribe = vi.fn();
  const subscribe = vi.fn((listener: (incoming: RealtimeEvent) => void) => {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
      unsubscribe();
    };
  });
  let props: RecordingNoticeProps = {
    conferenceId: "room",
    ownerId: "owner",
    userId: "self",
    participants: [self, owner],
    subscribe,
    ...initial,
  };
  const view = render(<RecordingNotice {...props} />);
  return {
    view,
    subscribe,
    unsubscribe,
    deliver: (incoming: RealtimeEvent) =>
      act(() => {
        listeners.forEach((listener) => listener(incoming));
      }),
    update: (changes: Partial<RecordingNoticeProps>) => {
      props = { ...props, ...changes };
      view.rerender(<RecordingNotice {...props} />);
    },
    reconnect: () => {
      const next = vi.fn((listener: (incoming: RealtimeEvent) => void) => {
        listeners.add(listener);
        return () => {
          listeners.delete(listener);
          unsubscribe();
        };
      });
      props = { ...props, subscribe: next };
      view.rerender(<RecordingNotice {...props} />);
      return next;
    },
  };
}

it("объявляет участнику фактическое начало записи с именем из разрешённого состава", () => {
  const subject = notice();
  subject.deliver(event());
  const status = screen.getByRole("status");
  expect(status).toHaveTextContent(
    "Началась запись встречи. Инициатор: Сергей.",
  );
  expect(status).toHaveAttribute("aria-live", "polite");
  expect(status).toHaveAttribute("aria-atomic", "true");
  expect(status).toHaveAttribute("data-recording-id", "record-a");
  expect(
    screen.getByRole("button", { name: "Скрыть уведомление о записи" }),
  ).toBeEnabled();
  expect(
    screen.queryByRole("button", { name: /Остановить|Начать/ }),
  ).not.toBeInTheDocument();
});

it("не объявляет инициатору собственное начало ни по событию, ни по опросу", () => {
  const subject = notice({ userId: "owner" });
  subject.deliver(event());
  subject.update({ recordings: [row()] });
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

it("подавляет собственное событие по requestedBy, даже если инициатор не совпал с ownerId", () => {
  const subject = notice();
  subject.deliver(event({ data: { ...started, requestedBy: "self" } }));
  subject.update({ recordings: [row()] });
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

it("использует организатора как безопасный запасной текст без раскрытия имени другой комнаты", () => {
  const subject = notice({
    participants: [
      self,
      { ...owner, conferenceId: "other-room", displayName: "Чужое имя" },
    ],
  });
  subject.deliver(event());
  expect(screen.getByRole("status")).toHaveTextContent(
    "Участник начал запись встречи.",
  );
  expect(screen.queryByText(/Чужое имя/)).not.toBeInTheDocument();
});

it.each(["recording", "degraded"] as const)(
  "поздний участник узнаёт о состоянии %s из подтверждённого опроса",
  (status) => {
    const subject = notice({ recordings: [row(status)] });
    expect(screen.getByRole("status")).toHaveTextContent(
      "Во встрече идёт запись.",
    );
    subject.deliver(event({ id: "late-event", data: { ...started, status } }));
    expect(screen.getByRole("status")).toHaveTextContent(
      "Во встрече идёт запись.",
    );
    expect(screen.queryByText(/Инициатор/)).not.toBeInTheDocument();
  },
);

it("не выдаёт starting за фактическое начало и объединяет повторы по UUID, а не envelope.id", () => {
  const subject = notice({ recordings: [row("starting")] });
  subject.deliver(
    event({
      type: "recording.starting",
      data: { ...started, status: "starting" },
    }),
  );
  subject.deliver(
    event({ type: "recording.starting", id: "starting-retry", data: started }),
  );
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  subject.deliver(event());
  subject.deliver(event({ id: "started-outbox-retry" }));
  subject.update({ recordings: [row()] });
  expect(screen.getAllByRole("status")).toHaveLength(1);
  expect(screen.getByRole("status")).toHaveTextContent("Инициатор: Сергей");
});

it("не закрывает фактическое объявление старым пустым снимком незавершённого initial GET", () => {
  const subject = notice();
  subject.deliver(event());
  subject.update({ recordings: [] });
  expect(screen.getByRole("status")).toHaveTextContent("Инициатор: Сергей");
  subject.update({ recordings: [row()] });
  expect(screen.getAllByRole("status")).toHaveLength(1);
});

it("ручное закрытие сохраняется при повторе события, polling и переподключении", () => {
  const subject = notice();
  subject.deliver(event());
  fireEvent.click(
    screen.getByRole("button", { name: "Скрыть уведомление о записи" }),
  );
  subject.deliver(event({ id: "event-retry" }));
  subject.update({ recordings: [row()] });
  subject.reconnect();
  subject.deliver(event({ id: "after-reconnect" }));
  subject.update({ recordings: [row()] });
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  expect(subject.unsubscribe).toHaveBeenCalledOnce();
  subject.deliver(event({ data: { ...started, recordingId: "record-b" } }));
  expect(screen.getByRole("status")).toHaveAttribute(
    "data-recording-id",
    "record-b",
  );
});

it("не скрывает уведомление по таймеру", () => {
  vi.useFakeTimers();
  const subject = notice();
  subject.deliver(event());
  act(() => vi.advanceTimersByTime(120_000));
  expect(screen.getByRole("status")).toBeInTheDocument();
  expect(vi.getTimerCount()).toBe(0);
});

it.each(["stopping", "processing", "ready", "failed", "cancelled"] as const)(
  "состояние %s из query убирает плашку и блокирует запоздалый started",
  (status) => {
    const subject = notice();
    subject.deliver(event());
    subject.update({ recordings: [row(status)] });
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    subject.deliver(event({ id: "stale-started" }));
    subject.update({ recordings: [row()] });
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  },
);

it.each([
  ["recording.stopping", "stopping"],
  ["recording.processing", "processing"],
  ["recording.ready", "ready"],
  ["recording.failed", "failed"],
  ["recording.cancelled", "cancelled"],
  ["recording.ended", "ended"],
  ["recording.stopped", "ready"],
])(
  "событие %s закрывает соответствующий UUID и запрещает его повторное начало",
  (type, status) => {
    const subject = notice();
    subject.deliver(event());
    subject.deliver(
      event({
        type,
        data: { recordingId: "record-a", conferenceId: "room", status },
      }),
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    subject.deliver(event({ id: "delayed-start" }));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  },
);

it("не объявляет уже завершённый UUID даже при первом запоздалом событии после подключения", () => {
  const subject = notice({ recordings: [row("ready")] });
  subject.deliver(event());
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

it("закрытие другой записи не убирает текущую плашку", () => {
  const subject = notice();
  subject.deliver(event());
  subject.deliver(
    event({
      type: "recording.failed",
      data: { recordingId: "record-b", conferenceId: "room", status: "failed" },
    }),
  );
  expect(screen.getByRole("status")).toHaveAttribute(
    "data-recording-id",
    "record-a",
  );
});

it("отбрасывает чужую комнату в конверте, нагрузке и query, не закрывая текущую запись", () => {
  const subject = notice({
    recordings: [row("recording", "foreign-record", "other-room")],
  });
  subject.deliver(event({ conferenceId: "other-room" }));
  subject.deliver(event({ data: { ...started, conferenceId: "other-room" } }));
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  subject.deliver(event());
  subject.deliver(
    event({
      type: "recording.stopping",
      conferenceId: "other-room",
      data: { ...started, status: "stopping" },
    }),
  );
  expect(screen.getByRole("status")).toHaveAttribute(
    "data-recording-id",
    "record-a",
  );
});

it.each([
  null,
  [],
  "not-an-object",
  {},
  { ...started, recordingId: "" },
  { ...started, recordingId: "bad id" },
  { ...started, recordingId: 123 },
  { ...started, recordingId: "x".repeat(129) },
  { ...started, conferenceId: undefined },
  { ...started, requestedBy: undefined },
  { ...started, requestedBy: {} },
  { ...started, mode: "unknown-mode" },
  { ...started, status: "starting" },
  { ...started, status: "unknown" },
])("игнорирует некорректную нагрузку события %#", (data) => {
  const subject = notice();
  subject.deliver(event({ data }));
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

it.each([
  { ...self, status: "left" },
  { ...self, status: "waiting", admissionState: "waiting" },
  { ...self, status: "joined", admissionState: "kicked" },
  { ...self, conferenceId: "other-room" },
])(
  "не уведомляет пользователя без текущего joined/admitted членства %#",
  (member) => {
    const subject = notice({
      participants: [member as Participant, owner],
      recordings: [row()],
    });
    subject.deliver(event());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  },
);

it("при поздней загрузке своего разрешённого членства использует query fallback", () => {
  const subject = notice({ participants: [owner], recordings: [row()] });
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  subject.update({ participants: [self, owner] });
  expect(screen.getByRole("status")).toHaveTextContent(
    "Во встрече идёт запись.",
  );
  subject.update({ participants: [{ ...self, status: "left" }, owner] });
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

it("сбрасывает память при смене комнаты и очищает старую подписку", () => {
  const subject = notice();
  const stale = subject.subscribe.mock.calls[0][0];
  subject.deliver(event());
  fireEvent.click(
    screen.getByRole("button", { name: "Скрыть уведомление о записи" }),
  );
  subject.update({
    conferenceId: "other-room",
    participants: [self, owner].map((person) => ({
      ...person,
      conferenceId: "other-room",
    })),
  });
  expect(subject.unsubscribe).toHaveBeenCalledOnce();
  act(() =>
    stale(event({ data: { ...started, recordingId: "old-room-record" } })),
  );
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  subject.deliver(
    event({
      conferenceId: "other-room",
      data: { ...started, conferenceId: "other-room" },
    }),
  );
  expect(screen.getByRole("status")).toHaveAttribute(
    "data-recording-id",
    "record-a",
  );
  subject.view.unmount();
  expect(subject.unsubscribe).toHaveBeenCalledTimes(2);
});

it("сохраняет UUID-дедуп последних 64 записей при ограниченной памяти", () => {
  const subject = notice();
  for (let index = 0; index < 80; index++)
    subject.deliver(
      event({ data: { ...started, recordingId: `record-${index}` } }),
    );
  fireEvent.click(
    screen.getByRole("button", { name: "Скрыть уведомление о записи" }),
  );
  for (let index = 16; index < 80; index++)
    subject.deliver(
      event({
        id: `retry-${index}`,
        data: { ...started, recordingId: `record-${index}` },
      }),
    );
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

it("большая история terminal-записей не вытесняет дедуп закрытой активной плашки", () => {
  const items = [
    row(),
    ...Array.from({ length: 80 }, (_, index) =>
      row("ready", `history-${index}`),
    ),
  ];
  const subject = notice({ recordings: items });
  expect(screen.getByRole("status")).toHaveTextContent(
    "Во встрече идёт запись.",
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Скрыть уведомление о записи" }),
  );
  subject.update({ recordings: items.map((item) => ({ ...item })) });
  subject.deliver(event({ id: "duplicate-after-large-poll" }));
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
});

it.each(["self", "owner"])(
  "announces audio once to %s including the initiator",
  (userId) => {
    const subject = notice({ userId });
    subject.deliver(event());
    subject.deliver(event({ id: "duplicate" }));
    subject.update({ recordings: [row()] });
    expect(sound.play).toHaveBeenCalledOnce();
  },
);
it("does not sound for starting, foreign events, or a stale completed recording", () => {
  const subject = notice({ recordings: [row("ready")] });
  subject.deliver(event());
  subject.deliver(event({ conferenceId: "other" }));
  subject.deliver(
    event({
      type: "recording.starting",
      data: { ...started, status: "starting" },
    }),
  );
  expect(sound.play).not.toHaveBeenCalled();
});
it("cancels pending audio on stop and on leaving the room", () => {
  const subject = notice();
  subject.deliver(event());
  const cancel = sound.play.mock.results[0].value;
  subject.update({ recordings: [row("stopping")] });
  expect(cancel).toHaveBeenCalledOnce();
  subject.deliver(event({ data: { ...started, recordingId: "record-b" } }));
  const nextCancel = sound.play.mock.results[1].value;
  subject.view.unmount();
  expect(nextCancel).toHaveBeenCalledOnce();
});
