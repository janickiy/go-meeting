import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "../api";
import { SearchPage } from "./SearchPage";

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "user" } }) }));
vi.mock("../queries", () => ({
  useConferences: () => ({
    data: { pages: [{ items: [{ id: "room", title: "План запуска" }] }] },
    hasNextPage: false,
  }),
}));
let clients: QueryClient[] = [];

/**
 * Открывает поиск с изолированным кешем и проверяемыми URL-фильтрами.
 * @parameters url — начальный маршрут поиска.
 * @return Результат монтирования страницы.
 */
function show(url = "/app/search") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[url]}>
        <SearchPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.spyOn(api, "search").mockResolvedValue({
    status: "success",
    items: [],
    total: 0,
    limit: 20,
    offset: 0,
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients = [];
  vi.restoreAllMocks();
});

describe("поиск по доступным материалам", () => {
  it("не запрашивает результаты до явного ввода запроса", async () => {
    show();
    expect(api.search).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Поисковый запрос"), {
      target: { value: "план" },
    });
    expect(api.search).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Найти" }));
    await screen.findByText(/Ничего не найдено/);
    expect(api.search).toHaveBeenCalledWith(
      expect.objectContaining({ q: "план", source: "all" }),
      0,
      expect.any(AbortSignal),
    );
  });
  it("передаёт UTC-фильтры ссылки и экранирует фрагмент как обычный текст", async () => {
    vi.mocked(api.search).mockResolvedValue({
      status: "success",
      total: 1,
      offset: 0,
      limit: 20,
      items: [
        {
          type: "transcript",
          conferenceId: "room",
          conferenceTitle: "План запуска",
          recordingId: "record",
          segmentId: "segment",
          startMs: 42500,
          snippet: "<script>alert(1)</script>",
          rank: 1,
        },
      ],
    });
    const { container } = show(
      "/app/search?q=план&source=transcript&conferenceId=room&from=2026-10-01T00%3A00%3A00Z&to=2026-10-02T00%3A00%3A00Z",
    );
    await screen.findByText("<script>alert(1)</script>");
    expect(container.querySelector("script")).toBeNull();
    expect(api.search).toHaveBeenCalledWith(
      expect.objectContaining({
        source: "transcript",
        conferenceId: "room",
        from: "2026-10-01T00:00:00Z",
        to: "2026-10-02T00:00:00Z",
      }),
      0,
      expect.any(AbortSignal),
    );
    expect(
      screen.getByRole("link", { name: "Открыть фрагмент записи" }),
    ).toHaveAttribute(
      "href",
      "/conferences/room?recording=record&tab=transcript&t=42500&segment=segment",
    );
  });
  it("отклоняет перевёрнутый диапазон дат до обращения к API", async () => {
    show();
    fireEvent.change(screen.getByLabelText("Поисковый запрос"), {
      target: { value: "план" },
    });
    fireEvent.change(screen.getByLabelText("С даты"), {
      target: { value: "2026-10-03" },
    });
    fireEvent.change(screen.getByLabelText("По дату"), {
      target: { value: "2026-10-01" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Найти" }));
    await screen.findByText(/Дата начала должна быть не позже/);
    expect(api.search).not.toHaveBeenCalled();
  });
  it("пагинирует результаты без подмены фильтров", async () => {
    vi.mocked(api.search).mockImplementation(async (_filters, offset) => ({
      status: "success",
      total: 2,
      offset,
      limit: 1,
      items: [
        {
          type: "conference",
          conferenceId: `room-${offset}`,
          conferenceTitle: `Встреча ${offset}`,
          snippet: "План",
          rank: 1,
        },
      ],
    }));
    show("/app/search?q=план");
    fireEvent.click(
      await screen.findByRole("button", { name: "Ещё результаты" }),
    );
    await screen.findByRole("link", { name: "Встреча 1" });
    await waitFor(() =>
      expect(api.search).toHaveBeenCalledWith(
        expect.objectContaining({ q: "план" }),
        1,
        expect.any(AbortSignal),
      ),
    );
  });
});
