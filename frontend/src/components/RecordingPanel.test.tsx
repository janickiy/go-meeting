import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RecordingPanel } from "./RecordingPanel";
import { api, ApiError } from "../api";
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
 *   - onStarted — наблюдаемый обработчик успешного запуска записи.
 *
 * @return Изолированный клиент запросов для очистки после проверки.
 */
function show(
  role: Participant["role"],
  items: ConferenceRecording[],
  onStarted?: () => void,
  load?: typeof api.recordings,
) {
  const recordings = vi.spyOn(api, "recordings");
  if (load) recordings.mockImplementation(load);
  else recordings.mockResolvedValue({ status: "success", items });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RecordingPanel
        conference={conference}
        membership={{ ...membership, role }}
        onStarted={onStarted}
      />
    </QueryClientProvider>,
  );
  return client;
}
/** Дожидается подтверждённого свободного состояния записи перед нажатием в тесте. */
async function readyStart() {
  const button = await screen.findByRole("button", { name: "Начать запись" });
  await waitFor(() => expect(button).toBeEnabled());
  return button;
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
  it("по умолчанию сохраняет готовую запись, превью и скачивание файлов в отдельной панели", async () => {
    const ready: ConferenceRecording = {
      ...row,
      status: "ready",
      files: [
        { fileType: "final_mp4", url: "https://files.example.test/record.mp4" },
        {
          fileType: "preview_jpg",
          url: "https://files.example.test/preview.jpg",
        },
        {
          fileType: "tracks_archive",
          url: "https://files.example.test/tracks.zip",
        },
      ],
    };
    const client = show("owner", [ready]);
    expect(await screen.findByTestId("recording-record")).toHaveTextContent(
      "Запись готова",
    );
    expect(
      screen.getByRole("link", { name: "Посмотреть превью" }),
    ).toHaveAttribute("href", ready.files[1].url);
    expect(screen.getByRole("link", { name: "Скачать MP4" })).toHaveAttribute(
      "href",
      ready.files[0].url,
    );
    expect(
      screen.getByRole("link", { name: "Скачать дорожки и манифест (ZIP)" }),
    ).toHaveAttribute("href", ready.files[2].url);
    expect(document.querySelector(".recording-list")).not.toBeNull();
    expect(await readyStart()).toBeEnabled();
    client.clear();
  });
  it("уведомляет о запуске только после успешного ответа сервера", async () => {
    let complete!: (
      response: Awaited<ReturnType<typeof api.startRecording>>,
    ) => void;
    const start = vi.spyOn(api, "startRecording").mockImplementation(
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    );
    const onStarted = vi.fn();
    const client = show("owner", [], onStarted);
    fireEvent.click(await readyStart());
    await waitFor(() =>
      expect(start).toHaveBeenCalledWith("room", "composite"),
    );
    expect(onStarted).not.toHaveBeenCalled();
    expect(
      screen.getByRole("button", { name: "Начать запись" }),
    ).toBeDisabled();
    complete({ status: "success", item: { ...row, status: "starting" } });
    await waitFor(() => expect(onStarted).toHaveBeenCalledOnce());
    client.clear();
  });
  it("не уведомляет об успешном запуске при ошибке сервера", async () => {
    vi.spyOn(api, "startRecording").mockRejectedValue(
      new ApiError(409, "Запуск отклонён"),
    );
    const onStarted = vi.fn();
    const client = show("owner", [], onStarted);
    fireEvent.click(await readyStart());
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Запуск отклонён",
    );
    expect(onStarted).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Начать запись" }),
      ).toBeEnabled(),
    );
    client.clear();
  });
  it("не вызывает обработчик запуска после успешной остановки записи", async () => {
    const stopped = { ...row, status: "stopping" } as ConferenceRecording;
    const items = [row];
    const stop = vi.spyOn(api, "stopRecording").mockImplementation(async () => {
      items[0] = stopped;
      return { status: "success", item: stopped };
    });
    const onStarted = vi.fn();
    const client = show("owner", items, onStarted);
    fireEvent.click(
      await screen.findByRole("button", { name: "Остановить запись" }),
    );
    await waitFor(() => expect(stop).toHaveBeenCalledWith("room", "record"));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Остановить запись" }),
      ).toBeDisabled(),
    );
    expect(onStarted).not.toHaveBeenCalled();
    client.clear();
  });
  it.each([
    "starting",
    "recording",
    "degraded",
    "stopping",
    "processing",
  ] as const)(
    "не предлагает новую запись при незавершённом состоянии %s",
    async (status) => {
      const client = show("owner", [{ ...row, status }]);
      await screen.findByTestId("recording-indicator");
      expect(
        screen.queryByRole("button", { name: "Начать запись" }),
      ).toBeNull();
      expect(
        screen.queryByRole("combobox", { name: "Режим записи" }),
      ).toBeNull();
      client.clear();
    },
  );
  it("не разрешает запуск до ответа запроса состояния", async () => {
    const start = vi.spyOn(api, "startRecording");
    let complete!: (value: Awaited<ReturnType<typeof api.recordings>>) => void;
    const client = show(
      "owner",
      [],
      undefined,
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    );
    const button = screen.getByRole("button", { name: "Начать запись" });
    expect(button).toBeDisabled();
    fireEvent.click(button);
    expect(start).not.toHaveBeenCalled();
    complete({ status: "success", items: [row] });
    await screen.findByTestId("recording-indicator");
    expect(screen.queryByRole("button", { name: "Начать запись" })).toBeNull();
    client.clear();
  });
  it("запрещает запуск, если состояние записи не удалось загрузить", async () => {
    const start = vi.spyOn(api, "startRecording");
    const client = show("owner", [], undefined, async () => {
      throw new ApiError(503, "Состояние записи недоступно");
    });
    await screen.findByRole("alert");
    const button = screen.getByRole("button", { name: "Начать запись" });
    expect(button).toBeDisabled();
    fireEvent.click(button);
    expect(start).not.toHaveBeenCalled();
    client.clear();
  });
  it("не отправляет повторный запуск во время ожидающего запроса", async () => {
    let complete!: (
      value: Awaited<ReturnType<typeof api.startRecording>>,
    ) => void;
    const start = vi.spyOn(api, "startRecording").mockImplementation(
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    );
    const client = show("owner", []);
    const button = await readyStart();
    fireEvent.click(button);
    fireEvent.click(button);
    await waitFor(() => expect(start).toHaveBeenCalledOnce());
    vi.mocked(api.recordings).mockResolvedValue({
      status: "success",
      items: [{ ...row, status: "starting" }],
    });
    complete({ status: "success", item: { ...row, status: "starting" } });
    await waitFor(() =>
      expect(client.getQueryData(["recordings", "room"])).toMatchObject({
        items: [
          expect.objectContaining({ uuid: "record", status: "starting" }),
        ],
      }),
    );
    client.clear();
  });
});
