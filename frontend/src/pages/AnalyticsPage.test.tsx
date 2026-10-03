import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import axe from "axe-core";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api, ApiError } from "../api";
import { AnalyticsPage } from "./AnalyticsPage";
import type { Conference, MeetingAnalytics } from "../types";

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "user" } }) }));
const clients: QueryClient[] = [];
const enabledCapabilities = {
  status: "success",
  buildVersion: "test",
  capabilities: {
    liveCaptions: false,
    transcription: false,
    aiSummary: false,
    semanticSearch: false,
    meetingAnalytics: true,
    recordingModes: [],
  },
};
const analytics: MeetingAnalytics = {
  enabled: true,
  durationMs: 900000,
  participantCount: 2,
  recordingAvailable: true,
  transcriptAvailable: false,
  timeline: [
    { atMs: 0, count: 1 },
    { atMs: 60000, count: 2 },
  ],
  participants: [
    {
      participantId: "2",
      displayName: "Яна",
      participationMs: 700000,
      speakingMs: 300000,
      observedAudioMs: 700000,
      screenMs: 100000,
      messageCount: 3,
      handRaises: 1,
    },
    {
      participantId: "1",
      displayName: "Александр",
      participationMs: 900000,
      speakingMs: 500000,
      observedAudioMs: 850000,
      screenMs: 200000,
      messageCount: 2,
      handRaises: 0,
    },
  ],
};

/** Создаёт отдельный кеш для проверки настоящих цепочек выбора встречи.
 * @args url — адрес страницы с необязательным идентификатором встречи.
 * @return Смонтированная страница аналитики.
 */
function show(url = "/analytics") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[url]}>
        <AnalyticsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.spyOn(api, "capabilities").mockResolvedValue(enabledCapabilities);
  vi.spyOn(api, "myConferences").mockResolvedValue({
    status: "success",
    items: [
      {
        id: "first",
        title: "Первый созвон",
        status: "finished",
        createdAt: "2026-10-01T10:00:00Z",
      },
      {
        id: "second",
        title: "План продукта",
        status: "finished",
        createdAt: "2026-10-02T10:00:00Z",
      },
    ] as Conference[],
    nextCursor: null,
  });
  vi.spyOn(api, "analytics").mockResolvedValue({
    status: "success",
    item: analytics,
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients.length = 0;
  vi.restoreAllMocks();
});

describe("аналитика по фактическим встречам", () => {
  it("предоставляет доступную таблицу и текстовые значения графика", async () => {
    show();
    await screen.findByRole("table");
    const results = await axe.run(document.body, {
      runOnly: {
        type: "tag",
        values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
      },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(results.violations.map((violation) => violation.id)).toEqual([]);
  });
  it("запрашивает агрегаты только выбранной встречи, а не каждой строки списка", async () => {
    show();
    await screen.findAllByText("15:00");
    expect(api.myConferences).toHaveBeenCalledWith(
      { view: "past" },
      undefined,
      expect.any(AbortSignal),
    );
    expect(api.analytics).toHaveBeenCalledTimes(1);
    expect(api.analytics).toHaveBeenCalledWith(
      "first",
      expect.any(AbortSignal),
    );
    fireEvent.change(
      screen.getByRole("combobox", { name: "Встреча для анализа" }),
      { target: { value: "second" } },
    );
    await waitFor(() =>
      expect(api.analytics).toHaveBeenCalledWith(
        "second",
        expect.any(AbortSignal),
      ),
    );
    expect(api.analytics).toHaveBeenCalledTimes(2);
    expect(
      screen.getByText(
        /Сводные показатели по всем встречам сервер пока не предоставляет/,
      ),
    ).toBeInTheDocument();
  });
  it("переключает серверный фильтр на active и сбрасывает выбранную завершённую встречу", async () => {
    vi.mocked(api.myConferences).mockImplementation(async (filters = {}) => ({
      status: "success",
      items: [
        {
          id: filters.view === "active" ? "active-room" : "past-room",
          title:
            filters.view === "active" ? "Текущий созвон" : "Завершённый созвон",
          status: filters.view === "active" ? "active" : "finished",
          createdAt: "2026-10-01T10:00:00Z",
        },
      ] as Conference[],
      nextCursor: null,
    }));
    show("/analytics?conference=past-room");
    await screen.findByRole("table");
    fireEvent.change(screen.getByRole("combobox", { name: "Статус встречи" }), {
      target: { value: "active" },
    });
    await waitFor(() =>
      expect(api.myConferences).toHaveBeenCalledWith(
        { view: "active" },
        undefined,
        expect.any(AbortSignal),
      ),
    );
    await waitFor(() =>
      expect(api.analytics).toHaveBeenCalledWith(
        "active-room",
        expect.any(AbortSignal),
      ),
    );
    expect(
      screen.getByRole("combobox", { name: "Встреча для анализа" }),
    ).toHaveValue("active-room");
    expect(
      screen.queryByRole("option", { name: /Завершённый созвон/ }),
    ).toBeNull();
  });
  it("не подменяет пустую историю предстоящими встречами", async () => {
    vi.mocked(api.myConferences).mockResolvedValue({
      status: "success",
      items: [],
      nextCursor: null,
    });
    show("/analytics?view=upcoming");
    await screen.findByText(/Завершённых встреч пока нет/);
    expect(api.myConferences).toHaveBeenCalledWith(
      { view: "past" },
      undefined,
      expect.any(AbortSignal),
    );
    expect(api.analytics).not.toHaveBeenCalled();
    expect(
      screen.getByRole("combobox", { name: "Встреча для анализа" }),
    ).toBeDisabled();
  });
  it("показывает отдельное пустое состояние для active", async () => {
    vi.mocked(api.myConferences).mockResolvedValue({
      status: "success",
      items: [],
      nextCursor: null,
    });
    show("/analytics?view=active");
    await screen.findByText(/Сейчас нет активных встреч/);
    expect(api.myConferences).toHaveBeenCalledWith(
      { view: "active" },
      undefined,
      expect.any(AbortSignal),
    );
    expect(api.analytics).not.toHaveBeenCalled();
  });
  it("сохраняет алфавитный порядок участников, не выдавая речь за рейтинг", async () => {
    show();
    const table = await screen.findByRole("table");
    const rows = within(table).getAllByRole("row");
    expect(rows[1]).toHaveTextContent("Александр");
    expect(rows[2]).toHaveTextContent("Яна");
    expect(
      screen.getByText(/Это не оценка продуктивности/),
    ).toBeInTheDocument();
  });
  it("не запрашивает список и аналитику, если capability выключена", async () => {
    vi.mocked(api.capabilities).mockResolvedValue({
      ...enabledCapabilities,
      capabilities: {
        ...enabledCapabilities.capabilities,
        meetingAnalytics: false,
      },
    });
    show("/analytics?conference=secret");
    await screen.findByRole("heading", { name: "Аналитика сейчас недоступна" });
    expect(api.myConferences).not.toHaveBeenCalled();
    expect(api.analytics).not.toHaveBeenCalled();
  });
  it("ошибка capabilities закрывает доступ к аналитике", async () => {
    vi.mocked(api.capabilities).mockRejectedValue(
      new ApiError(503, "Сервис недоступен"),
    );
    show();
    await screen.findByRole("heading", { name: "Аналитика сейчас недоступна" });
    expect(api.analytics).not.toHaveBeenCalled();
  });
  it("не показывает старые показатели при отказе в доступе к встрече по ссылке", async () => {
    vi.mocked(api.analytics).mockRejectedValue(
      new ApiError(403, "Нет доступа"),
    );
    show("/analytics?conference=private");
    await screen.findByText("Нет доступа");
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText("15:00")).toBeNull();
  });
});
