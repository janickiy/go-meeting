import { expect, test, type Locator, type Page } from "@playwright/test";
import type { ConferenceRecording, Participant } from "../src/types";

const roomId = "room-recording-controls";
const recordId = "recording-controls";
const stamp = "2026-10-04T09:00:00Z";

/**
 * Изолирует управление записью от настоящего API, WebSocket, устройств и данных пользователя.
 * @args page — тестовая страница; origin — адрес проверяемой сборки;
 * role — роль текущего участника; status — начальное состояние записи;
 * stopReply — управляемый ответ на попытку остановки, по умолчанию успешный.
 * @return Счётчик остановок, журнал отклонённых маршрутов и управление серверным состоянием фикстуры.
 */
async function recordingFixture(
  page: Page,
  origin: string,
  {
    role = "owner",
    status = "recording",
    files = [],
    stopReply = async () => "stopping" as const,
  }: {
    role?: Participant["role"];
    status?: ConferenceRecording["status"];
    files?: ConferenceRecording["files"];
    stopReply?: (attempt: number) => Promise<"error" | "stopping">;
  } = {},
) {
  const denied: string[] = [];
  let stops = 0;
  let recording: ConferenceRecording = {
    uuid: recordId,
    conferenceId: roomId,
    mode: "composite",
    status,
    createdAt: stamp,
    files,
  };
  const conference = {
    id: roomId,
    title: "Командная встреча",
    status: "active",
    ownerId: role === "owner" ? "self" : "organizer",
    createdAt: stamp,
    updatedAt: stamp,
    inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWX12345678",
    waitingRoomEnabled: false,
  };
  const membership = {
    id: "membership-self",
    userId: "self",
    conferenceId: roomId,
    displayName: "Александр",
    role,
    status: "joined",
    admissionState: "admitted",
    microphoneEnabled: false,
    cameraEnabled: false,
    createdAt: stamp,
    updatedAt: stamp,
    joinedAt: stamp,
    online: true,
    connections: 1,
    connectionIds: ["fixture-connection"],
  };
  await page.addInitScript(() =>
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "isolated-recording-controls-token",
        expiresAt: Date.now() + 1_800_000,
      }),
    ),
  );
  await page.route("**/*", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const method = request.method();
    if (url.origin !== origin) {
      denied.push(`${method} ${url.origin}${url.pathname}`);
      return route.abort("blockedbyclient");
    }
    if (!url.pathname.startsWith("/api/")) return route.continue();
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const respond = (json: unknown, code = 200) =>
      route.fulfill({ status: code, json });
    if (method === "GET" && path === "/auth/me")
      return respond({
        status: "success",
        user: {
          id: "self",
          displayName: membership.displayName,
          email: "fixture@example.test",
          createdAt: stamp,
          updatedAt: stamp,
        },
      });
    if (method === "GET" && path === "/capabilities")
      return respond({
        status: "success",
        capabilities: {
          liveCaptions: false,
          transcription: false,
          aiSummary: false,
          semanticSearch: false,
          meetingAnalytics: false,
          recordingModes: ["composite"],
        },
      });
    if (method === "GET" && path === "/notifications/events")
      return route.fulfill({
        contentType: "text/event-stream",
        body: ": isolated recording fixture\n\n",
      });
    if (method === "GET" && path === "/notifications")
      return respond({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
      });
    if (path.startsWith(`/conferences/${roomId}`))
      expect(request.headers().authorization).toBe(
        "Bearer isolated-recording-controls-token",
      );
    if (method === "GET" && path === `/conferences/${roomId}`)
      return respond({ status: "success", item: conference });
    if (method === "GET" && path === `/conferences/${roomId}/participants/me`)
      return respond({ status: "success", item: membership });
    if (
      method === "PUT" &&
      path === `/conferences/${roomId}/participants/me/media`
    )
      return respond({ status: "success", item: membership });
    if (method === "GET" && path === `/conferences/${roomId}/participants`)
      return respond({ status: "success", items: [membership] });
    if (method === "GET" && path === `/conferences/${roomId}/recordings`)
      return respond({ status: "success", items: [recording] });
    if (
      method === "POST" &&
      path === `/conferences/${roomId}/recordings/${recordId}/stop`
    ) {
      stops += 1;
      const response = await stopReply(stops);
      if (response === "error")
        return respond(
          { message: "operation unavailable in current state" },
          409,
        );
      recording = { ...recording, status: "stopping" };
      return respond({ status: "success", item: recording });
    }
    if (method === "GET" && path === `/conferences/${roomId}/messages`)
      return respond({ status: "success", items: [], nextCursor: null });
    if (method === "GET" && path === `/conferences/${roomId}/chat/read`)
      return respond({
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: null },
      });
    if (method === "POST" && path === `/conferences/${roomId}/ws-ticket`)
      return respond({
        ticket: "isolated-fixture-ticket",
        expiresAt: "2099-01-01T00:00:00Z",
      });
    denied.push(`${method} ${path}`);
    return respond({ message: "unknown fixture route denied" }, 404);
  });
  await page.routeWebSocket("**/*", (socket) => {
    const url = new URL(socket.url());
    // Стандартный локальный прогон использует Vite: его канал тоже подменяется, не соединяясь с сервером.
    if (
      url.host === new URL(origin).host &&
      url.pathname === "/" &&
      socket.protocols().includes("vite-hmr")
    ) {
      socket.send(JSON.stringify({ type: "connected" }));
      return;
    }
    if (url.pathname !== `/api/v1/conferences/${roomId}/ws`) {
      denied.push(`WS ${url.pathname}`);
      void socket.close({ code: 1008, reason: "unknown fixture socket" });
      return;
    }
    socket.send(
      JSON.stringify({
        version: 1,
        id: "recording-fixture-state",
        type: "conference.state",
        conferenceId: roomId,
        timestamp: stamp,
        data: {
          connectionId: "fixture-connection",
          participantId: membership.id,
          status: "active",
          participants: [membership],
        },
      }),
    );
  });
  return {
    denied,
    stopCount: () => stops,
    setStatus: (next: ConferenceRecording["status"]) => {
      recording = { ...recording, status: next };
    },
  };
}

/**
 * Проверяет доступность верхней кнопки и отсутствие горизонтального переполнения.
 * @args page — текущая страница; button — кнопка управления записью.
 * @return Завершённая проверка геометрии на текущем размере экрана.
 */
async function expectHeaderFits(page: Page, button: Locator) {
  const viewport = page.viewportSize()!;
  const box = await button.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewport.width);
  expect(box!.y).toBeGreaterThanOrEqual(0);
  expect(box!.y + box!.height).toBeLessThanOrEqual(viewport.height);
  await expect
    .poll(() =>
      page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    )
    .toBe(true);
}

for (const viewport of [
  { width: 1440, height: 1000, name: "desktop" },
  { width: 390, height: 844, name: "mobile" },
]) {
  test(`${viewport.name}: окно управления не показывает прошлые записи и файлы`, async ({
    page,
    baseURL,
  }, info) => {
    await page.setViewportSize(viewport);
    const fixture = await recordingFixture(page, new URL(baseURL!).origin, {
      status: "ready",
      files: [
        { fileType: "final_mp4", url: `${baseURL}/private-fixture/final.mp4` },
        {
          fileType: "preview_jpg",
          url: `${baseURL}/private-fixture/preview.jpg`,
        },
        {
          fileType: "tracks_archive",
          url: `${baseURL}/private-fixture/tracks.zip`,
        },
      ],
    });
    await page.goto(`/conferences/${roomId}`);
    await page
      .getByRole("button", { name: "Записи конференции", exact: true })
      .click();
    const dialog = page.getByRole("dialog", {
      name: "Записи конференции",
      exact: true,
    });
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByRole("combobox", { name: "Режим записи" }),
    ).toBeEnabled();
    await expect(
      dialog.getByRole("button", { name: "Начать запись", exact: true }),
    ).toBeEnabled();
    await expect(dialog.locator(".recording-list, .recording-row")).toHaveCount(
      0,
    );
    await expect(dialog.getByRole("link")).toHaveCount(0);
    await expect(
      dialog.getByText("Запись готова", { exact: true }),
    ).toHaveCount(0);
    await expect(dialog.getByText(/Записей пока нет/)).toHaveCount(0);
    await page.screenshot({
      path: info.outputPath("recording-controls-only-modal.png"),
      fullPage: true,
    });
    expect(fixture.denied).toEqual([]);
  });
  test(`${viewport.name}: остановка из верхней панели без диалога и повторных запросов`, async ({
    page,
    baseURL,
  }, info) => {
    await page.setViewportSize(viewport);
    let finish!: (response: "stopping") => void;
    const response = new Promise<"stopping">((resolve) => {
      finish = resolve;
    });
    const fixture = await recordingFixture(page, new URL(baseURL!).origin, {
      stopReply: () => response,
    });
    await page.goto(`/conferences/${roomId}`);
    const stop = page.locator(".room-header").getByRole("button", {
      name: "Остановить запись",
      exact: true,
    });
    const history = page.getByRole("button", {
      name: "Записи конференции",
      exact: true,
    });
    await expect(stop).toBeVisible();
    await expect(stop).toBeEnabled();
    await expectHeaderFits(page, stop);
    const reconnect = page
      .locator(".room-header")
      .getByRole("button", { name: "Переподключиться", exact: true });
    await expect(reconnect).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Переподключиться", exact: true }),
    ).toHaveCount(1);
    await expectHeaderFits(page, reconnect);
    await expect(
      page.getByText("Состояние медиасвязи", { exact: true }),
    ).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Реакция / })).toHaveCount(
      0,
    );
    await page.screenshot({
      path: info.outputPath("header-stop-available.png"),
      fullPage: true,
    });
    await stop.click();
    await expect.poll(fixture.stopCount).toBe(1);
    await expect(stop).toBeDisabled();
    await expect(stop).toHaveAttribute("aria-busy", "true");
    await expect(stop).toContainText("Останавливаем…");
    await stop.click({ force: true });
    expect(fixture.stopCount()).toBe(1);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(history).toBeEnabled();
    await page.screenshot({
      path: info.outputPath("header-stop-pending.png"),
      fullPage: true,
    });
    finish("stopping");
    await expect(page.getByTestId("recording-indicator")).toHaveText(
      "Запись останавливается",
    );
    await expect(stop).toBeDisabled();
    await expectHeaderFits(page, stop);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await history.click();
    const dialog = page.getByRole("dialog", {
      name: "Записи конференции",
      exact: true,
    });
    await expect(dialog).toBeVisible();
    await expect(dialog.locator(".recording-list, .recording-row")).toHaveCount(
      0,
    );
    await expect(
      dialog.getByRole("button", { name: "Остановить запись", exact: true }),
    ).toBeDisabled();
    await dialog.getByRole("button", { name: "Закрыть окно" }).click();
    fixture.setStatus("processing");
    await expect(stop).toHaveCount(0);
    await expect(page.getByTestId("recording-indicator")).toHaveCount(0);
    await expect(history).toBeVisible();
    await page.screenshot({
      path: info.outputPath("header-stop-finished.png"),
      fullPage: true,
    });
    expect(fixture.stopCount()).toBe(1);
    expect(fixture.denied).toEqual([]);
  });
}

test("ошибка остановки отображается без диалога и позволяет повторить запрос", async ({
  page,
  baseURL,
}, info) => {
  const fixture = await recordingFixture(page, new URL(baseURL!).origin, {
    stopReply: async (attempt) => (attempt === 1 ? "error" : "stopping"),
  });
  await page.goto(`/conferences/${roomId}`);
  const stop = page
    .locator(".room-header")
    .getByRole("button", { name: "Остановить запись", exact: true });
  await stop.click();
  await expect(page.locator(".room-errors").getByRole("alert")).toBeVisible();
  await expect(stop).toBeEnabled();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.screenshot({
    path: info.outputPath("header-stop-error.png"),
    fullPage: true,
  });
  await stop.click();
  await expect.poll(fixture.stopCount).toBe(2);
  await expect(page.getByTestId("recording-indicator")).toHaveText(
    "Запись останавливается",
  );
  await expect(stop).toBeDisabled();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(fixture.denied).toEqual([]);
});

for (const status of ["starting", "degraded"] as const) {
  test(`остановка доступна для состояния ${status}`, async ({
    page,
    baseURL,
  }) => {
    const fixture = await recordingFixture(page, new URL(baseURL!).origin, {
      status,
    });
    await page.goto(`/conferences/${roomId}`);
    const stop = page
      .locator(".room-header")
      .getByRole("button", { name: "Остановить запись", exact: true });
    await expect(stop).toBeEnabled();
    await stop.click();
    await expect.poll(fixture.stopCount).toBe(1);
    await expect(stop).toBeDisabled();
    await expect(page.getByTestId("recording-indicator")).toHaveText(
      "Запись останавливается",
    );
    await expect(page.getByRole("dialog")).toHaveCount(0);
    expect(fixture.denied).toEqual([]);
  });
}

for (const role of ["participant", "co_host"] as const) {
  test(`роль ${role} видит запись без команды остановки`, async ({
    page,
    baseURL,
  }) => {
    const fixture = await recordingFixture(page, new URL(baseURL!).origin, {
      role,
    });
    await page.goto(`/conferences/${roomId}`);
    await expect(page.getByTestId("recording-indicator")).toHaveText(
      "Идёт запись",
    );
    await expect(
      page.getByRole("button", { name: "Остановить запись", exact: true }),
    ).toHaveCount(0);
    await page
      .getByRole("button", { name: "Записи конференции", exact: true })
      .click();
    const dialog = page.getByRole("dialog", {
      name: "Записи конференции",
      exact: true,
    });
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByRole("button", { name: "Остановить запись", exact: true }),
    ).toHaveCount(0);
    expect(fixture.stopCount()).toBe(0);
    expect(fixture.denied).toEqual([]);
  });
}
