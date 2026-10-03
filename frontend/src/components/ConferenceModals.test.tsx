import { describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { CreateConference } from "./ConferenceModals";

/** @args route — маршрут формы с начальными параметрами планирования и возврата. */
function show(route: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[route]}>
        <Routes>
          <Route path="/meetings/new" element={<CreateConference />} />
          <Route path="/calendar" element={<h1>Календарь возврата</h1>} />
          <Route path="/app" element={<h1>Главная возврата</h1>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("планирование из календаря", () => {
  it("включает расписание и корректное выбранное местное время", () => {
    const future = new Date();
    future.setDate(future.getDate() + 5);
    const at = `${future.getFullYear()}-${String(future.getMonth() + 1).padStart(2, "0")}-${String(future.getDate()).padStart(2, "0")}T09:30`;
    show(
      `/meetings/new?scheduled=1&returnTo=calendar&at=${encodeURIComponent(at)}`,
    );
    expect(
      screen.getByRole("checkbox", { name: /Запланировать встречу/ }),
    ).toBeChecked();
    expect(screen.getByLabelText("Дата и время")).toHaveValue(at);
    fireEvent.click(screen.getByRole("button", { name: "Отмена" }));
    expect(
      screen.getByRole("heading", { name: "Календарь возврата" }),
    ).toBeInTheDocument();
  });
  it("не использует произвольный внешний адрес возврата или некорректную дату", () => {
    show("/meetings/new?scheduled=1&returnTo=https://other.example&at=wrong");
    expect(screen.getByLabelText("Дата и время")).not.toHaveValue("wrong");
    fireEvent.click(screen.getByRole("button", { name: "Отмена" }));
    expect(
      screen.getByRole("heading", { name: "Главная возврата" }),
    ).toBeInTheDocument();
  });
});
