import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import axe from "axe-core";
const fixturePath = process.env.MEET_FOLDER_FIXTURE;
test.skip(!fixturePath, "isolated folder API required");
type Actor = { id: string; token: string; name: string };
const fixture = fixturePath
  ? (JSON.parse(readFileSync(fixturePath, "utf8")) as {
      url: string;
      alice: Actor;
      bob: Actor;
      charlie: Actor;
      dana: Actor;
      conversationId: string;
      groupId: string;
      meetingId: string;
    })
  : null;
async function login(page: Page, actor: Actor, path = "/folders") {
  await page.goto("/login");
  await page.waitForLoadState("networkidle");
  await page.evaluate(async (actor) => {
    const helper = "/src/utils.ts";
    const utils = await import(helper);
    utils.saveSession({ token: actor.token, expiresAt: Date.now() + 1800000 });
  }, actor);
  await page.goto(path);
  await expect(
    page.getByRole("heading", {
      name: path.startsWith("/personal") ? "Личные" : "Папки",
      exact: true,
    }),
  ).toBeVisible();
}
async function audit(page: Page, dialog = false) {
  await page.evaluate(axe.source);
  const violations = await page.evaluate(async (dialog) => {
    const runtime = window as unknown as { axe: { run: typeof axe.run } };
    const result = await runtime.axe.run(
      dialog
        ? document.querySelector('[role="dialog"]')!
        : document.getElementById("workspace-main")!,
      {
        runOnly: {
          type: "tag",
          values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
        },
      },
    );
    return result.violations.map(({ id, nodes }) => ({
      id,
      targets: nodes.map((node) => node.target),
    }));
  }, dialog);
  expect(violations).toEqual([]);
  const width = await page.evaluate(() => ({
    inner: innerWidth,
    scroll: document.documentElement.scrollWidth,
  }));
  expect(width.scroll).toBeLessThanOrEqual(width.inner + 1);
  for (const size of await page
    .locator(".folder-candidates .personal-avatar")
    .evaluateAll((elements) =>
      elements.map((element) => {
        const box = element.getBoundingClientRect();
        return { width: box.width, height: box.height };
      }),
    )) {
    expect(Math.abs(size.width - size.height)).toBeLessThanOrEqual(1);
  }
}
async function create(page: Page, name: string) {
  await page.getByRole("button", { name: "Новая папка", exact: true }).click();
  await page.getByRole("textbox", { name: "Название папки" }).fill(name);
  await page.getByRole("button", { name: "Создать", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    page.locator(".folder-link").filter({ hasText: name }),
  ).toBeVisible();
}

test("personal folders organize direct/group/meeting independently, sync devices, preserve items and mobile focus", async ({
  browser,
  page,
}, info) => {
  test.setTimeout(120000);
  page.setDefaultTimeout(10000);
  const { alice, bob, conversationId, groupId, meetingId } = fixture!;
  const name = `Проект MeetSpace ${Date.now()}`,
    second = `Работа ${Date.now()}`,
    renamed = `Команда проекта ${Date.now()}`;
  for (const actor of [alice, bob]) {
    const existing = await page.request.get(`${fixture!.url}/api/v1/folders`, {
      headers: { Authorization: `Bearer ${actor.token}` },
    });
    expect(existing.ok()).toBeTruthy();
    for (const item of (await existing.json()).items) {
      const removed = await page.request.delete(
        `${fixture!.url}/api/v1/folders/${item.id}`,
        { headers: { Authorization: `Bearer ${actor.token}` } },
      );
      expect(removed.ok()).toBeTruthy();
    }
  }
  const another = await browser.newContext();
  const sameActor = await another.newPage();
  const outsider = await browser.newContext();
  const bobPage = await outsider.newPage();
  await login(page, alice);
  await login(sameActor, alice);
  await login(bobPage, bob);
  await expect(bobPage.getByText("У вас пока нет папок")).toBeVisible();
  await create(page, name);
  await expect(
    sameActor.locator(".folder-link").filter({ hasText: name }),
  ).toBeVisible();
  await expect(bobPage.locator(".folder-link")).toHaveCount(0);
  await create(page, second);
  await page.locator(".folder-link").filter({ hasText: name }).click();
  const folderId = page.url().split("/").at(-1)!;
  await expect(page.getByText("В этой папке пока ничего нет")).toBeVisible();
  await page.getByRole("button", { name: "Добавить", exact: true }).click();
  const candidate = page.getByRole("dialog", { name: "Добавить в папку" });
  await candidate
    .getByRole("checkbox", { name: bob.name, exact: true })
    .check();
  await expect(
    candidate.getByRole("checkbox", { name: bob.name, exact: true }),
  ).toBeEnabled();
  await candidate
    .getByRole("checkbox", { name: "Команда папок", exact: true })
    .check();
  await expect(
    candidate.getByRole("checkbox", { name: "Команда папок", exact: true }),
  ).toBeEnabled();
  await candidate
    .getByRole("checkbox", { name: "Stage 2", exact: true })
    .check();
  await expect(
    candidate.getByRole("checkbox", { name: "Stage 2", exact: true }),
  ).toBeEnabled();
  await audit(page, true);
  await candidate.getByRole("button", { name: "Готово" }).click();
  await expect(page.locator(".folder-link")).toHaveCount(3);
  await expect(
    page.locator(`.folder-link[href="/personal/${conversationId}"]`),
  ).toBeVisible();
  await expect(
    page.locator(`.folder-link[href="/personal/${groupId}"]`),
  ).toBeVisible();
  await expect(
    page.locator(`.folder-link[href="/meetings/${meetingId}"]`),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Встречи", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Чаты", exact: true }),
  ).toBeVisible();
  await audit(page);
  await page.screenshot({ path: info.outputPath("folder-detail-1440.png") });
  await page.getByRole("button", { name: "Встречи", exact: true }).click();
  await expect(page.locator(".folder-link")).toHaveCount(1);
  await page.getByRole("button", { name: "Чаты", exact: true }).click();
  await expect(page.locator(".folder-link")).toHaveCount(2);
  await page.getByRole("button", { name: "Все", exact: true }).click();
  await page.getByRole("textbox", { name: "Поиск в папке" }).fill("Команда");
  await expect(page.locator(".folder-link")).toHaveCount(1);
  await page.getByRole("textbox", { name: "Поиск в папке" }).fill("");
  await expect(page.locator(".folder-link")).toHaveCount(3);
  // Same item in two folders, without changing the other participant's organization.
  await page
    .getByRole("button", { name: `Действия с элементом: ${bob.name}` })
    .click();
  await page.getByRole("menuitem", { name: "Добавить в папку" }).click();
  const picker = page.getByRole("dialog", { name: "Добавить в папку" });
  await expect(
    picker.getByRole("checkbox", { name: new RegExp(name) }),
  ).toBeChecked();
  await picker.getByRole("checkbox", { name: new RegExp(second) }).check();
  await expect(
    picker.getByRole("checkbox", { name: new RegExp(second) }),
  ).toBeEnabled();
  await audit(page, true);
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("button", { name: `Действия с элементом: ${bob.name}` }),
  ).toBeFocused();
  await page
    .getByRole("button", { name: `Действия с элементом: ${bob.name}` })
    .click();
  await page.getByRole("menuitem", { name: "Удалить из папки" }).click();
  await expect(page.locator(".folder-link")).toHaveCount(2);
  await page.getByRole("link", { name: "Все папки" }).click();
  await page.locator(".folder-link").filter({ hasText: second }).click();
  await expect(
    page.locator(`.folder-link[href="/personal/${conversationId}"]`),
  ).toBeVisible();
  // Existing conversation and conference menus use the same folder picker.
  await page.goto("/personal");
  await page
    .locator(".personal-conversation-row")
    .filter({ has: page.locator(`a[href="/personal/${groupId}"]`) })
    .getByRole("button", { name: "Действия с перепиской: Команда папок" })
    .click();
  await page.getByRole("menuitem", { name: "Добавить в папку" }).click();
  await expect(
    page.getByRole("checkbox", { name: new RegExp(name) }),
  ).toBeChecked();
  await page.keyboard.press("Escape");
  await page.goto("/conferences");
  await page.getByRole("tab", { name: "Активные" }).click();
  await page
    .getByRole("button", { name: "Действия с конференцией: Stage 2" })
    .click();
  await expect(
    page.getByRole("menuitem", { name: "Информация о чате" }),
  ).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Покинуть чат" }),
  ).toBeVisible();
  await page.getByRole("menuitem", { name: "Добавить в папку" }).click();
  await expect(
    page.getByRole("checkbox", { name: new RegExp(name) }),
  ).toBeChecked();
  await page.keyboard.press("Escape");
  await page.goto(`/folders/${folderId}`);
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    await audit(page);
    await page.screenshot({
      path: info.outputPath(`folder-detail-${width}.png`),
    });
    await page.getByRole("button", { name: "Добавить", exact: true }).click();
    await audit(page, true);
    await page.screenshot({
      path: info.outputPath(`folder-picker-${width}.png`),
    });
    await page.keyboard.press("Escape");
    await expect(
      page.getByRole("button", { name: "Добавить", exact: true }),
    ).toBeFocused();
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page
    .getByRole("button", { name: `Действия с папкой: ${name}` })
    .click();
  await page.getByRole("menuitem", { name: "Переименовать" }).click();
  await page.getByRole("textbox", { name: "Название папки" }).fill(renamed);
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: renamed, exact: true }),
  ).toBeVisible();
  await expect(
    sameActor.locator(".folder-link").filter({ hasText: renamed }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Все папки" }).click();
  await page
    .getByRole("button", { name: `Действия с папкой: ${second}` })
    .click();
  await page.getByRole("menuitem", { name: "Переместить выше" }).click();
  await expect(page.locator(".folder-link").first()).toContainText(second);
  await page.reload();
  await expect(page.locator(".folder-link").first()).toContainText(second);
  await page
    .getByRole("button", { name: `Действия с папкой: ${renamed}` })
    .click();
  await page.getByRole("menuitem", { name: "Удалить папку" }).click();
  await audit(page, true);
  await expect(page.getByRole("dialog")).toContainText(
    "Чаты и встречи сохранятся",
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Удалить папку", exact: true })
    .click();
  await expect(
    page.locator(".folder-link").filter({ hasText: renamed }),
  ).toHaveCount(0);
  await expect(
    sameActor.locator(".folder-link").filter({ hasText: renamed }),
  ).toHaveCount(0);
  await expect(bobPage.locator(".folder-link")).toHaveCount(0);
  for (const path of [
    `/conversations/${conversationId}`,
    `/conversations/${groupId}`,
    `/conferences/${meetingId}`,
  ]) {
    const response = await page.request.get(`${fixture!.url}/api/v1${path}`, {
      headers: { Authorization: `Bearer ${alice.token}` },
    });
    expect(response.ok()).toBeTruthy();
  }
  await page.goto(`/personal/${conversationId}`);
  await page
    .getByRole("textbox", { name: "Сообщение", exact: true })
    .fill("Сообщение после удаления папки");
  await page.getByRole("button", { name: "Отправить", exact: true }).click();
  await expect(page.getByRole("log")).toContainText(
    "Сообщение после удаления папки",
  );
  await bobPage.goto(`/personal/${conversationId}`);
  await expect(bobPage.getByRole("log")).toContainText(
    "Сообщение после удаления папки",
  );
  await another.close();
  await outsider.close();
});
test("folder recovery hides missed revokes immediately on mapping denial and refreshes lost meeting access", async ({
  page,
}, info) => {
  test.setTimeout(60000);
  page.setDefaultTimeout(10000);
  const { alice, bob, groupId, meetingId } = fixture!;
  const restored = await page.request.post(
    `${fixture!.url}/api/v1/conversations/${groupId}/members`,
    {
      headers: { Authorization: `Bearer ${alice.token}` },
      data: { userIds: [bob.id] },
    },
  );
  expect(restored.ok()).toBeTruthy();
  await page.routeWebSocket(/\/api\/v1\/ws(?:\?|$)/, (socket) =>
    socket.close(),
  );
  const created = await page.request.post(`${fixture!.url}/api/v1/folders`, {
    headers: { Authorization: `Bearer ${bob.token}` },
    data: { name: `Приватные проверки ${Date.now()}` },
  });
  expect(created.ok()).toBeTruthy();
  const id = (await created.json()).item.id;
  for (const [kind, itemId] of [
    ["conversation", groupId],
    ["conference", meetingId],
  ]) {
    const response = await page.request.put(
      `${fixture!.url}/api/v1/folders/${id}/items/${kind}/${itemId}`,
      { headers: { Authorization: `Bearer ${bob.token}` } },
    );
    expect(response.ok()).toBeTruthy();
  }
  await login(page, bob);
  await page.locator(`.folder-link[href="/folders/${id}"]`).click();
  await expect(
    page.locator(`.folder-link[href="/personal/${groupId}"]`),
  ).toBeVisible();
  await expect(
    page.locator(`.folder-link[href="/meetings/${meetingId}"]`),
  ).toBeVisible();
  await page.getByRole("button", { name: "Добавить", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(
    dialog.getByRole("checkbox", { name: "Команда папок", exact: true }),
  ).toBeChecked();
  const removed = await page.request.delete(
    `${fixture!.url}/api/v1/conversations/${groupId}/members/${bob.id}`,
    { headers: { Authorization: `Bearer ${alice.token}` } },
  );
  expect(removed.ok()).toBeTruthy();
  await page.route(/\/api\/v1\/folder-items(?:\?|$)/, (route) =>
    route.abort("internetdisconnected"),
  );
  // Mapping PUT is denied authoritatively; its failed refetch must never restore the preview.
  await dialog
    .getByRole("checkbox", { name: "Команда папок", exact: true })
    .uncheck();
  // DELETE mapping can succeed after access loss by design; immediately re-adding exercises the 403 path.
  const candidate = dialog.getByRole("checkbox", {
    name: "Команда папок",
    exact: true,
  });
  if (await candidate.count()) {
    await expect(candidate).toBeEnabled();
    await candidate.check();
  }
  await expect(page.getByText("Команда папок", { exact: true })).toHaveCount(0);
  await page.unroute(/\/api\/v1\/folder-items(?:\?|$)/);
  if (await page.getByRole("dialog").count())
    await page.keyboard.press("Escape");
  const left = await page.request.delete(
    `${fixture!.url}/api/v1/conferences/${meetingId}/chat/membership`,
    { headers: { Authorization: `Bearer ${bob.token}` }, data: {} },
  );
  expect(left.ok()).toBeTruthy();
  await expect(
    page.locator(`.folder-link[href="/meetings/${meetingId}"]`),
  ).toHaveCount(0, { timeout: 23000 });
  await expect(page.getByText("В этой папке пока ничего нет")).toBeVisible();
  await audit(page);
  await page.screenshot({ path: info.outputPath("folder-revoked-empty.png") });
});
test("focused folder geometry and readable cards on the final CSS", async ({
  page,
}, info) => {
  test.setTimeout(45000);
  page.setDefaultTimeout(10000);
  const { alice, conversationId, groupId, meetingId } = fixture!;
  await page.routeWebSocket(/\/api\/v1\/ws(?:\?|$)/, (socket) =>
    socket.close(),
  );
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const response = await route.fetch({
      url: `${fixture!.url}${url.pathname}${url.search}`,
    });
    await route.fulfill({ response });
  });
  const created = await page.request.post(`${fixture!.url}/api/v1/folders`, {
    headers: { Authorization: `Bearer ${alice.token}` },
    data: { name: `Проект MeetSpace · UI ${Date.now()}` },
  });
  expect(created.ok()).toBeTruthy();
  const folder = (await created.json()).item;
  try {
    for (const [kind, id] of [
      ["conversation", conversationId],
      ["conversation", groupId],
      ["conference", meetingId],
    ]) {
      const response = await page.request.put(
        `${fixture!.url}/api/v1/folders/${folder.id}/items/${kind}/${id}`,
        { headers: { Authorization: `Bearer ${alice.token}` } },
      );
      expect(response.ok()).toBeTruthy();
    }
    await login(page, alice);
    await page.locator(`.folder-link[href="/folders/${folder.id}"]`).click();
    await expect(page.locator(".folder-link")).toHaveCount(3);
    for (const width of [1440, 390, 320]) {
      await page.setViewportSize({
        width,
        height: width === 1440 ? 1000 : 844,
      });
      await audit(page);
      await page.screenshot({
        path: info.outputPath(`folder-detail-${width}.png`),
      });
      await page.getByRole("button", { name: "Добавить", exact: true }).click();
      await expect(
        page.getByRole("checkbox", { name: "Команда папок", exact: true }),
      ).toBeVisible();
      await audit(page, true);
      await page.screenshot({
        path: info.outputPath(`folder-picker-${width}.png`),
      });
      await page.keyboard.press("Escape");
      await expect(
        page.getByRole("button", { name: "Добавить", exact: true }),
      ).toBeFocused();
    }
  } finally {
    const deleted = await page.request.delete(
      `${fixture!.url}/api/v1/folders/${folder.id}`,
      { headers: { Authorization: `Bearer ${alice.token}` } },
    );
    expect(deleted.ok()).toBeTruthy();
  }
});
