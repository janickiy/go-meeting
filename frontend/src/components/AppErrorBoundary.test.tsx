import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { AppErrorBoundary } from "./AppErrorBoundary";
import { reportClientError } from "../clientTelemetry";

vi.mock("../clientTelemetry", () => ({ reportClientError: vi.fn() }));

it("сохраняет доступный экран восстановления и передаёт только источник отказа", () => {
  const failure = new Error("private transcript");
  function Broken(): never {
    throw failure;
  }
  vi.spyOn(console, "error").mockImplementation(() => {});
  render(
    <AppErrorBoundary>
      <Broken />
    </AppErrorBoundary>,
  );
  expect(
    screen.getByRole("heading", { name: "Не удалось открыть страницу" }),
  ).toBeVisible();
  expect(
    screen.getByRole("button", { name: "Обновить страницу" }),
  ).toBeVisible();
  expect(screen.queryByText("private transcript")).toBeNull();
  expect(reportClientError).toHaveBeenCalledWith("react_render_error", failure);
});
