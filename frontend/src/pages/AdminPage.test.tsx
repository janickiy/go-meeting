import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ApiError, api } from "../api";
import type { AdminSummary } from "../types";
import { AdminPage } from "./AdminPage";

const auth = vi.hoisted(() => ({
  user: { id: "person", isAdmin: true } as {
    id: string;
    isAdmin: boolean;
  } | null,
}));
vi.mock("../auth", () => ({ useAuth: () => auth }));

const item: AdminSummary = {
  asOf: "2026-10-03T08:00:00Z",
  activeConferences: 2,
  joinedParticipants: 5,
  activeRecordings: 1,
  queuedJobs: 3,
  failedJobs24h: 1,
  failedRecordings24h: 0,
  failedTranscriptions24h: 1,
  apiReady: true,
  mediaWorkerReady: false,
  dependencies: { postgres: true, redis: true, minio: true, rabbitmq: false },
  recentFailures: [
    {
      kind: "content.transcribe",
      code: "<img src=x onerror=alert(1)>",
      at: "2026-10-03T07:30:00Z",
    },
  ],
};

let clients: QueryClient[] = [];
function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <AdminPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  auth.user = { id: "person", isAdmin: true };
  vi.spyOn(api, "adminSummary").mockResolvedValue({ status: "success", item });
  vi.spyOn(api, "capabilities").mockResolvedValue({
    status: "success",
    buildVersion: "abcdef123456",
    capabilities: {
      liveCaptions: false,
      transcription: false,
      aiSummary: false,
      semanticSearch: false,
      meetingAnalytics: false,
      recordingModes: ["composite"],
    },
  });
});

afterEach(() => {
  clients.forEach((client) => client.clear());
  clients = [];
  vi.restoreAllMocks();
});

it("не загружает административные данные для обычного пользователя", () => {
  auth.user = { id: "person", isAdmin: false };
  show();
  expect(screen.getByRole("alert")).toHaveTextContent("Нет доступа");
  expect(api.adminSummary).not.toHaveBeenCalled();
});

it("показывает администратору только агрегаты и безопасно экранирует коды", async () => {
  const { container } = show();
  expect(await screen.findByText("Активные встречи")).toBeInTheDocument();
  expect(screen.getByText("5")).toBeInTheDocument();
  expect(screen.getByText("abcdef123456")).toBeInTheDocument();
  expect(screen.getByText("<img src=x onerror=alert(1)>")).toBeInTheDocument();
  expect(container.querySelector("img")).toBeNull();
  expect(screen.getByText("RabbitMQ").parentElement).toHaveTextContent(
    "Недоступен",
  );
});

it("скрывает ранее загруженную сводку после серверного 403", async () => {
  show();
  expect(await screen.findByText("Активные встречи")).toBeInTheDocument();
  vi.mocked(api.adminSummary).mockRejectedValueOnce(
    new ApiError(403, "Доступ отозван"),
  );
  fireEvent.click(screen.getByRole("button", { name: "Обновить" }));
  await waitFor(() =>
    expect(screen.getByRole("alert")).toHaveTextContent("право изменилось"),
  );
  expect(screen.queryByText("Активные встречи")).toBeNull();
});
