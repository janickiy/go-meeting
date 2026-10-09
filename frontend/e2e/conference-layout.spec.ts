import { personalBackground } from "./helpers/personal-background";
import { expect, test, type Page, type WebSocketRoute } from "@playwright/test";
import type { PresenceParticipant } from "../src/types";

const stamp = "2026-10-03T12:00:00Z";
const room = {
  id: "room-visual",
  title: "Продуктовая встреча",
  status: "active",
  ownerId: "owner",
  createdAt: stamp,
  updatedAt: stamp,
  startedAt: null,
  finishedAt: null,
  inviteCode: "ABCDEFGHIJKLMNOPQRSTUVWX12345678",
  waitingRoomEnabled: true,
};
const people = [
  "Алексей Петров",
  "Мария Соколова",
  "Иван Ким",
  "Елена Смирнова",
].map(
  (name, index) =>
    ({
      id: `participant-${index}`,
      userId: index ? `user-${index}` : "owner",
      conferenceId: room.id,
      displayName: name,
      role: index === 0 ? "owner" : "participant",
      status: "joined",
      admissionState: "admitted",
      createdAt: stamp,
      updatedAt: stamp,
      joinedAt: stamp,
      leftAt: null,
      microphoneEnabled: false,
      cameraEnabled: false,
      online: true,
      connections: 1,
      connectionIds: [`connection-${index}`],
    }) as PresenceParticipant,
);

/** Подменяет HTTP/WS только в тестовом браузере и запрещает неизвестные API-запросы.
 * @args page — изолированная страница; origin — адрес локального Vite;
 * count — начальное число участников; canvas — тестовые видеопотоки вместо SFU;
 * guest — ограниченная сессия гостя текущей конференции.
 * nameSuffix — добавление к именам для проверки переноса длинного текста.
 * waitingRoomEnabled — серверный признак включённого зала ожидания.
 * @return Управление снимком, счётчик соединений и журналы ошибок браузера.
 */
async function fixture(
  page: Page,
  origin: string,
  {
    count = 4,
    canvas = false,
    guest = false,
    nameSuffix = "",
    waitingRoomEnabled = true,
  } = {},
) {
  const denied: string[] = [];
  const errors: string[] = [];
  const basePerson: PresenceParticipant = guest
    ? {
        ...people[0],
        userId: "guest",
        displayName: "Гость",
        role: "guest",
      }
    : people[0];
  const participants = [basePerson, ...people.slice(1)].map((person) =>
    nameSuffix
      ? { ...person, displayName: person.displayName + nameSuffix }
      : person,
  );
  const localPerson = participants[0];
  let current = participants.slice(0, count);
  let socket: WebSocketRoute | undefined;
  let sequence = 0;
  let connections = 0;
  page.on("pageerror", (error) => errors.push(error.message));
  await page.addInitScript(() => {
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "visual-only-token",
        expiresAt: Date.now() + 1_800_000,
      }),
    );
    const runtime = window as Window & { layoutDeviceCalls?: number };
    runtime.layoutDeviceCalls = 0;
    if (navigator.mediaDevices) {
      Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
        value: () => {
          runtime.layoutDeviceCalls = (runtime.layoutDeviceCalls ?? 0) + 1;
          return Promise.reject(new Error("Устройства запрещены в UI-тесте"));
        },
      });
      Object.defineProperty(navigator.mediaDevices, "getDisplayMedia", {
        value: () => {
          runtime.layoutDeviceCalls = (runtime.layoutDeviceCalls ?? 0) + 1;
          return Promise.reject(new Error("Захват экрана запрещён в UI-тесте"));
        },
      });
      Object.defineProperty(navigator.mediaDevices, "enumerateDevices", {
        value: async () => [],
      });
    }
  });
  await page.route("**/*", async (route) => {
    const url = new URL(route.request().url());
    const method = route.request().method();
    if (url.origin !== origin) {
      denied.push(`${method} ${url.origin}${url.pathname}`);
      return route.abort("blockedbyclient");
    }
    if (canvas && url.pathname === "/src/useMedia.ts") {
      return route.fulfill({
        contentType: "application/javascript",
        body: 'export { useMedia } from "/e2e/helpers/layout-media-fixture.tsx";',
      });
    }
    if (!url.pathname.startsWith("/api/")) return route.continue();
    const path = url.pathname.replace(/^\/api\/v1/, "");
    if (await personalBackground(route, path)) return;
    const respond = (data: unknown) => route.fulfill({ json: data });
    if (
      (method === "GET" && path === "/auth/me") ||
      (method === "POST" && ["/auth/session", "/auth/refresh"].includes(path))
    )
      return respond({
        status: "success",
        ...(method === "POST"
          ? { accessToken: "visual-only-token", expiresIn: 3600 }
          : {}),
        user: {
          id: localPerson.userId,
          email: guest ? "" : "owner@example.test",
          displayName: localPerson.displayName,
          ...(guest ? { guestConferenceId: room.id } : {}),
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
        body: ": visual fixture\n\n",
      });
    if (method === "GET" && path === "/notifications")
      return respond({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
      });
    if (method === "GET" && path === `/conferences/${room.id}`)
      return respond({
        status: "success",
        item: { ...room, waitingRoomEnabled },
      });
    if (method === "GET" && path === `/conferences/${room.id}/participants/me`)
      return respond({ status: "success", item: localPerson });
    if (
      method === "PUT" &&
      path === `/conferences/${room.id}/participants/me/media`
    )
      return respond({ status: "success", item: localPerson });
    if (method === "GET" && path === `/conferences/${room.id}/participants`)
      return respond({ status: "success", items: current });
    if (method === "GET" && path === `/conferences/${room.id}/recordings`)
      return respond({ status: "success", items: [] });
    if (method === "GET" && path === `/conferences/${room.id}/messages`)
      return respond({ status: "success", items: [], nextCursor: null });
    if (method === "GET" && path === `/conferences/${room.id}/chat/read`)
      return respond({
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: null },
      });
    if (method === "POST" && path === `/conferences/${room.id}/ws-ticket`)
      return respond({
        ticket: "visual-ticket",
        expiresAt: "2099-01-01T00:00:00Z",
      });
    denied.push(`${method} ${path}`);
    return route.fulfill({
      status: 404,
      json: { message: "unavailable test route" },
    });
  });
  /** Передаёт новый состав через тестовый WebSocket, не создавая backend-данных.
   * @args nextCount — число присутствующих; type — тип доверенного снимка присутствия.
   */
  const emit = (nextCount: number, type = "participant.connected") => {
    current = participants.slice(0, nextCount);
    if (!socket) throw new Error("Тестовый WebSocket ещё не подключён");
    socket.send(
      JSON.stringify({
        version: 1,
        id: `layout-state-${++sequence}`,
        type,
        conferenceId: room.id,
        timestamp: stamp,
        data: {
          connectionId: "connection-0",
          participantId: localPerson.id,
          status: "active",
          participants: current,
        },
      }),
    );
  };
  await page.routeWebSocket("**/*", (ws) => {
    const url = new URL(ws.url());
    if (
      url.host === new URL(origin).host &&
      url.pathname === "/" &&
      ws.protocols().includes("vite-hmr")
    ) {
      ws.send(JSON.stringify({ type: "connected" }));
      return;
    }
    if (
      url.host !== new URL(origin).host ||
      url.pathname !== `/api/v1/conferences/${room.id}/ws`
    ) {
      denied.push(`WS ${url.host}${url.pathname}`);
      void ws.close({ code: 1008, reason: "Неизвестный тестовый канал" });
      return;
    }
    connections++;
    socket = ws;
    emit(count, "conference.state");
  });
  return { emit, denied, errors, connections: () => connections };
}

test("тёмная комната: настоящие пустые плитки, чат и разрешённая модерация", async ({
  page,
  baseURL,
}, info) => {
  const isolated = await fixture(page, new URL(baseURL!).origin);
  const removedRequests: string[] = [];
  page.on("request", (request) => {
    if (
      /\/hands$|\/participants\/[^/]+\/hand$/.test(
        new URL(request.url()).pathname,
      )
    )
      removedRequests.push(request.url());
  });
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByRole("heading", { name: room.title })).toBeVisible();
  await expect(page.getByRole("button", { name: "Поднять руку" })).toHaveCount(
    0,
  );
  await expect(
    page.getByRole("region", { name: "Реакции", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Ещё", exact: true }),
  ).toHaveCount(0);
  await expect(
    page
      .locator(".conference-control-bar")
      .getByRole("button", { name: "Переподключиться", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("h");
  expect(removedRequests).toEqual([]);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(4);
  await expect(
    page.getByRole("navigation", { name: "Основная навигация" }),
  ).toBeHidden();
  await expect(
    page.getByRole("tab", { name: "Чат", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("conference-chat.png"),
    fullPage: true,
  });
  await page.getByRole("tab", { name: "Участники (4)", exact: true }).click();
  await expect(page.getByLabel("Управление: Мария Соколова")).toBeVisible();
  await expect(page.getByLabel("Управление: Алексей Петров")).toHaveCount(0);
  await page.screenshot({
    path: info.outputPath("conference-participants.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Записи конференции", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Записи конференции", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Начать запись", exact: true }),
  ).toBeVisible();
  expect(isolated.denied).toEqual([]);
  expect(isolated.errors).toEqual([]);
});

test("мобильная комната открывает и закрывает панель без горизонтального переполнения", async ({
  page,
  baseURL,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const isolated = await fixture(page, new URL(baseURL!).origin);
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(4);
  await expect(
    page.getByRole("tab", { name: "Чат", exact: true }),
  ).toBeHidden();
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({
    path: info.outputPath("conference-mobile.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Чат", exact: true }).click();
  await expect(
    page.getByRole("tab", { name: "Чат", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("conference-mobile-chat.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Закрыть панель встречи" }).click();
  await expect(
    page.getByRole("tab", { name: "Чат", exact: true }),
  ).toBeHidden();
  expect(isolated.denied).toEqual([]);
  expect(isolated.errors).toEqual([]);
});

test("предпросмотр не запрашивает устройства до явного действия", async ({
  page,
  baseURL,
}, info) => {
  const isolated = await fixture(page, new URL(baseURL!).origin);
  await page.goto(`/conferences/${room.id}/join`);
  await expect(page.getByRole("heading", { name: room.title })).toBeVisible();
  await expect(
    page.getByText("Камера выключена", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Войти во встречу" }),
  ).toBeEnabled();
  await page.screenshot({
    path: info.outputPath("prejoin-desktop.png"),
    fullPage: true,
  });
  expect(isolated.denied).toEqual([]);
  expect(isolated.errors).toEqual([]);
});

/** Проверяет широкие плитки 16:9, центрирование и отдельную вертикальную прокрутку.
 * @args page — настоящая страница комнаты с двумя отрисованными участниками;
 * fullyVisible — требуется ли полная видимость обоих окон без прокрутки.
 * @return Завершённые проверки геометрии без изменения CSS и разметки приложения.
 */
async function expectPairGeometry(page: Page, fullyVisible = true) {
  const grid = page.getByRole("region", {
    name: "Видео участников",
    exact: true,
  });
  await expect(grid).toHaveClass(/media-grid-pair/);
  await expect(grid).toHaveAttribute("tabindex", "0");
  await expect(grid.locator(".media-tile")).toHaveCount(2);
  const dimensions = await grid.evaluate((element) => {
    const gridBox = element.getBoundingClientRect();
    return {
      clientWidth: element.clientWidth,
      clientHeight: element.clientHeight,
      scrollWidth: element.scrollWidth,
      scrollHeight: element.scrollHeight,
      gridX: gridBox.x,
      gridY: gridBox.y,
      gap: parseFloat(getComputedStyle(element).rowGap),
      overflow: getComputedStyle(element).overflowY,
      tiles: [...element.querySelectorAll(".media-tile")].map((tile) => {
        const box = tile.getBoundingClientRect();
        return { x: box.x, y: box.y, width: box.width, height: box.height };
      }),
    };
  });
  expect(dimensions.overflow).toBe("auto");
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth);
  for (const box of dimensions.tiles) {
    expect(Math.abs(box.width / box.height - 16 / 9)).toBeLessThan(0.015);
    const fitWidth = Math.min(
      dimensions.clientWidth,
      960,
      Math.max(360, ((dimensions.clientHeight - dimensions.gap) * 8) / 9),
    );
    expect(Math.abs(box.width - fitWidth)).toBeLessThan(3);
    const expectedCenter = dimensions.gridX + dimensions.clientWidth / 2;
    expect(Math.abs(box.x + box.width / 2 - expectedCenter)).toBeLessThan(3);
    if (fullyVisible) {
      expect(box.y).toBeGreaterThanOrEqual(dimensions.gridY - 1);
      expect(box.y + box.height).toBeLessThanOrEqual(
        dimensions.gridY + dimensions.clientHeight + 1,
      );
    }
  }
  expect(Math.abs(dimensions.tiles[0].x - dimensions.tiles[1].x)).toBeLessThan(
    2,
  );
  expect(dimensions.tiles[1].y).toBeGreaterThan(
    dimensions.tiles[0].y + dimensions.tiles[0].height,
  );
  await expectNoOverflow(page);
  const controls = page.locator(".conference-control-bar");
  const controlBox = await controls.boundingBox();
  expect(controlBox).not.toBeNull();
  expect(controlBox!.y).toBeGreaterThanOrEqual(0);
  expect(controlBox!.y + controlBox!.height).toBeLessThanOrEqual(
    page.viewportSize()!.height + 1,
  );
}

/** Подтверждает отсутствие горизонтального переполнения страницы и доступа к физическим устройствам.
 * @args page — проверяемая страница.
 */
async function expectNoOverflow(page: Page) {
  expect(
    await page.evaluate(() => ({
      fits: document.documentElement.scrollWidth <= window.innerWidth,
      calls: (window as Window & { layoutDeviceCalls?: number })
        .layoutDeviceCalls,
    })),
  ).toEqual({ fits: true, calls: 0 });
}

for (const layout of [
  { name: "desktop с чатом", width: 1440, height: 1000, chat: true },
  { name: "desktop без чата", width: 1440, height: 1000, chat: false },
  { name: "mobile", width: 390, height: 844, chat: false },
]) {
  test(`две крупные плитки идут вертикально по центру: ${layout.name}`, async ({
    page,
    baseURL,
  }, info) => {
    await page.setViewportSize({ width: layout.width, height: layout.height });
    const isolated = await fixture(page, new URL(baseURL!).origin, {
      count: 2,
    });
    await page.goto(`/conferences/${room.id}`);
    await expect(page.getByTestId("participant-placeholder")).toHaveCount(2);
    if (!layout.chat && layout.width > 900)
      await page
        .getByRole("button", { name: "Закрыть панель встречи" })
        .click();
    await expectPairGeometry(page);
    if (layout.chat) {
      const rail = await page.locator(".conference-stage-rail").boundingBox();
      const grid = await page.locator(".media-grid-pair").boundingBox();
      expect(rail!.x).toBeGreaterThanOrEqual(grid!.x + grid!.width);
    }
    await page.screenshot({
      path: info.outputPath("two-participants-centered.png"),
      fullPage: true,
    });
    expect(
      await page
        .locator(".media-grid-pair")
        .evaluate(
          (element) => element.scrollHeight <= element.clientHeight + 1,
        ),
    ).toBe(true);
    expect(isolated.denied).toEqual([]);
    expect(isolated.errors).toEqual([]);
  });
}

for (const size of [
  { name: "desktop", width: 1440, height: 1000, textSize: 100 },
  { name: "mobile", width: 390, height: 844, textSize: 100 },
  { name: "mobile-200", width: 390, height: 844, textSize: 200 },
]) {
  for (const waitingRoomEnabled of [true, false]) {
    test(`подпись доступа по центру футера — ${size.name}, зал ожидания: ${waitingRoomEnabled}`, async ({
      page,
      baseURL,
    }, info) => {
      await page.setViewportSize(size);
      const isolated = await fixture(page, new URL(baseURL!).origin, {
        count: 2,
        waitingRoomEnabled,
      });
      await page.addInitScript((textSize) => {
        localStorage.setItem(
          "go-recorder.appearance.v1:owner",
          JSON.stringify({ version: 1, theme: "light", textSize }),
        );
      }, size.textSize);
      await page.goto(`/meetings/${room.id}`);
      const footer = page.locator(".room-footer");
      const label = footer.getByText("Доступно по приглашению", {
        exact: true,
      });
      if (waitingRoomEnabled) {
        await expect(label).toBeInViewport();
        const footerBox = (await footer.boundingBox())!;
        const labelBox = (await label.boundingBox())!;
        expect(
          Math.abs(
            footerBox.x + footerBox.width / 2 - labelBox.x - labelBox.width / 2,
          ),
        ).toBeLessThan(1.1);
      } else {
        await expect(label).toHaveCount(0);
        await expect(footer.locator(".room-footer-meta")).toHaveCount(0);
      }
      await page.screenshot({
        path: info.outputPath(
          `access-note-${size.name}-${waitingRoomEnabled}.png`,
        ),
      });
      await expectNoOverflow(page);
      expect(isolated.errors).toEqual([]);
      expect(isolated.denied).toEqual([]);
    });
  }
}

test("короткая видеообласть прокручивается с клавиатуры, не пряча кнопки управления", async ({
  page,
  baseURL,
}, info) => {
  await page.setViewportSize({ width: 1440, height: 480 });
  const isolated = await fixture(page, new URL(baseURL!).origin, { count: 2 });
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(2);
  // Низкий viewport проверяет fallback без изменения CSS и ослабления CSP.
  await expectPairGeometry(page, false);
  const grid = page.getByRole("region", {
    name: "Видео участников",
    exact: true,
  });
  expect(
    await grid.evaluate(
      (element) => element.scrollHeight > element.clientHeight,
    ),
  ).toBe(true);
  await grid.focus();
  await page.keyboard.press("End");
  await expect
    .poll(() => grid.evaluate((element) => element.scrollTop))
    .toBeGreaterThan(0);
  await expectPairGeometry(page, false);
  await page.screenshot({
    path: info.outputPath("short-pair-keyboard-scroll.png"),
    fullPage: true,
  });
  expect(isolated.denied).toEqual([]);
  expect(isolated.errors).toEqual([]);
});

test("переходы 1→2→3→2 переключают парную раскладку по текущему составу", async ({
  page,
  baseURL,
}) => {
  const isolated = await fixture(page, new URL(baseURL!).origin, { count: 1 });
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(1);
  await expect(page.locator(".media-grid-pair")).toHaveCount(0);
  isolated.emit(2);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(2);
  await expectPairGeometry(page);
  isolated.emit(3);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(3);
  await expect(page.locator(".media-grid-pair")).toHaveCount(0);
  isolated.emit(2, "participant.disconnected");
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(2);
  await expectPairGeometry(page);
  expect(isolated.denied).toEqual([]);
  expect(isolated.errors).toEqual([]);
});

test("два настоящих тестовых video сохраняют пропорции, а экран полностью заменяет пару", async ({
  page,
  baseURL,
}, info) => {
  const isolated = await fixture(page, new URL(baseURL!).origin, {
    count: 2,
    canvas: true,
  });
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByTestId("participant-placeholder")).toHaveCount(2);
  await page
    .getByRole("button", { name: "Включить камеру и микрофон", exact: true })
    .click();
  await expect(page.locator(".media-grid-pair video")).toHaveCount(2);
  await expectPairGeometry(page);
  const camera = page.getByTestId("remote-media").locator("video");
  await expect(camera).toHaveCSS("object-fit", "contain");
  await camera.evaluate((video: HTMLVideoElement) => {
    const runtime = window as Window & {
      layoutVideo?: HTMLVideoElement;
      layoutStream?: MediaProvider | null;
    };
    runtime.layoutVideo = video;
    runtime.layoutStream = video.srcObject;
  });
  await page.screenshot({
    path: info.outputPath("two-videos-centered.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Показать экран", exact: true })
    .click();
  await expect(page.locator(".media-grid-pair")).toHaveCount(0);
  const sharing = page.locator(".media-grid-sharing");
  const screen = sharing.locator(".media-tile-screen");
  await expect(screen).toBeVisible();
  for (const tile of await sharing
    .locator(".media-tile:not(.media-tile-screen)")
    .all())
    await expect(tile).toBeHidden();
  const boxes = await sharing.evaluate((grid) => {
    const frame = grid.getBoundingClientRect();
    const display = grid
      .querySelector(".media-tile-screen")!
      .getBoundingClientRect();
    return {
      frame: { width: frame.width, height: frame.height },
      display: { width: display.width, height: display.height },
    };
  });
  expect(Math.abs(boxes.frame.width - boxes.display.width)).toBeLessThan(2);
  expect(Math.abs(boxes.frame.height - boxes.display.height)).toBeLessThan(2);
  await expect(screen.locator("video")).toHaveCSS("object-fit", "contain");
  expect(
    await camera.evaluate((video: HTMLVideoElement) => {
      const runtime = window as Window & {
        layoutVideo?: HTMLVideoElement;
        layoutStream?: MediaProvider | null;
      };
      return (
        video === runtime.layoutVideo &&
        video.srcObject === runtime.layoutStream
      );
    }),
  ).toBe(true);
  await expectNoOverflow(page);
  await page.screenshot({
    path: info.outputPath("shared-screen-dominates-pair.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Остановить демонстрацию", exact: true })
    .click();
  await expectPairGeometry(page);
  await expect(camera).toBeVisible();
  expect(isolated.denied).toEqual([]);
  expect(isolated.errors).toEqual([]);
});

test("настройки аккаунта из конференции сохраняют видео, медиасессию и фокус", async ({
  page,
  baseURL,
}, info) => {
  const isolated = await fixture(page, new URL(baseURL!).origin, {
    count: 2,
    canvas: true,
  });
  await page.goto(`/conferences/${room.id}`);
  await page
    .getByRole("button", { name: "Включить камеру и микрофон", exact: true })
    .click();
  const video = page.getByTestId("local-media").locator("video");
  await expect(video).toBeVisible();
  await expect
    .poll(() => video.evaluate((node: HTMLVideoElement) => node.currentTime))
    .toBeGreaterThan(0);
  await video.evaluate((node: HTMLVideoElement) => {
    const runtime = window as Window & {
      layoutSettingsVideo?: HTMLVideoElement;
      layoutSettingsStream?: MediaProvider | null;
      layoutSettingsTrack?: MediaStreamTrack;
    };
    runtime.layoutSettingsVideo = node;
    runtime.layoutSettingsStream = node.srcObject;
    runtime.layoutSettingsTrack = (
      node.srcObject as MediaStream
    ).getVideoTracks()[0];
  });
  const mediaState = () =>
    page.evaluate(
      () =>
        (
          window as Window & {
            layoutMediaState?: {
              peerId: string | undefined;
              microphoneEnabled: boolean;
              cameraEnabled: boolean;
            };
          }
        ).layoutMediaState,
    );
  await expect.poll(mediaState).toMatchObject({ peerId: "layout-local-peer" });
  const beforeMedia = await mediaState();
  const beforeURL = page.url();
  const beforeFrames = await video.evaluate(
    (node: HTMLVideoElement) => node.currentTime,
  );
  const rail = page.locator(".conference-stage-rail");
  const beforeChat = await rail.isVisible();
  await expect(page.getByRole("region", { name: "Реакции" })).toHaveCount(0);
  await expect(page.locator(".room-footer .reaction-buttons")).toHaveCount(0);
  const leave = page.getByRole("button", {
    name: "Покинуть конференцию",
    exact: true,
  });
  await leave.click();
  const confirmation = page.getByRole("dialog", {
    name: "Выйти из встречи?",
    exact: true,
  });
  await expect(confirmation).toBeVisible();
  expect(await mediaState()).toEqual(beforeMedia);
  await confirmation
    .getByRole("button", { name: "Вернуться к встрече", exact: true })
    .click();
  await expect(confirmation).toHaveCount(0);
  await expect(leave).toBeFocused();
  const settings = page.getByRole("button", { name: "Настройки", exact: true });
  await expect(
    page.getByRole("button", { name: "Отключить медиа", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Устройства", exact: true }),
  ).toHaveCount(0);
  await settings.click();
  const dialog = page.getByRole("dialog", {
    name: "Настройки аккаунта",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  for (const name of ["Профиль", "Аудио", "Видео", "Уведомления", "Оформление"])
    await expect(dialog.getByRole("tab", { name, exact: true })).toBeVisible();
  await expect(page.locator(".app-shell")).toHaveAttribute("inert", "");
  await dialog.getByRole("tab", { name: "Профиль", exact: true }).focus();
  for (const key of ["m", "v", "c"]) await page.keyboard.press(key);
  expect(await mediaState()).toEqual(beforeMedia);
  expect(await rail.isVisible()).toBe(beforeChat);
  expect(page.url()).toBe(beforeURL);
  expect(isolated.connections()).toBe(1);
  await expect
    .poll(() => video.evaluate((node: HTMLVideoElement) => node.currentTime))
    .toBeGreaterThan(beforeFrames);
  await page.screenshot({
    path: info.outputPath("conference-account-settings.png"),
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(settings).toBeFocused();
  expect(page.url()).toBe(beforeURL);
  expect(await mediaState()).toEqual(beforeMedia);
  expect(isolated.connections()).toBe(1);
  expect(
    await video.evaluate((node: HTMLVideoElement) => {
      const runtime = window as Window & {
        layoutSettingsVideo?: HTMLVideoElement;
        layoutSettingsStream?: MediaProvider | null;
        layoutSettingsTrack?: MediaStreamTrack;
      };
      return (
        node === runtime.layoutSettingsVideo &&
        node.srcObject === runtime.layoutSettingsStream &&
        (node.srcObject as MediaStream).getVideoTracks()[0] ===
          runtime.layoutSettingsTrack &&
        runtime.layoutSettingsTrack.readyState === "live"
      );
    }),
  ).toBe(true);
  const resumedFrames = await video.evaluate(
    (node: HTMLVideoElement) => node.currentTime,
  );
  await expect
    .poll(() => video.evaluate((node: HTMLVideoElement) => node.currentTime))
    .toBeGreaterThan(resumedFrames);
  await expectNoOverflow(page);
  expect(isolated.denied).toEqual([]);
  expect(isolated.errors).toEqual([]);
});

test("мобильный гость открывает общие настройки до подключения медиа без разделов аккаунта", async ({
  page,
  baseURL,
}, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const isolated = await fixture(page, new URL(baseURL!).origin, {
    count: 1,
    guest: true,
  });
  await page.goto(`/conferences/${room.id}`);
  await expect(page.getByRole("region", { name: "Реакции" })).toHaveCount(0);
  await expect(page.locator(".room-footer .reaction-buttons")).toHaveCount(0);
  const beforeURL = page.url();
  const settings = page.getByRole("button", { name: "Настройки", exact: true });
  await expect(settings).toBeEnabled();
  await settings.click();
  const dialog = page.getByRole("dialog", {
    name: "Настройки аккаунта",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("tab")).toHaveText([
    "Аудио",
    "Видео",
    "Оформление",
  ]);
  await expect(
    dialog.getByRole("tab", { name: "Аудио", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(page.locator(".guest-room-shell")).toHaveAttribute("inert", "");
  await dialog.getByRole("tab", { name: "Оформление", exact: true }).click();
  await expect(
    dialog.getByRole("tabpanel", { name: "Оформление", exact: true }),
  ).toBeVisible();
  await expectNoOverflow(page);
  await page.screenshot({
    path: info.outputPath("conference-guest-settings-mobile.png"),
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(settings).toBeFocused();
  expect(page.url()).toBe(beforeURL);
  expect(isolated.connections()).toBe(1);
  expect(isolated.denied).toEqual([]);
  expect(isolated.errors).toEqual([]);
});

for (const { name, width, height, textSize } of [
  { name: "desktop", width: 1440, height: 1000, textSize: 100 },
  { name: "mobile", width: 390, height: 844, textSize: 100 },
  { name: "mobile-200", width: 390, height: 844, textSize: 200 },
]) {
  test(`переподключение вместо меню и прямой доступ к управлению: ${name}`, async ({
    page,
    baseURL,
  }, info) => {
    await page.setViewportSize({ width, height });
    const isolated = await fixture(page, new URL(baseURL!).origin, {
      count: 2,
      canvas: true,
    });
    await page.addInitScript((size) => {
      localStorage.setItem(
        "go-recorder.appearance.v1:owner",
        JSON.stringify({ version: 1, theme: "light", textSize: size }),
      );
    }, textSize);
    await page.goto(`/conferences/${room.id}`);
    await page
      .getByRole("button", { name: "Включить камеру и микрофон", exact: true })
      .click();
    await expect(
      page.getByTestId("local-media").locator("video"),
    ).toBeVisible();
    const controls = page.locator(".conference-control-bar");
    for (const label of [
      "Настройки",
      "Участники",
      "Чат",
      "Показать экран",
      "Переподключиться",
      "Покинуть конференцию",
    ])
      await expect(
        controls.getByRole("button", { name: label, exact: true }),
      ).toBeVisible();
    await expect(
      controls.getByRole("button", { name: "Ещё", exact: true }),
    ).toHaveCount(0);
    await expect(controls.getByRole("menu")).toHaveCount(0);
    await expect(
      page.getByRole("region", { name: "Реакции", exact: true }),
    ).toHaveCount(0);
    const reconnect = controls.getByRole("button", {
      name: "Переподключиться",
      exact: true,
    });
    await expect(reconnect).toHaveAttribute("title", "Переподключиться");
    await expect(reconnect.locator(".lucide-refresh-cw")).toBeVisible();
    await expectNoOverflow(page);
    await page.screenshot({
      path: info.outputPath(`conference-controls-${name}.png`),
      fullPage: true,
    });
    const beforeURL = page.url();
    expect(isolated.connections()).toBe(1);
    await reconnect.click();
    await expect.poll(() => isolated.connections()).toBe(2);
    await expect(page.getByTestId("local-media")).toHaveCount(0);
    await expect(
      controls.getByRole("button", {
        name: "Включить камеру и микрофон",
        exact: true,
      }),
    ).toBeEnabled();
    expect(page.url()).toBe(beforeURL);
    await expectNoOverflow(page);
    expect(isolated.denied).toEqual([]);
    expect(isolated.errors).toEqual([]);
    expect(
      await page.evaluate(
        () =>
          (window as Window & { layoutDeviceCalls?: number }).layoutDeviceCalls,
      ),
    ).toBe(0);
  });
}

for (const size of [
  {
    name: "desktop",
    width: 1440,
    height: 1000,
    textSize: 100,
    theme: "light",
    count: 1,
  },
  {
    name: "desktop-dark",
    width: 1440,
    height: 1000,
    textSize: 100,
    theme: "dark",
    count: 4,
  },
  {
    name: "narrow-200",
    width: 1024,
    height: 768,
    textSize: 200,
    theme: "dark",
    count: 4,
  },
  {
    name: "tablet",
    width: 834,
    height: 1000,
    textSize: 100,
    theme: "light",
    count: 4,
  },
  {
    name: "mobile",
    width: 390,
    height: 844,
    textSize: 100,
    theme: "light",
    count: 1,
  },
  {
    name: "mobile-200",
    width: 390,
    height: 844,
    textSize: 200,
    theme: "dark",
    count: 4,
  },
  {
    name: "landscape",
    width: 844,
    height: 390,
    textSize: 100,
    theme: "light",
    count: 4,
  },
]) {
  test(`правая панель: отступы, длинные имена, поиск и прокрутка — ${size.name}`, async ({
    page,
    baseURL,
  }, info) => {
    await page.setViewportSize({ width: size.width, height: size.height });
    const isolated = await fixture(page, new URL(baseURL!).origin, {
      count: size.count,
      nameSuffix:
        size.textSize === 200
          ? " — ОченьДлинноеИмяУчастникаДляПроверкиПереноса"
          : "",
    });
    await page.addInitScript(({ theme, textSize }) => {
      localStorage.setItem(
        "go-recorder.appearance.v1:owner",
        JSON.stringify({ version: 1, theme, textSize }),
      );
    }, size);
    await page.goto(`/meetings/${room.id}`);
    if (size.name === "landscape") {
      const media = page.getByRole("region", {
        name: "Медиапотоки встречи",
        exact: true,
      });
      await media.focus();
      await page.keyboard.press("End");
      await expect
        .poll(() => media.evaluate((element) => element.scrollTop))
        .toBeGreaterThan(0);
    }
    await page.getByRole("button", { name: "Чат", exact: true }).click();
    const message = page.getByRole("textbox", {
      name: "Сообщение",
      exact: true,
    });
    await message.fill("Черновик сохраняется при переключении вкладок");
    await page
      .getByRole("tab", { name: `Участники (${size.count})`, exact: true })
      .click();
    const rail = page.locator(".conference-stage-rail");
    const panel = page.locator("#meeting-panel-participants");
    const search = panel.getByRole("textbox", {
      name: "Поиск участника",
      exact: true,
    });
    await expect(search).toBeVisible();
    await expect(search).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    const tabsBox = (await rail.getByRole("tablist").boundingBox())!;
    const searchBox = (await panel
      .locator(".room-participant-search")
      .boundingBox())!;
    const headingBox = (await rail
      .locator(".room-panel-header strong")
      .boundingBox())!;
    expect(searchBox.y - tabsBox.y - tabsBox.height).toBeGreaterThanOrEqual(12);
    expect(Math.abs(searchBox.x - headingBox.x)).toBeLessThan(1.1);
    expect(
      await panel.evaluate(
        (element) => element.scrollWidth <= element.clientWidth,
      ),
    ).toBe(true);
    for (const identity of await panel
      .locator(".room-participant-identity")
      .all()) {
      const name = (await identity.locator("strong").boundingBox())!;
      const icons = (await identity
        .locator(".room-participant-media")
        .boundingBox())!;
      expect(name.x + name.width).toBeLessThanOrEqual(icons.x - 10);
      expect(
        await identity.evaluate(
          (element) => element.scrollWidth <= element.clientWidth,
        ),
      ).toBe(true);
    }
    const invite = panel.getByRole("button", {
      name: "Пригласить участников",
      exact: true,
    });
    const finish = panel.getByRole("button", {
      name: "Завершить конференцию",
      exact: true,
    });
    await finish.scrollIntoViewIfNeeded();
    await expect(finish).toBeInViewport();
    await expect(rail.getByRole("tablist")).toBeInViewport();
    const inviteBox = (await invite.boundingBox())!;
    const finishBox = (await finish.boundingBox())!;
    expect(Math.abs(inviteBox.x - searchBox.x)).toBeLessThan(1.1);
    expect(Math.abs(inviteBox.width - searchBox.width)).toBeLessThan(1.1);
    expect(Math.abs(finishBox.width - inviteBox.width)).toBeLessThan(1.1);
    expect(finishBox.y - inviteBox.y - inviteBox.height).toBeGreaterThanOrEqual(
      7,
    );
    await search.fill("Алексей");
    await expect(panel.locator(".room-participant-row")).toHaveCount(1);
    await search.focus();
    await expect(panel.locator(".room-participant-search")).toHaveCSS(
      "outline-style",
      "solid",
    );
    await expect(search).toHaveCSS("box-shadow", "none");
    await page.screenshot({
      path: info.outputPath(`sidebar-participants-${size.name}.png`),
      fullPage: true,
    });
    await page.getByRole("tab", { name: "Чат", exact: true }).click();
    await expect(message).toHaveValue(
      "Черновик сохраняется при переключении вкладок",
    );
    await page
      .getByRole("tab", { name: `Участники (${size.count})`, exact: true })
      .click();
    await expect(search).toHaveValue("Алексей");
    await expectNoOverflow(page);
    expect(isolated.connections()).toBe(1);
    expect(isolated.denied).toEqual([]);
    expect(isolated.errors).toEqual([]);
  });
}
