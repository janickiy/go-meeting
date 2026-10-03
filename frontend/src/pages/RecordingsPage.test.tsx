import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type {
  Conference,
  ConferenceHistory,
  ConferenceRecording,
} from "../types";
import { RecordingsPage } from "./RecordingsPage";

const auth = vi.hoisted(() => ({ user: { id: "user-one" } }));
vi.mock("../auth", () => ({ useAuth: () => auth }));
const meeting = {
  id: "first",
  title: "Стратегическая сессия",
  status: "finished",
  createdAt: "2026-10-01T10:00:00Z",
} as Conference;
const other = { ...meeting, id: "second", title: "Демо продукта" };
const history: ConferenceHistory = {
  conference: meeting,
  owner: { id: "owner", displayName: "Александр" },
  durationSec: 200,
  participantCount: 4,
  participants: [],
  participantsTruncated: false,
  recordings: { total: 1, ready: 1, processing: 0, failed: 0 },
  chatAvailable: true,
  chatReadOnly: true,
};
const record: ConferenceRecording = {
  uuid: "record-one",
  conferenceId: "first",
  status: "ready",
  mode: "composite",
  createdAt: "2026-10-01T10:00:00Z",
  durationSec: 80,
  files: [
    {
      fileType: "final_mp4",
      url: "https://media.example.test/video?signature=allowed",
      sizeBytes: 1048576,
    },
    { fileType: "preview_jpg", url: "/media/preview?signature=allowed" },
  ],
};
const clients: QueryClient[] = [];

/** show открывает записи с отдельным кешем и настоящими хуками пагинации.
 * @args url — исходная ссылка; client — необязательный кеш для проверки повторных запросов.
 * @return контейнер страницы и средство обновления контекста авторизации.
 */
function show(
  url = "/recordings",
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  clients.push(client);
  const tree = () => (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[url]}>
        <RecordingsPage />
      </MemoryRouter>
    </QueryClientProvider>
  );
  const rendered = render(tree());
  return { ...rendered, client, refreshAuth: () => rendered.rerender(tree()) };
}

beforeEach(() => {
  auth.user = { id: "user-one" };
  vi.spyOn(api, "myConferences").mockResolvedValue({
    status: "success",
    items: [meeting, other],
    nextCursor: "next",
  });
  vi.spyOn(api, "conference").mockImplementation(async (id) => ({
    status: "success",
    item: id === "second" ? other : meeting,
  }));
  vi.spyOn(api, "recordings").mockResolvedValue({
    status: "success",
    items: [record],
  });
  vi.spyOn(api, "history").mockResolvedValue({
    status: "success",
    item: history,
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
});

describe("записи выбранной встречи", () => {
  it("начинает с завершённых встреч и читает материалы только выбранной встречи", async () => {
    show();
    expect(await screen.findByTestId("recordings-row")).toBeInTheDocument();
    expect(api.myConferences).toHaveBeenCalledWith(
      { view: "past" },
      undefined,
      expect.any(AbortSignal),
    );
    expect(api.recordings).toHaveBeenCalledTimes(1);
    expect(api.recordings).toHaveBeenCalledWith(
      "first",
      expect.any(AbortSignal),
      { limit: 20, offset: 0 },
    );
    expect(api.history).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("link", {
        name: "Смотреть запись: Стратегическая сессия",
      }),
    ).toHaveAttribute("href", "/recordings/record-one?conference=first");
    expect(
      await screen.findByText("Организатор: Александр"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Обработка|ready|failed/)).toBeNull();
    expect(
      screen.queryByText(/Все записи|Мои записи|Общие со мной/),
    ).toBeNull();
  });

  it("переключает серверный вид и не запрашивает историю активной встречи", async () => {
    vi.mocked(api.myConferences).mockImplementation(async (filters) => ({
      status: "success",
      nextCursor: null,
      items:
        filters?.view === "active"
          ? [{ ...other, status: "active" }]
          : [meeting],
    }));
    vi.mocked(api.conference).mockImplementation(async (id) => ({
      status: "success",
      item: id === "second" ? { ...other, status: "active" } : meeting,
    }));
    show();
    await screen.findByTestId("recordings-row");
    vi.mocked(api.history).mockClear();
    fireEvent.click(screen.getByRole("button", { name: "В эфире" }));
    await waitFor(() =>
      expect(api.myConferences).toHaveBeenCalledWith(
        { view: "active" },
        undefined,
        expect.any(AbortSignal),
      ),
    );
    await screen.findByRole("heading", { name: "Демо продукта" });
    expect(api.history).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("link", { name: "Открыть историю встречи" }),
    ).toBeNull();
  });

  it("ищет по загруженным названиям без запросов записей соседних встреч", async () => {
    show();
    await screen.findByTestId("recordings-row");
    fireEvent.change(screen.getByRole("searchbox"), {
      target: { value: "ДЕМО" },
    });
    expect(
      screen.getByRole("option", { name: "Демо продукта" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Поиск выполняется по загруженным встречам/),
    ).toBeInTheDocument();
    expect(api.recordings).toHaveBeenCalledTimes(1);
    fireEvent.change(screen.getByRole("searchbox"), {
      target: { value: "Нет совпадений" },
    });
    expect(screen.getByRole("status")).toHaveTextContent(
      "В загруженных встречах совпадений нет",
    );
    expect(screen.getByTestId("recordings-row")).toBeInTheDocument();
  });

  it("продолжает серверную пагинацию списка встреч курсором", async () => {
    vi.mocked(api.myConferences)
      .mockResolvedValueOnce({
        status: "success",
        items: [meeting],
        nextCursor: "next",
      })
      .mockResolvedValueOnce({
        status: "success",
        items: [other],
        nextCursor: null,
      });
    show();
    await screen.findByTestId("recordings-row");
    fireEvent.click(screen.getByRole("button", { name: "Ещё встречи" }));
    await screen.findByRole("option", { name: other.title });
    expect(api.myConferences).toHaveBeenLastCalledWith(
      { view: "past" },
      "next",
      expect.any(AbortSignal),
    );
    expect(api.recordings).toHaveBeenCalledTimes(1);
  });

  it("рассчитывает следующий offset по сырым строкам до скрытия неготовых записей", async () => {
    const pending = Array.from({ length: 20 }, (_, index) => ({
      ...record,
      uuid: `pending-${index}`,
      status: "processing" as const,
    }));
    vi.mocked(api.recordings).mockImplementation(
      async (_id, _signal, page) => ({
        status: "success",
        items: page?.offset === 20 ? [record] : pending,
      }),
    );
    show();
    expect(
      await screen.findByRole("heading", {
        name: "Среди загруженных записей совпадений нет",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Доступных записей пока нет")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Ещё записи" }));
    await screen.findByTestId("recordings-row");
    expect(api.recordings).toHaveBeenLastCalledWith(
      "first",
      expect.any(AbortSignal),
      { limit: 20, offset: 20 },
    );
    expect(screen.queryByRole("button", { name: "Ещё записи" })).toBeNull();
  });

  it("сортирует только загруженные готовые записи и отбрасывает другой контекст", async () => {
    const older = {
      ...record,
      uuid: "older",
      createdAt: "2026-09-01T10:00:00Z",
    };
    vi.mocked(api.recordings).mockResolvedValue({
      status: "success",
      items: [
        older,
        record,
        { ...record, uuid: "wrong", conferenceId: "second" },
      ],
    });
    show();
    await screen.findAllByTestId("recordings-row");
    let rows = screen.getAllByTestId("recordings-row");
    expect(rows).toHaveLength(2);
    expect(
      within(rows[0]).getByRole("link", { name: "Смотреть" }),
    ).toHaveAttribute("href", "/recordings/record-one?conference=first");
    fireEvent.change(
      screen.getByRole("combobox", { name: "Сортировка загруженных записей" }),
      { target: { value: "oldest" } },
    );
    rows = screen.getAllByTestId("recordings-row");
    expect(
      within(rows[0]).getByRole("link", { name: "Смотреть" }),
    ).toHaveAttribute("href", "/recordings/older?conference=first");
    expect(api.recordings).toHaveBeenCalledTimes(1);
  });

  it("показывает безопасное сообщение при отказе доступа без технических подробностей", async () => {
    vi.mocked(api.recordings).mockRejectedValue(
      new ApiError(403, "secret technical message"),
    );
    show();
    expect(
      await screen.findByText(/Не удалось загрузить записи этой встречи/),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("recordings-row")).toBeNull();
    expect(screen.queryByText("secret technical message")).toBeNull();
  });

  it("скрывает ранее прочитанные файлы после отказа при повторной загрузке", async () => {
    const { client } = show();
    await screen.findByTestId("recordings-row");
    vi.mocked(api.recordings).mockRejectedValue(new ApiError(403, "denied"));
    await act(() =>
      client.refetchQueries({
        queryKey: ["conference-recording-pages", "user-one", "first"],
      }),
    );
    await screen.findByText(/Не удалось загрузить записи этой встречи/);
    expect(screen.queryByTestId("recordings-row")).toBeNull();
  });

  it("не показывает сведения чужой встречи при несовпадении ответа", async () => {
    vi.mocked(api.conference).mockResolvedValue({
      status: "success",
      item: other,
    });
    show("/recordings?conference=first");
    expect(
      await screen.findByRole("heading", { name: "Встреча недоступна" }),
    ).toBeInTheDocument();
    expect(api.recordings).not.toHaveBeenCalled();
    expect(api.history).not.toHaveBeenCalled();
  });

  it("не подмешивает сводку другой встречи", async () => {
    vi.mocked(api.history).mockResolvedValue({
      status: "success",
      item: { ...history, conference: other },
    });
    show();
    await screen.findByTestId("recordings-row");
    expect(screen.queryByText("Организатор: Александр")).toBeNull();
    expect(screen.queryByText("4 участников встречи")).toBeNull();
  });

  it("при смене пользователя не сохраняет приватные записи предыдущей учётной записи", async () => {
    const { refreshAuth } = show();
    await screen.findByTestId("recordings-row");
    vi.mocked(api.recordings).mockResolvedValue({
      status: "success",
      items: [],
    });
    auth.user = { id: "user-two" };
    refreshAuth();
    await screen.findByRole("heading", { name: "Доступных записей пока нет" });
    expect(screen.queryByTestId("recordings-row")).toBeNull();
    expect(api.recordings).toHaveBeenCalledTimes(2);
  });

  it("переключает контекст без старых карточек", async () => {
    vi.mocked(api.recordings).mockImplementation(async (id) => ({
      status: "success",
      items: id === "first" ? [record] : [],
    }));
    show();
    await screen.findByTestId("recordings-row");
    fireEvent.change(screen.getByRole("combobox", { name: "Встреча" }), {
      target: { value: "second" },
    });
    await screen.findByRole("heading", { name: "Доступных записей пока нет" });
    expect(screen.queryByTestId("recordings-row")).toBeNull();
    expect(api.recordings).toHaveBeenLastCalledWith(
      "second",
      expect.any(AbortSignal),
      { limit: 20, offset: 0 },
    );
  });
});
