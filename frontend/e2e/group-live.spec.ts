import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import axe from "axe-core";

const fixturePath = process.env.MEET_GROUP_FIXTURE;
test.skip(!fixturePath, "isolated group API required");
type Actor = { id: string; token: string; name: string };
const fixture = fixturePath
  ? (JSON.parse(readFileSync(fixturePath, "utf8")) as {
      url: string;
      alice: Actor;
      bob: Actor;
      charlie: Actor;
      dana: Actor;
    })
  : null;
async function login(page: Page, actor: Actor) {
  await page.goto("/login");
  // Let the anonymous bootstrap finish before installing fixture credentials.
  await page.waitForLoadState("networkidle");
  await page.evaluate(async (actor) => {
    const helperPath = "/src/utils.ts";
    const utils = await import(helperPath);
    utils.saveSession({ token: actor.token, expiresAt: Date.now() + 1800000 });
  }, actor);
  await page.goto("/personal");
  await expect(
    page.getByRole("heading", { name: "Личные", exact: true }),
  ).toBeVisible();
}
async function modalAxe(page: Page) {
  await page.evaluate(axe.source);
  const violations = await page.evaluate(async () => {
    const runtime = window as unknown as { axe: { run: typeof axe.run } };
    const result = await runtime.axe.run(
      document.querySelector('[role="dialog"]')!,
      {
        runOnly: {
          type: "tag",
          values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
        },
      },
    );
    return result.violations.map(({ id, nodes }) => ({
      id,
      nodes: nodes.map((node) => node.target),
    }));
  });
  expect(violations).toEqual([]);
}

test("group lifecycle with actual API, realtime, files, permissions, revocation and mobile", async ({
  browser,
  page,
}, info) => {
  test.setTimeout(90000);
  page.setDefaultTimeout(10000);
  const groupName = `Команда браузера ${Date.now()}`;
  const updatedName = `Обновлённая группа ${Date.now()}`;
  const { alice, bob, charlie, dana } = fixture!;
  const context = await browser.newContext();
  const bobPage = await context.newPage();
  await login(page, alice);
  await login(bobPage, bob);
  await page
    .locator(".personal-sidebar")
    .getByRole("button", { name: "Новый чат", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Создать группу", exact: true })
    .click();
  await page.getByRole("textbox", { name: "Название группы" }).fill(groupName);
  await page
    .getByRole("textbox", { name: "Описание (необязательно)" })
    .fill("Проверка групповых сообщений");
  await page
    .getByRole("searchbox", { name: "Найти пользователя" })
    .fill(bob.name.slice(0, 3));
  await page.getByRole("checkbox", { name: bob.name, exact: true }).check();
  await modalAxe(page);
  await page.getByRole("button", { name: "Создать", exact: true }).click();
  await expect(page).toHaveURL(/\/personal\/[0-9a-f-]+$/);
  const id = page.url().split("/").at(-1)!;
  const thread = page.getByRole("log", { name: "Личные сообщения" });
  await expect(thread).toBeVisible();
  await expect(
    bobPage.locator(".personal-conversation").filter({ hasText: groupName }),
  ).toBeVisible();
  await page
    .getByRole("textbox", { name: "Сообщение", exact: true })
    .fill("Сообщение группе");
  await page.getByRole("button", { name: "Отправить", exact: true }).click();
  await expect(thread).toContainText("Сообщение группе");
  await expect(
    bobPage.locator(".personal-conversation").filter({ hasText: groupName }),
  ).toContainText("Сообщение группе");
  await bobPage
    .locator(".personal-conversation")
    .filter({ hasText: groupName })
    .click();
  await expect(bobPage.getByRole("log")).toContainText("Сообщение группе");
  await bobPage.getByRole("button", { name: /Действия с сообщением:/ }).click();
  await bobPage
    .getByRole("menuitem", { name: "Ответить", exact: true })
    .click();
  await bobPage
    .getByRole("textbox", { name: "Сообщение", exact: true })
    .fill("Ответ участника");
  await bobPage.getByRole("button", { name: "Отправить", exact: true }).click();
  await expect(thread).toContainText("Ответ участника");
  await bobPage
    .locator(".chat-message-own")
    .getByRole("button", { name: /Действия с сообщением:/ })
    .click();
  await expect(
    bobPage.getByRole("menuitem", { name: "Изменить", exact: true }),
  ).toHaveCount(1);
  await bobPage.keyboard.press("Escape");
  await page
    .locator(".chat-message-own")
    .getByRole("button", { name: /Действия с сообщением:/ })
    .click();
  await page.getByRole("menuitem", { name: "Изменить", exact: true }).click();
  await page.locator("#edit-message").fill("Исправлено для группы");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Сохранить" })
    .click();
  await expect(bobPage.getByRole("log")).toContainText("Исправлено для группы");
  await page.getByLabel("Выбрать файлы для сообщения").setInputFiles({
    name: "group-note.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("Private group bytes"),
  });
  await expect(page.locator(".chat-upload-details")).toContainText(
    "group-note.txt",
  );
  await page.getByRole("button", { name: "Отправить", exact: true }).click();
  await expect(bobPage.getByRole("log")).toContainText("group-note.txt");
  await bobPage
    .getByRole("button", { name: "Получить ссылку", exact: true })
    .click();
  await expect(
    bobPage.getByRole("link", { name: "Скачать файл", exact: true }),
  ).toHaveAttribute("href", /^blob:/);
  await page.getByRole("button", { name: "Информация о группе" }).click();
  await modalAxe(page);
  await page.screenshot({ path: info.outputPath("group-info-desktop.png") });
  await expect(
    page.getByRole("list", { name: "Участники группы" }),
  ).toContainText(bob.name);
  await page
    .getByRole("button", { name: `Действия с участником: ${bob.name}` })
    .click();
  await page
    .getByRole("menuitem", { name: "Назначить администратором" })
    .click();
  await page
    .getByRole("button", { name: `Действия с участником: ${bob.name}` })
    .click();
  await expect(
    page.getByRole("menuitem", { name: "Снять администратора" }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await page
    .getByRole("button", { name: "Добавить участников", exact: true })
    .click();
  await page
    .getByRole("searchbox", { name: "Найти пользователя" })
    .fill(charlie.name.slice(0, 3));
  await page.getByRole("checkbox", { name: charlie.name, exact: true }).check();
  await page.getByRole("button", { name: "Добавить выбранных" }).click();
  await expect(
    page.getByRole("list", { name: "Участники группы" }),
  ).toContainText(charlie.name);
  await page.getByRole("button", { name: "О группе", exact: true }).click();
  await page.getByRole("button", { name: "Настройки группы" }).click();
  await page
    .getByRole("textbox", { name: "Название группы" })
    .fill(updatedName);
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText(updatedName);
  await page.keyboard.press("Escape");
  await bobPage.getByRole("button", { name: "Информация о группе" }).click();
  await expect(
    bobPage.getByRole("button", { name: "Настройки группы" }),
  ).toBeVisible();
  await expect(
    bobPage.getByRole("button", { name: "Удалить группу", exact: true }),
  ).toHaveCount(0);
  await bobPage.keyboard.press("Escape");
  await page.getByRole("button", { name: "Информация о группе" }).click();
  const bobRow = page.getByRole("listitem").filter({ hasText: bob.name });
  await bobRow.getByRole("button", { name: /Действия с участником:/ }).click();
  await page
    .getByRole("menuitem", { name: "Удалить участника", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Подтвердить удаление участника" })
    .click();
  await expect(bobPage).toHaveURL(/\/personal$/);
  await expect(
    bobPage.locator(".personal-conversation").filter({ hasText: updatedName }),
  ).toHaveCount(0);
  const denied = await bobPage.request.get(
    `${fixture!.url}/api/v1/conversations/${id}/messages`,
    { headers: { Authorization: `Bearer ${bob.token}` } },
  );
  expect(denied.status()).toBe(403);
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  for (const width of [390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    await expect(page.locator(".personal-sidebar")).toBeHidden();
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
      .toBeLessThanOrEqual(width);
    await page.getByRole("button", { name: "Информация о группе" }).click();
    await modalAxe(page);
    await page.screenshot({
      path: info.outputPath(`group-mobile-${width}.png`),
    });
    await page.keyboard.press("Escape");
  }
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.getByRole("button", { name: "Информация о группе" }).click();
  await page
    .getByRole("button", { name: "Выйти из группы", exact: true })
    .click();
  await expect(
    page.getByText(/Перед выходом передайте владение/),
  ).toBeVisible();
  await page.getByRole("button", { name: "Выбрать нового владельца" }).click();
  await page
    .getByRole("radio", { name: `Передать владение: ${charlie.name}` })
    .check();
  await page
    .getByRole("button", { name: "Передать владение", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Удалить группу", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Выйти из группы", exact: true })
    .click();
  await page.getByRole("button", { name: "Подтвердить выход" }).click();
  await expect(page).toHaveURL(/\/personal$/);
  await page.getByRole("button", { name: "Группы", exact: true }).click();
  await expect(page.locator(".personal-conversation")).toHaveCount(0);
  await page.getByRole("button", { name: "Личные", exact: true }).click();
  await expect(page).toHaveURL(/filter=direct/);
  await page.getByRole("button", { name: "Новые", exact: true }).click();
  await expect(page).toHaveURL(/filter=unread/);
  await context.close();
  void dana;
});

test("avatar retry stays idempotent and missed WebSocket revoke closes cached members", async ({
  browser,
  page,
}, info) => {
  test.setTimeout(45000);
  page.setDefaultTimeout(10000);
  const { alice, bob } = fixture!;
  const name = `Аватар ${Date.now()}`;
  let creates = 0,
    uploads = 0;
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      new URL(request.url()).pathname === "/api/v1/conversations/group"
    )
      creates++;
  });
  await page.route("**/api/v1/conversations/*/avatar", async (route) => {
    if (route.request().method() !== "PUT") return route.continue();
    if (++uploads === 1)
      return route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ status: "error" }),
      });
    return route.continue();
  });
  await login(page, alice);
  await page
    .locator(".personal-sidebar")
    .getByRole("button", { name: "Новый чат", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Создать группу", exact: true })
    .click();
  await page.getByRole("textbox", { name: "Название группы" }).fill(name);
  await page
    .getByRole("searchbox", { name: "Найти пользователя" })
    .fill(bob.name);
  await page.getByRole("checkbox", { name: bob.name, exact: true }).check();
  const png = await page.evaluate(() => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 64;
    const context = canvas.getContext("2d")!;
    context.fillStyle = "#1763ed";
    context.fillRect(0, 0, 64, 64);
    return canvas.toDataURL("image/png").split(",")[1];
  });
  await page.getByLabel("Файл аватара группы").setInputFiles({
    name: "avatar.png",
    mimeType: "image/png",
    buffer: Buffer.from(png, "base64"),
  });
  await page.getByRole("button", { name: "Создать", exact: true }).click();
  await expect(page.getByText(/Группа создана/)).toBeVisible();
  await page
    .getByRole("button", { name: "Повторить загрузку аватара" })
    .click();
  await expect(page).toHaveURL(/\/personal\/[0-9a-f-]+$/);
  expect(creates).toBe(1);
  expect(uploads).toBe(2);
  const id = page.url().split("/").at(-1)!;
  await expect(
    page.locator(".personal-header .group-avatar img"),
  ).toHaveAttribute("src", /^blob:/);
  const anonymousAvatar = await page.request.get(
    `${fixture!.url}/api/v1/conversations/${id}/avatar/content`,
  );
  expect(anonymousAvatar.status()).toBe(401);
  await page
    .getByRole("textbox", { name: "Сообщение", exact: true })
    .fill("Приватное до удаления");
  await page.getByRole("button", { name: "Отправить", exact: true }).click();
  await expect(page.getByRole("log")).toContainText("Приватное до удаления");
  const other = await browser.newContext();
  const bobPage = await other.newPage();
  await bobPage.routeWebSocket(/\/api\/v1\/ws(?:\?|$)/, (socket) =>
    socket.close(),
  );
  await login(bobPage, bob);
  await bobPage
    .locator(".personal-conversation")
    .filter({ hasText: name })
    .click();
  await expect(bobPage.getByRole("log")).toContainText("Приватное до удаления");
  await bobPage.setViewportSize({ width: 320, height: 844 });
  await bobPage.getByRole("button", { name: "Информация о группе" }).click();
  await bobPage.getByRole("button", { name: "Участники (2)" }).click();
  await expect(
    bobPage.getByRole("list", { name: "Участники группы" }),
  ).toContainText(alice.name);
  await modalAxe(bobPage);
  await bobPage.screenshot({ path: info.outputPath("group-members-320.png") });
  const removed = await page.request.delete(
    `${fixture!.url}/api/v1/conversations/${id}/members/${bob.id}`,
    { headers: { Authorization: `Bearer ${alice.token}` } },
  );
  expect(removed.ok()).toBeTruthy();
  await expect(bobPage).toHaveURL(/\/personal$/, { timeout: 9000 });
  await expect(bobPage.getByRole("dialog")).toHaveCount(0);
  await expect(
    bobPage.locator(".personal-conversation").filter({ hasText: name }),
  ).toHaveCount(0);
  await expect(
    bobPage.getByText("Приватное до удаления", { exact: true }),
  ).toHaveCount(0);
  await other.close();
});
