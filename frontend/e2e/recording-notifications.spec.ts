import { expect, test, type Page, type WebSocketRoute } from "@playwright/test";
import type { ConferenceRecording, PresenceParticipant } from "../src/types";

const roomId = "room-recording-notifications";
const recordId = "recording-notifications-first";
const stamp = "2026-10-04T12:00:00Z";
const token = "isolated-recording-notifications-token";
const noticeSelector = ".room-recording-notice";
const startedText = "Началась запись встречи. Инициатор: Алексей.";
const fallbackText = "Во встрече идёт запись.";
type Recording = ConferenceRecording & { requestedBy?: string };

test.use({ serviceWorkers: "block" });
test.setTimeout(45_000);

/**
 * Создаёт только тестовую карточку записи, без настоящих пользователей или файлов.
 * @args status — серверное состояние; uuid — идентификатор текущей записи; requestedBy — инициатор.
 * @return Изолированная карточка текущей конференции.
 */
function recording(
  status: ConferenceRecording["status"],
  uuid = recordId,
  requestedBy = "organizer",
): Recording {
  return {
    uuid,
    conferenceId: roomId,
    mode: "composite",
    status,
    requestedBy,
    createdAt: stamp,
    files: [],
  };
}

/**
 * Изолирует настоящий интерфейс уведомлений от API, устройств и серверных WebSocket.
 * @args page — тестовая страница; origin — адрес локальной сборки; options — начальная запись,
 * роль пользователя, отказ чтения и задержка первого снимка для проверки гонки.
 * @return Управление снимками и событиями, счётчики запросов и журнал запрещённых обращений.
 */
async function notificationFixture(
  page: Page,
  origin: string,
  {
    records = [],
    role = "participant",
    guest = false,
    readError = false,
    deferFirstRead = false,
  }: {
    records?: Recording[];
    role?: "owner" | "participant";
    guest?: boolean;
    readError?: boolean;
    deferFirstRead?: boolean;
  } = {},
) {
  const denied: string[] = [];
  const pageErrors: string[] = [];
  const sockets: WebSocketRoute[] = [];
  let sequence = 0;
  let reads = 0;
  let completedReads = 0;
  let releaseFirstRead!: () => void;
  const firstRead = new Promise<void>((resolve) => {
    releaseFirstRead = resolve;
  });
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
    leftAt: null,
    online: true,
    connections: 1,
    connectionIds: ["notification-fixture-self"],
  } satisfies PresenceParticipant;
  const organizer = {
    ...membership,
    id: "membership-organizer",
    userId: "organizer",
    displayName: "Алексей",
    role: "owner" as const,
    connectionIds: ["notification-fixture-organizer"],
  };
  const participants =
    role === "owner" ? [membership] : [membership, organizer];
  const conference = {
    id: roomId,
    title: "Изолированная встреча с записью",
    status: "active",
    ownerId: role === "owner" ? "self" : "organizer",
    createdAt: stamp,
    updatedAt: stamp,
    inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWX12345678",
    waitingRoomEnabled: false,
  };
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.addInitScript(
    (sessionToken) =>
      sessionStorage.setItem(
        "meet.session.v1",
        JSON.stringify({
          token: sessionToken,
          expiresAt: Date.now() + 1_800_000,
        }),
      ),
    token,
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
    const respond = (json: unknown, status = 200) =>
      route.fulfill({ status, json });
    if (["/auth/session", "/auth/refresh"].includes(path))
      return respond({
        accessToken: token,
        expiresIn: 3600,
        status: "success",
        user: {
          id: "self",
          displayName: membership.displayName,
          email: guest ? "" : "fixture@example.test",
          guestConferenceId: guest ? roomId : undefined,
          createdAt: stamp,
          updatedAt: stamp,
        },
      });
    if (method === "GET" && path === "/auth/me")
      return respond({
        status: "success",
        user: {
          id: "self",
          displayName: membership.displayName,
          email: guest ? "" : "fixture@example.test",
          guestConferenceId: guest ? roomId : undefined,
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
        body: ": isolated notification fixture\n\n",
      });
    if (method === "GET" && path === "/notifications")
      return respond({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
      });
    if (path.startsWith(`/conferences/${roomId}`))
      expect(request.headers().authorization).toBe(`Bearer ${token}`);
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
      return respond({ status: "success", items: participants });
    if (method === "GET" && path === `/conferences/${roomId}/recordings`) {
      const number = ++reads;
      const snapshot = records.map((item) => ({ ...item }));
      const failed = readError;
      if (deferFirstRead && number === 1) await firstRead;
      await respond(
        failed
          ? { status: "failed", message: "isolated read unavailable" }
          : { status: "success", items: snapshot },
        failed ? 403 : 200,
      );
      completedReads += 1;
      return;
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
        ticket: "isolated-notification-ticket",
        expiresAt: "2099-01-01T00:00:00Z",
      });
    denied.push(`${method} ${path}`);
    return respond(
      { status: "failed", message: "unknown fixture route denied" },
      404,
    );
  });
  await page.routeWebSocket("**/*", (socket) => {
    const url = new URL(socket.url());
    if (
      url.host === new URL(origin).host &&
      url.pathname === "/" &&
      socket.protocols().includes("vite-hmr")
    ) {
      // Канал разработки тоже подменяется: настоящий серверный WebSocket не открывается.
      socket.send(JSON.stringify({ type: "connected" }));
      return;
    }
    if (
      url.host !== new URL(origin).host ||
      url.pathname !== `/api/v1/conferences/${roomId}/ws`
    ) {
      denied.push(`WS ${url.host}${url.pathname}`);
      void socket.close({ code: 1008, reason: "unknown fixture socket" });
      return;
    }
    expect(url.searchParams.get("ticket")).toBe("isolated-notification-ticket");
    sockets.push(socket);
    socket.send(
      JSON.stringify({
        version: 1,
        id: `notification-state-${++sequence}`,
        type: "conference.state",
        conferenceId: roomId,
        timestamp: stamp,
        data: {
          connectionId: "notification-fixture-self",
          participantId: membership.id,
          status: "active",
          participants,
        },
      }),
    );
  });
  return {
    denied,
    pageErrors,
    reads: () => reads,
    completedReads: () => completedReads,
    connections: () => sockets.length,
    releaseFirstRead,
    setRecords: (next: Recording[]) => {
      records = next;
    },
    setReadError: (failed: boolean) => {
      readError = failed;
    },
    /**
     * Отправляет проверяемое событие только в замоканный сокет страницы.
     * @args type — серверное событие; item — состояние и доверенный инициатор записи.
     * @return Значение не возвращается; событие доставляется настоящему useRealtime.
     */
    emit: (type: string, item: Recording) => {
      if (!sockets.length)
        throw new Error("Тестовый WebSocket ещё не подключён.");
      sockets.at(-1)!.send(
        JSON.stringify({
          version: 1,
          id: `notification-event-${++sequence}`,
          type,
          conferenceId: roomId,
          timestamp: stamp,
          data: {
            recordingId: item.uuid,
            conferenceId: roomId,
            status: item.status,
            mode: item.mode,
            requestedBy: item.requestedBy,
          },
        }),
      );
    },
    disconnect: async () => {
      if (!sockets.length)
        throw new Error("Тестовый WebSocket ещё не подключён.");
      await sockets.at(-1)!.close({ code: 1001, reason: "isolated reconnect" });
    },
  };
}

/**
 * Ждёт настоящего интерфейса и замоканного транспорта, не заменяя компоненты страницы.
 * @args page — текущая страница; fixture — управление изолированным серверным состоянием.
 * @return Завершённое подключение к проверяемой комнате.
 */
async function openMeeting(
  page: Page,
  fixture: Awaited<ReturnType<typeof notificationFixture>>,
) {
  await page.goto(`/conferences/${roomId}`);
  await expect(
    page.getByRole("heading", { name: "Изолированная встреча с записью" }),
  ).toBeVisible();
  await expect.poll(fixture.connections).toBe(1);
}

for (const viewport of [
  { width: 1440, height: 1000, name: "desktop" },
  { width: 390, height: 844, name: "mobile" },
]) {
  test(`${viewport.name}: участник видит фактический старт, скрывает плашку и не получает повтор UUID`, async ({
    page,
    baseURL,
  }, info) => {
    await page.setViewportSize(viewport);
    const fixture = await notificationFixture(page, new URL(baseURL!).origin, {
      records: [recording("starting")],
    });
    await openMeeting(page, fixture);
    await expect(page.getByTestId("recording-indicator")).toHaveText(
      "Запись запускается",
    );
    const notice = page.locator(noticeSelector);
    await expect(notice).toHaveCount(0);
    const beforeStarting = fixture.completedReads();
    fixture.emit("recording.starting", recording("starting"));
    await expect.poll(fixture.completedReads).toBeGreaterThan(beforeStarting);
    await expect(notice).toHaveCount(0);
    fixture.setRecords([recording("recording")]);
    fixture.emit("recording.started", recording("recording"));
    await expect(notice).toContainText(startedText);
    await expect(notice).toHaveAttribute("role", "status");
    await expect(notice).toHaveAttribute("aria-live", "polite");
    await expect(notice).toHaveAttribute("data-recording-id", recordId);
    await expect(
      page.getByRole("button", { name: "Остановить запись", exact: true }),
    ).toHaveCount(0);
    await expect
      .poll(() =>
        page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      )
      .toBe(true);
    const dismiss = notice.getByRole("button", {
      name: "Скрыть уведомление о записи",
    });
    const box = await dismiss.boundingBox();
    expect(box?.width).toBeGreaterThanOrEqual(44);
    expect(box?.height).toBeGreaterThanOrEqual(44);
    await page.screenshot({
      path: info.outputPath("recording-started-notice.png"),
      fullPage: true,
    });
    await dismiss.focus();
    await dismiss.press("Enter");
    await expect(notice).toHaveCount(0);
    const beforeDuplicate = fixture.completedReads();
    fixture.emit("recording.started", recording("recording"));
    await expect.poll(fixture.completedReads).toBeGreaterThan(beforeDuplicate);
    await expect(notice).toHaveCount(0);
    const next = recording("recording", "recording-notifications-second");
    fixture.setRecords([next, recording("ready")]);
    fixture.emit("recording.started", next);
    await expect(notice).toContainText(startedText);
    await expect(notice).toHaveAttribute("data-recording-id", next.uuid);
    fixture.setRecords([{ ...next, status: "stopping" }]);
    fixture.emit("recording.stopping", { ...next, status: "stopping" });
    await expect(notice).toHaveCount(0);
    fixture.emit("recording.started", next);
    await expect(page.getByTestId("recording-indicator")).toHaveText(
      "Запись останавливается",
    );
    await expect(notice).toHaveCount(0);
    expect(fixture.connections()).toBe(1);
    expect(fixture.denied).toEqual([]);
    expect(fixture.pageErrors).toEqual([]);
  });
}

test("позднее подключение восстанавливает общее предупреждение из списка без события старта", async ({
  page,
  baseURL,
}) => {
  const fixture = await notificationFixture(page, new URL(baseURL!).origin, {
    records: [recording("recording")],
  });
  await openMeeting(page, fixture);
  const notice = page.locator(noticeSelector);
  await expect(notice).toContainText(fallbackText);
  await expect(notice).not.toContainText("Инициатор:");
  await notice
    .getByRole("button", { name: "Скрыть уведомление о записи" })
    .click();
  fixture.emit("recording.started", recording("recording"));
  await expect.poll(fixture.completedReads).toBeGreaterThan(1);
  await expect(notice).toHaveCount(0);
  expect(fixture.denied).toEqual([]);
  expect(fixture.pageErrors).toEqual([]);
});

test("инициатор не получает уведомление о собственной записи ни из события, ни из снимка", async ({
  page,
  baseURL,
}) => {
  const own = recording("recording", recordId, "self");
  const fixture = await notificationFixture(page, new URL(baseURL!).origin, {
    role: "owner",
    records: [own],
  });
  await openMeeting(page, fixture);
  await expect(page.getByTestId("recording-indicator")).toHaveText(
    "Идёт запись",
  );
  await expect(page.locator(noticeSelector)).toHaveCount(0);
  const before = fixture.completedReads();
  fixture.emit("recording.started", own);
  await expect.poll(fixture.completedReads).toBeGreaterThan(before);
  await expect(page.locator(noticeSelector)).toHaveCount(0);
  expect(fixture.denied).toEqual([]);
  expect(fixture.pageErrors).toEqual([]);
});

test("отказ чтения списка не скрывает подтверждённое событие фактического запуска", async ({
  page,
  baseURL,
}) => {
  const fixture = await notificationFixture(page, new URL(baseURL!).origin, {
    readError: true,
  });
  await openMeeting(page, fixture);
  await expect(page.locator(".room-errors").getByRole("alert")).toBeVisible();
  await expect(page.locator(noticeSelector)).toHaveCount(0);
  const before = fixture.completedReads();
  fixture.emit("recording.started", recording("recording"));
  await expect(page.locator(noticeSelector)).toContainText(startedText);
  await expect.poll(fixture.completedReads).toBeGreaterThan(before);
  await expect(page.locator(noticeSelector)).toContainText(startedText);
  expect(fixture.denied).toEqual([]);
  expect(fixture.pageErrors).toEqual([]);
});

test("переподключение обновляет список записей и не повторяет закрытое предупреждение", async ({
  page,
  baseURL,
}) => {
  const fixture = await notificationFixture(page, new URL(baseURL!).origin, {
    records: [recording("starting")],
  });
  await openMeeting(page, fixture);
  await expect(page.getByTestId("recording-indicator")).toHaveText(
    "Запись запускается",
  );
  await expect(page.locator(noticeSelector)).toHaveCount(0);
  fixture.setRecords([recording("recording")]);
  await fixture.disconnect();
  await expect.poll(fixture.connections, { timeout: 10_000 }).toBe(2);
  const notice = page.locator(noticeSelector);
  await expect(notice).toContainText(fallbackText);
  await notice
    .getByRole("button", { name: "Скрыть уведомление о записи" })
    .click();
  await fixture.disconnect();
  await expect.poll(fixture.connections, { timeout: 10_000 }).toBe(3);
  await expect(page.getByTestId("recording-indicator")).toHaveText(
    "Идёт запись",
  );
  await expect(notice).toHaveCount(0);
  expect(fixture.denied).toEqual([]);
  expect(fixture.pageErrors).toEqual([]);
});

test("запоздавший пустой начальный снимок не отменяет подтверждённое уведомление запуска", async ({
  page,
  baseURL,
}) => {
  const fixture = await notificationFixture(page, new URL(baseURL!).origin, {
    deferFirstRead: true,
  });
  await openMeeting(page, fixture);
  await expect.poll(fixture.reads).toBe(1);
  fixture.setRecords([recording("recording")]);
  fixture.emit("recording.started", recording("recording"));
  const notice = page.locator(noticeSelector);
  await expect(notice).toContainText(startedText);
  fixture.releaseFirstRead();
  await expect.poll(fixture.completedReads).toBeGreaterThanOrEqual(1);
  await expect(notice).toContainText(startedText);
  await expect(page.getByTestId("recording-indicator")).toHaveText(
    "Идёт запись",
  );
  await expect(notice).toContainText(startedText);
  expect(fixture.denied).toEqual([]);
  expect(fixture.pageErrors).toEqual([]);
});

for (const identity of ["participant", "initiator", "guest"] as const) {
  test(`английский звук воспроизводится один раз: ${identity}`, async ({
    page,
    baseURL,
  }) => {
    await page.addInitScript(() => {
      const original = HTMLMediaElement.prototype.play;
      (window as unknown as { announcements: number }).announcements = 0;
      HTMLMediaElement.prototype.play = function () {
        const announcement =
          this.src.includes("recording-started-en") &&
          !this.muted &&
          this.volume > 0;
        return original.call(this).then(() => {
          if (announcement)
            (window as unknown as { announcements: number }).announcements++;
        });
      };
    });
    const fixture = await notificationFixture(page, new URL(baseURL!).origin, {
      guest: identity === "guest",
      records: [recording("starting")],
    });
    await openMeeting(page, fixture);
    // Настоящий жест разрешает звук; сетевые API/WS изолированы, аудиофайл настоящий.
    await page
      .getByRole("heading", { name: "Изолированная встреча с записью" })
      .click();
    const item = recording(
      "recording",
      recordId,
      identity === "initiator" ? "self" : "organizer",
    );
    fixture.setRecords([item]);
    fixture.emit("recording.started", item);
    const sounds = () =>
      page.evaluate(
        () => (window as unknown as { announcements: number }).announcements,
      );
    await expect.poll(sounds).toBe(1);
    const before = fixture.completedReads();
    fixture.emit("recording.started", item);
    await expect.poll(fixture.completedReads).toBeGreaterThan(before);
    expect(await sounds()).toBe(1);
    expect(fixture.denied).toEqual([]);
    expect(fixture.pageErrors).toEqual([]);
  });
}
