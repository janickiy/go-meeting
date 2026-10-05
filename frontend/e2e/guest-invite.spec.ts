import { expect, test, type Page } from "@playwright/test";

// Permission-denied coverage; media-enabled coverage runs against real SFU separately.
test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    navigator.mediaDevices.getUserMedia = async () => {
      throw new DOMException("Permission denied", "NotAllowedError");
    };
  });
});

const code = "GUESTinvitation12345678901234567";
const room = "11111111-1111-4111-8111-111111111111";
const now = "2026-10-05T00:00:00Z";

async function fixture(page: Page, account = false, closed = false) {
  let joined = false;
  const writes: string[] = [];
  let user = {
    id: "guest-id",
    email: "",
    displayName: "Гость",
    guestConferenceId: room,
    createdAt: now,
    updatedAt: now,
  };
  const member = () => ({
    id: "participant-id",
    conferenceId: room,
    userId: user.id,
    displayName: user.displayName,
    role: "participant",
    status: "joined",
    admissionState: "admitted",
    joinedAt: null,
    leftAt: null,
    createdAt: now,
    updatedAt: now,
  });
  if (account) {
    user = {
      ...user,
      id: "account-id",
      email: "anna@example.test",
      displayName: "Анна Смирнова",
      guestConferenceId: "",
    };
    await page.addInitScript(() =>
      sessionStorage.setItem(
        "meet.session.v1",
        JSON.stringify({
          token: "account-token",
          expiresAt: Date.now() + 3600000,
        }),
      ),
    );
  }
  await page.route("**/api/v1/**", async (route) => {
    const req = route.request(),
      path = new URL(req.url()).pathname.replace("/api/v1", "");
    if (req.method() !== "GET") writes.push(path);
    let json: unknown = { status: "success", items: [] };
    if (path === `/conference-invites/${code}`)
      json = {
        status: "success",
        item: {
          id: room,
          title: "Обсуждение проекта",
          status: closed ? "finished" : "active",
          waitingRoomEnabled: true,
        },
      };
    else if (path === `/conference-invites/${code}/guest`) {
      user = { ...user, displayName: req.postDataJSON().displayName };
      joined = true;
      json = {
        status: "success",
        accessToken: "guest-token",
        expiresIn: 3600,
        tokenType: "Bearer",
        user,
        item: member(),
      };
    } else if (path === `/conference-invites/${code}/join`) {
      joined = true;
      json = { status: "success", item: member() };
    } else if (path === "/auth/me") json = { status: "success", user };
    else if (path === `/conferences/${room}/participants/me`) {
      if (!joined)
        return route.fulfill({
          status: 403,
          json: { message: "access denied" },
        });
      json = { status: "success", item: member() };
    } else if (path === `/conferences/${room}`)
      json = {
        status: "success",
        item: {
          id: room,
          ownerId: "owner",
          title: "Обсуждение проекта",
          status: "active",
          waitingRoomEnabled: true,
          createdAt: now,
          updatedAt: now,
          startedAt: now,
          finishedAt: null,
        },
      };
    else if (path.endsWith("/chat/read"))
      json = {
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: null },
      };
    else if (path.endsWith("/messages"))
      json = {
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
        lastReadMessageId: null,
      };
    else if (path.endsWith("/ws-ticket"))
      return route.fulfill({
        status: 503,
        json: { message: "No realtime server in fixture" },
      });
    else if (path === "/capabilities")
      json = {
        status: "success",
        buildVersion: "test",
        capabilities: {
          liveCaptions: false,
          transcription: false,
          aiSummary: false,
          semanticSearch: false,
          meetingAnalytics: false,
          recordingModes: [],
        },
      };
    await route.fulfill({ json });
  });
  return writes;
}

test("anonymous invitation opens directly, edits guest name and joins without waiting", async ({
  page,
}, info) => {
  const writes = await fixture(page);
  await page.goto(`/i/${code}`);
  await expect(page).toHaveURL(new RegExp(`/i/${code}$`));
  await expect(
    page.getByRole("heading", { name: "Обсуждение проекта" }),
  ).toBeVisible();
  await expect(page.getByLabel("Имя на встрече")).toHaveValue("Гость");
  expect(writes).toEqual([]);
  await page.getByLabel("Имя на встрече").fill("Мария");
  await page.screenshot({
    path: info.outputPath("guest-desktop.png"),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page.getByRole("button", { name: "Подключиться", exact: true }),
  ).toBeInViewport();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: info.outputPath("guest-mobile.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Настройки устройств", exact: true })
    .click();
  await expect(
    page.getByRole("dialog", { name: "Настройки устройств" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("button", { name: "Настройки устройств", exact: true }),
  ).toBeFocused();
  await page.getByRole("button", { name: "Подключиться", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/conferences/${room}$`));
  await expect(page.getByTestId("waiting-room")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Покинуть/ })).toBeVisible();
  await expect(
    page.getByRole("navigation", { name: "Основная навигация" }),
  ).toHaveCount(0);
  expect(writes.filter((path) => path.endsWith("/guest"))).toHaveLength(1);
  expect(writes.some((path) => /auth\/(register|login)/.test(path))).toBe(
    false,
  );
  await page.reload();
  await expect(page.getByTestId("waiting-room")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Покинуть/ })).toBeVisible();
});

test("signed in invitation displays the account name and uses its existing membership", async ({
  page,
}) => {
  const writes = await fixture(page, true);
  await page.goto(`/i/${code}`);
  await expect(
    page.getByRole("heading", { name: "Анна Смирнова" }),
  ).toBeVisible();
  await expect(page.getByLabel("Имя на встрече")).toHaveCount(0);
  await page.getByRole("button", { name: "Подключиться", exact: true }).click();
  await expect(page.getByTestId("waiting-room")).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Покинуть/ })).toBeVisible();
  expect(writes.filter((path) => path.endsWith("/join"))).toHaveLength(1);
  expect(writes.some((path) => path.endsWith("/guest"))).toBe(false);
});

test("closed invitation is public but cannot create a guest", async ({
  page,
}) => {
  const writes = await fixture(page, false, true);
  await page.goto(`/i/${code}`);
  await expect(page.getByText("Встреча завершена.")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Подключиться", exact: true }),
  ).toBeDisabled();
  expect(writes).toEqual([]);
});
