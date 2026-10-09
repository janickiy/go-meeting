import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { Dashboard } from "./Dashboard";
import type { Conference, ConferenceFilters } from "../types";

const list = vi.hoisted(() => vi.fn());
vi.mock("../auth", () => ({
  useAuth: () => ({ user: { id: "user", displayName: "Александр" } }),
}));
vi.mock("../queries", () => ({ useConferences: list }));
vi.mock("../components/ConferenceModals", () => ({
  CreateConference: () => null,
  JoinByLink: () => null,
}));

const scheduled: Conference = {
  id: "upcoming",
  ownerId: "user",
  title: "Настоящая запланированная встреча",
  status: "scheduled",
  inviteCode: "invite",
  inviteUrl: "",
  createdAt: "2026-10-03T10:00:00Z",
  updatedAt: "2026-10-03T10:00:00Z",
  startedAt: null,
  finishedAt: null,
  scheduledAt: "2026-10-05T10:00:00Z",
  plannedDurationMin: 30,
};
const past = {
  ...scheduled,
  id: "past",
  title: "Завершённый разговор",
  status: "finished",
  finishedAt: "2026-10-02T10:00:00Z",
};

beforeEach(() => {
  list.mockReset().mockImplementation((filters: ConferenceFilters) => ({
    data: {
      pages: [{ items: filters.view === "past" ? [past] : [scheduled] }],
    },
    isPending: false,
    isError: false,
    hasNextPage: false,
    error: null,
  }));
});

describe("обновлённый кабинет", () => {
  it("показывает реальные встречи и историю двумя списочными запросами", () => {
    render(
      <MemoryRouter>
        <Dashboard />
      </MemoryRouter>,
    );
    expect(
      screen.getByRole("heading", { name: "Добро пожаловать, Александр." }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Запланировать" })).toHaveAttribute(
      "href",
      "/meetings/new?scheduled=1",
    );
    expect(
      screen.getByRole("link", { name: /Настоящая запланированная встреча/ }),
    ).toHaveAttribute("href", "/meetings/upcoming");
    expect(
      screen.getByRole("link", { name: /Завершённый разговор/ }),
    ).toHaveAttribute("href", "/history/past");
    expect(list).toHaveBeenCalledTimes(2);
    expect(list).toHaveBeenCalledWith({
      view: "past",
      scope: "all",
      status: "finished",
    });
    expect(screen.getByText("Открыть материалы")).toBeInTheDocument();
    expect(document.querySelector("img")).toBeNull();
  });
  it("в полном списке не запускает дополнительную загрузку последних встреч", () => {
    render(
      <MemoryRouter>
        <Dashboard all />
      </MemoryRouter>,
    );
    expect(list).toHaveBeenCalledTimes(1);
    fireEvent.change(
      screen.getByRole("textbox", { name: "Поиск конференции по названию" }),
      { target: { value: "не найдено" } },
    );
    expect(screen.getByText("Встречи не найдены")).toBeInTheDocument();
    expect(
      screen.getByText(/Поиск работает по загруженным встречам/),
    ).toBeInTheDocument();
    expect(screen.queryByText("Недавние встречи")).not.toBeInTheDocument();
  });
});
