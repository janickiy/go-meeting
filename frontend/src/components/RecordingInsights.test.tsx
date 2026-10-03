import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api, ApiError } from "../api";
import { RecordingInsights } from "./RecordingInsights";
import type {
  ConferenceRecording,
  Participant,
  SummaryView,
  TranscriptView,
} from "../types";

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "user" } }) }));

const member = {
  id: "member",
  userId: "user",
  role: "participant",
  status: "left",
  admissionState: "admitted",
} as Participant;
const record = {
  uuid: "record",
  conferenceId: "room",
  status: "ready",
  createdAt: "2026-10-01T10:00:00Z",
  files: [
    {
      fileType: "final_mp4",
      url: "https://storage.test/record.mp4?signature=one",
    },
  ],
} as ConferenceRecording;
const transcript = {
  status: "success",
  enabled: true,
  canRetry: false,
  providerMode: "mock",
  item: {
    id: "transcript",
    conferenceId: "room",
    recordingId: "record",
    status: "ready",
    language: "ru",
    provider: "mock",
    createdAt: "now",
    updatedAt: "now",
  },
} as TranscriptView;
let clients: QueryClient[] = [];

/**
 * Монтирует материалы с отдельным кешем и адресом выбранной записи.
 * @args url — адрес с глубокой ссылкой; membership — проверяемые права.
 * @return Кеш и контейнер для проверки позиции видео.
 */
function show(url = "/conferences/room?recording=record", membership = member) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[url]}>
          <RecordingInsights
            conferenceId="room"
            membership={membership}
            recordings={[record]}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    ),
  };
}

beforeEach(() => {
  vi.spyOn(api, "capabilities").mockResolvedValue({
    status: "success",
    buildVersion: "test",
    capabilities: {
      liveCaptions: false,
      transcription: true,
      aiSummary: true,
      semanticSearch: false,
      meetingAnalytics: false,
      recordingModes: ["composite"],
    },
  });
  vi.spyOn(api, "recording").mockResolvedValue({
    status: "success",
    item: record,
  });
  vi.spyOn(api, "transcript").mockResolvedValue(transcript);
  vi.spyOn(api, "summary").mockResolvedValue({
    status: "success",
    item: null,
    enabled: false,
    canRegenerate: false,
    providerMode: "noop",
  });
  vi.spyOn(api, "transcriptSegments").mockResolvedValue({
    status: "success",
    items: [
      {
        id: "segment",
        transcriptId: "transcript",
        startMs: 42500,
        endMs: 44000,
        speakerLabel: "Говорящий 1",
        text: "<img src=x onerror=alert(1)> Обсуждение запуска",
      },
    ],
    total: 1,
    limit: 100,
    offset: 0,
  });
});
afterEach(() => {
  clients.forEach((client) => client.clear());
  clients = [];
  vi.restoreAllMocks();
});

describe("приватные материалы записи", () => {
  it("переходит к миллисекундам глубокой ссылки только после metadata", async () => {
    const { container } = show(
      "/conferences/room?recording=record&t=42500&segment=segment",
    );
    await screen.findByText(/Обсуждение запуска/);
    const video = container.querySelector("video")!;
    expect(video.currentTime).toBe(0);
    Object.defineProperties(video, {
      readyState: { value: 1, configurable: true },
      duration: { value: 90, configurable: true },
    });
    fireEvent.loadedMetadata(video);
    expect(video.currentTime).toBe(42.5);
    expect(video.autoplay).toBe(false);
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText(/Тестовые данные/)).toBeInTheDocument();
  });
  it("кнопка времени ограничивает позицию длительностью записи", async () => {
    const { container } = show();
    const button = await screen.findByRole("button", {
      name: "Перейти к 00:42",
    });
    const video = container.querySelector("video")!;
    Object.defineProperties(video, {
      readyState: { value: 1, configurable: true },
      duration: { value: 30, configurable: true },
    });
    fireEvent.click(button);
    expect(video.currentTime).toBe(30);
  });
  it("загружает итоги ИИ только после открытия вкладки", async () => {
    show();
    await screen.findByText(/Обсуждение запуска/);
    expect(api.summary).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("tab", { name: "Итоги ИИ" }));
    await waitFor(() => expect(api.summary).toHaveBeenCalledTimes(1));
  });
  it("по ссылке на итоги не загружает расшифровку и сегменты заранее", async () => {
    show("/conferences/room?recording=record&tab=summary");
    await waitFor(() => expect(api.summary).toHaveBeenCalledTimes(1));
    expect(api.transcript).not.toHaveBeenCalled();
    expect(api.transcriptSegments).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("tab", { name: "Расшифровка" }));
    await screen.findByText(/Обсуждение запуска/);
    expect(api.transcript).toHaveBeenCalledTimes(1);
  });
  it("скрывает выключенные материалы и не запрашивает их даже по глубокой ссылке", async () => {
    vi.mocked(api.capabilities).mockResolvedValue({
      status: "success",
      buildVersion: "test",
      capabilities: {
        liveCaptions: false,
        transcription: false,
        aiSummary: false,
        semanticSearch: false,
        meetingAnalytics: false,
        recordingModes: ["composite"],
      },
    });
    const { container } = show(
      "/conferences/room?recording=record&tab=summary",
    );
    await screen.findByText(/Расшифровка отключена администратором/);
    expect(container.querySelector("video")).not.toBeNull();
    expect(screen.queryByRole("tab")).toBeNull();
    expect(api.summary).not.toHaveBeenCalled();
    expect(api.transcript).not.toHaveBeenCalled();
    expect(api.transcriptSegments).not.toHaveBeenCalled();
  });
  it("переключение вкладок не возвращает проигрывание к старой метке", async () => {
    const { container } = show("/conferences/room?recording=record&t=42500");
    await screen.findByText(/Обсуждение запуска/);
    const video = container.querySelector("video")!;
    Object.defineProperties(video, {
      readyState: { value: 1, configurable: true },
      duration: { value: 90, configurable: true },
    });
    fireEvent.loadedMetadata(video);
    video.currentTime = 60;
    fireEvent.click(screen.getByRole("tab", { name: "Итоги ИИ" }));
    expect(video.currentTime).toBe(60);
    fireEvent.keyDown(screen.getByRole("tab", { name: "Итоги ИИ" }), {
      key: "ArrowLeft",
    });
    expect(screen.getByRole("tab", { name: "Расшифровка" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(video.currentTime).toBe(60);
  });
  it("обновление signed URL в кеше не сбрасывает текущий источник", async () => {
    const { container, client } = show();
    await screen.findByText(/Обсуждение запуска/);
    const video = container.querySelector("video")!;
    const source = video.getAttribute("src");
    client.setQueryData(["recording-detail", "user", "room", "record"], {
      status: "success",
      item: {
        ...record,
        files: [
          {
            fileType: "final_mp4",
            url: "https://storage.test/record.mp4?signature=two",
          },
        ],
      },
    });
    await waitFor(() => expect(video.getAttribute("src")).toBe(source));
  });
  it("не даёт обычному участнику платные действия даже при canRetry", async () => {
    vi.mocked(api.transcript).mockResolvedValue({
      ...transcript,
      item: null,
      canRetry: true,
    });
    show();
    await screen.findByText("Расшифровка ещё не создана.");
    expect(
      screen.queryByRole("button", { name: "Создать расшифровку" }),
    ).toBeNull();
  });
  it("владелец завершённой встречи может явно повторить неуспешную обработку", async () => {
    vi.mocked(api.transcript).mockResolvedValue({
      ...transcript,
      item: { ...transcript.item!, status: "failed" },
      canRetry: true,
    });
    const retry = vi
      .spyOn(api, "retryTranscript")
      .mockResolvedValue(transcript);
    show(undefined, { ...member, role: "owner" });
    fireEvent.click(
      await screen.findByRole("button", { name: "Повторить расшифровку" }),
    );
    await waitFor(() => expect(retry).toHaveBeenCalledWith("room", "record"));
  });
  it("соорганизатор видит повтор только при разрешении сервера", async () => {
    vi.mocked(api.transcript).mockResolvedValue({
      ...transcript,
      item: null,
      canRetry: true,
    });
    const retry = vi
      .spyOn(api, "retryTranscript")
      .mockResolvedValue(transcript);
    show(undefined, { ...member, role: "co_host" });
    fireEvent.click(
      await screen.findByRole("button", { name: "Создать расшифровку" }),
    );
    await waitFor(() => expect(retry).toHaveBeenCalledWith("room", "record"));
  });
  it("не показывает организатору повтор, запрещённый лимитом сервера", async () => {
    vi.mocked(api.transcript).mockResolvedValue({
      ...transcript,
      item: null,
      canRetry: false,
    });
    show(undefined, { ...member, role: "co_host" });
    await screen.findByText("Расшифровка ещё не создана.");
    expect(
      screen.queryByRole("button", { name: "Создать расшифровку" }),
    ).toBeNull();
  });
  it("показывает отключённого провайдера без ложной обработки", async () => {
    vi.mocked(api.transcript).mockResolvedValue({
      ...transcript,
      item: null,
      providerMode: "noop",
      enabled: false,
    });
    show();
    await screen.findByText(/Расшифровка отключена/);
    expect(screen.queryByText("В очереди на обработку")).toBeNull();
  });
  it("после отказа в доступе не показывает видео и не читает расшифровку", async () => {
    vi.mocked(api.recording).mockRejectedValue(
      new ApiError(403, "Нет доступа"),
    );
    const { container } = show();
    await screen.findByText("Нет доступа");
    expect(container.querySelector("video")).toBeNull();
    expect(api.transcript).not.toHaveBeenCalled();
  });
  it("не запрашивает материалы ожидающего допуска участника", () => {
    const { container } = show(undefined, {
      ...member,
      admissionState: "waiting",
      status: "waiting",
    });
    expect(container.querySelector("video")).toBeNull();
    expect(api.recording).not.toHaveBeenCalled();
  });
  it("не выдумывает ответственного и срок в итогах", async () => {
    vi.mocked(api.summary).mockResolvedValue({
      status: "success",
      enabled: true,
      canRegenerate: false,
      providerMode: "http",
      item: {
        id: "summary",
        conferenceId: "room",
        transcriptId: "transcript",
        status: "ready",
        summary: "Обсудили выпуск",
        keyPoints: ["Нужна проверка"],
        actionItems: [
          {
            text: "Проверить выпуск",
            assignee: null,
            dueDate: null,
            sourceSegmentIds: ["segment"],
          },
        ],
        topics: ["Выпуск"],
        provider: "http",
        model: "test",
        promptVersion: "1",
        schemaVersion: "1",
      },
    } as SummaryView);
    show("/conferences/room?recording=record&tab=summary");
    await screen.findByText("Ответственный: не указан · Срок: не указан");
    expect(
      screen.getByText(/Автоматические итоги могут содержать ошибки/),
    ).toBeInTheDocument();
  });
});
