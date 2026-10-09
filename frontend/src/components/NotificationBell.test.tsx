import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { api } from "../api";
import { NotificationBell } from "./NotificationBell";
import type { Notification } from "../types";

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "user" } }) }));
const clients: QueryClient[] = [];
const notice: Notification = {
  id: "notice",
  userId: "user",
  type: "recording.ready",
  version: 1,
  payload: { conferenceId: "room" },
  createdAt: "2026-10-09T10:00:00Z",
  readAt: null,
};

/** Монтирует колокольчик с серверным числом непрочитанных в изолированном кеше.
 * @args unreadCount — глобальное число непрочитанных; items — загруженные уведомления.
 * @return клиент кеша и результат монтирования компонента.
 */
function show(unreadCount: number, items: Notification[] = []) {
  const page = {
    status: "success" as const,
    unreadCount,
    items,
    nextCursor: null,
  };
  vi.spyOn(api, "notifications").mockResolvedValue(page);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  client.setQueryData(["notifications", "user"], {
    pages: [page],
    pageParams: [undefined],
  });
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <NotificationBell />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { client, ...view };
}

afterEach(() => {
  cleanup();
  for (const client of clients.splice(0)) client.clear();
  vi.restoreAllMocks();
});

describe("индикатор непрочитанных уведомлений", () => {
  it("не показывает кружок, когда непрочитанных нет", () => {
    const { container } = show(0);
    expect(container.querySelector(".notification-dot")).toBeNull();
    expect(
      screen.getByRole("button", { name: "Уведомления" }),
    ).not.toHaveAttribute("aria-describedby");
  });

  it("показывает одну точку без числа независимо от размера и содержания страницы", () => {
    const { container } = show(150);
    const dot = container.querySelector(".notification-dot");
    expect(dot).toBeInTheDocument();
    expect(dot).toHaveAttribute("aria-hidden", "true");
    expect(dot).toBeEmptyDOMElement();
    expect(
      screen.getByRole("button", { name: "Уведомления" }),
    ).toHaveAccessibleDescription("150 непрочитанных уведомлений");
  });

  it("не скрывает индикатор и не отмечает уведомления прочитанными при открытии окна", async () => {
    const read = vi.spyOn(api, "readNotification");
    const { container } = show(1, [notice]);
    fireEvent.click(screen.getByRole("button", { name: "Уведомления" }));
    expect(
      await screen.findByRole("dialog", { name: "Уведомления" }),
    ).toBeVisible();
    expect(container.querySelector(".notification-dot")).toBeInTheDocument();
    expect(read).not.toHaveBeenCalled();
  });

  it("обновляет кружок при новых данных и убирает его только после прочтения последнего уведомления", async () => {
    const { client, container } = show(0);
    await waitFor(() =>
      expect(client.isFetching({ queryKey: ["notifications", "user"] })).toBe(
        0,
      ),
    );
    for (const unreadCount of [2, 1, 0]) {
      await act(async () => {
        client.setQueryData(["notifications", "user"], {
          pages: [
            { status: "success", unreadCount, items: [], nextCursor: null },
          ],
          pageParams: [undefined],
        });
      });
      await waitFor(() => {
        if (unreadCount)
          expect(
            container.querySelector(".notification-dot"),
          ).toBeInTheDocument();
        else expect(container.querySelector(".notification-dot")).toBeNull();
      });
    }
  });
});
