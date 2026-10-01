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
      ...Array.from({ length: 99 }, (_, i) => member(`p-${i}`, `u-${i}`)),
      { ...member("current", current.id), status: "joined", joinedAt: now },
    ];
  }
  let list: Conference[] = options.empty ? [] : [conference];
  const writes: { path: string; body: unknown }[] = [];
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace("/api/v1", "");
    const post = request.method() === "POST";
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
      if (Object.keys(body).join(",") !== "title")
        return respond({ message: "invalid JSON" }, 400);
      conference = { ...conference, title: body.title };
      list = [conference];
      return respond({ status: "success", item: conference }, 201);
    }
    if (path === "/conferences") {
      if (options.failList)
        return respond({ message: "postgres: secret database error" }, 500);
      return respond({
        status: "success",
        items: list.slice(
          Number(url.searchParams.get("offset") || 0),
          Number(url.searchParams.get("offset") || 0) + 20,
        ),
      });
    }
    if (path === `/conferences/${conference.id}`)
      return respond({ status: "success", item: conference });
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
      let own = participants.find((item) => item.userId === current.id);
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
        participants.forEach((item) => {
          if (item.status === "joined") {
            item.status = "left";
            item.leftAt = now;
          }
        });
      }
      list = [conference];
      return respond({ status: "success", item: conference });
    }
    return respond({ message: "not found" }, 404);
  });
  return { writes };
}
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
async function noOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
}

test("landing, login, registration and success match the reference at desktop size", async ({
  page,
}, info) => {
  await mockApi(page, { empty: true });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Встречайтесь без границ" }),
  ).toBeVisible();
  const hero = page.getByRole("img");
  await expect(hero).toBeVisible();
  expect(
    await hero.evaluate((node) => (node as HTMLImageElement).naturalWidth),
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

test("login -> dashboard -> create -> share -> lifecycle -> logout", async ({
  page,
}, info) => {
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
  await expect(page.getByLabel(/Описание/)).toBeDisabled();
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
  expect(writes.find((item) => item.path === "/conferences")?.body).toEqual({
    title: "Демо React интерфейса",
  });
  await page.screenshot({
    path: info.outputPath("07-share.png"),
    fullPage: true,
  });
  await page.getByRole("link", { name: "Перейти в конференцию" }).click();
  await page
    .getByRole("button", { name: "Присоединиться", exact: true })
    .click();
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
    await page.evaluate(() => sessionStorage.getItem("meet.session.v1")),
  ).toBeNull();
});

test("an invitation survives login and a participant cannot see owner controls", async ({
  page,
}) => {
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
    .getByRole("button", { name: "Присоединиться к конференции" })
    .click();
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

test("finds current membership after the first 100 participants", async ({
  page,
}) => {
  await mockApi(page, { participant: true, large: true });
  await login(page);
  await page.goto("/conferences/conference-1");
  await expect(
    page.getByRole("button", { name: "Покинуть конференцию" }),
  ).toBeVisible();
  await expect(page.getByText("Участники 101")).toBeVisible();
});

test("restores a session, but an expired token redirects safely to login", async ({
  page,
}) => {
  await mockApi(page);
  await login(page);
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Добро пожаловать, Александр!" }),
  ).toBeVisible();
  await page.route("**/api/v1/auth/me", (route) =>
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
    await page.evaluate(() => sessionStorage.getItem("meet.session.v1")),
  ).toBeNull();
});

test("handles errors without leaking server details or pretending registration failed", async ({
  page,
}) => {
  await mockApi(page, { failList: true });
  await login(page);
  await expect(page.getByRole("alert")).toContainText(
    "Сервис временно недоступен",
  );
  await expect(page.getByText(/postgres|secret database/)).toHaveCount(0);
  await page.goto("/register"); // Already authenticated users are redirected.
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

test("registration counts characters, not bytes, and accepts eight plain letters", async ({
  page,
}) => {
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
  expect(writes.filter((item) => item.path === "/auth/register")).toHaveLength(
    0,
  );
  await password.fill("abcdefgh");
  await submit.click();
  await expect(
    page.getByRole("heading", { name: "Аккаунт создан!" }),
  ).toBeVisible();
  expect(writes.filter((item) => item.path === "/auth/register")).toHaveLength(
    1,
  );
});

test("mobile layouts, menu, keyboard dialog dismissal and deep-link refresh", async ({
  page,
}, info) => {
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
  await expect(page.getByRole("dialog", { name: "Меню Meet" })).toBeVisible();
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
