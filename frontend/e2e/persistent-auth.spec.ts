import { personalBackground } from "./helpers/personal-background";
import { expect, test, type BrowserContext, type Page } from "@playwright/test";

const person = {
  id: "persistent-account-fixture",
  email: "session@example.test",
  displayName: "Постоянный участник",
  createdAt: "2026-10-06T00:00:00Z",
  updatedAt: "2026-10-06T00:00:00Z",
};
const cookieName = "meet_auth_fixture";
async function fixture(context: BrowserContext, baseURL: string) {
  const state = {
    active: true,
    generation: 0,
    refreshes: 0,
    logouts: 0,
    offlineLogout: false,
    requests: [] as string[],
    unexpected: [] as string[],
  };
  const origin = new URL(baseURL).origin;
  await context.addCookies([
    {
      name: cookieName,
      value: "opaque-fixture-session",
      url: origin,
      httpOnly: true,
      sameSite: "Strict",
    },
  ]);
  await context.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    if (await personalBackground(route, path)) return;
    const method = request.method();
    state.requests.push(`${method} ${path}`);
    const reply = (
      body: unknown,
      status = 200,
      headers?: Record<string, string>,
    ) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body),
        headers,
      });
    const account = () => ({
      status: "success",
      accessToken: `access-${++state.generation}`,
      expiresIn: 3600,
      user: person,
    });
    if (path === "/auth/login") {
      state.active = true;
      return reply(account(), 200, {
        "Set-Cookie": `${cookieName}=opaque-fixture-session; Path=/api/v1/auth; HttpOnly; SameSite=Strict`,
      });
    }
    if (path === "/auth/logout") {
      state.logouts++;
      if (state.offlineLogout) return route.abort("internetdisconnected");
      state.active = false;
      return route.fulfill({
        status: 204,
        headers: {
          "Set-Cookie": `${cookieName}=; Path=/api/v1/auth; HttpOnly; SameSite=Strict; Max-Age=0`,
        },
      });
    }
    if (path === "/auth/refresh") {
      state.refreshes++;
      expect(request.headers().authorization).toBeUndefined();
      const cookie = request.headers().cookie || "";
      if (!state.active || !cookie.includes(`${cookieName}=`))
        return reply({ message: "unauthorized" }, 401);
      return reply(account());
    }
    if (path === "/auth/session") return reply(account());
    if (path === "/auth/me") return reply({ status: "success", user: person });
    if (path === "/capabilities")
      return reply({
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
    if (path === "/notifications/events")
      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: ": isolated persistent-auth fixture\n\n",
      });
    if (path === "/notifications")
      return reply({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
      });
    if (path === "/me/conferences")
      return reply({ status: "success", items: [], nextCursor: null });
    state.unexpected.push(`${method} ${path}`);
    return reply(
      { message: "Unexpected API blocked by persistent auth QA" },
      501,
    );
  });
  return state;
}
async function seed(page: Page, expired = false) {
  await page.addInitScript(
    ({ person, expired }) => {
      sessionStorage.setItem(
        "meet.session.v1",
        JSON.stringify({
          token: "legacy-fixture-access",
          expiresAt: Date.now() + (expired ? -86_400_000 : 3600_000),
          user: person,
        }),
      );
    },
    { person, expired },
  );
}
async function dashboard(page: Page) {
  await expect(
    page.getByRole("heading", { name: /Добро пожаловать/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Выйти из аккаунта", exact: true }),
  ).toBeVisible();
}

test("renews after one hour without removing the mounted dashboard", async ({
  page,
  context,
  baseURL,
}) => {
  const state = await fixture(context, baseURL!);
  await page.clock.install();
  await seed(page);
  await page.goto("/app");
  await dashboard(page);
  const heading = page.getByRole("heading", { name: /Добро пожаловать/ });
  await heading.evaluate((element) => {
    (element as HTMLElement & { sessionSentinel?: string }).sessionSentinel =
      "same mounted element";
  });
  await page.clock.fastForward(3600_000 + 5000);
  await expect.poll(() => state.refreshes).toBe(1);
  await dashboard(page);
  await expect(heading).toHaveJSProperty(
    "sessionSentinel",
    "same mounted element",
  );
  expect(
    await page.evaluate(
      () =>
        JSON.parse(sessionStorage.getItem("meet.session.v1") || "null").token,
    ),
  ).toBe("access-2");
  expect(
    await page.evaluate(() => localStorage.getItem("meet.session.v1")),
  ).toBeNull();
  expect(state.unexpected).toEqual([]);
});

test("restores an expired access token and a new tab from an unreadable HttpOnly cookie", async ({
  page,
  context,
  baseURL,
}) => {
  const state = await fixture(context, baseURL!);
  await seed(page, true);
  await page.goto("/app");
  await dashboard(page);
  // React's development StrictMode may abort the first mount's restoration.
  expect(state.refreshes).toBeGreaterThanOrEqual(1);
  expect(state.refreshes).toBeLessThanOrEqual(2);
  expect(await page.evaluate(() => document.cookie)).not.toContain(cookieName);
  const other = await context.newPage();
  const restoredBeforeNewTab = state.refreshes;
  await other.goto("/app");
  await dashboard(other);
  expect(state.refreshes - restoredBeforeNewTab).toBeGreaterThanOrEqual(1);
  expect(state.refreshes - restoredBeforeNewTab).toBeLessThanOrEqual(2);
  expect(
    state.requests.filter((request) => request === "POST /auth/login"),
  ).toHaveLength(0);
  expect(state.unexpected).toEqual([]);
});

test("explicit offline logout closes other tabs and blocks stale cookie restoration after reload", async ({
  page,
  context,
  baseURL,
}) => {
  const state = await fixture(context, baseURL!);
  await seed(page);
  await page.goto("/app");
  await dashboard(page);
  const other = await context.newPage();
  await other.goto("/app");
  await dashboard(other);
  state.offlineLogout = true;
  await page
    .getByRole("button", { name: "Выйти из аккаунта", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Вход в Meetrix", exact: true }),
  ).toBeVisible();
  await expect(
    other.getByRole("heading", { name: "Вход в Meetrix", exact: true }),
  ).toBeVisible();
  const refreshesBeforeReload = state.refreshes;
  state.offlineLogout = false;
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Вход в Meetrix", exact: true }),
  ).toBeVisible();
  await expect.poll(() => state.logouts).toBeGreaterThanOrEqual(2);
  expect(state.refreshes).toBe(refreshesBeforeReload);
  expect(
    await page.evaluate(() => localStorage.getItem("meet.auth.logout.v1")),
  ).not.toBeNull();
  await page.getByLabel("Email", { exact: true }).fill(person.email);
  await page
    .getByLabel("Пароль", { exact: true })
    .fill("password-fixture-only");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await dashboard(page);
  expect(
    await page.evaluate(() => localStorage.getItem("meet.auth.logout.v1")),
  ).toBeNull();
  expect(state.unexpected).toEqual([]);
});
