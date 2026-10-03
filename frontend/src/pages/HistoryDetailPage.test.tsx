import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "../api";
import type { ConferenceHistory, Participant } from "../types";
import { HistoryDetailPage } from "./HistoryDetailPage";

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "user" } }) }));
vi.mock("../components/RecordingPanel", () => ({
  RecordingPanel: () => <p>Материалы записи загружены</p>,
}));
vi.mock("../components/ChatPanel", () => ({
  ChatPanel: () => <p>История чата загружена</p>,
}));
vi.mock("../components/AnalyticsPanel", () => ({
  AnalyticsPanel: () => <p>Аналитика загружена</p>,
}));

const member = {
  id: "member",
  userId: "user",
  status: "left",
  admissionState: "admitted",
  role: "participant",
  displayName: "Участник",
} as Participant;
const history = {
  conference: {
    id: "room",
    title: "План запуска",
    status: "finished",
    ownerId: "owner",
    createdAt: "2026-10-01T10:00:00Z",
    finishedAt: "2026-10-01T11:00:00Z",
  },
  owner: { id: "owner", displayName: "Организатор" },
  durationSec: 3600,
  participantCount: 2,
  participants: [member],
  participantsTruncated: false,
  recordings: { total: 1, ready: 1, processing: 0, failed: 0 },
  chatAvailable: true,
  chatReadOnly: true,
} as ConferenceHistory;
const clients: QueryClient[] = [];

function show(url = "/history/room") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[url]}>
        <Routes>
          <Route path="/history/:id" element={<HistoryDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.spyOn(api, "history").mockResolvedValue({
    status: "success",
    item: history,
  });
  vi.spyOn(api, "myMembership").mockResolvedValue({
    status: "success",
    item: member,
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
});

describe("история встречи", () => {
  it("показывает обзор и загружает тяжёлые панели только по выбранной вкладке", async () => {
    show();
    expect(
      await screen.findByRole("heading", { name: "План запуска" }),
    ).toBeInTheDocument();
    expect(screen.getByText("60 мин")).toBeInTheDocument();
    expect(
      screen.queryByText("Материалы записи загружены"),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Расшифровка" }));
    expect(screen.getByText("Материалы записи загружены")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Чат" }));
    expect(screen.getByText("История чата загружена")).toBeInTheDocument();
    expect(
      screen.queryByText("Материалы записи загружены"),
    ).not.toBeInTheDocument();
  });

  it("не открывает материалы без подтверждённого допуска", async () => {
    vi.mocked(api.myMembership).mockResolvedValue({
      status: "success",
      item: { ...member, status: "waiting", admissionState: "waiting" },
    });
    show();
    expect(
      await screen.findByRole("heading", {
        name: "История встречи недоступна",
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Материалы записи загружены"),
    ).not.toBeInTheDocument();
  });
});
