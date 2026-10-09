import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
import { ConferencePage } from "./ConferencePage";
import { api } from "../api";
import type { Conference, ConferenceHistory } from "../types";

const fixture = vi.hoisted(() => ({
  membership: {
    id: "member",
    userId: "self",
    displayName: "Алиса",
    role: "participant",
    status: "joined",
    admissionState: "admitted",
  },
  captions: true,
  analytics: true,
  capabilitiesError: false,
  conferenceStatus: "active",
  waitingRoomEnabled: true as boolean | undefined,
  ownerId: "other",
  guest: false,
  realtimeOwners: 0,
  realtimeOwnersCreated: 0,
  realtimeMaxOwners: 0,
  realtimeEnabled: [] as boolean[],
  mediaMounts: 0,
  mediaUnmounts: 0,
  reconnectTarget: null as HTMLElement | null | undefined,
}));

beforeEach(() => {
  fixture.membership.status = "joined";
  fixture.membership.admissionState = "admitted";
  fixture.captions = true;
  fixture.analytics = true;
  fixture.capabilitiesError = false;
  fixture.membership.role = "participant";
  fixture.conferenceStatus = "active";
  fixture.waitingRoomEnabled = true;
  fixture.ownerId = "other";
  fixture.guest = false;
  fixture.realtimeOwners = 0;
  fixture.realtimeOwnersCreated = 0;
  fixture.realtimeMaxOwners = 0;
  fixture.realtimeEnabled = [];
  fixture.mediaMounts = 0;
  fixture.mediaUnmounts = 0;
  fixture.reconnectTarget = null;
});

vi.mock("../auth", () => ({
  useAuth: () => ({
    user: {
      id: "self",
      ...(fixture.guest ? { guestConferenceId: "room" } : {}),
    },
  }),
}));
vi.mock("../queries", () => ({
  useConference: () => ({
    isPending: false,
    isError: false,
    data: {
      item: {
        id: "room",
        title: "Командная встреча",
        status: fixture.conferenceStatus,
        ownerId: fixture.ownerId,
        createdAt: "2026-10-01T10:00:00Z",
        inviteCode: "a".repeat(32),
        waitingRoomEnabled: fixture.waitingRoomEnabled,
      },
    },
  }),
  useMembership: () => ({
    data: fixture.membership,
    isPending: false,
    isError: false,
  }),
  useParticipants: () => ({
    data: { pages: [{ items: [fixture.membership] }] },
    isPending: false,
    hasNextPage: false,
  }),
}));
vi.mock("../realtime", async () => {
  const { useEffect } = await import("react");
  return {
    useRealtime: (_id: string, enabled: boolean) => {
      useEffect(() => {
        fixture.realtimeOwners++;
        fixture.realtimeOwnersCreated++;
        fixture.realtimeMaxOwners = Math.max(
          fixture.realtimeMaxOwners,
          fixture.realtimeOwners,
        );
        return () => {
          fixture.realtimeOwners--;
        };
      }, []);
      useEffect(() => {
        fixture.realtimeEnabled.push(enabled);
      }, [enabled]);
      return {
        state: { connectionId: "connection", participants: [] },
        status: "Подключено",
        subscribe: () => () => {},
      };
    },
  };
});
vi.mock("../useCapabilities", () => ({
  useCapabilities: () => ({
    isSuccess: !fixture.capabilitiesError,
    isError: fixture.capabilitiesError,
    data: {
      capabilities: {
        liveCaptions: fixture.captions,
        meetingAnalytics: fixture.analytics,
      },
    },
  }),
}));
vi.mock("../components/RealtimePanel", async () => {
  const { useEffect } = await import("react");
  return {
    RealtimePanel: ({
      controls,
      reconnectTarget,
    }: {
      controls?: import("react").ReactNode;
      reconnectTarget?: HTMLElement | null;
    }) => {
      fixture.reconnectTarget = reconnectTarget;
      useEffect(() => {
        fixture.mediaMounts++;
        return () => {
          fixture.mediaUnmounts++;
        };
      }, []);
      return <div data-testid="realtime-panel">Медиа{controls}</div>;
    },
  };
});
vi.mock("../components/ReactionsPanel", () => ({
  ReactionsPanel: () => <div data-testid="reactions-panel">Реакции</div>,
}));
vi.mock("../components/ChatPanel", () => ({
  ChatPanel: ({
    conferenceId,
    readOnly,
    focusMessageId,
  }: {
    conferenceId: string;
    readOnly?: boolean;
    focusMessageId?: string;
  }) => (
    <textarea
      id={`chat-text-${conferenceId}`}
      aria-label="Сообщение"
      readOnly={readOnly}
      data-focus-message={focusMessageId}
    />
  ),
}));
vi.mock("../components/CaptionsPanel", () => ({
  CaptionsPanel: () => <div data-testid="captions-panel" />,
}));
vi.mock("../components/AnalyticsPanel", () => ({
  AnalyticsPanel: () => <div data-testid="analytics-panel" />,
}));
vi.mock("../components/RecordingPanel", () => ({ RecordingPanel: () => null }));
vi.mock("../components/WaitingRoomPanel", () => ({
  WaitingRoomPanel: () => null,
}));

/** Монтирует страницу с изолированным кешем и сохраняет элемент для проверки смены состояния.
 * @args entry — адрес встречи и параметры фокуса чата.
 * @return Представление, исходный элемент и клиент запросов.
 */
function page(entry = "/conferences/room") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const createElement = () => (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route path="/conferences/:id" element={<ConferencePage />} />
          <Route path="/meetings/:id" element={<ConferencePage />} />
          <Route
            path="/conferences/:id/join"
            element={<p>Подготовка к встрече</p>}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
  return { ...render(createElement()), client, createElement };
}

it("puts media first and supports roving tab keys and safe H/C shortcuts", async () => {
  const view = page();
  const stage = screen.getByRole("region", { name: "Активная встреча" });
  expect(stage.querySelector("[data-testid='realtime-panel']")).not.toBeNull();
  expect(screen.queryByTestId("reactions-panel")).not.toBeInTheDocument();
  const chat = screen.getByRole("tab", { name: "Чат" });
  const participants = screen.getByRole("tab", { name: /Участники/ });
  const captions = screen.getByRole("tab", { name: "Субтитры" });
  expect(chat).toHaveAttribute("tabindex", "0");
  expect(participants).toHaveAttribute("tabindex", "-1");
  fireEvent.keyDown(chat, { key: "ArrowRight" });
  expect(participants).toHaveFocus();
  expect(participants).toHaveAttribute("aria-selected", "true");
  fireEvent.keyDown(participants, { key: "End" });
  expect(captions).toHaveFocus();
  fireEvent.keyDown(captions, { key: "Home" });
  expect(chat).toHaveFocus();
  fireEvent.keyDown(window, { key: "h" });
  expect(screen.queryByRole("button", { name: "Поднять руку" })).toBeNull();
  const composer = screen.getByRole("textbox", { name: "Сообщение" });
  fireEvent.keyDown(composer, { key: "h" });
  expect(screen.queryByRole("button", { name: "Поднять руку" })).toBeNull();
  fireEvent.keyDown(window, { key: "c" });
  await waitFor(() => expect(composer).toHaveFocus());
  view.client.clear();
});

it("предлагает организатору приглашать со страницы запланированной встречи", () => {
  fixture.ownerId = "self";
  fixture.membership.role = "owner";
  fixture.conferenceStatus = "scheduled";
  const view = page();
  fireEvent.click(
    screen.getByRole("button", { name: "Пригласить участников" }),
  );
  expect(
    screen.getByRole("dialog", { name: "Пригласить участников" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("textbox", { name: "Email участников" }),
  ).toBeInTheDocument();
  view.client.clear();
});

it("направляет кнопку переподключения в верхний ряд после приглашения", () => {
  const view = page();
  const target = fixture.reconnectTarget;
  expect(target).toBeInstanceOf(HTMLElement);
  expect(target).toHaveClass("room-reconnect-slot");
  const header = target?.closest(".room-header");
  const actions = header?.querySelector(".room-header-actions");
  expect(actions?.lastElementChild).toBe(target);
  expect(screen.getByRole("button", { name: "Пригласить" }).parentElement).toBe(
    actions,
  );
  expect(target?.closest(".room-footer")).toBeNull();
  expect(fixture.mediaMounts).toBe(1);
  view.client.clear();
});

it("сохраняет обычным участникам и гостям только ссылку без поиска пользователей", () => {
  const search = vi.spyOn(api, "invitationUsers");
  const account = page();
  fireEvent.click(screen.getByRole("button", { name: "Пригласить" }));
  expect(
    screen.getByRole("button", { name: "Копировать" }),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("textbox", { name: "Email участников" }),
  ).not.toBeInTheDocument();
  expect(search).not.toHaveBeenCalled();
  account.unmount();
  account.client.clear();
  fixture.guest = true;
  const guest = page();
  fireEvent.click(screen.getByRole("button", { name: "Пригласить" }));
  expect(screen.queryByRole("searchbox")).not.toBeInTheDocument();
  expect(search).not.toHaveBeenCalled();
  guest.client.clear();
});

it("допускает форму соорганизатору только во время активного участия", () => {
  fixture.membership.role = "co_host";
  const joined = page();
  fireEvent.click(screen.getByRole("button", { name: "Пригласить" }));
  expect(
    screen.getByRole("textbox", { name: "Email участников" }),
  ).toBeInTheDocument();
  joined.unmount();
  joined.client.clear();
  fixture.membership.status = "left";
  const left = page();
  expect(
    screen.queryByRole("button", { name: "Пригласить участников" }),
  ).not.toBeInTheDocument();
  left.client.clear();
});

it.each([true, false, undefined])(
  "показывает подпись доступа только при включённом зале ожидания: %s",
  (waitingRoomEnabled) => {
    fixture.waitingRoomEnabled = waitingRoomEnabled;
    const view = page();
    const label = screen.queryByText("Доступно по приглашению");
    if (waitingRoomEnabled === true) expect(label).toBeInTheDocument();
    else {
      expect(label).not.toBeInTheDocument();
      expect(view.container.querySelector(".room-footer-meta")).toBeNull();
    }
    view.client.clear();
  },
);

it("hides optional panels behind server flags and never mounts media while waiting", () => {
  fixture.captions = false;
  fixture.analytics = false;
  const view = page();
  expect(screen.queryByRole("tab", { name: "Субтитры" })).toBeNull();
  expect(screen.queryByTestId("analytics-panel")).toBeNull();
  expect(
    screen.queryByText("Субтитры отключены для этой установки."),
  ).not.toBeInTheDocument();
  view.unmount();
  fixture.membership.status = "waiting";
  fixture.membership.admissionState = "waiting";
  const waiting = page();
  expect(screen.queryByTestId("realtime-panel")).toBeNull();
  expect(screen.queryByTestId("reactions-panel")).toBeNull();
  waiting.client.clear();
  view.client.clear();
});

it("не использует устаревшие разрешения субтитров и аналитики после ошибки capabilities", () => {
  fixture.capabilitiesError = true;
  const active = page();
  expect(
    screen.queryByRole("tab", { name: "Субтитры" }),
  ).not.toBeInTheDocument();
  expect(screen.queryByTestId("captions-panel")).not.toBeInTheDocument();
  expect(screen.getByTestId("realtime-panel")).toBeInTheDocument();
  active.unmount();
  active.client.clear();
  fixture.membership.status = "left";
  const left = page();
  expect(screen.queryByTestId("analytics-panel")).not.toBeInTheDocument();
  expect(screen.queryByTestId("captions-panel")).not.toBeInTheDocument();
  left.client.clear();
});

it("не пересоздаёт медиасвязь и владельца realtime при переключении и закрытии панелей", () => {
  const view = page();
  const composer = screen.getByRole("textbox", { name: "Сообщение" });
  fireEvent.change(composer, { target: { value: "Несохранённое сообщение" } });
  fireEvent.click(screen.getByRole("tab", { name: /Участники/ }));
  fireEvent.click(screen.getByRole("tab", { name: "Субтитры" }));
  fireEvent.click(
    screen.getByRole("button", { name: "Закрыть панель встречи" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "Чат" }));
  expect(screen.getByRole("textbox", { name: "Сообщение" })).toBe(composer);
  expect(composer).toHaveValue("Несохранённое сообщение");
  expect(fixture.mediaMounts).toBe(1);
  expect(fixture.mediaUnmounts).toBe(0);
  expect(fixture.realtimeOwnersCreated).toBe(1);
  expect(fixture.realtimeMaxOwners).toBe(1);
  view.unmount();
  expect(fixture.mediaUnmounts).toBe(1);
  expect(fixture.realtimeOwners).toBe(0);
  view.client.clear();
});

it("при завершении переключается на историю и отключает realtime без второго владельца", async () => {
  const history: ConferenceHistory = {
    conference: { id: "room", status: "finished" } as Conference,
    owner: { id: "other", displayName: "Организатор" },
    durationSec: 120,
    participantCount: 1,
    participants: [],
    participantsTruncated: false,
    recordings: { total: 0, ready: 0, processing: 0, failed: 0 },
    chatAvailable: true,
    chatReadOnly: true,
  };
  const historyRequest = vi
    .spyOn(api, "history")
    .mockResolvedValue({ status: "success", item: history });
  const view = page("/conferences/room?chat=1&message=saved-message");
  expect(fixture.realtimeEnabled).toEqual([true]);
  fixture.conferenceStatus = "finished";
  view.rerender(view.createElement());
  expect(screen.queryByTestId("realtime-panel")).not.toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "Сообщение" })).toHaveAttribute(
    "readonly",
  );
  expect(screen.getByRole("textbox", { name: "Сообщение" })).toHaveAttribute(
    "data-focus-message",
    "saved-message",
  );
  await waitFor(() => expect(historyRequest).toHaveBeenCalled());
  expect(
    screen.getByRole("link", { name: "К материалам встречи" }),
  ).toHaveAttribute("href", "/history/room");
  expect(screen.getByText("Чат завершённой встречи")).toBeInTheDocument();
  expect(screen.getByText("Только чтение")).toBeInTheDocument();
  expect(screen.queryByTestId("analytics-panel")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Пригласить участников" }),
  ).not.toBeInTheDocument();
  expect(fixture.realtimeEnabled).toEqual([true, false]);
  expect(fixture.realtimeOwnersCreated).toBe(1);
  expect(fixture.realtimeMaxOwners).toBe(1);
  view.unmount();
  view.client.clear();
  historyRequest.mockRestore();
});

it("в ожидании показывает только имя встречи и безопасный выход без материалов", () => {
  fixture.membership.status = "waiting";
  fixture.membership.admissionState = "waiting";
  const view = page();
  expect(
    view.container.querySelector(".conference-waiting-page"),
  ).toBeInTheDocument();
  expect(screen.getByText("Имя во встрече: Алиса")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "К встречам" })).toHaveAttribute(
    "href",
    "/conferences",
  );
  expect(screen.queryByRole("textbox", { name: "Сообщение" })).toBeNull();
  expect(screen.queryByTestId("analytics-panel")).toBeNull();
  expect(screen.queryByTestId("realtime-panel")).toBeNull();
  expect(
    screen.queryByRole("link", { name: "К материалам встречи" }),
  ).toBeNull();
  view.client.clear();
});

it("перед повторным входом открывает проверку устройств без подключения медиа", () => {
  fixture.membership.status = "left";
  const view = page();
  expect(screen.queryByTestId("realtime-panel")).not.toBeInTheDocument();
  expect(fixture.realtimeEnabled).toEqual([false]);
  fireEvent.click(screen.getByRole("button", { name: "Присоединиться" }));
  expect(screen.getByText("Подготовка к встрече")).toBeInTheDocument();
  expect(fixture.mediaMounts).toBe(0);
  expect(fixture.realtimeOwners).toBe(0);
  view.client.clear();
});
