import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
import { ConferencePage } from "./ConferencePage";

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
}));

beforeEach(() => {
  fixture.membership.status = "joined";
  fixture.membership.admissionState = "admitted";
  fixture.captions = true;
  fixture.analytics = true;
});

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "self" } }) }));
vi.mock("../queries", () => ({
  useConference: () => ({
    isPending: false,
    isError: false,
    data: {
      item: {
        id: "room",
        title: "Командная встреча",
        status: "active",
        ownerId: "other",
        createdAt: "2026-10-01T10:00:00Z",
        inviteCode: "a".repeat(32),
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
vi.mock("../realtime", () => ({
  useRealtime: () => ({
    state: { connectionId: "connection", participants: [] },
    status: "Подключено",
  }),
}));
vi.mock("../useCapabilities", () => ({
  useCapabilities: () => ({
    isSuccess: true,
    data: {
      capabilities: {
        liveCaptions: fixture.captions,
        meetingAnalytics: fixture.analytics,
      },
    },
  }),
}));
vi.mock("../components/RealtimePanel", () => ({
  RealtimePanel: () => <div data-testid="realtime-panel">Медиа</div>,
}));
vi.mock("../components/HandReactionsPanel", () => ({
  HandReactionsPanel: ({
    handShortcutToken,
  }: {
    handShortcutToken: number;
  }) => <output data-testid="hand-shortcut">{handShortcutToken}</output>,
}));
vi.mock("../components/ChatPanel", () => ({
  ChatPanel: ({ conferenceId }: { conferenceId: string }) => (
    <textarea id={`chat-text-${conferenceId}`} aria-label="Сообщение" />
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

function page() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const element = (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/conferences/room"]}>
        <Routes>
          <Route path="/conferences/:id" element={<ConferencePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
  return { ...render(element), client };
}

it("puts media first and supports roving tab keys and safe H/C shortcuts", async () => {
  const view = page();
  const stage = screen.getByRole("region", { name: "Активная встреча" });
  expect(stage.querySelector("[data-testid='realtime-panel']")).not.toBeNull();
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
  expect(screen.getByTestId("hand-shortcut")).toHaveTextContent("1");
  const composer = screen.getByRole("textbox", { name: "Сообщение" });
  fireEvent.keyDown(composer, { key: "h" });
  expect(screen.getByTestId("hand-shortcut")).toHaveTextContent("1");
  fireEvent.keyDown(window, { key: "c" });
  await waitFor(() => expect(composer).toHaveFocus());
  view.client.clear();
});

it("hides optional panels behind server flags and never mounts media while waiting", () => {
  fixture.captions = false;
  fixture.analytics = false;
  const view = page();
  expect(screen.queryByRole("tab", { name: "Субтитры" })).toBeNull();
  expect(screen.queryByTestId("analytics-panel")).toBeNull();
  expect(
    screen.getByText("Субтитры отключены для этой установки."),
  ).toBeInTheDocument();
  view.unmount();
  fixture.membership.status = "waiting";
  fixture.membership.admissionState = "waiting";
  const waiting = page();
  expect(screen.queryByTestId("realtime-panel")).toBeNull();
  expect(screen.queryByTestId("hand-shortcut")).toBeNull();
  waiting.client.clear();
  view.client.clear();
});
