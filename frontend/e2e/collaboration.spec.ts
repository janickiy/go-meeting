import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { execFile } from "node:child_process";
import { randomUUID } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { promisify } from "node:util";

const execute = promisify(execFile);
const root = resolve(import.meta.dirname, "../..");
const base = process.env.MEET_STAGE5_DOCKER_UI || "https://localhost:18482";
const apiOrigin = process.env.MEET_STAGE5_DOCKER_API || "http://127.0.0.1:8085";
test.skip(
  process.env.MEET_STAGE5_DOCKER !== "true",
  "explicit local Compose acceptance opt-in required",
);
/**
 * Actor объединяет страницу, авторизацию и тестовую идентичность участника.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - token — токен текущей авторизации; null отключает авторизованные запросы.
 *   - email — адрес электронной почты.
 *   - name — отображаемое имя пользователя для инициалов.
 */
type Actor = { id: string; token: string; email: string; name: string };
/**
 * RecordCard описывает минимальные сведения записи для сквозной проверки.
 *
 * @params:
 *   - uuid — внешний UUID записи.
 *   - status — HTTP-статус либо состояние встречи.
 *   - files — доступные артефакты и выданные сервером ссылки.
 */
type RecordCard = {
  uuid: string;
  status: string;
  files: { fileType: string; url?: string }[];
};
/**
 * Message описывает серверное сообщение сквозной проверки чата.
 *
 * @params:
 *   - id — идентификатор ресурса или конференции данного запроса.
 *   - text — обычный текст сообщения.
 *   - deletedAt — время мягкого удаления либо null.
 *   - attachments — метаданные прикреплённых файлов.
 */
type Message = {
  id: string;
  text: string;
  deletedAt?: string;
  attachments: { id: string }[];
};
/**
 * request отправляет JSON-запрос к API, добавляет Bearer-токен для приватного маршрута и проверяет ответ; при 401 уведомляет владельца использованной сессии.
 *
 * @args
 *   - client (APIRequestContext) — входное значение client текущего шага обработки.
 *   - path (string) — локальный путь API без базового префикса.
 *   - token (string) — токен текущей авторизации; null отключает авторизованные запросы (необязательный параметр).
 *   - method — входное значение method текущего шага обработки (по умолчанию "GET").
 *   - data (unknown) — нагрузка события, проверяемая перед чтением (необязательный параметр).
 *
 * @returns Promise<T> — Promise с результатом описанной асинхронной операции; отказ передаётся через отклонение Promise.
 */
async function request<T>(
  client: APIRequestContext,
  path: string,
  token?: string,
  method = "GET",
  data?: unknown,
): Promise<T> {
  let result = await client.fetch(`${apiOrigin}/api/v1${path}`, {
    method,
    data,
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  // Consecutive acceptance suites share the real 5/min registration limit.
  // Retry only an explicitly rejected setup request, without changing budgets.
  if (
    result.status() === 429 &&
    ["/auth/register", "/auth/login"].includes(path)
  ) {
    const seconds = Math.min(
      60,
      Math.max(1, Number(result.headers()["retry-after"]) || 60),
    );
    await new Promise(
      /**
       * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
       *
       * @args
       *   - resolve — завершает ожидающий Promise успешным результатом.
       *
       * @returns вычисленное значение: setTimeout(resolve, (seconds + 1) * 1000).
       */ (resolve) => setTimeout(resolve, (seconds + 1) * 1000),
    );
    result = await client.fetch(`${apiOrigin}/api/v1${path}`, {
      method,
      data,
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    });
  }
  if (!result.ok())
    throw new Error(
      `${method} ${path}: HTTP ${result.status()} ${await result.text()}`,
    );
  return result.json() as Promise<T>;
}
/**
 * login отправляет учётные данные и получает токен и сведения пользователя.
 *
 * @args
 *   - page (Page) — изолированная страница Playwright.
 *   - actor (Actor) — входное значение actor текущего шага обработки.
 *   - password (string) — пароль из формы; не предназначен для журналирования.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
async function login(page: Page, actor: Actor, password: string) {
  await page.goto(`${base}/login`);
  await page.getByLabel("Email").fill(actor.email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL(/\/app/);
}
/**
 * enableMedia включает тестовые источники через интерфейс и ожидает связи.
 *
 * @args
 *   - page (Page) — изолированная страница Playwright.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
async function enableMedia(page: Page) {
  await page
    .getByRole("button", { name: "Включить камеру и микрофон", exact: true })
    .click();
  await expect(page.getByTestId("media-status")).toHaveText(
    "Медиасвязь подключена",
    { timeout: 30000 },
  );
}
/**
 * send передаёт исходящее событие через действующее соединение.
 *
 * @args
 *   - page (Page) — изолированная страница Playwright.
 *   - text (string) — обычный текст сообщения.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
async function send(page: Page, text: string) {
  await page.getByLabel("Сообщение", { exact: true }).fill(text);
  await page.getByRole("button", { name: "Отправить", exact: true }).click();
}

test("scheduled waiting room, durable chat/files, engagement, recording and history in real Docker", /**
 * Проверка: scheduled waiting room, durable chat/files, engagement, recording and history in real Docker выполняет тестовый сценарий «scheduled waiting room, durable chat/files, engagement, recording and history in real Docker» и проверяет ожидаемые результаты.
 *
 * @args
 *   - объект параметров: browser — браузер Playwright с отдельными тестовыми контекстами; request — параметры сообщения или другого API-действия.
 *   - info — контекст запуска для диагностических вложений.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ browser, request: client }, info) => {
  for (const address of [base, apiOrigin]) {
    const url = new URL(address);
    expect(["localhost", "127.0.0.1", "[::1]"]).toContain(url.hostname);
    expect(url.username || url.password || url.search).toBe("");
  }
  const runId = `stage5-docker-${randomUUID()}`;
  const password = `Smoke-${randomUUID()}`;
  const actors: Actor[] = [];
  const contexts = await Promise.all(
    [0, 1, 2].map(
      /**
       * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
       *
       *
       * @returns преобразованное значение текущего элемента для результирующего набора.
       */ () =>
        browser.newContext({
          ignoreHTTPSErrors: true,
          timezoneId: "UTC",
          viewport: { width: 1440, height: 1100 },
        }),
    ),
  );
  const pages = await Promise.all(
    contexts.map(
      /**
       * Обработчик contexts.map преобразует один элемент набора в представление или данные следующего шага.
       *
       * @args
       *   - context — входное значение context текущего шага обработки.
       *
       * @returns преобразованное значение текущего элемента для результирующего набора.
       */ (context) => context.newPage(),
    ),
  );
  const [owner, bob, rejected] = pages;
  const summary: Record<string, unknown> = { runId, fakeDevicesOnly: true };
  const roomSockets = [0, 0, 0];
  const socketErrors: unknown[] = [];
  const consoleErrors: string[] = [];
  for (const [index, page] of pages.entries()) {
    page.on(
      "pageerror",
      /**
       * Обработчик page.on выполняет переданный шаг вызова page.on в проверках клиентского поведения.
       *
       * @args
       *   - error — пойманная ошибка API или сети.
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */ (error) => consoleErrors.push(`${index}:${error.message}`),
    );
    page.on(
      "websocket",
      /**
       * Обработчик page.on выполняет переданный шаг вызова page.on в проверках клиентского поведения.
       *
       * @args
       *   - socket — входное значение socket текущего шага обработки.
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ (socket) => {
        roomSockets[index]++;
        socket.on(
          "framereceived",
          /**
           * Обработчик socket.on выполняет переданный шаг вызова socket.on в проверках клиентского поведения.
           *
           * @args
           *   - объект параметров: payload — ссылки и состояние уведомления без выдачи прав на ресурс.
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ ({ payload }) => {
            try {
              const event = JSON.parse(String(payload));
              if (event.type === "error")
                socketErrors.push({ index, data: event.data });
            } catch {
              /* ping */
            }
          },
        );
      },
    );
  }
  let conferenceId = "";
  let phase = "setup";
  try {
    for (const [index, name] of [
      "Stage5 owner",
      "Stage5 Bob",
      "Stage5 rejected",
    ].entries()) {
      const email = `${runId}-${index}@smoke.invalid`;
      const result = await request<{ user: { id: string } }>(
        client,
        "/auth/register",
        undefined,
        "POST",
        { email, password, displayName: name },
      );
      const actor = { id: result.user.id, email, name, token: "" };
      actors.push(actor);
      actor.token = (
        await request<{ accessToken: string }>(
          client,
          "/auth/login",
          undefined,
          "POST",
          { email, password },
        )
      ).accessToken;
    }
    await Promise.all(
      pages.map(
        /**
         * Обработчик pages.map преобразует один элемент набора в представление или данные следующего шага.
         *
         * @args
         *   - page — изолированная страница Playwright.
         *   - index — входное значение index текущего шага обработки.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (page, index) => login(page, actors[index], password),
      ),
    );
    phase = "schedule-create";
    await owner
      .getByRole("link", { name: "Новая конференция", exact: true })
      .click();
    await owner.getByLabel("Название конференции").fill(runId);
    await owner.getByRole("checkbox", { name: /Зал ожидания/ }).check();
    await owner
      .getByRole("checkbox", { name: /Запланировать встречу/ })
      .check();
    const scheduledAt = new Date(Date.now() + 10 * 60000)
      .toISOString()
      .slice(0, 16);
    await owner.getByLabel("Дата и время", { exact: true }).fill(scheduledAt);
    await owner.getByLabel(/Плановая длительность/).fill("30");
    const createResponse = owner.waitForResponse(
      /**
       * Обработчик owner.waitForResponse выполняет переданный шаг вызова owner.waitForResponse в проверках клиентского поведения.
       *
       * @args
       *   - response — входное значение response текущего шага обработки.
       *
       * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
       */
      (response) =>
        response.url().endsWith("/api/v1/conferences") &&
        response.request().method() === "POST",
    );
    await owner
      .getByRole("button", { name: "Создать конференцию", exact: true })
      .click();
    const created = (await (await createResponse).json()).item;
    conferenceId = created.id;
    summary.conferenceId = conferenceId;
    expect(created.status).toBe("scheduled");
    expect(created.waitingRoomEnabled).toBe(true);
    expect(created.scheduledAt).toBe(`${scheduledAt}:00Z`);
    await owner
      .getByRole("link", { name: "Перейти в конференцию", exact: true })
      .click();
    await bob.goto(`${base}/i/${created.inviteCode}`);
    await bob.getByRole("button", { name: "Добавить в мои встречи" }).click();
    await expect(bob.getByText(/Встреча добавлена/)).toBeVisible();
    await bob.goto(`${base}/app`);
    await expect(bob.getByRole("link", { name: runId })).toBeVisible();
    await bob.getByRole("link", { name: runId }).click();
    await bob.getByRole("button", { name: "Уведомления", exact: true }).click();
    await expect(bob.getByRole("dialog")).toContainText(
      "Скоро начнётся встреча",
      { timeout: 20000 },
    );
    await bob.keyboard.press("Escape");
    phase = "waiting-admission";
    await owner
      .getByRole("button", { name: "Начать конференцию", exact: true })
      .click();
    await owner
      .getByRole("button", { name: "Присоединиться", exact: true })
      .click();
    await owner.getByRole("button", { name: "Войти во встречу" }).click();
    await expect(owner.getByTestId("connection-id")).toBeVisible();
    await bob.reload();
    await expect(bob.getByTestId("waiting-room")).toContainText(
      "Вы в зале ожидания",
    );
    await expect(bob.getByTestId("connection-id")).toHaveCount(0);
    await expect(
      bob.getByRole("region", { name: "Чат конференции" }),
    ).toHaveCount(0);
    expect(roomSockets[1]).toBe(0);
    for (const suffix of ["messages", "recordings", "participants"]) {
      const forbidden = await client.get(
        `${apiOrigin}/api/v1/conferences/${conferenceId}/${suffix}`,
        { headers: { Authorization: `Bearer ${actors[1].token}` } },
      );
      expect(forbidden.status()).toBe(403);
    }
    await bob.reload();
    await expect(bob.getByTestId("waiting-room")).toBeVisible();
    await owner.getByRole("button", { name: "Допустить: Stage5 Bob" }).click();
    await expect(bob.getByTestId("connection-id")).toBeVisible();
    await rejected.goto(`${base}/i/${created.inviteCode}`);
    await rejected
      .getByRole("button", {
        name: "Проверить устройства и войти",
        exact: true,
      })
      .click();
    await rejected.getByRole("button", { name: "Войти во встречу" }).click();
    await expect(rejected.getByTestId("waiting-room")).toBeVisible();
    await owner
      .getByRole("button", { name: "Отклонить: Stage5 rejected" })
      .click();
    await expect(rejected.getByTestId("waiting-room")).toContainText(
      "Вход во встречу отклонён",
    );
    expect(roomSockets[2]).toBe(0);
    await expect(
      owner.locator('[aria-label="Управление: Stage5 rejected"]'),
    ).toHaveCount(0);
    await expect(bob.getByTestId(`presence-${actors[2].id}`)).toHaveCount(0);
    await Promise.all([enableMedia(owner), enableMedia(bob)]);
    await expect(owner.getByTestId("remote-media")).toHaveCount(1);
    await expect
      .poll(
        /**
         * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
         *
         *
         * @returns актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
         */ () =>
          owner
            .getByTestId("remote-media")
            .locator("video")
            .evaluate(
              /**
               * Обработчик evaluate выполняет переданный шаг вызова evaluate в проверках клиентского поведения.
               *
               * @args
               *   - video (HTMLVideoElement) — входное значение video текущего шага обработки.
               *
               * @returns вычисленное значение: video.getVideoPlaybackQuality().totalVideoFrames.
               */
              (video: HTMLVideoElement) =>
                video.getVideoPlaybackQuality().totalVideoFrames,
            ),
      )
      .toBeGreaterThan(10);
    await owner
      .getByRole("button", { name: "Начать запись", exact: true })
      .click();
    await expect(owner.getByTestId("recording-indicator")).toContainText(
      "Идёт запись",
    );
    await expect(bob.getByTestId("recording-indicator")).toContainText(
      "Идёт запись",
    );
    phase = "chat-idempotency";
    /**
     * frameSample читает число декодированных кадров для проверки движения видео.
     *
     *
     * @returns Promise, который после завершения операции возвращает: вычисленные данные текущего шага, которые использует вызывающая операция.
     */
    const frameSample = async () =>
      owner
        .getByTestId("remote-media")
        .locator("video")
        .evaluate(
          /**
           * Обработчик evaluate выполняет переданный шаг вызова evaluate в проверках клиентского поведения.
           *
           * @args
           *   - video (HTMLVideoElement) — входное значение video текущего шага обработки.
           *
           * @returns новый объект вычисленных данных.
           */ (video: HTMLVideoElement) => ({
            at: Date.now(),
            frames: video.getVideoPlaybackQuality().totalVideoFrames,
          }),
        );
    const framesBeforeChat = await frameSample();
    await bob.getByRole("button", { name: "Свернуть чат" }).click();
    let loseFirstResponse = true;
    await owner.route(
      `**/api/v1/conferences/${conferenceId}/messages`,
      /**
       * Обработчик owner.route выполняет браузерную часть проверяемого сценария в изолированном тестовом контексте.
       *
       * @args
       *   - route — входное значение route текущего шага обработки.
       *
       * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      async (route) => {
        if (route.request().method() === "POST" && loseFirstResponse) {
          loseFirstResponse = false;
          const result = await route.fetch();
          expect(result.ok()).toBe(true);
          await route.abort("failed");
        } else await route.continue();
      },
    );
    await send(owner, "Сообщение после разрыва связи");
    await owner.getByRole("button", { name: "Повторить отправку" }).click();
    await expect(owner.getByLabel("Сообщение", { exact: true })).toHaveValue(
      "",
    );
    await owner.unroute(`**/api/v1/conferences/${conferenceId}/messages`);
    /**
     * chat читает сообщения тестовой конференции для проверки постоянного состояния.
     *
     *
     * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
     */
    const chat = () =>
      request<{ items: Message[] }>(
        client,
        `/conferences/${conferenceId}/messages`,
        actors[0].token,
      );
    await expect
      .poll(
        /**
         * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
         *
         *
         * @returns Promise, который после завершения операции возвращает: актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
         */
        async () =>
          (await chat()).items.filter(
            /**
             * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
             *
             * @args
             *   - message — текущий объект сообщения чата.
             *
             * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
             */
            (message) => message.text === "Сообщение после разрыва связи",
          ).length,
      )
      .toBe(1);
    await expect(bob.getByTestId("chat-unread")).toHaveText("1");
    await bob.getByRole("button", { name: "Открыть чат" }).click();
    const first = (await chat()).items[0];
    await bob.getByTestId(`chat-message-${first.id}`).scrollIntoViewIfNeeded();
    await expect
      .poll(
        /**
         * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
         *
         *
         * @returns Promise, который после завершения операции возвращает: актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
         */
        async () =>
          (
            await request<{ item: { unreadCount: number } }>(
              client,
              `/conferences/${conferenceId}/chat/read`,
              actors[1].token,
            )
          ).item.unreadCount,
      )
      .toBe(0);
    await bob
      .getByTestId(`chat-message-${first.id}`)
      .getByRole("button", { name: "Ответить" })
      .click();
    await send(bob, "Ответ участника");
    await expect(owner.getByRole("log")).toContainText("Ответ участника");
    const original = owner.getByTestId(`chat-message-${first.id}`);
    await original
      .getByRole("button", { name: "Изменить", exact: true })
      .click();
    await owner
      .getByRole("dialog")
      .getByLabel("Текст")
      .fill("Исправленное сообщение");
    await owner.getByRole("button", { name: "Сохранить сообщение" }).click();
    await expect(bob.getByTestId(`chat-message-${first.id}`)).toContainText(
      "Исправленное сообщение",
    );
    await original
      .getByRole("button", { name: "Удалить", exact: true })
      .click();
    await owner.getByRole("button", { name: "Да, удалить сообщение" }).click();
    await expect(bob.getByTestId(`chat-message-${first.id}`)).toContainText(
      "Сообщение удалено",
    );
    phase = "private-attachment";
    const fileBody = Buffer.from(
      "Stage five private attachment.\n".repeat(40000),
    );
    await owner.getByLabel("Выбрать файлы для сообщения").setInputFiles({
      name: "private-notes.txt",
      mimeType: "text/plain",
      buffer: fileBody,
    });
    await expect(owner.getByText(/Готов к отправке/)).toBeVisible();
    await send(owner, "Материалы встречи");
    await expect(
      bob.getByText("private-notes.txt", { exact: false }),
    ).toBeVisible();
    const attached = (await chat()).items.find(
      /**
       * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @args
       *   - message — текущий объект сообщения чата.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      (message) => message.attachments.length,
    )!;
    expect(attached).toBeTruthy();
    const attachmentId = attached.attachments[0].id;
    const fileMessage = bob.getByTestId(`chat-message-${attached.id}`);
    await fileMessage.getByRole("button", { name: "Получить ссылку" }).click();
    const signed = await fileMessage
      .getByRole("link", { name: "Скачать файл" })
      .getAttribute("href");
    expect(signed).toBeTruthy();
    const download = await client.get(signed!);
    expect(download.status()).toBe(200);
    expect(await download.body()).toEqual(fileBody);
    const unsigned = new URL(signed!);
    unsigned.search = "";
    expect((await client.get(unsigned.toString())).status()).toBe(403);
    const unauthorizedFile = await client.get(
      `${apiOrigin}/api/v1/conferences/${conferenceId}/attachments/${attachmentId}/download`,
      { headers: { Authorization: `Bearer ${actors[2].token}` } },
    );
    expect(unauthorizedFile.status()).toBe(403);
    summary.privateAttachmentBytes = fileBody.length;
    phase = "engagement";
    await bob
      .getByRole("button", { name: "Поднять руку", exact: true })
      .click();
    await expect(
      owner.getByRole("list", { name: "Поднятые руки" }),
    ).toContainText("Stage5 Bob");
    await bob.getByRole("button", { name: "Реакция 👍", exact: true }).click();
    await expect(owner.locator(".reaction-bubble")).toContainText("Stage5 Bob");
    await owner
      .getByRole("button", { name: "Опустить руку: Stage5 Bob" })
      .click();
    await expect(
      bob.getByRole("button", { name: "Поднять руку", exact: true }),
    ).toBeVisible();
    await owner.screenshot({
      path: info.outputPath("stage5-room-chat.png"),
      fullPage: true,
    });
    await expect(owner.getByTestId("media-status")).toHaveText(
      "Медиасвязь подключена",
    );
    const framesAfterChat = await frameSample();
    summary.mediaDuringCollaboration = {
      before: framesBeforeChat,
      after: framesAfterChat,
      decodedFPS:
        ((framesAfterChat.frames - framesBeforeChat.frames) * 1000) /
        (framesAfterChat.at - framesBeforeChat.at),
    };
    expect(framesAfterChat.frames).toBeGreaterThan(framesBeforeChat.frames);
    const containerIDs = (
      await execute(
        "docker",
        ["compose", "ps", "-q", "media-worker", "worker"],
        { cwd: root, timeout: 10000 },
      )
    ).stdout
      .trim()
      .split(/\s+/);
    summary.resourceSample = (
      await execute(
        "docker",
        ["stats", "--no-stream", "--format", "{{json .}}", ...containerIDs],
        { timeout: 10000 },
      )
    ).stdout
      .trim()
      .split("\n")
      .map(
        /**
         * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
         *
         * @args
         *   - line — строка входящего текстового потока.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (line) => JSON.parse(line),
      );
    phase = "recording-history-notifications";
    await owner
      .getByRole("button", { name: "Остановить запись", exact: true })
      .click();
    /**
     * records читает записи тестовой конференции для проверки готовности.
     *
     *
     * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
     */
    const records = () =>
      request<{ items: RecordCard[] }>(
        client,
        `/conferences/${conferenceId}/recordings`,
        actors[0].token,
      );
    await expect
      .poll(
        /**
         * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
         *
         *
         * @returns Promise, который после завершения операции возвращает: актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
         */ async () => (await records()).items[0]?.status,
        { timeout: 90000 },
      )
      .toBe("ready");
    const record = (await records()).items[0];
    summary.recordingId = record.uuid;
    expect(
      record.files.some(
        /**
         * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
         *
         * @args
         *   - file — выбранный пользователем файл для проверки или передачи.
         *
         * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
         */ (file) => file.fileType === "preview_jpg",
      ),
    ).toBe(true);
    await bob.getByRole("button", { name: "Уведомления", exact: true }).click();
    await expect(bob.getByRole("dialog")).toContainText(
      "Запись встречи готова",
      { timeout: 25000 },
    );
    await expect(bob.getByRole("dialog")).toContainText(
      "Вас пригласили войти во встречу",
    );
    await bob
      .getByRole("button", { name: "Прочитано: Запись встречи готова" })
      .click();
    await expect
      .poll(
        /**
         * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
         *
         *
         * @returns Promise, который после завершения операции возвращает: актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
         */
        async () =>
          (
            await request<{ items: { type: string; readAt: string | null }[] }>(
              client,
              "/notifications",
              actors[1].token,
            )
          ).items.find(
            /**
             * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
             *
             * @args
             *   - item — элемент списка, который обрабатывает текущий шаг.
             *
             * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
             */ (item) => item.type === "recording.ready",
          )?.readAt,
      )
      .toBeTruthy();
    await bob.keyboard.press("Escape");
    await owner
      .getByRole("button", { name: "Завершить конференцию", exact: true })
      .click();
    await owner
      .getByRole("button", { name: "Да, завершить", exact: true })
      .click();
    await expect(
      owner.getByRole("region", { name: "История встречи" }),
    ).toBeVisible();
    await expect(owner.getByLabel("Сообщение", { exact: true })).toHaveCount(0);
    await expect(owner.getByRole("log")).toContainText("Материалы встречи");
    await bob.reload();
    await expect(
      bob.getByRole("region", { name: "История встречи" }),
    ).toBeVisible();
    await expect(bob.getByLabel("Сообщение", { exact: true })).toHaveCount(0);
    await expect(bob.getByTestId("media-status")).toHaveCount(0);
    await owner.screenshot({
      path: info.outputPath("stage5-history.png"),
      fullPage: true,
    });
    await bob.goto(`${base}/conferences?view=past`);
    await expect(bob.getByRole("link", { name: runId })).toBeVisible();
    expect(socketErrors).toEqual([]);
    expect(consoleErrors).toEqual([]);
    summary.completed = true;
  } finally {
    summary.phase = phase;
    summary.socketErrors = socketErrors;
    summary.pageErrors = consoleErrors;
    summary.roomSockets = roomSockets;
    for (const [index, page] of pages.entries()) {
      if (!summary.completed)
        await page
          .screenshot({
            path: info.outputPath(`failure-${index}.png`),
            fullPage: true,
          })
          .catch(
            /**
             * Обработчик catch выполняет переданный шаг вызова catch в проверках клиентского поведения.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ () => {},
          );
    }
    await Promise.all(
      contexts.map(
        /**
         * Обработчик contexts.map преобразует один элемент набора в представление или данные следующего шага.
         *
         * @args
         *   - context — входное значение context текущего шага обработки.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (context) => context.close(),
      ),
    );
    let terminal = true;
    if (conferenceId && actors[0]?.token) {
      try {
        const state = (
          await request<{ item: { status: string } }>(
            client,
            `/conferences/${conferenceId}`,
            actors[0].token,
          )
        ).item.status;
        if (state === "active")
          await request(
            client,
            `/conferences/${conferenceId}/finish`,
            actors[0].token,
            "POST",
          );
        else if (["created", "scheduled"].includes(state))
          await request(
            client,
            `/conferences/${conferenceId}/cancel`,
            actors[0].token,
            "POST",
          );
        await expect
          .poll(
            /**
             * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
             *
             *
             * @returns Promise, который после завершения операции возвращает: актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
             */
            async () =>
              (
                await request<{ items: RecordCard[] }>(
                  client,
                  `/conferences/${conferenceId}/recordings`,
                  actors[0].token,
                )
              ).items.every(
                /**
                 * Обработчик every проверяет, соответствует ли текущий элемент условию выборки или поиска.
                 *
                 * @args
                 *   - item — элемент списка, который обрабатывает текущий шаг.
                 *
                 * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                 */ (item) =>
                  ["ready", "failed", "cancelled"].includes(item.status),
              ),
            { timeout: 90000 },
          )
          .toBe(true);
      } catch {
        terminal = false;
      }
    }
    const manifest = info.outputPath("cleanup.json");
    await writeFile(
      manifest,
      JSON.stringify({
        runId,
        conferenceId,
        userIds: actors.map(
          /**
           * Обработчик actors.map преобразует один элемент набора в представление или данные следующего шага.
           *
           * @args
           *   - actor — входное значение actor текущего шага обработки.
           *
           * @returns преобразованное значение текущего элемента для результирующего набора.
           */ (actor) => actor.id,
        ),
      }),
    );
    if (terminal) {
      try {
        summary.cleanup = (
          await execute("go", ["run", "./tools/smoke_cleanup", manifest], {
            cwd: root,
            timeout: 60000,
          })
        ).stdout.trim();
      } catch (error) {
        summary.cleanupError = String(error).slice(0, 1200);
      }
    } else
      summary.cleanupError = "recording not terminal; exact manifest retained";
    await writeFile(
      info.outputPath("acceptance.json"),
      JSON.stringify(summary, null, 2),
    );
    if (summary.completed) expect(summary.cleanupError).toBeUndefined();
  }
});
