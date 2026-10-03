import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "../api";
import { NotificationsPage } from "./NotificationsPage";

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "user" } }) }));

const clients: QueryClient[] = [];
function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <NotificationsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.spyOn(api, "notifications").mockResolvedValue({
    status: "success",
    unreadCount: 1,
    nextCursor: null,
    items: [
      {
        id: "notice",
        userId: "user",
        type: "summary.ready",
        version: 1,
        payload: { conferenceId: "room", recordingId: "record" },
        createdAt: "2026-10-01T10:00:00Z",
        readAt: null,
      },
    ],
  });
  vi.spyOn(api, "readNotification").mockResolvedValue({
    status: "success",
    item: {
      id: "notice",
      userId: "user",
      type: "summary.ready",
      version: 1,
      payload: { conferenceId: "room", recordingId: "record" },
      createdAt: "2026-10-01T10:00:00Z",
      readAt: "2026-10-01T10:10:00Z",
    },
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
});

describe("уведомления", () => {
  it("открывает разрешённый материал и позволяет отметить запись как прочитанную", async () => {
    show();
    expect(await screen.findByText("Итоги встречи готовы")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Непрочитанных: 1");
    expect(
      screen.getByRole("link", { name: "Открыть встречу" }),
    ).toHaveAttribute(
      "href",
      "/history/room?recording=record&tab=summary&section=summary",
    );
    fireEvent.click(
      screen.getByRole("button", { name: /Отметить как прочитанное/ }),
    );
    await waitFor(() => expect(api.readNotification).toHaveBeenCalled());
    expect(vi.mocked(api.readNotification).mock.calls[0][0]).toBe("notice");
  });
});
