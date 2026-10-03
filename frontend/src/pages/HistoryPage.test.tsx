import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "../api";
import { HistoryPage } from "./AccountPages";
import type { Conference } from "../types";

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "user" } }) }));
const clients: QueryClient[] = [];

/** Монтирует историю с отдельным кешем и настоящим хуком серверной пагинации.
 * @return Контейнер отрисованной истории.
 */
function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <HistoryPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.spyOn(api, "myConferences").mockResolvedValue({
    status: "success",
    nextCursor: "next-page",
    items: [
      {
        id: "first",
        title: "Стратегическая сессия",
        status: "finished",
        createdAt: "2026-10-01T10:00:00Z",
      },
      {
        id: "second",
        title: "Демо продукта",
        status: "finished",
        createdAt: "2026-10-02T10:00:00Z",
      },
    ] as Conference[],
  });
  vi.spyOn(api, "history");
  vi.spyOn(api, "recordings");
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
});

describe("список истории", () => {
  it("читает серверный список без N+1 запросов материалов", async () => {
    show();
    expect(
      await screen.findByRole("link", { name: /Стратегическая сессия/ }),
    ).toHaveAttribute("href", "/history/first");
    expect(api.myConferences).toHaveBeenCalledWith(
      { view: "past" },
      undefined,
      expect.any(AbortSignal),
    );
    expect(api.myConferences).toHaveBeenCalledTimes(1);
    expect(api.history).not.toHaveBeenCalled();
    expect(api.recordings).not.toHaveBeenCalled();
  });
  it("честно обозначает область поиска по загруженным страницам", async () => {
    show();
    await screen.findByRole("link", { name: /Стратегическая сессия/ });
    fireEvent.change(
      screen.getByRole("searchbox", { name: "Поиск по загруженным встречам" }),
      { target: { value: "ДЕМО" } },
    );
    expect(
      screen.getByRole("link", { name: /Демо продукта/ }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: /Стратегическая сессия/ }),
    ).toBeNull();
    expect(
      screen.getByText(/Поиск выполняется по загруженным встречам/),
    ).toBeInTheDocument();
    expect(api.myConferences).toHaveBeenCalledTimes(1);
  });
  it("отличает отсутствие совпадений от пустой истории", async () => {
    show();
    await screen.findByRole("link", { name: /Стратегическая сессия/ });
    fireEvent.change(screen.getByRole("searchbox"), {
      target: { value: "Несуществующая встреча" },
    });
    expect(
      screen.getByRole("heading", { name: "Совпадений не найдено" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Завершённых встреч пока нет")).toBeNull();
  });
});
