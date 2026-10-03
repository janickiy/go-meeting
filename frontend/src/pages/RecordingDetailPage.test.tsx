import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useNavigate } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type {
  Conference,
  ConferenceHistory,
  ConferenceRecording,
} from "../types";
import { RecordingDetailPage } from "./RecordingDetailPage";

const auth = vi.hoisted(() => ({
  user: { id: "user-one" } as { id: string } | null,
}));
vi.mock("../auth", () => ({ useAuth: () => auth }));

const conferenceId = "a81c47c4-0b89-4494-8d04-021ea72cf001";
const recordingId = "f630a13a-cd7d-44eb-99b7-828acb131001";
const otherConferenceId = "a81c47c4-0b89-4494-8d04-021ea72cf002";
const otherRecordingId = "f630a13a-cd7d-44eb-99b7-828acb131002";
const detailUrl = `/recordings/${recordingId}?conference=${conferenceId}`;
const meeting = {
  id: conferenceId,
  title: "Стратегическая сессия Q2",
  ownerId: "owner",
  status: "finished",
  createdAt: "2026-10-01T10:00:00Z",
  startedAt: "2026-10-01T10:00:00Z",
  finishedAt: "2026-10-01T11:00:00Z",
} as Conference;
const record: ConferenceRecording = {
  uuid: recordingId,
  conferenceId,
  mode: "composite",
  status: "ready",
  createdAt: "2026-10-01T10:04:00Z",
  durationSec: 3600,
  files: [
    {
      fileType: "final_mp4",
      url: "https://media.example.test/video?signature=allowed",
      sizeBytes: 25_165_824,
    },
    {
      fileType: "final_audio",
      url: "/private-media/audio?signature=allowed",
      sizeBytes: 1_048_576,
    },
    {
      fileType: "preview_jpg",
      url: "https://media.example.test/preview?signature=allowed",
      sizeBytes: 65_536,
    },
    {
      fileType: "tracks_archive",
      url: "/private-media/archive?signature=allowed",
      sizeBytes: 50_331_648,
    },
  ],
};
const history: ConferenceHistory = {
  conference: meeting,
  owner: { id: "owner", displayName: "Александр Иванов" },
  durationSec: 5400,
  participantCount: 12,
  participants: [],
  participantsTruncated: false,
  recordings: { total: 1, ready: 1, processing: 0, failed: 0 },
  chatAvailable: true,
  chatReadOnly: true,
};
const clients: QueryClient[] = [];

/** NavigationControls меняет контекст ссылки без перезапуска общего кеша теста. */
function NavigationControls() {
  const navigate = useNavigate();
  return (
    <button
      onClick={() =>
        navigate(
          `/recordings/${otherRecordingId}?conference=${otherConferenceId}`,
        )
      }
    >
      Другая запись
    </button>
  );
}

/** show монтирует приватную страницу записи с изолированным либо переданным кешем запросов.
 * @args url — проверяемая ссылка; client — общий кеш для проверки разделения доступа.
 * @return контейнер страницы и средство повторной отрисовки после изменения текущей учётной записи.
 */
function show(
  url = detailUrl,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  clients.push(client);
  const tree = () => (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[url]}>
        <NavigationControls />
        <Routes>
          <Route path="/recordings/:id" element={<RecordingDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
  const mounted = render(tree());
  return { ...mounted, client, refreshAuth: () => mounted.rerender(tree()) };
}

beforeEach(() => {
  auth.user = { id: "user-one" };
  vi.spyOn(api, "conference").mockResolvedValue({
    status: "success",
    item: meeting,
  });
  vi.spyOn(api, "recording").mockResolvedValue({
    status: "success",
    item: record,
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

describe("отдельная страница записи", () => {
  it("показывает ожидание, не читая запись до подтверждения встречи", async () => {
    vi.mocked(api.conference).mockReturnValue(new Promise(() => {}));
    show();
    expect(screen.getByRole("status")).toHaveTextContent("Загружаем");
    expect(api.conference).toHaveBeenCalledWith(
      conferenceId,
      expect.any(AbortSignal),
    );
    expect(api.recording).not.toHaveBeenCalled();
    expect(api.history).not.toHaveBeenCalled();
  });

  it("для неполной ссылки предлагает записи и не отправляет запросы", () => {
    show(`/recordings/${recordingId}`);
    expect(
      screen.getByRole("heading", { name: "Не удалось открыть запись" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "К записям" })).toHaveAttribute(
      "href",
      "/recordings",
    );
    expect(api.conference).not.toHaveBeenCalled();
    expect(api.recording).not.toHaveBeenCalled();
    expect(api.history).not.toHaveBeenCalled();
  });

  it("не читает материалы без авторизации", () => {
    auth.user = null;
    show();
    expect(
      screen.getByRole("heading", { name: "Войдите, чтобы открыть запись" }),
    ).toBeInTheDocument();
    expect(api.conference).not.toHaveBeenCalled();
    expect(api.recording).not.toHaveBeenCalled();
  });

  it("показывает настоящий видеоплеер, серверные материалы и историческую сводку без лишних вкладок", async () => {
    const { container } = show();
    await screen.findByRole("heading", { name: meeting.title });
    const video = screen.getByLabelText("Видеозапись встречи");
    expect(video.tagName).toBe("VIDEO");
    expect(video).toHaveAttribute("src", record.files[0].url);
    expect(video).toHaveAttribute("poster", record.files[2].url);
    expect(video).toHaveAttribute("controls");
    expect(video).toHaveAttribute("playsinline");
    expect(video).toHaveAttribute("preload", "metadata");
    expect(video).not.toHaveAttribute("autoplay");
    expect(
      screen.getByRole("link", { name: "Назад к записям" }),
    ).toHaveAttribute("href", `/recordings?conference=${conferenceId}`);
    const labels = [
      "Видеозапись (MP4)",
      "Аудиозапись",
      "Превью записи",
      "Архив дорожек",
    ];
    labels.forEach((name, index) => {
      const link = screen.getByRole("link", { name: `Скачать: ${name}` });
      expect(link).toHaveAttribute("href", record.files[index].url);
      expect(link).toHaveAttribute("download", "");
    });
    expect(await screen.findByText("Александр Иванов")).toBeInTheDocument();
    expect(screen.getByText("Участники встречи")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getAllByText("1:00:00")).toHaveLength(2);
    expect(screen.queryByText("1:30:00")).not.toBeInTheDocument();
    expect(
      container.querySelectorAll("[role=tab], [role=tablist]"),
    ).toHaveLength(0);
    expect(
      screen.queryByText(/Транскрипт|Итоги ИИ|Аналитика|Обработка|ready/),
    ).not.toBeInTheDocument();
    expect(api.recording).toHaveBeenCalledWith(
      conferenceId,
      recordingId,
      expect.any(AbortSignal),
    );
    expect(api.history).toHaveBeenCalledWith(
      conferenceId,
      expect.any(AbortSignal),
    );
    const materials = screen.getByRole("region", { name: "Материалы записи" });
    expect(materials.closest("aside")).toHaveClass("recording-detail-sidebar");
    expect(materials).toHaveClass("recording-detail-section");
    expect(
      container.querySelector(
        ".recording-detail-main .recording-detail-materials",
      ),
    ).toBeNull();
    expect(
      screen.getByText(
        "Если файл открылся в браузере, сохраните его через меню плеера или браузера.",
      ),
    ).toBeInTheDocument();
  });

  it("для аудиорежима выбирает настоящую аудиодорожку даже при наличии MP4", async () => {
    vi.mocked(api.recording).mockResolvedValue({
      status: "success",
      item: { ...record, mode: "audio_only" },
    });
    const { container } = show();
    await screen.findByRole("heading", { name: meeting.title });
    const audio = screen.getByLabelText("Аудиозапись встречи");
    expect(audio.tagName).toBe("AUDIO");
    expect(audio).toHaveAttribute("src", record.files[1].url);
    expect(audio).toHaveAttribute("controls");
    expect(container.querySelector("video")).toBeNull();
  });

  it.each([
    "starting",
    "recording",
    "stopping",
    "processing",
    "failed",
    "cancelled",
  ] as const)(
    "не воспроизводит и не показывает техническое состояние %s",
    async (status) => {
      vi.mocked(api.recording).mockResolvedValue({
        status: "success",
        item: {
          ...record,
          status,
          errorMessage: "FFmpeg failed with private secret",
        },
      });
      const { container } = show();
      await screen.findByRole("heading", { name: "Запись пока недоступна" });
      expect(container.querySelector("video, audio")).toBeNull();
      expect(container.textContent).not.toContain(status);
      expect(container.textContent).not.toContain("private secret");
      expect(
        screen.queryByRole("link", { name: "Скачать запись" }),
      ).not.toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Обновить запись" }),
      ).toBeInTheDocument();
      expect(api.recording).toHaveBeenCalledTimes(1);
    },
  );

  it("не конструирует файлы из ключей хранилища и не вставляет опасные URL", async () => {
    vi.mocked(api.recording).mockResolvedValue({
      status: "success",
      item: {
        ...record,
        files: [
          { fileType: "final_mp4", url: "javascript:alert(1)" },
          { fileType: "final_audio", url: "//evil.example/audio" },
          {
            fileType: "preview_jpg",
            url: "https://name:secret@media.example/image",
          },
          { fileType: "tracks_archive" },
          {
            fileType: "internal_manifest",
            url: "https://media.example/internal",
          },
          { fileType: "toString", url: "https://media.example/internal" },
        ],
      },
    });
    const { container } = show();
    await screen.findByRole("heading", { name: "Запись пока недоступна" });
    expect(container.querySelector("video, audio, img")).toBeNull();
    expect(
      screen.queryByRole("link", { name: /Скачать/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("Доступных материалов пока нет."),
    ).toBeInTheDocument();
  });

  it.each([403, 404, 500])(
    "санитизирует отказ %s, не раскрывая заголовок встречи или кешированные файлы",
    async (status) => {
      vi.mocked(api.recording).mockRejectedValue(
        new ApiError(status, "private bucket and secret"),
      );
      const { container } = show();
      await screen.findByRole("heading", { name: "Не удалось открыть запись" });
      expect(screen.getByRole("alert")).toHaveTextContent("Запись недоступна");
      expect(container.textContent).not.toContain("private bucket");
      expect(screen.queryByText(meeting.title)).not.toBeInTheDocument();
      expect(container.querySelector("video, audio")).toBeNull();
      expect(api.history).not.toHaveBeenCalled();
    },
  );

  it("не запрашивает запись после отказа доступа к встрече", async () => {
    vi.mocked(api.conference).mockRejectedValue(
      new ApiError(403, "private details"),
    );
    show();
    await screen.findByRole("heading", { name: "Не удалось открыть запись" });
    expect(api.recording).not.toHaveBeenCalled();
    expect(api.history).not.toHaveBeenCalled();
  });

  it.each(["active", "scheduled", "created"] as const)(
    "не загружает историю для незавершённой встречи %s",
    async (status) => {
      vi.mocked(api.conference).mockResolvedValue({
        status: "success",
        item: { ...meeting, status },
      });
      show();
      await screen.findByRole("heading", { name: meeting.title });
      expect(api.history).not.toHaveBeenCalled();
      expect(screen.queryByText("Организатор встречи")).not.toBeInTheDocument();
    },
  );

  it("не блокирует просмотр, если необязательная история недоступна", async () => {
    vi.mocked(api.history).mockRejectedValue(
      new ApiError(403, "private history"),
    );
    const { container } = show();
    await screen.findByLabelText("Видеозапись встречи");
    await waitFor(() => expect(api.history).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(container.textContent).not.toContain("private history");
    expect(screen.queryByText("Организатор встречи")).not.toBeInTheDocument();
  });

  it("читает разрешённую сводку отменённой встречи без предположений о её участниках", async () => {
    vi.mocked(api.conference).mockResolvedValue({
      status: "success",
      item: { ...meeting, status: "cancelled" },
    });
    show();
    await screen.findByLabelText("Видеозапись встречи");
    expect(await screen.findByText("Александр Иванов")).toBeInTheDocument();
    expect(screen.getByText("Участники встречи")).toBeInTheDocument();
    expect(api.history).toHaveBeenCalledTimes(1);
  });

  it("обновляет истёкшую ссылку только по явному нажатию и не зацикливает запросы", async () => {
    show();
    const video = await screen.findByLabelText("Видеозапись встречи");
    fireEvent.error(video);
    expect(
      screen.queryByLabelText("Видеозапись встречи"),
    ).not.toBeInTheDocument();
    expect(api.recording).toHaveBeenCalledTimes(1);
    vi.mocked(api.recording).mockResolvedValue({
      status: "success",
      item: {
        ...record,
        files: [
          {
            ...record.files[0],
            url: "https://media.example.test/video?signature=renewed",
          },
        ],
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Обновить ссылку" }));
    const renewed = await screen.findByLabelText("Видеозапись встречи");
    expect(renewed).toHaveAttribute(
      "src",
      "https://media.example.test/video?signature=renewed",
    );
    expect(api.recording).toHaveBeenCalledTimes(2);
    expect(api.conference).toHaveBeenCalledTimes(2);
    fireEvent.error(renewed);
    expect(api.recording).toHaveBeenCalledTimes(2);
    expect(
      screen.getByRole("button", { name: "Обновить ссылку" }),
    ).toBeInTheDocument();
  });

  it("скрывает ранее показанные материалы, если обновление отозвало доступ", async () => {
    const { container } = show();
    const video = await screen.findByLabelText("Видеозапись встречи");
    fireEvent.error(video);
    vi.mocked(api.recording).mockRejectedValue(
      new ApiError(403, "no longer allowed"),
    );
    fireEvent.click(screen.getByRole("button", { name: "Обновить ссылку" }));
    await screen.findByRole("heading", { name: "Не удалось открыть запись" });
    expect(container.querySelector("video, audio")).toBeNull();
    expect(
      screen.queryByRole("link", { name: /Скачать/ }),
    ).not.toBeInTheDocument();
    expect(container.textContent).not.toContain(meeting.title);
  });

  it("блокирует повторный запрос обновления, пока проверяется доступ", async () => {
    show();
    const video = await screen.findByLabelText("Видеозапись встречи");
    fireEvent.error(video);
    vi.mocked(api.conference).mockReturnValue(new Promise(() => {}));
    const refresh = screen.getByRole("button", { name: "Обновить ссылку" });
    fireEvent.click(refresh);
    expect(refresh).toBeDisabled();
    fireEvent.click(refresh);
    expect(api.conference).toHaveBeenCalledTimes(2);
    expect(api.recording).toHaveBeenCalledTimes(1);
  });

  it("не пересекает приватный кеш страницы с прежним списком записей встречи", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["recordings", conferenceId], {
      status: "success",
      items: [
        {
          ...record,
          files: [{ fileType: "final_mp4", url: "/private-media/old-user" }],
        },
      ],
    });
    const { container } = show(detailUrl, client);
    await screen.findByLabelText("Видеозапись встречи");
    expect(container.querySelector("video")).toHaveAttribute(
      "src",
      record.files[0].url,
    );
    expect(container.innerHTML).not.toContain("/private-media/old-user");
  });

  it("отбрасывает несоответствующий контекст серверного ответа", async () => {
    vi.mocked(api.recording).mockResolvedValue({
      status: "success",
      item: { ...record, conferenceId: otherConferenceId },
    });
    const { container } = show();
    await screen.findByRole("heading", { name: "Не удалось открыть запись" });
    expect(container.querySelector("video, audio")).toBeNull();
    expect(api.history).not.toHaveBeenCalled();
  });

  it("разделяет кеш и интерфейс при переходе к другой встрече", async () => {
    const { container, client } = show();
    await screen.findByLabelText("Видеозапись встречи");
    vi.mocked(api.conference).mockReturnValue(new Promise(() => {}));
    fireEvent.click(screen.getByRole("button", { name: "Другая запись" }));
    expect(container.querySelector("video, audio")).toBeNull();
    expect(container.textContent).not.toContain(meeting.title);
    expect(api.conference).toHaveBeenLastCalledWith(
      otherConferenceId,
      expect.any(AbortSignal),
    );
    expect(
      client.getQueryCache().find({
        queryKey: ["recording-detail", "user-one", conferenceId, recordingId],
      })?.state.data,
    ).toBeDefined();
  });

  it("не выдаёт материалы из кеша предыдущей учётной записи", async () => {
    const { container, client, refreshAuth } = show();
    await screen.findByLabelText("Видеозапись встречи");
    auth.user = { id: "user-two" };
    vi.mocked(api.conference).mockReturnValue(new Promise(() => {}));
    refreshAuth();
    await waitFor(() => expect(api.conference).toHaveBeenCalledTimes(2));
    expect(container.querySelector("video, audio")).toBeNull();
    expect(container.textContent).not.toContain(meeting.title);
    expect(
      client.getQueryCache().find({
        queryKey: ["recording-detail", "user-two", conferenceId, recordingId],
      })?.state.data,
    ).toBeUndefined();
  });
});
