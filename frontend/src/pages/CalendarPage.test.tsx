import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import axe from "axe-core";
import {
  CalendarPage,
  calendarDayKey,
  calendarRange,
  scheduledMeetingDate,
} from "./CalendarPage";
import type { Conference } from "../types";

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  transition: vi.fn(),
  userId: "owner",
}));
vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: mocks.userId } }) }));
vi.mock("../queries", () => ({ useConferences: mocks.list }));
vi.mock("../api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../api")>()),
  api: { transition: mocks.transition },
}));

/** Создаёт встречу в местном дне теста, чтобы тест не зависел от UTC-смещения машины. */
function meeting(): Conference {
  const today = new Date();
  return {
    id: "scheduled-meeting",
    ownerId: "owner",
    title: "Обсуждение проекта",
    status: "scheduled",
    inviteCode: "invite",
    inviteUrl: "",
    waitingRoomEnabled: false,
    createdAt: today.toISOString(),
    updatedAt: today.toISOString(),
    startedAt: null,
    finishedAt: null,
    scheduledAt: new Date(
      today.getFullYear(),
      today.getMonth(),
      today.getDate(),
      10,
      30,
    ).toISOString(),
    plannedDurationMin: 45,
  };
}

/** Подключает настоящий кеш мутаций, оставляя сетевой список тестовым. */
function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <main>
          <CalendarPage />
        </main>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  mocks.userId = "owner";
  mocks.transition
    .mockReset()
    .mockResolvedValue({ item: { ...meeting(), status: "cancelled" } });
  mocks.list.mockReset().mockReturnValue({
    data: { pages: [{ items: [meeting()], nextCursor: null }] },
    isPending: false,
    isError: false,
    error: null,
    hasNextPage: false,
    fetchNextPage: vi.fn(),
    refetch: vi.fn(),
  });
});

describe("границы календаря", () => {
  it("считает понедельник и конец воскресенья в местном часовом поясе", () => {
    const range = calendarRange(new Date(2026, 9, 4, 12), "week");
    expect(calendarDayKey(range.days[0])).toBe("2026-09-28");
    expect(calendarDayKey(range.days[6])).toBe("2026-10-04");
    expect(new Date(range.from).getHours()).toBe(0);
    expect(new Date(range.to).getHours()).toBe(23);
    expect(new Date(range.to).getMilliseconds()).toBe(999);
  });
  it("не прибавляет фиксированные 24 часа на неделе перехода DST", () => {
    const range = calendarRange(new Date(2026, 2, 29, 12), "week");
    expect(range.days).toHaveLength(7);
    expect(range.days.map((day) => day.getHours())).toEqual([
      0, 0, 0, 0, 0, 0, 0,
    ]);
    expect(new Set(range.days.map(calendarDayKey)).size).toBe(7);
  });
  it("строит 42 дня месяца без пропуска дней соседних месяцев", () => {
    const range = calendarRange(new Date(2026, 1, 17), "month");
    expect(range.days).toHaveLength(42);
    expect(calendarDayKey(range.days[0])).toBe("2026-01-26");
    expect(calendarDayKey(range.days[41])).toBe("2026-03-08");
  });
  it("не изображает дату создания как запланированную встречу", () => {
    expect(
      scheduledMeetingDate({ ...meeting(), scheduledAt: null }),
    ).toBeNull();
    expect(
      scheduledMeetingDate({ ...meeting(), scheduledAt: "bad-date" }),
    ).toBeNull();
  });
});

describe("календарь встреч", () => {
  it("запрашивает предстоящие встречи за период и переключает неделю/месяц", () => {
    show();
    expect(mocks.list).toHaveBeenCalledWith(
      expect.objectContaining({
        view: "upcoming",
        scope: "all",
        from: expect.any(String),
        to: expect.any(String),
      }),
    );
    expect(
      screen.getByText(/предстоящие встречи с указанной датой/),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Новая встреча" })).toHaveAttribute(
      "href",
      "/meetings/new?scheduled=1&returnTo=calendar",
    );
    fireEvent.click(screen.getByRole("button", { name: "Месяц" }));
    expect(screen.getByRole("button", { name: "Месяц" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    const previousFrom = mocks.list.mock.calls.at(-1)![0].from;
    fireEvent.click(screen.getByRole("button", { name: "Следующий период" }));
    expect(mocks.list.mock.calls.at(-1)![0].from).not.toBe(previousFrom);
  });
  it("отменяет встречу владельца только после подтверждения и ответа API", async () => {
    show();
    fireEvent.click(
      screen.getAllByRole("button", { name: /Обсуждение проекта,/ })[0],
    );
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByRole("button", { name: "Изменить расписание" }),
    ).toBeInTheDocument();
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Отменить встречу" }),
    );
    expect(mocks.transition).not.toHaveBeenCalled();
    fireEvent.click(
      within(dialog).getByRole("button", { name: "Да, отменить встречу" }),
    );
    await waitFor(() =>
      expect(mocks.transition).toHaveBeenCalledWith(
        "scheduled-meeting",
        "cancel",
      ),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });
  it("не показывает участнику операции изменения чужого расписания", () => {
    mocks.userId = "participant";
    show();
    fireEvent.click(
      screen.getAllByRole("button", { name: /Обсуждение проекта,/ })[0],
    );
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).queryByRole("button", { name: "Изменить расписание" }),
    ).not.toBeInTheDocument();
    expect(
      within(dialog).queryByRole("button", { name: "Отменить встречу" }),
    ).not.toBeInTheDocument();
    expect(
      within(dialog).getByRole("link", { name: "Открыть встречу" }),
    ).toHaveAttribute("href", "/meetings/scheduled-meeting");
  });
  it("явно показывает неполный период и загружает серверное продолжение", () => {
    const fetchNextPage = vi.fn();
    mocks.list.mockReturnValue({
      ...mocks.list(),
      hasNextPage: true,
      fetchNextPage,
    });
    show();
    expect(
      screen.getByText(/Показана часть встреч периода/),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Загрузить ещё встречи" }),
    );
    expect(fetchNextPage).toHaveBeenCalledOnce();
  });
  it("проходит доступность календаря и диалога встречи", async () => {
    show();
    const results = await axe.run(document.body, {
      runOnly: {
        type: "tag",
        values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
      },
      rules: { "color-contrast": { enabled: false } },
    });
    expect(results.violations.map((violation) => violation.id)).toEqual([]);
  });
});
