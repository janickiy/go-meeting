import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { api, ApiError } from "../api";
import type { Conference, ConferenceRecording, Participant } from "../types";
import { ConferencePage } from "./ConferencePage";

const fixture = vi.hoisted(() => ({
  conference: {
    id: "room",
    title: "Командная встреча",
    status: "active",
    ownerId: "self",
    createdAt: "2026-10-01T10:00:00Z",
    inviteCode: "a".repeat(32),
  } as Conference,
  membership: {
    id: "member",
    conferenceId: "room",
    userId: "self",
    displayName: "Алиса",
    role: "owner",
    status: "joined",
    admissionState: "admitted",
  } as Participant,
  items: [] as ConferenceRecording[],
}));

vi.mock("../auth", () => ({ useAuth: () => ({ user: { id: "self" } }) }));
vi.mock("../queries", () => ({
  useConference: () => ({
    data: { item: fixture.conference },
    isPending: false,
    isError: false,
  }),
  useMembership: () => ({
    data: fixture.membership,
    isPending: false,
    isError: false,
  }),
  useParticipants: () => ({
    data: { pages: [{ items: [fixture.membership] }] },
    isPending: false,
    hasNextPage: false,
  }),
}));
vi.mock("../realtime", () => ({
  useRealtime: () => ({
    state: { connectionId: "connection", participants: [] },
    status: "Подключено",
    subscribe: () => () => {},
  }),
}));
vi.mock("../useCapabilities", () => ({
  useCapabilities: () => ({
    isSuccess: true,
    isError: false,
    data: {
      capabilities: {
        recordingModes: ["composite"],
        liveCaptions: false,
        meetingAnalytics: false,
      },
    },
  }),
}));
vi.mock("../components/RealtimePanel", () => ({
  RealtimePanel: () => <div>Медиа</div>,
}));
vi.mock("../components/ReactionsPanel", () => ({ ReactionsPanel: () => null }));
vi.mock("../components/ChatPanel", () => ({
  ChatPanel: () => <textarea aria-label="Сообщение" />,
}));
vi.mock("../components/CaptionsPanel", () => ({ CaptionsPanel: () => null }));
vi.mock("../components/AnalyticsPanel", () => ({ AnalyticsPanel: () => null }));
vi.mock("../components/WaitingRoomPanel", () => ({
  WaitingRoomPanel: () => null,
}));

const clients: QueryClient[] = [];
const row = {
  uuid: "record",
  conferenceId: "room",
  mode: "composite",
  status: "recording",
  createdAt: "2026-10-01T10:00:00Z",
  files: [],
} as ConferenceRecording;

beforeEach(() => {
  fixture.items = [];
  fixture.conference.status = "active";
  fixture.conference.ownerId = "self";
  fixture.membership.role = "owner";
  fixture.membership.status = "joined";
  fixture.membership.admissionState = "admitted";
  vi.spyOn(api, "recordings").mockImplementation(async () => ({
    status: "success",
    items: fixture.items,
  }));
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.reject(new Error("Внешние запросы запрещены в этой проверке")),
    ),
  );
});
afterEach(() => {
  expect(globalThis.fetch).not.toHaveBeenCalled();
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

/**
 * Монтирует настоящую страницу встречи с изолированным кешем и локальными ответами API.
 * @return Клиент запросов и средства управления смонтированной страницей.
 */
function showConference() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  clients.push(client);
  const view = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/conferences/room"]}>
        <Routes>
          <Route path="/conferences/:id" element={<ConferencePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { client, ...view };
}

/**
 * Открывает настоящий диалог ConferencePage с настоящей панелью записи и локальными ответами API.
 * @return Кнопка открытия и диалог для проверки закрытия и восстановления клавиатурного фокуса.
 */
async function openRecordingModal() {
  showConference();
  const trigger = screen.getByRole("button", {
    name: "Записи конференции",
  });
  trigger.focus();
  fireEvent.click(trigger);
  const dialog = await screen.findByRole("dialog", {
    name: "Записи конференции",
  });
  return { trigger, dialog };
}

it("после успешного запуска закрывает настоящий диалог и возвращает фокус на кнопку записи", async () => {
  let complete!: (
    response: Awaited<ReturnType<typeof api.startRecording>>,
  ) => void;
  const start = vi.spyOn(api, "startRecording").mockImplementation(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const { trigger, dialog } = await openRecordingModal();
  fireEvent.click(
    await within(dialog).findByRole("button", { name: "Начать запись" }),
  );
  await waitFor(() => expect(start).toHaveBeenCalledWith("room", "composite"));
  expect(dialog).toBeInTheDocument();
  expect(trigger).not.toHaveFocus();
  expect(
    within(dialog).getByRole("button", { name: "Начать запись" }),
  ).toBeDisabled();
  fixture.items = [{ ...row, status: "starting" }];
  complete({ status: "success", item: fixture.items[0] });
  await waitFor(() =>
    expect(
      screen.queryByRole("dialog", { name: "Записи конференции" }),
    ).not.toBeInTheDocument(),
  );
  expect(trigger).toHaveFocus();
  expect(await screen.findByTestId("recording-indicator")).toHaveTextContent(
    "Запись запускается",
  );
});

it("сохраняет диалог при отказе запуска и позволяет успешно повторить запрос", async () => {
  const start = vi
    .spyOn(api, "startRecording")
    .mockRejectedValueOnce(new ApiError(409, "Запуск отклонён"))
    .mockResolvedValueOnce({
      status: "success",
      item: { ...row, status: "starting" },
    });
  const { trigger, dialog } = await openRecordingModal();
  fireEvent.click(
    await within(dialog).findByRole("button", { name: "Начать запись" }),
  );
  expect(await within(dialog).findByRole("alert")).toHaveTextContent(
    "Запуск отклонён",
  );
  expect(dialog).toBeInTheDocument();
  expect(trigger).not.toHaveFocus();
  const retry = within(dialog).getByRole("button", { name: "Начать запись" });
  expect(retry).toBeEnabled();
  fireEvent.click(retry);
  await waitFor(() => expect(start).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(dialog).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
});

it("оставляет настоящий диалог открытым после успешной остановки записи", async () => {
  fixture.items = [row];
  const start = vi.spyOn(api, "startRecording");
  const stop = vi.spyOn(api, "stopRecording").mockImplementation(async () => {
    fixture.items = [{ ...row, status: "stopping" }];
    return { status: "success", item: fixture.items[0] };
  });
  const { trigger, dialog } = await openRecordingModal();
  const stopButton = await within(dialog).findByRole("button", {
    name: "Остановить запись",
  });
  expect(
    within(dialog).queryByRole("button", { name: "Начать запись" }),
  ).not.toBeInTheDocument();
  expect(dialog.querySelector(".recording-list")).toBeNull();
  fireEvent.click(stopButton);
  await waitFor(() => expect(stop).toHaveBeenCalledWith("room", "record"));
  await waitFor(() =>
    expect(within(dialog).getByTestId("recording-indicator")).toHaveTextContent(
      "Запись останавливается",
    ),
  );
  expect(dialog).toBeInTheDocument();
  expect(trigger).not.toHaveFocus();
  expect(start).not.toHaveBeenCalled();
});

it("настоящий диалог управления не показывает готовую запись, MP4, превью и ZIP, но разрешает новый запуск", async () => {
  fixture.items = [
    {
      ...row,
      uuid: "ready-history",
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
    },
  ];
  const { dialog } = await openRecordingModal();
  const start = await within(dialog).findByRole("button", {
    name: "Начать запись",
  });
  await waitFor(() => expect(start).toBeEnabled());
  expect(
    within(dialog).getByRole("combobox", { name: "Режим записи" }),
  ).toBeEnabled();
  expect(dialog.querySelector(".recording-list")).toBeNull();
  expect(
    within(dialog).queryByTestId("recording-ready-history"),
  ).not.toBeInTheDocument();
  expect(within(dialog).queryByText("Запись готова")).not.toBeInTheDocument();
  expect(within(dialog).queryByRole("link")).not.toBeInTheDocument();
  expect(
    within(dialog).queryByText(/Записей пока нет/),
  ).not.toBeInTheDocument();
});

it("диалог без истории не показывает пустой список и подсказку об отсутствии записей", async () => {
  const { dialog } = await openRecordingModal();
  const start = await within(dialog).findByRole("button", {
    name: "Начать запись",
  });
  await waitFor(() => expect(start).toBeEnabled());
  expect(dialog.querySelector(".recording-list")).toBeNull();
  expect(
    within(dialog).queryByText(/Записей пока нет/),
  ).not.toBeInTheDocument();
  expect(within(dialog).queryByRole("link")).not.toBeInTheDocument();
});

it.each(["starting", "recording", "degraded"] as const)(
  "останавливает запись в состоянии %s прямо из верхней панели без открытия диалога",
  async (status) => {
    fixture.items = [{ ...row, status }];
    const stop = vi.spyOn(api, "stopRecording").mockImplementation(async () => {
      fixture.items = [{ ...row, status: "stopping" }];
      return { status: "success", item: fixture.items[0] };
    });
    showConference();
    const stopButton = await screen.findByRole("button", {
      name: "Остановить запись",
    });
    const recordingsButton = screen.getByRole("button", {
      name: "Записи конференции",
    });
    expect(stopButton).toBeEnabled();
    expect(stopButton.querySelector("svg")).not.toBeNull();
    expect(
      stopButton.compareDocumentPosition(recordingsButton) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(stopButton);
    await waitFor(() => expect(stop).toHaveBeenCalledWith("room", "record"));
    await waitFor(() => expect(stopButton).toBeDisabled());
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(recordingsButton).toBeEnabled();
  },
);

it("блокирует повторную остановку при ожидании ответа, сразу применяет stopping и скрывает кнопку после завершения", async () => {
  fixture.items = [row];
  let complete!: (
    response: Awaited<ReturnType<typeof api.stopRecording>>,
  ) => void;
  const stop = vi.spyOn(api, "stopRecording").mockImplementation(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const { client } = showConference();
  const stopButton = await screen.findByRole("button", {
    name: "Остановить запись",
  });
  // Незавершённое обновление списка не должно задерживать подтверждённое состояние остановки.
  vi.mocked(api.recordings).mockImplementation(() => new Promise(() => {}));
  fireEvent.click(stopButton);
  await waitFor(() => expect(stop).toHaveBeenCalledOnce());
  await waitFor(() => expect(stopButton).toBeDisabled());
  expect(stopButton).toHaveTextContent("Останавливаем…");
  fireEvent.click(stopButton);
  fireEvent.click(stopButton);
  expect(stop).toHaveBeenCalledOnce();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  complete({ status: "success", item: { ...row, status: "stopping" } });
  await waitFor(() =>
    expect(
      client.getQueryData<{ items: ConferenceRecording[] }>([
        "recordings",
        "room",
      ])?.items[0].status,
    ).toBe("stopping"),
  );
  expect(screen.getByTestId("recording-indicator")).toHaveTextContent(
    "Запись останавливается",
  );
  expect(stopButton).toBeDisabled();
  expect(stopButton).toHaveTextContent("Останавливаем…");
  await act(async () => {
    client.setQueryData(["recordings", "room"], {
      status: "success",
      items: [{ ...row, status: "ready" }],
    });
  });
  await waitFor(() => expect(stopButton).not.toBeInTheDocument());
  expect(screen.queryByTestId("recording-indicator")).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Записи конференции" }),
  ).toBeEnabled();
  expect(stop).toHaveBeenCalledOnce();
});

it("показывает ошибку прямой остановки в комнате и разрешает повторить её без диалога", async () => {
  fixture.items = [row];
  const stop = vi
    .spyOn(api, "stopRecording")
    .mockRejectedValueOnce(new ApiError(409, "Остановка отклонена"))
    .mockImplementationOnce(async () => {
      fixture.items = [{ ...row, status: "stopping" }];
      return { status: "success", item: fixture.items[0] };
    });
  showConference();
  const stopButton = await screen.findByRole("button", {
    name: "Остановить запись",
  });
  fireEvent.click(stopButton);
  const error = await screen.findByRole("alert");
  expect(error).toHaveTextContent("Остановка отклонена");
  expect(error.closest(".room-errors")).not.toBeNull();
  expect(stopButton).toBeEnabled();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  fireEvent.click(stopButton);
  await waitFor(() => expect(stop).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(stopButton).toBeDisabled());
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

it.each(["participant", "co_host"] as const)(
  "не показывает верхнюю кнопку остановки для роли %s, сохраняя доступ к записям",
  async (role) => {
    fixture.items = [row];
    fixture.conference.ownerId = "other";
    fixture.membership.role = role;
    const stop = vi.spyOn(api, "stopRecording");
    showConference();
    expect(await screen.findByTestId("recording-indicator")).toHaveTextContent(
      "Идёт запись",
    );
    expect(
      screen.queryByRole("button", { name: "Остановить запись" }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Записи конференции" }));
    const dialog = await screen.findByRole("dialog", {
      name: "Записи конференции",
    });
    expect(
      within(dialog).queryByRole("button", { name: "Остановить запись" }),
    ).not.toBeInTheDocument();
    expect(stop).not.toHaveBeenCalled();
  },
);

it("оставляет уже останавливаемую запись заблокированной до серверного завершения", async () => {
  fixture.items = [{ ...row, status: "stopping" }];
  const stop = vi.spyOn(api, "stopRecording");
  showConference();
  const stopButton = await screen.findByRole("button", {
    name: /Остановить запись|Останавливаем/,
  });
  expect(stopButton).toBeDisabled();
  expect(stopButton).toHaveTextContent("Останавливаем…");
  fireEvent.click(stopButton);
  expect(stop).not.toHaveBeenCalled();
  expect(
    screen.getByRole("button", { name: "Записи конференции" }),
  ).toBeEnabled();
});

it("не разрешает остановку владельцу членства, если текущий пользователь не совпадает с владельцем конференции", async () => {
  fixture.items = [row];
  fixture.conference.ownerId = "other";
  const stop = vi.spyOn(api, "stopRecording");
  showConference();
  expect(await screen.findByTestId("recording-indicator")).toHaveTextContent(
    "Идёт запись",
  );
  expect(
    screen.queryByRole("button", { name: "Остановить запись" }),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Записи конференции" }),
  ).toBeEnabled();
  expect(stop).not.toHaveBeenCalled();
});

it("не выводит кнопку и индикатор записи другой конференции", async () => {
  fixture.items = [{ ...row, conferenceId: "other-room" }];
  const stop = vi.spyOn(api, "stopRecording");
  const { client } = showConference();
  await waitFor(() =>
    expect(
      client.getQueryData<{ items: ConferenceRecording[] }>([
        "recordings",
        "room",
      ])?.items[0].conferenceId,
    ).toBe("other-room"),
  );
  expect(
    screen.queryByRole("button", { name: "Остановить запись" }),
  ).not.toBeInTheDocument();
  expect(screen.queryByTestId("recording-indicator")).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Записи конференции" }),
  ).toBeEnabled();
  expect(stop).not.toHaveBeenCalled();
});

it("скрывает команду и устаревший индикатор после отказа загрузки записей и сообщает об ошибке", async () => {
  fixture.items = [row];
  const stop = vi.spyOn(api, "stopRecording");
  const { client } = showConference();
  const stopButton = await screen.findByRole("button", {
    name: "Остановить запись",
  });
  expect(stopButton).toBeEnabled();
  vi.mocked(api.recordings).mockRejectedValue(
    new ApiError(403, "Записи недоступны"),
  );
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["recordings", "room"] });
  });
  const error = await screen.findByRole("alert");
  expect(error).toHaveTextContent("Записи недоступны");
  expect(error.closest(".room-errors")).not.toBeNull();
  expect(
    client.getQueryData<{ items: ConferenceRecording[] }>([
      "recordings",
      "room",
    ])?.items[0].uuid,
  ).toBe("record");
  expect(stopButton).not.toBeInTheDocument();
  expect(screen.queryByTestId("recording-indicator")).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Записи конференции" }),
  ).toBeEnabled();
  expect(stop).not.toHaveBeenCalled();
});
