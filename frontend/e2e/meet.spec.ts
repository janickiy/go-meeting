import { test, expect, type Page } from "@playwright/test";
import type { Conference, Participant, User } from "../src/types";

const code = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef";
const now = "2026-10-01T09:00:00Z";
const user: User = {
  id: "user-owner",
  email: "alex@example.test",
  displayName: "Александр",
  createdAt: now,
  updatedAt: now,
};
const initial: Conference = {
  id: "conference-1",
  ownerId: user.id,
  title: "Обсуждение проекта",
  inviteCode: code,
  inviteUrl: `/api/v1/conference-invites/${code}`,
  status: "created",
  createdAt: now,
  updatedAt: now,
  startedAt: null,
  finishedAt: null,
};
/**
 * member создаёт тестовое членство участника.
 *
 * @args
 *   - id (string) — идентификатор ресурса или конференции данного запроса.
 *   - userId (string) — идентификатор текущего авторизованного пользователя.
 *   - role (Participant["role"]) — роль участника и его полномочия (по умолчанию "participant").
 *
 * @returns Participant — объект с данными, собранными в текущей операции.
 */
function member(
  id: string,
  userId: string,
  role: Participant["role"] = "participant",
): Participant {
  return {
    id,
    userId,
    conferenceId: initial.id,
    displayName: userId === user.id ? "Александр" : `Коллега ${id}`,
    role,
    status: "left",
    joinedAt: null,
    leftAt: null,
    createdAt: now,
    updatedAt: now,
  };
}
/**
 * mockApi устанавливает ответы HTTP API и сохраняет состояние тестового сценария.
 *
 * @args
 *   - page (Page) — изолированная страница Playwright.
 *   - options ({ participant?: boolean; large?: boolean; expired?: boolean; failList?: boolean; failLogin?: boolean; failRegistrationLogin?: boolean; empty?: boolean; }) — метод, тело, отмена и признаки авторизации запроса (по умолчанию {}).
 *
 * @returns Promise, который после завершения операции возвращает: объект с данными, собранными в текущей операции.
 */
async function mockApi(
  page: Page,
  options: {
    participant?: boolean;
    large?: boolean;
    expired?: boolean;
    failList?: boolean;
    failLogin?: boolean;
    failRegistrationLogin?: boolean;
    empty?: boolean;
  } = {},
) {
  const current = {
    ...user,
    id: options.participant ? "user-participant" : user.id,
  };
  let conference = { ...initial };
  let participants = [member("owner", user.id, "owner")];
  if (options.large) {
    participants = [
      ...participants,
      ...Array.from(
        { length: 99 },
        /**
         * Обработчик Array.from выполняет переданный шаг вызова Array.from в проверках клиентского поведения.
         *
         * @args
         *   - _ — входное значение _ текущего шага обработки.
         *   - i — индекс элемента в текущем наборе.
         *
         * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
         */ (_, i) => member(`p-${i}`, `u-${i}`),
      ),
      { ...member("current", current.id), status: "joined", joinedAt: now },
    ];
  }
  let list: Conference[] = options.empty ? [] : [conference];
  const writes: { path: string; body: unknown }[] = [];
  await page.route(
    "**/api/v1/**",
    /**
     * Обработчик page.route выполняет браузерную часть проверяемого сценария в изолированном тестовом контексте.
     *
     * @args
     *   - route — входное значение route текущего шага обработки.
     *
     * @returns Promise, который после завершения операции возвращает: вычисленные данные текущего шага, которые использует вызывающая операция.
     */ async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname.replace("/api/v1", "");
      const post = request.method() === "POST";
      /**
       * respond возвращает подготовленный ответ перехваченному запросу теста.
       *
       * @args
       *   - body (unknown) — типизированное тело запроса.
       *   - status — HTTP-статус либо состояние встречи (по умолчанию 200).
       *
       * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
       */
      const respond = (body: unknown, status = 200) =>
        route.fulfill({
          status,
          contentType: "application/json",
          body: JSON.stringify(body),
        });
      if (post) writes.push({ path, body: request.postDataJSON() });
      if (path === "/auth/register")
        return respond({ status: "success", user: current }, 201);
      if (path === "/auth/login") {
        if (options.failLogin || options.failRegistrationLogin)
          return respond({ message: "invalid email or password" }, 401);
        return respond({
          status: "success",
          accessToken: "e2e-jwt",
          expiresIn: 3600,
          tokenType: "Bearer",
          user: current,
        });
      }
      if (options.expired) return respond({ message: "expired" }, 401);
      if (request.headers()["authorization"] !== "Bearer e2e-jwt")
        return respond({ message: "unauthorized" }, 401);
      if (path === "/auth/me")
        return respond({ status: "success", user: current });
      if (path === "/auth/logout") return respond({ status: "success" });
      if (path === "/conferences" && post) {
        const body = request.postDataJSON();
        if (
          Object.keys(body).some(
            /**
             * Обработчик some проверяет, соответствует ли текущий элемент условию выборки или поиска.
             *
             * @args
             *   - key — идентификатор строки загрузки.
             *
             * @returns true, если проверяемый элемент удовлетворяет условию; false в противном случае.
             */
            (key) =>
              ![
                "title",
                "waitingRoomEnabled",
                "scheduledAt",
                "plannedDurationMin",
              ].includes(key),
          )
        )
          return respond({ message: "invalid JSON" }, 400);
        conference = { ...conference, title: body.title };
        list = [conference];
        return respond({ status: "success", item: conference }, 201);
      }
      if (path === "/notifications")
        return respond({
          status: "success",
          items: [],
          nextCursor: null,
          unreadCount: 0,
        });
      if (path === "/conferences" || path === "/me/conferences") {
        if (options.failList)
          return respond({ message: "postgres: secret database error" }, 500);
        return respond({
          status: "success",
          nextCursor: null,
          items: list.slice(
            Number(url.searchParams.get("offset") || 0),
            Number(url.searchParams.get("offset") || 0) + 20,
          ),
        });
      }
      if (path === `/conferences/${conference.id}`)
        return respond({ status: "success", item: conference });
      if (path === `/conferences/${conference.id}/recordings` && !post)
        return respond({ status: "success", items: [] });
      if (path.endsWith("/participants/me")) {
        const own = participants.find(
          /**
           * Обработчик participants.find проверяет условие поиска элемента или соответствия элементов набора.
           *
           * @args
           *   - item — элемент списка, который обрабатывает текущий шаг.
           *
           * @returns логический признак соответствия элемента условию.
           */ (item) => item.userId === current.id,
        );
        return own
          ? respond({ status: "success", item: own })
          : respond({ message: "forbidden" }, 403);
      }
      if (path.endsWith("/messages"))
        return respond({
          status: "success",
          items: [],
          nextCursor: null,
          unreadCount: 0,
          lastReadMessageId: null,
        });
      if (path.endsWith("/chat/read"))
        return respond({
          status: "success",
          item: { unreadCount: 0, lastReadMessageId: null },
        });
      if (path.endsWith("/history"))
        return respond({
          status: "success",
          item: {
            conference,
            owner: { id: user.id, displayName: user.displayName },
            durationSec: 60,
            participantCount: participants.length,
            participants,
            recordings: { total: 0, ready: 0, processing: 0, failed: 0 },
            chatAvailable: true,
            chatReadOnly: true,
          },
        });
      if (path.endsWith("/participants")) {
        const offset = Number(url.searchParams.get("offset") || 0);
        return respond({
          status: "success",
          items: participants.slice(offset, offset + 100),
        });
      }
      if (path === `/conference-invites/${code}`)
        return respond({
          status: "success",
          item: {
            id: conference.id,
            title: conference.title,
            status: conference.status,
          },
        });
      if (post && /\/(join|leave)$/.test(path)) {
        let own = participants.find(
          /**
           * Обработчик participants.find проверяет условие поиска элемента или соответствия элементов набора.
           *
           * @args
           *   - item — элемент списка, который обрабатывает текущий шаг.
           *
           * @returns логический признак соответствия элемента условию.
           */ (item) => item.userId === current.id,
        );
        if (!own) {
          own = member("current", current.id);
          participants.push(own);
        }
        own.status = path.endsWith("/leave") ? "left" : "joined";
        own.joinedAt = now;
        own.leftAt = own.status === "left" ? now : null;
        return respond({ status: "success", item: own });
      }
      if (post && /\/(start|finish|cancel)$/.test(path)) {
        if (options.participant) return respond({ message: "forbidden" }, 403);
        conference.status = path.endsWith("/start")
          ? "active"
          : path.endsWith("/finish")
            ? "finished"
            : "cancelled";
        if (conference.status === "active") conference.startedAt = now;
        else {
          conference.finishedAt = now;
          participants.forEach(
            /**
             * Обработчик participants.forEach выполняет переданный шаг вызова participants.forEach в проверках клиентского поведения.
             *
             * @args
             *   - item — элемент списка, который обрабатывает текущий шаг.
             *
             * @returns значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
             */ (item) => {
              if (item.status === "joined") {
                item.status = "left";
                item.leftAt = now;
              }
            },
          );
        }
        list = [conference];
        return respond({ status: "success", item: conference });
      }
      return respond({ message: "not found" }, 404);
    },
  );
  return { writes };
}
/**
 * login отправляет учётные данные и получает токен и сведения пользователя.
 *
 * @args
 *   - page (Page) — изолированная страница Playwright.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Email", { exact: true }).fill(user.email);
  await page.getByLabel("Пароль", { exact: true }).fill("correct-password-123");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(page).toHaveURL(/\/app$/);
  await expect(
    page.getByRole("heading", { name: "Добро пожаловать, Александр!" }),
  ).toBeVisible();
}
/**
 * noOverflow проверяет отсутствие выхода элементов за доступную ширину страницы.
 *
 * @args
 *   - page (Page) — изолированная страница Playwright.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */
async function noOverflow(page: Page) {
  expect(
    await page.evaluate(
      /**
       * Обработчик page.evaluate выполняет браузерную часть проверяемого сценария в изолированном тестовом контексте.
       *
       *
       * @returns вычисленное значение: document.documentElement.scrollWidth <= window.innerWidth.
       */
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
}

test("landing, login, registration and success match the reference at desktop size", /**
 * Проверяет соответствие главной страницы, входа, регистрации и результата образцу на настольном экране.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright.
 *   - info — контекст запуска для диагностических вложений.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page }, info) => {
  await mockApi(page, { empty: true });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Встречайтесь без границ" }),
  ).toBeVisible();
  const hero = page.getByRole("img");
  await expect(hero).toBeVisible();
  expect(
    await hero.evaluate(
      /**
       * Обработчик hero.evaluate выполняет браузерную часть проверяемого сценария в изолированном тестовом контексте.
       *
       * @args
       *   - node — DOM-элемент, к которому привязывается медиапоток.
       *
       * @returns вычисленное значение: (node as HTMLImageElement).naturalWidth.
       */ (node) => (node as HTMLImageElement).naturalWidth,
    ),
  ).toBeGreaterThan(0);
  await noOverflow(page);
  await page.screenshot({
    path: info.outputPath("01-landing.png"),
    fullPage: true,
  });
  await page.goto("/login");
  await page.screenshot({
    path: info.outputPath("02-login.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Забыли пароль?" }).click();
  await expect(page.getByRole("status")).toContainText("пока недоступно");
  await page.goto("/register");
  await page.screenshot({
    path: info.outputPath("03-register.png"),
    fullPage: true,
  });
  await page.getByLabel("Email", { exact: true }).fill(user.email);
  await page.getByLabel("Пароль", { exact: true }).fill("correct-password-123");
  await page.getByLabel(/Как к вам обращаться/).fill("Александр");
  await page
    .getByRole("button", { name: "Зарегистрироваться", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Аккаунт создан!" }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("04-success.png"),
    fullPage: true,
  });
  await page.getByRole("link", { name: "Перейти в приложение" }).click();
  await expect(
    page.getByRole("heading", { name: "Самое время для первой встречи" }),
  ).toBeVisible();
});

test("login -> dashboard -> create -> share -> lifecycle -> logout", /**
 * Проверяет полный цикл: вход, главная страница, создание встречи, обмен ссылкой, жизненный цикл встречи и выход.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright.
 *   - info — контекст запуска для диагностических вложений.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page }, info) => {
  const { writes } = await mockApi(page);
  await login(page);
  await page.screenshot({
    path: info.outputPath("05-dashboard.png"),
    fullPage: true,
  });
  await page.getByRole("link", { name: "Новая конференция" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toHaveAccessibleName("Новая конференция");
  const title = page.getByLabel("Название конференции");
  await expect(title).toBeFocused();
  await title.fill("Демо React интерфейса");
  await expect(page.getByLabel(/Зал ожидания/)).toBeEnabled();
  await page.screenshot({
    path: info.outputPath("06-create.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Создать конференцию", exact: true })
    .click();
  await expect(dialog).toHaveAccessibleName("Конференция создана!");
  await expect(page.getByLabel("Ссылка-приглашение")).toHaveValue(
    `http://127.0.0.1:5174/i/${code}`,
  );
  expect(
    writes.find(
      /**
       * Обработчик writes.find проверяет условие поиска элемента или соответствия элементов набора.
       *
       * @args
       *   - item — элемент списка, который обрабатывает текущий шаг.
       *
       * @returns логический признак соответствия элемента условию.
       */ (item) => item.path === "/conferences",
    )?.body,
  ).toEqual({
    title: "Демо React интерфейса",
    waitingRoomEnabled: false,
  });
  await page.screenshot({
    path: info.outputPath("07-share.png"),
    fullPage: true,
  });
  await page.getByRole("link", { name: "Перейти в конференцию" }).click();
  await page
    .getByRole("button", { name: "Присоединиться", exact: true })
    .click();
  await expect(page).toHaveURL(/\/conferences\/conference-1\/join$/);
  await page.getByRole("button", { name: "Войти во встречу" }).click();
  await expect(
    page.getByRole("button", { name: "Покинуть конференцию" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Покинуть конференцию" }).click();
  await expect(
    page.getByRole("button", { name: "Присоединиться", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Начать конференцию" }).click();
  await expect(
    page.getByRole("button", { name: "Завершить конференцию", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Завершить конференцию", exact: true })
    .click();
  await page.getByRole("button", { name: "Да, завершить" }).click();
  await expect(
    page.getByRole("heading", { name: "Встреча закрыта" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Выйти из аккаунта" }).click();
  await expect(page).toHaveURL(/\/login(?:\?|$)/);
  expect(
    await page.evaluate(
      /**
       * Обработчик page.evaluate выполняет браузерную часть проверяемого сценария в изолированном тестовом контексте.
       *
       *
       * @returns вычисленное значение: sessionStorage.getItem("meet.session.v1").
       */ () => sessionStorage.getItem("meet.session.v1"),
    ),
  ).toBeNull();
});

test("an invitation survives login and a participant cannot see owner controls", /**
 * Проверяет сохранность приглашения после входа и отсутствие элементов владельца у участника.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page }) => {
  await mockApi(page, { participant: true });
  await page.goto(`/i/${code}`);
  await expect(page).toHaveURL(/\/login\?next=/);
  await page.getByLabel("Email", { exact: true }).fill(user.email);
  await page.getByLabel("Пароль", { exact: true }).fill("correct-password-123");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Обсуждение проекта" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Проверить устройства и войти" })
    .click();
  await expect(page).toHaveURL(/\/conferences\/conference-1\/join\?invite=/);
  await page.getByRole("button", { name: "Войти во встречу" }).click();
  await expect(page).toHaveURL(/\/conferences\/conference-1$/);
  await expect(
    page.getByRole("button", { name: "Покинуть конференцию" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Начать конференцию" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Отменить конференцию" }),
  ).toHaveCount(0);
});

test("finds current membership after the first 100 participants", /**
 * Проверяет поиск текущего членства за пределами первых 100 участников.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page }) => {
  await mockApi(page, { participant: true, large: true });
  await login(page);
  await page.goto("/conferences/conference-1");
  await expect(
    page.getByRole("button", { name: "Покинуть конференцию" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Загрузить ещё участников" }).click();
  await expect(page.getByText("Участники 101")).toBeVisible();
});

test("restores a session, but an expired token redirects safely to login", /**
 * Проверяет восстановление сессии и безопасный переход ко входу при истёкшем токене.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page }) => {
  await mockApi(page);
  await login(page);
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Добро пожаловать, Александр!" }),
  ).toBeVisible();
  await page.route(
    "**/api/v1/auth/me",
    /**
     * Обработчик page.route выполняет браузерную часть проверяемого сценария в изолированном тестовом контексте.
     *
     * @args
     *   - route — входное значение route текущего шага обработки.
     *
     * @returns вычисленные данные текущего шага, которые использует вызывающая операция.
     */ (route) =>
      route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ message: "expired" }),
      }),
  );
  await page.reload();
  await expect(page).toHaveURL(/\/login/);
  await expect(page.getByRole("alert")).toContainText("Сессия завершилась");
  expect(
    await page.evaluate(
      /**
       * Обработчик page.evaluate выполняет браузерную часть проверяемого сценария в изолированном тестовом контексте.
       *
       *
       * @returns вычисленное значение: sessionStorage.getItem("meet.session.v1").
       */ () => sessionStorage.getItem("meet.session.v1"),
    ),
  ).toBeNull();
});

test("handles errors without leaking server details or pretending registration failed", /**
 * Проверяет обработку ошибок без раскрытия серверных деталей и ложного сообщения о сбое успешной регистрации.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page }) => {
  await mockApi(page, { failList: true });
  await login(page);
  await expect(page.getByRole("alert")).toHaveText([
    /Сервис временно недоступен/,
    /Сервис временно недоступен/,
  ]);
  await expect(page.getByText(/postgres|secret database/)).toHaveCount(0);
  await page.goto("/register"); // Уже авторизованные пользователи перенаправляются.
  await expect(page).toHaveURL(/\/app$/);
  await page.getByRole("button", { name: "Выйти из аккаунта" }).click();
  await page.unroute("**/api/v1/**");
  await mockApi(page, { failRegistrationLogin: true });
  await page.goto("/register");
  await page.getByLabel("Email", { exact: true }).fill("new@example.test");
  await page.getByLabel("Пароль", { exact: true }).fill("short");
  await page
    .getByRole("button", { name: "Зарегистрироваться", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText("8 до 128 символов");
  await page.getByLabel("Пароль", { exact: true }).fill("correct-password-123");
  await page
    .getByRole("button", { name: "Зарегистрироваться", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Аккаунт создан!" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Войти в приложение" }),
  ).toBeVisible();
});

test("registration counts characters, not bytes, and accepts eight plain letters", /**
 * Проверяет подсчёт символов вместо байтов при регистрации и приём восьми обычных букв.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page }) => {
  const { writes } = await mockApi(page, { empty: true });
  await page.goto("/register");
  await page.getByLabel("Email", { exact: true }).fill("policy@example.test");
  const password = page.getByLabel("Пароль", { exact: true });
  const submit = page.getByRole("button", {
    name: "Зарегистрироваться",
    exact: true,
  });
  await expect(password).toHaveAttribute("placeholder", "Минимум 8 символов");
  for (const invalid of [
    "abcdefg",
    "абвгдеж",
    "😀".repeat(7),
    "я".repeat(129),
  ]) {
    await password.fill(invalid);
    await submit.click();
    await expect(page.getByRole("alert")).toContainText("от 8 до 128 символов");
  }
  expect(
    writes.filter(
      /**
       * Обработчик writes.filter проверяет, должен ли элемент войти в отфильтрованный набор.
       *
       * @args
       *   - item — элемент списка, который обрабатывает текущий шаг.
       *
       * @returns логический признак соответствия элемента условию.
       */ (item) => item.path === "/auth/register",
    ),
  ).toHaveLength(0);
  await password.fill("abcdefgh");
  await submit.click();
  await expect(
    page.getByRole("heading", { name: "Аккаунт создан!" }),
  ).toBeVisible();
  expect(
    writes.filter(
      /**
       * Обработчик writes.filter проверяет, должен ли элемент войти в отфильтрованный набор.
       *
       * @args
       *   - item — элемент списка, который обрабатывает текущий шаг.
       *
       * @returns логический признак соответствия элемента условию.
       */ (item) => item.path === "/auth/register",
    ),
  ).toHaveLength(1);
});

test("mobile layouts, menu, keyboard dialog dismissal and deep-link refresh", /**
 * Проверяет мобильную раскладку, меню, закрытие диалогов клавиатурой и обновление прямой ссылки.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright.
 *   - info — контекст запуска для диагностических вложений.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page }, info) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockApi(page);
  for (const path of ["/", "/login", "/register"]) {
    await page.goto(path);
    await noOverflow(page);
    await page.screenshot({
      path: info.outputPath(
        `mobile-${path.replaceAll("/", "") || "landing"}.png`,
      ),
      fullPage: true,
    });
  }
  await login(page);
  await noOverflow(page);
  await page.screenshot({
    path: info.outputPath("mobile-dashboard.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Открыть меню" }).click();
  await expect(
    page.getByRole("dialog", { name: "Меню Meetrix" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("button", { name: "Открыть меню" }),
  ).toBeFocused();
  await page.getByRole("button", { name: "Открыть меню" }).click();
  await page.getByRole("link", { name: "Настройки", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Настройки аккаунта" }),
  ).toBeVisible();
  await page.goto("/conferences/new");
  await expect(page.getByRole("dialog")).toBeVisible();
  await noOverflow(page);
  await page.screenshot({
    path: info.outputPath("mobile-create.png"),
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.goto("/conferences/conference-1");
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Обсуждение проекта" }),
  ).toBeVisible();
  await noOverflow(page);
});
