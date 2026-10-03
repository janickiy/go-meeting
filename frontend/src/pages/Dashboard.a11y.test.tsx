import axe from "axe-core";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { Dashboard } from "./Dashboard";

vi.mock("../auth", () => ({
  useAuth: () => ({ user: { displayName: "Алиса" } }),
}));
vi.mock("../queries", () => ({
  useConferences: () => ({
    data: { pages: [{ items: [] }] },
    isPending: false,
    isError: false,
    error: null,
    hasNextPage: false,
  }),
}));
vi.mock("../components/ConferenceModals", () => ({
  CreateConference: () => null,
  JoinByLink: () => null,
}));

function SearchState() {
  return (
    <span hidden data-testid="location-search">
      {useLocation().search}
    </span>
  );
}

function showDashboard() {
  render(
    <MemoryRouter initialEntries={["/conferences?view=upcoming&source=invite"]}>
      <Routes>
        <Route
          path="/conferences"
          element={
            <main>
              <Dashboard all />
              <SearchState />
            </main>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

describe("вкладки встреч", () => {
  it("переключает статус стрелками, Home и End без потери других параметров", async () => {
    const user = userEvent.setup();
    showDashboard();
    const upcoming = screen.getByRole("tab", { name: "Предстоящие" });
    const active = screen.getByRole("tab", { name: "Активные" });
    const past = screen.getByRole("tab", { name: "Завершённые" });
    expect(upcoming).toHaveAttribute("tabindex", "0");
    expect(active).toHaveAttribute("tabindex", "-1");
    upcoming.focus();
    await user.keyboard("{ArrowRight}");
    expect(active).toHaveFocus();
    expect(active).toHaveAttribute("aria-selected", "true");
    expect(screen.getByTestId("location-search")).toHaveTextContent(
      "source=invite",
    );
    await user.keyboard("{End}");
    expect(past).toHaveFocus();
    expect(past).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{Home}");
    expect(upcoming).toHaveFocus();
    expect(upcoming).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toHaveAttribute("tabindex", "0");
  });

  it("проходит автоматические правила WCAG для списка встреч", async () => {
    showDashboard();
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
