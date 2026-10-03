import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RecordingPanel } from "./RecordingPanel";
import { api } from "../api";
import type { Conference, Participant, ConferenceRecording } from "../types";

const capabilityState = vi.hoisted(() => ({
  modes: ["composite", "audio_only", "individual_tracks", "screen_focus"],
}));
vi.mock("../useCapabilities", () => ({
  useCapabilities: () => ({
    data: { capabilities: { recordingModes: capabilityState.modes } },
  }),
}));

const conference = { id: "room", status: "active" } as Conference;
const membership = {
  id: "member",
  role: "participant",
  status: "joined",
} as Participant;
const row = {
  uuid: "record",
  mode: "composite",
  conferenceId: "room",
  status: "recording",
  createdAt: "2026-10-01T10:00:00Z",
  files: [],
} as ConferenceRecording;
/**
 * show монтирует проверяемый компонент с изолированными провайдерами.
 *
 * @args
 *   - role (Participant["role"]) — роль участника и его полномочия.
 *   - items (ConferenceRecording[]) — элементы результата для объединения или отображения.
 *
 * @returns вычисленное значение: client.
 */
function show(role: Participant["role"], items: ConferenceRecording[]) {
  vi.spyOn(api, "recordings").mockResolvedValue({ status: "success", items });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RecordingPanel
        conference={conference}
        membership={{ ...membership, role }}
      />
    </QueryClientProvider>,
  );
  return client;
}
afterEach(
  /**
   * Обработчик afterEach выполняет переданный шаг вызова afterEach в проверках клиентского поведения.
   *
   *
   * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ () => {
    cleanup();
    vi.restoreAllMocks();
    capabilityState.modes = [
      "composite",
      "audio_only",
      "individual_tracks",
      "screen_focus",
    ];
  },
);
describe("conference recording controls", /**
 * Проверяет управление записью конференции.
 *
 *
 * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ () => {
  it("shows active recording to participants but exposes no recording command", /**
   * Проверяет показ активной записи участникам без доступа к командам записи.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const client = show("participant", [row]);
    expect(await screen.findByTestId("recording-indicator")).toHaveTextContent(
      "Идёт запись",
    );
    expect(screen.queryByRole("button", { name: /запись/ })).toBeNull();
    client.clear();
  });
  it("allows owner stop and does not duplicate start while recording", /**
   * Проверяет остановку владельцем и отсутствие повторного запуска во время записи.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const client = show("owner", [row]);
    expect(
      await screen.findByRole("button", { name: "Остановить запись" }),
    ).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Начать запись" })).toBeNull();
    client.clear();
  });
  it("keeps cohost recording permission owner-only", /**
   * Проверяет, что соведущий не получает права владельца на запись.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */ async () => {
    const client = show("co_host", []);
    await screen.findByText(/Записей пока нет/);
    expect(screen.queryByRole("button", { name: "Начать запись" })).toBeNull();
    client.clear();
  });
  it("shows only recording modes enabled by server capabilities", async () => {
    capabilityState.modes = ["composite"];
    const client = show("owner", []);
    const modes = await screen.findByRole("combobox", { name: "Режим записи" });
    expect(modes.querySelectorAll("option")).toHaveLength(1);
    expect(modes).toHaveValue("composite");
    client.clear();
  });
  it("does not offer recording when the server exposes no modes", async () => {
    capabilityState.modes = [];
    const client = show("owner", []);
    expect(
      await screen.findByText("Режимы записи сейчас недоступны."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Начать запись" })).toBeNull();
    client.clear();
  });
});
