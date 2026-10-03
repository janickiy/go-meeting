import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { execFile } from "node:child_process";
import { randomUUID } from "node:crypto";
import { cp, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { promisify } from "node:util";

const execute = promisify(execFile);
test.skip(
  process.env.MEET_STAGE4_DOCKER !== "true",
  "explicit local Compose acceptance opt-in required",
);
const root = resolve(import.meta.dirname, "../..");
const base = process.env.MEET_STAGE4_DOCKER_UI || "http://127.0.0.1:5173";
const api = process.env.MEET_STAGE4_DOCKER_API || "http://127.0.0.1:8085";
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
 * Card описывает сведения записи браузерного сценария.
 *
 * @params:
 *   - uuid — внешний UUID записи.
 *   - status — HTTP-статус либо состояние встречи.
 *   - errorMessage — безопасная причина отказа.
 *   - files — доступные артефакты и выданные сервером ссылки.
 */
type Card = {
  uuid: string;
  status: string;
  errorMessage?: string;
  files: { fileType: string; url?: string; sizeBytes?: number }[];
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
  const response = await client.fetch(`${api}/api/v1${path}`, {
    method,
    data,
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!response.ok())
    throw new Error(`${method} ${path}: HTTP ${response.status()}`);
  return (await response.json()) as T;
}

/**
 * login отправляет учётные данные и получает токен и сведения пользователя.
 *
 * @args
 *   - page (Page) — изолированная страница Playwright.
 *   - actor (Actor) — входное значение actor текущего шага обработки.
 *   - password (string) — пароль из формы; не предназначен для журналирования.
 *   - conference (string) — входное значение conference текущего шага обработки.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
async function login(
  page: Page,
  actor: Actor,
  password: string,
  conference: string,
) {
  await page.goto(`${base}/login`);
  await page.getByLabel("Email").fill(actor.email);
  await page.getByLabel("Пароль", { exact: true }).fill(password);
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL(/\/app/);
  await page.goto(`${base}/conferences/${conference}`);
  await expect(page.getByTestId("connection-id")).toBeVisible();
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

test("real Docker UI conference recording produces private MP4 and preview", /**
 * Проверяет, что запись конференции через реальный интерфейс Docker создаёт приватные MP4 и превью.
 *
 * @args
 *   - объект параметров: browser — браузер Playwright с отдельными тестовыми контекстами; request — параметры сообщения или другого API-действия.
 *   - info — контекст запуска для диагностических вложений.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ browser, request: client }, info) => {
  for (const address of [base, api]) {
    const endpoint = new URL(address);
    expect(["localhost", "127.0.0.1", "[::1]"]).toContain(endpoint.hostname);
    expect(endpoint.username || endpoint.password || endpoint.search).toBe("");
  }
  const runId = `stage4-docker-${randomUUID()}`;
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
  const [owner, bob, carol] = pages;
  let conferenceId = "";
  let recordingId = "";
  let phase = "setup";
  const samples: Record<string, unknown>[] = [];
  const ffmpegProcessSamples: Record<string, unknown>[] = [];
  const events: Record<string, unknown>[] = [];
  const summary: Record<string, unknown> = {
    runId,
    syntheticDevices: true,
    samples,
    ffmpegProcessSamples,
    events,
  };
  let timer: ReturnType<typeof setInterval> | undefined;
  let sampling: Promise<void> | null = null;
  let processTimer: ReturnType<typeof setInterval> | undefined;
  let processSampling: Promise<void> | null = null;
  for (const [index, page] of pages.entries()) {
    await page.exposeFunction(
      "stageFourDiagnostic",
      /**
       * Обработчик page.exposeFunction выполняет переданный шаг вызова page.exposeFunction в проверках клиентского поведения.
       *
       * @args
       *   - event (Record<string, unknown>) — проверенный конверт события комнаты.
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */
      (event: Record<string, unknown>) => {
        events.push({ browser: index, at: new Date().toISOString(), ...event });
      },
    );
    await page.addInitScript(
      /**
       * Обработчик page.addInitScript выполняет переданный шаг вызова page.addInitScript в проверках клиентского поведения.
       *
       *
       * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
       */ () => {
        const peers: RTCPeerConnection[] = [];
        Object.assign(window, { stageFourPeers: peers });
        const NativePeer = window.RTCPeerConnection;
        window.RTCPeerConnection = class extends NativePeer {
          /**
           * constructor создаёт подставной объект с переданными параметрами для проверки записи.
           *
           * @args
           *   - configuration (RTCConfiguration) — входное значение configuration текущего шага обработки (необязательный параметр).
           *
           * @returns инициализированный экземпляр текущего класса.
           */
          constructor(configuration?: RTCConfiguration) {
            super(configuration);
            peers.push(this);
          }
        };
        /**
         * report собирает диагностику медиа и записи для протокола теста.
         *
         * @args
         *   - event (Record<string, unknown>) — проверенный конверт события комнаты.
         *
         * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
         */
        const report = (event: Record<string, unknown>) => {
          void (
            window as unknown as {
              stageFourDiagnostic: /**
               * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
               *
               * @args
               *   - event (Record<string, unknown>) — проверенный конверт события комнаты.
               *
               * @returns Promise<void> — Promise с результатом описанной асинхронной операции; отказ передаётся через отклонение Promise.
               */ (event: Record<string, unknown>) => Promise<void>;
            }
          ).stageFourDiagnostic(event);
        };
        const NativeWebSocket = window.WebSocket;
        window.WebSocket = class extends NativeWebSocket {
          /**
           * constructor создаёт подставной объект с переданными параметрами для проверки записи.
           *
           * @args
           *   - url (string | URL) — адрес запроса или ресурса.
           *   - protocols (string | string[]) — входное значение protocols текущего шага обработки (необязательный параметр).
           *
           * @returns инициализированный экземпляр текущего класса.
           */
          constructor(url: string | URL, protocols?: string | string[]) {
            super(url, protocols);
            this.addEventListener(
              "close",
              /**
               * Обработчик addEventListener выполняет переданный шаг вызова addEventListener в проверках клиентского поведения.
               *
               * @args
               *   - event — проверенный конверт события комнаты.
               *
               * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
               */ (event) =>
                report({
                  type: "diagnostic.websocket.closed",
                  code: event.code,
                  reason: event.reason,
                }),
            );
          }
          /**
           * close закрывает форму или соединение с предусмотренной очисткой.
           *
           * @args
           *   - code (number) — проверенный код приглашения (необязательный параметр).
           *   - reason (string) — входное значение reason текущего шага обработки (необязательный параметр).
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */
          override close(code?: number, reason?: string) {
            report({
              type: "diagnostic.websocket.close.called",
              code,
              reason,
              stack: new Error().stack,
            });
            super.close(code, reason);
          }
        };
        const nativeClose = RTCPeerConnection.prototype.close;
        RTCPeerConnection.prototype.close =
          /**
           * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
           *
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */ function () {
            report({
              type: "diagnostic.peer.close.called",
              state: this.connectionState,
              stack: new Error().stack,
            });
            nativeClose.call(this);
          };
        navigator.mediaDevices.getDisplayMedia =
          /**
           * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
           *
           *
           * @returns Promise, который после завершения операции возвращает: вычисленное значение: stream.
           */ async () => {
            const canvas = document.createElement("canvas");
            canvas.width = 960;
            canvas.height = 540;
            /**
             * paint рисует тестовое изображение камеры или экрана.
             *
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */
            const paint = () => {
              const c = canvas.getContext("2d")!;
              c.fillStyle = "#125bdd";
              c.fillRect(0, 0, 960, 540);
              c.fillStyle = "#fff";
              c.font = "48px sans-serif";
              c.fillText("Stage 4 recording screen", 45, 230);
              c.font = "28px monospace";
              c.fillText(new Date().toISOString(), 45, 295);
            };
            paint();
            const paintTimer = setInterval(paint, 66);
            const stream = canvas.captureStream(15);
            const video = stream.getVideoTracks()[0];
            const stop = video.stop.bind(video);
            video.stop =
              /**
               * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                clearInterval(paintTimer);
                stop();
              };
            return stream;
          };
      },
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
        /**
         * collect собирает контрольные замеры проверяемого сценария.
         *
         * @args
         *   - direction ("framereceived" | "framesent") — входное значение direction текущего шага обработки.
         *
         * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
         */
        const collect =
          (direction: "framereceived" | "framesent") =>
          /**
           * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
           *
           * @args
           *   - объект параметров: payload — ссылки и состояние уведомления без выдачи прав на ресурс.
           *
           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
           */
          ({ payload }: { payload: string | Buffer }) => {
            try {
              const event = JSON.parse(String(payload));
              if (
                event.type?.startsWith("recording.") ||
                event.type?.startsWith("media.") ||
                event.type === "conference.state" ||
                event.type === "participant.media.updated" ||
                event.type === "participant.role.updated" ||
                event.type === "error"
              )
                events.push({
                  browser: index,
                  at: new Date().toISOString(),
                  direction,
                  type: event.type,
                  ...(event.data?.connectionId
                    ? { connectionId: event.data.connectionId }
                    : {}),
                  ...(event.data?.code ? { code: event.data.code } : {}),
                });
            } catch {
              /* Игнорируем служебные кадры без JSON. */
            }
          };
        socket.on("framereceived", collect("framereceived"));
        socket.on("framesent", collect("framesent"));
      },
    );
  }
  /**
   * diagnostics читает диагностику тестового браузера и медиасервиса.
   *
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  const diagnostics = async () => {
    summary.browserMedia = await Promise.all(
      pages.map(
        /**
         * Обработчик pages.map преобразует один элемент набора в представление или данные следующего шага.
         *
         * @args
         *   - page — изолированная страница Playwright.
         *
         * @returns преобразованное значение текущего элемента для результирующего набора.
         */ (page) =>
          page
            .evaluate(
              /**
               * Обработчик page.evaluate выполняет браузерную часть проверяемого сценария в изолированном тестовом контексте.
               *
               *
               * @returns Promise, который после завершения операции возвращает: объект с данными, собранными в текущей операции.
               */ async () => {
                const peers =
                  (
                    window as unknown as {
                      stageFourPeers?: RTCPeerConnection[];
                    }
                  ).stageFourPeers || [];
                return {
                  buttons: [...document.querySelectorAll("button")].map(
                    /**
                     * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
                     *
                     * @args
                     *   - button — входное значение button текущего шага обработки.
                     *
                     * @returns преобразованное значение текущего элемента для результирующего набора.
                     */
                    (button) => button.textContent,
                  ),
                  videos: [...document.querySelectorAll("video")].map(
                    /**
                     * Обработчик map преобразует текущий элемент в данные или представление результирующего списка.
                     *
                     * @args
                     *   - video — входное значение video текущего шага обработки.
                     *
                     * @returns новый объект вычисленных данных.
                     */ (video) => ({
                      label: video.closest(".media-tile")?.textContent,
                      width: video.videoWidth,
                      height: video.videoHeight,
                      decoded: video.getVideoPlaybackQuality().totalVideoFrames,
                    }),
                  ),
                  peers: await Promise.all(
                    peers.map(
                      /**
                       * Обработчик peers.map преобразует один элемент набора в представление или данные следующего шага.
                       *
                       * @args
                       *   - peer — входное значение peer текущего шага обработки.
                       *
                       * @returns Promise, который после завершения операции возвращает: преобразованное значение текущего элемента для результирующего набора.
                       */ async (peer) => {
                        const reports: Record<string, unknown>[] = [];
                        (await peer.getStats()).forEach(
                          /**
                           * Обработчик forEach выполняет переданный шаг вызова forEach в проверках клиентского поведения.
                           *
                           * @args
                           *   - stat — входное значение stat текущего шага обработки.
                           *
                           * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
                           */ (stat) => {
                            if (
                              ["inbound-rtp", "outbound-rtp"].includes(
                                stat.type,
                              )
                            ) {
                              const safe: Record<string, unknown> = {};
                              for (const key of [
                                "type",
                                "kind",
                                "framesSent",
                                "framesEncoded",
                                "keyFramesEncoded",
                                "framesDecoded",
                                "keyFramesDecoded",
                                "packetsSent",
                                "packetsReceived",
                                "frameWidth",
                                "frameHeight",
                                "ssrc",
                                "mid",
                              ])
                                if (stat[key] !== undefined)
                                  safe[key] = stat[key];
                              reports.push(safe);
                            }
                          },
                        );
                        return { state: peer.connectionState, reports };
                      },
                    ),
                  ),
                };
              },
            )
            .catch(
              /**
               * Обработчик catch выполняет переданный шаг вызова catch в проверках клиентского поведения.
               *
               *
               * @returns новый объект вычисленных данных.
               */ () => ({ unavailable: true }),
            ),
      ),
    );
    if (recordingId && /^[0-9a-f-]{36}$/.test(recordingId)) {
      try {
        await cp(
          resolve(root, "dockers/storage/data/records", recordingId),
          info.outputPath("spool-snapshot"),
          { recursive: true },
        );
        summary.spoolSnapshot = "spool-snapshot";
      } catch {
        /* После загрузки воркер удаляет временные файлы успешной записи. */
      }
    }
  };
  try {
    for (const [index, name] of [
      "Smoke owner",
      "Smoke Bob",
      "Smoke Carol",
    ].entries()) {
      const email = `${runId}-${index}@smoke.invalid`;
      const registered = await request<{ user: { id: string } }>(
        client,
        "/auth/register",
        undefined,
        "POST",
        { email, password, displayName: name },
      );
      // Сразу сохраняем созданную учётную запись, чтобы очистить её и при последующем сбое входа.
      const actor = { id: registered.user.id, token: "", email, name };
      actors.push(actor);
      const session = await request<{ accessToken: string }>(
        client,
        "/auth/login",
        undefined,
        "POST",
        { email, password },
      );
      actor.token = session.accessToken;
    }
    const created = await request<{ item: { id: string; inviteCode: string } }>(
      client,
      "/conferences",
      actors[0].token,
      "POST",
      { title: runId },
    );
    conferenceId = created.item.id;
    summary.conferenceId = conferenceId;
    await request(
      client,
      `/conferences/${conferenceId}/join`,
      actors[0].token,
      "POST",
      {},
    );
    await request(
      client,
      `/conference-invites/${created.item.inviteCode}/join`,
      actors[1].token,
      "POST",
      {},
    );
    await login(owner, actors[0], password, conferenceId);
    await owner
      .getByRole("button", { name: "Начать конференцию", exact: true })
      .click();
    await expect(
      owner.getByRole("button", { name: "Завершить конференцию", exact: true }),
    ).toBeVisible();
    await login(bob, actors[1], password, conferenceId);
    await enableMedia(owner);
    await enableMedia(bob);
    for (const page of [owner, bob]) {
      await expect(page.getByTestId("remote-media")).toHaveCount(1);
      await expect
        .poll(
          /**
           * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
           *
           *
           * @returns актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
           */ () =>
            page
              .getByTestId("remote-media")
              .locator("video")
              .evaluate(
                /**
                 * Обработчик evaluate выполняет переданный шаг вызова evaluate в проверках клиентского поведения.
                 *
                 * @args
                 *   - node — DOM-элемент, к которому привязывается медиапоток.
                 *
                 * @returns вычисленное значение: video.videoWidth > 0 && stream.getAudioTracks().some( (track) => track.readyState === "live", ).
                 */ (node) => {
                  const video = node as HTMLVideoElement;
                  const stream = video.srcObject as MediaStream;
                  return (
                    video.videoWidth > 0 &&
                    stream.getAudioTracks().some(
                      /**
                       * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
                       *
                       * @args
                       *   - track — дорожка захваченного или удалённого MediaStream.
                       *
                       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                       */ (track) => track.readyState === "live",
                    )
                  );
                },
              ),
        )
        .toBe(true);
    }
    const containerIds = (
      await execute(
        "docker",
        ["compose", "ps", "-q", "media-worker", "worker"],
        { cwd: root },
      )
    ).stdout
      .trim()
      .split(/\s+/);
    const workerId = (
      await execute("docker", ["compose", "ps", "-q", "worker"], { cwd: root })
    ).stdout.trim();
    processTimer = setInterval(
      /**
       * Обработчик setInterval выполняет отложенную либо периодическую часть операции.
       *
       *
       * @returns следующее состояние, рассчитанное из предыдущего значения.
       */ () => {
        if (!processSampling)
          processSampling = execute(
            "docker",
            ["exec", workerId, "ps", "-o", "comm"],
            { cwd: root, timeout: 2000 },
          )
            .then(
              /**
               * Обработчик then выполняет переданный шаг вызова then в проверках клиентского поведения.
               *
               * @args
               *   - result — результат завершённой операции.
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ (result) => {
                ffmpegProcessSamples.push({
                  at: new Date().toISOString(),
                  phase,
                  count: result.stdout.split("\n").filter(
                    /**
                     * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
                     *
                     * @args
                     *   - line — строка входящего текстового потока.
                     *
                     * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
                     */ (line) => line.trim() === "ffmpeg",
                  ).length,
                });
              },
            )
            .catch(
              /**
               * Обработчик catch выполняет переданный шаг вызова catch в проверках клиентского поведения.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                /* Измерение ресурсов не меняет состояние записи. */
              },
            )
            .finally(
              /**
               * Обработчик finally выполняет переданный шаг вызова finally в проверках клиентского поведения.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                processSampling = null;
              },
            );
      },
      250,
    );
    /**
     * sample делает один ограниченный замер ресурсов или записи.
     *
     *
     * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
     */
    const sample = async () => {
      const samplePhase = phase;
      const sampledAt = new Date().toISOString();
      const [stats, processes] = await Promise.all([
        execute(
          "docker",
          [
            "stats",
            "--no-stream",
            "--format",
            "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.PIDs}}",
            ...containerIds,
          ],
          { cwd: root, timeout: 8000 },
        ),
        execute("docker", ["exec", workerId, "ps", "-o", "comm"], {
          cwd: root,
          timeout: 8000,
        }),
      ]);
      samples.push({
        at: sampledAt,
        phase: samplePhase,
        containers: stats.stdout.trim().split("\n"),
        ffmpegProcesses: processes.stdout.split("\n").filter(
          /**
           * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
           *
           * @args
           *   - line — строка входящего текстового потока.
           *
           * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
           */ (line) => line.trim() === "ffmpeg",
        ).length,
      });
    };
    timer = setInterval(
      /**
       * Обработчик setInterval выполняет отложенную либо периодическую часть операции.
       *
       *
       * @returns следующее состояние, рассчитанное из предыдущего значения.
       */ () => {
        if (!sampling)
          sampling = sample()
            .catch(
              /**
               * Обработчик catch выполняет переданный шаг вызова catch в проверках клиентского поведения.
               *
               * @args
               *   - error (unknown) — пойманная ошибка API или сети.
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ (error: unknown) => {
                samples.push({
                  phase,
                  sampleError: String(error).slice(0, 200),
                });
              },
            )
            .finally(
              /**
               * Обработчик finally выполняет переданный шаг вызова finally в проверках клиентского поведения.
               *
               *
               * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
               */ () => {
                sampling = null;
              },
            );
      },
      2000,
    );
    const started = owner.waitForResponse(
      /**
       * Обработчик owner.waitForResponse выполняет переданный шаг вызова owner.waitForResponse в проверках клиентского поведения.
       *
       * @args
       *   - response — входное значение response текущего шага обработки.
       *
       * @returns Promise с проверенным ответом API; сетевые ошибки и отказ сервера отклоняют Promise.
       */
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith(`/conferences/${conferenceId}/recordings`),
    );
    await owner
      .getByRole("button", { name: "Начать запись", exact: true })
      .click();
    const startResponse = await started;
    expect(startResponse.status()).toBe(202);
    recordingId = (await startResponse.json()).item.uuid;
    summary.recordingId = recordingId;
    for (const page of [owner, bob])
      await expect(page.getByTestId("recording-indicator")).toHaveText(
        "Идёт запись",
        { timeout: 30000 },
      );
    const duplicate = await request<{ item: Card }>(
      client,
      `/conferences/${conferenceId}/recordings`,
      actors[0].token,
      "POST",
      { segmentDurationSec: 5 },
    );
    expect(duplicate.item.uuid).toBe(recordingId);
    phase = "recording/grid";
    await owner.waitForTimeout(5500);
    await bob
      .getByRole("button", { name: "Показать экран", exact: true })
      .click();
    await expect(owner.locator(".media-tile-screen video")).toHaveCount(1);
    await expect
      .poll(
        /**
         * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
         *
         *
         * @returns актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
         */ () =>
          owner.locator(".media-tile-screen video").evaluate(
            /**
             * Обработчик evaluate выполняет переданный шаг вызова evaluate в проверках клиентского поведения.
             *
             * @args
             *   - video — входное значение video текущего шага обработки.
             *
             * @returns вычисленное значение: (video as HTMLVideoElement).videoWidth.
             */ (video) => (video as HTMLVideoElement).videoWidth,
          ),
      )
      .toBeGreaterThan(0);
    phase = "recording/screen";
    await owner.waitForTimeout(5500);
    await request(
      client,
      `/conference-invites/${created.item.inviteCode}/join`,
      actors[2].token,
      "POST",
      {},
    );
    await login(carol, actors[2], password, conferenceId);
    await enableMedia(carol);
    await expect(carol.getByTestId("recording-indicator")).toHaveText(
      "Идёт запись",
    );
    await expect(owner.getByTestId("remote-media")).toHaveCount(3);
    await expect
      .poll(
        /**
         * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
         *
         *
         * @returns актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
         */
        () =>
          owner
            .getByTestId("remote-media")
            .filter({ hasText: "Smoke Carol" })
            .locator("video")
            .evaluate(
              /**
               * Обработчик evaluate выполняет переданный шаг вызова evaluate в проверках клиентского поведения.
               *
               * @args
               *   - video — входное значение video текущего шага обработки.
               *
               * @returns вычисленное значение: (video as HTMLVideoElement).videoWidth.
               */ (video) => (video as HTMLVideoElement).videoWidth,
            ),
        { timeout: 15000 },
      )
      .toBeGreaterThan(0);
    await owner.screenshot({
      path: info.outputPath("recording-screen-three-participants.png"),
      fullPage: true,
    });
    phase = "recording/three-participants";
    await owner.waitForTimeout(3000);
    await owner
      .getByLabel("Управление: Smoke Carol", { exact: true })
      .getByRole("button", { name: "Отключить микрофон", exact: true })
      .click();
    await expect(
      carol.getByRole("button", { name: "Включить микрофон", exact: true }),
    ).toBeDisabled();
    await bob
      .getByRole("button", { name: "Остановить демонстрацию", exact: true })
      .click();
    await expect(owner.locator(".media-tile-screen video")).toHaveCount(0);
    phase = "recording/grid-after-screen";
    // Раскладка меняется на границах сегментов; включаем полный следующий сегмент сетки.
    await owner.waitForTimeout(6500);
    await diagnostics();
    const stopAt = Date.now();
    await owner
      .getByRole("button", { name: "Остановить запись", exact: true })
      .click();
    phase = "processing";
    expect(
      (
        await request<{ item: { status: string } }>(
          client,
          `/conferences/${conferenceId}`,
          actors[0].token,
        )
      ).item.status,
    ).toBe("active");
    let ready: Card | null = null;
    await expect
      .poll(
        /**
         * Обработчик expect.poll повторно читает проверяемое состояние до достижения ожидаемого результата или тайм-аута теста.
         *
         *
         * @returns Promise, который после завершения операции возвращает: актуальное проверяемое значение; тест повторяет чтение до достижения ожидаемого состояния.
         */
        async () => {
          const result = await request<{ item: Card }>(
            client,
            `/conferences/${conferenceId}/recordings/${recordingId}`,
            actors[0].token,
          );
          ready = result.item;
          if (ready.status === "failed")
            throw new Error(
              `recording failed: ${ready.errorMessage || "unspecified"}`,
            );
          return ready.status;
        },
        { timeout: 120000, intervals: [1000, 1500, 2500] },
      )
      .toBe("ready");
    summary.finalizationMs = Date.now() - stopAt;
    phase = "ready";
    await diagnostics();
    await expect(
      owner.getByRole("link", { name: "Скачать MP4", exact: true }),
    ).toBeVisible();
    const card = ready as unknown as Card;
    expect(
      (
        await client.get(
          `${api}/api/v1/conferences/${conferenceId}/recordings/${recordingId}`,
        )
      ).status(),
    ).toBe(401);
    const videoFile = card.files.find(
      /**
       * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @args
       *   - file — выбранный пользователем файл для проверки или передачи.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */ (file) => file.fileType === "final_mp4",
    );
    const previewFile = card.files.find(
      /**
       * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @args
       *   - file — выбранный пользователем файл для проверки или передачи.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      (file) => file.fileType === "preview_jpg",
    );
    expect(videoFile?.url).toBeTruthy();
    expect(previewFile?.url).toBeTruthy();
    const video = await client.get(videoFile!.url!);
    expect(video.ok()).toBe(true);
    const preview = await client.get(previewFile!.url!);
    expect(preview.ok()).toBe(true);
    const videoBytes = await video.body();
    const previewBytes = await preview.body();
    expect(videoBytes.length).toBeGreaterThan(10000);
    expect(previewBytes.length).toBeGreaterThan(500);
    expect(previewBytes.subarray(0, 2).toString("hex")).toBe("ffd8");
    const videoPath = info.outputPath("conference.mp4");
    await writeFile(videoPath, videoBytes);
    await writeFile(info.outputPath("preview.jpg"), previewBytes);
    const probe = JSON.parse(
      (
        await execute(
          process.env.RECORDER_TEST_FFPROBE || "/opt/homebrew/bin/ffprobe",
          [
            "-v",
            "error",
            "-show_streams",
            "-show_format",
            "-of",
            "json",
            videoPath,
          ],
        )
      ).stdout,
    );
    const videoStream = probe.streams.find(
      /**
       * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @args
       *   - stream ({ codec_type: string }) — поток браузерных медиа-дорожек.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      (stream: { codec_type: string }) => stream.codec_type === "video",
    );
    const audioStream = probe.streams.find(
      /**
       * Обработчик find проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @args
       *   - stream ({ codec_type: string }) — поток браузерных медиа-дорожек.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */
      (stream: { codec_type: string }) => stream.codec_type === "audio",
    );
    expect(videoStream?.codec_name).toBe("h264");
    expect(audioStream?.codec_name).toBe("aac");
    expect(Number(probe.format.duration)).toBeGreaterThan(15);
    expect(
      Math.abs(Number(videoStream.duration) - Number(audioStream.duration)),
    ).toBeLessThan(1.5);
    expect(probe.format.format_name).toContain("mp4");
    const unsigned = new URL(videoFile!.url!);
    unsigned.search = "";
    expect((await client.get(unsigned.toString())).status()).toBe(403);
    summary.validation = {
      duration: Number(probe.format.duration),
      video: videoStream.codec_name,
      audio: audioStream.codec_name,
      videoDuration: Number(videoStream.duration),
      audioDuration: Number(audioStream.duration),
      sizeBytes: videoBytes.length,
      previewBytes: previewBytes.length,
      unsignedAccess: 403,
      unauthenticatedAPI: 401,
    };
    const ffmpeg =
      process.env.RECORDER_TEST_FFMPEG || "/opt/homebrew/bin/ffmpeg";
    await execute(ffmpeg, [
      "-v",
      "error",
      "-ss",
      String(Math.min(12, Number(probe.format.duration) / 2)),
      "-i",
      videoPath,
      "-frames:v",
      "1",
      "-threads",
      "1",
      info.outputPath("recording-screen-frame.jpg"),
    ]);
    await execute(ffmpeg, [
      "-v",
      "error",
      "-ss",
      String(Math.max(0, Number(probe.format.duration) - 0.5)),
      "-i",
      videoPath,
      "-frames:v",
      "1",
      "-threads",
      "1",
      info.outputPath("recording-final-grid-frame.jpg"),
    ]);
    const finalPixels = (
      await execute(
        ffmpeg,
        [
          "-v",
          "error",
          "-ss",
          String(Math.max(0, Number(probe.format.duration) - 0.5)),
          "-i",
          videoPath,
          "-frames:v",
          "1",
          "-f",
          "rawvideo",
          "-pix_fmt",
          "rgb24",
          "-",
        ],
        { encoding: "buffer", maxBuffer: 5 * 1024 * 1024 },
      )
    ).stdout;
    // Должны присутствовать изображения трёх подставных камер Chrome, а не только три описания
    // раскладки. Подставные камеры зелёные, незанятая четвёртая ячейка — серо-синяя.
    const gridWidth = Number(videoStream.width);
    const gridHeight = Number(videoStream.height);
    const greenRatios: number[] = [];
    for (let tileY = 0; tileY < 2; tileY++)
      for (let tileX = 0; tileX < 2; tileX++) {
        let green = 0,
          inspected = 0;
        for (
          let y = (tileY * gridHeight) / 2;
          y < ((tileY + 1) * gridHeight) / 2;
          y += 8
        ) {
          for (
            let x = (tileX * gridWidth) / 2;
            x < ((tileX + 1) * gridWidth) / 2;
            x += 8
          ) {
            const offset = (y * gridWidth + x) * 3;
            const [r, g, b] = finalPixels.subarray(offset, offset + 3);
            if (g > 50 && g > r * 1.4 && g > b * 1.4) green++;
            inspected++;
          }
        }
        greenRatios.push(green / inspected);
      }
    summary.finalGridGreenRatios = greenRatios;
    expect(
      greenRatios.filter(
        /**
         * Обработчик greenRatios.filter проверяет, должен ли элемент войти в отфильтрованный набор.
         *
         * @args
         *   - ratio — входное значение ratio текущего шага обработки.
         *
         * @returns логический признак соответствия элемента условию.
         */ (ratio) => ratio > 0.4,
      ).length,
      "final composite must contain all three camera pictures",
    ).toBe(3);
    const black = await execute(
      ffmpeg,
      [
        "-hide_banner",
        "-i",
        videoPath,
        "-vf",
        // Подставная камера Chrome использует насыщенный зелёный цвет с нулевой яркостью Y.
        // Оцениваем яркость отображаемого RGB, а не плоскости Y, и отклоняем только полностью пустую сцену.
        "format=rgb24,format=gray,blackdetect=d=0.4:pix_th=0.1:pic_th=0.99",
        "-an",
        "-f",
        "null",
        "-",
      ],
      { maxBuffer: 2 * 1024 * 1024 },
    );
    const blackIntervals = black.stderr.split("\n").filter(
      /**
       * Обработчик filter проверяет, соответствует ли текущий элемент условию выборки или поиска.
       *
       * @args
       *   - line — строка входящего текстового потока.
       *
       * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
       */ (line) => line.includes("black_start:"),
    );
    summary.blackIntervals = blackIntervals;
    expect(
      blackIntervals,
      "whole composite must not go black at segment changes",
    ).toEqual([]);
    await owner
      .getByRole("button", { name: "Завершить конференцию", exact: true })
      .click();
    await owner
      .getByRole("button", { name: "Да, завершить", exact: true })
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
            await request<{ item: { status: string } }>(
              client,
              `/conferences/${conferenceId}`,
              actors[0].token,
            )
          ).item.status,
      )
      .toBe("finished");
    for (const page of pages)
      await expect(page.getByTestId("media-status")).toHaveCount(0);
    summary.completed = true;
  } finally {
    summary.lastPhase = phase;
    if (!summary.browserMedia) await diagnostics();
    if (timer) clearInterval(timer);
    if (processTimer) clearInterval(processTimer);
    await sampling;
    await processSampling;
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
        const state = await request<{ item: { status: string } }>(
          client,
          `/conferences/${conferenceId}`,
          actors[0].token,
        );
        if (state.item.status === "active")
          await request(
            client,
            `/conferences/${conferenceId}/finish`,
            actors[0].token,
            "POST",
          );
        else if (state.item.status === "created")
          await request(
            client,
            `/conferences/${conferenceId}/cancel`,
            actors[0].token,
            "POST",
          );
        const limit = Date.now() + 90000;
        for (;;) {
          const records = await request<{ items: Card[] }>(
            client,
            `/conferences/${conferenceId}/recordings`,
            actors[0].token,
          );
          terminal = records.items.every(
            /**
             * Обработчик every проверяет, соответствует ли текущий элемент условию выборки или поиска.
             *
             * @args
             *   - item — элемент списка, который обрабатывает текущий шаг.
             *
             * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
             */ (item) =>
              ["ready", "failed", "cancelled"].includes(item.status),
          );
          if (terminal || Date.now() > limit) break;
          await new Promise(
            /**
             * Вложенный обработчик выполняет шаг «Вложенный обработчик» в проверках клиентского поведения.
             *
             * @args
             *   - resolve — завершает ожидающий Promise успешным результатом.
             *
             * @returns вычисленное значение: setTimeout(resolve, 1500).
             */ (resolve) => setTimeout(resolve, 1500),
          );
        }
      } catch {
        terminal = false;
      }
    }
    const cleanupManifest = info.outputPath("cleanup.json");
    await writeFile(
      cleanupManifest,
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
        await execute("go", ["run", "./tools/smoke_cleanup", cleanupManifest], {
          cwd: root,
          timeout: 60000,
        });
        summary.cleanup =
          "created SQL identities, private recording prefix and local recorder files removed";
      } catch (error) {
        summary.cleanup = "failed; exact cleanup manifest retained";
        summary.cleanupError = String(error).slice(0, 600);
      }
    } else
      summary.cleanup =
        "deferred because recording is still active; exact cleanup manifest retained";
    await writeFile(
      info.outputPath("acceptance.json"),
      JSON.stringify(summary, null, 2),
    );
    if (summary.completed)
      expect(summary.cleanup).toBe(
        "created SQL identities, private recording prefix and local recorder files removed",
      );
  }
});
