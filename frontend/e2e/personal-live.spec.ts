import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
const path = process.env.MEET_PERSONAL_FIXTURE;
test.skip(!path, "isolated personal browser API required");
type Actor = { id: string; token: string; name: string };
const fixture = path
  ? (JSON.parse(readFileSync(path, "utf8")) as {
      url: string;
      alice: Actor;
      bob: Actor;
    })
  : null;
async function login(page: Page, actor: Actor) {
  // Use the application's actual storage helper to avoid hard-coding its key.
  await page.goto("/login");
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
test("two accounts: search, one pair, realtime, replies, edits, files, unread, reconnect and mobile", async ({
  browser,
  page,
}, info) => {
  test.setTimeout(90000);
  const alice = fixture!.alice,
    bob = fixture!.bob;
  const other = await browser.newContext();
  const bobPage = await other.newPage();
  await login(page, alice);
  await login(bobPage, bob);
  await page
    .getByRole("button", { name: "Новое сообщение", exact: true })
    .click();
  await page.getByRole("textbox", { name: "Имя пользователя" }).fill("Bo");
  await page.getByRole("button", { name: "B Bob" }).click();
  await expect(
    page.getByRole("log", { name: "Личные сообщения" }),
  ).toBeVisible();
  const conversationId = new URL(page.url()).pathname.split("/").at(-1)!;
  const send = page.getByRole("textbox", { name: "Сообщение", exact: true });
  await send.fill("Первое личное сообщение");
  await page.getByRole("button", { name: "Отправить", exact: true }).click();
  await expect(
    bobPage.locator(".personal-conversation").filter({ hasText: "Alice" }),
  ).toContainText("Первое личное сообщение");
  await expect(
    bobPage.locator('.sidebar-nav a[href="/personal"] .count-badge'),
  ).toHaveText("1");
  await bobPage
    .locator(".personal-conversation")
    .filter({ hasText: "Alice" })
    .click();
  await expect(bobPage.getByRole("log")).toContainText(
    "Первое личное сообщение",
  );
  await expect(
    bobPage.locator('.sidebar-nav a[href="/personal"] .count-badge'),
  ).toHaveCount(0, { timeout: 10000 });
  await bobPage.getByRole("button", { name: "Ответить", exact: true }).click();
  await bobPage
    .getByRole("textbox", { name: "Сообщение", exact: true })
    .fill("Ответ от Bob");
  await bobPage.getByRole("button", { name: "Отправить", exact: true }).click();
  await expect(page.getByRole("log")).toContainText("Ответ от Bob");
  await expect(page.locator(".personal-notice")).toHaveCount(0);
  await page.getByRole("button", { name: "Изменить", exact: true }).click();
  await page.locator("#edit-message").fill("Исправленное сообщение");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Сохранить" })
    .click();
  await expect(bobPage.getByRole("log")).toContainText(
    "Исправленное сообщение",
  );
  await page.locator('input[type="file"]').setInputFiles({
    name: "direct-note.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("Private file"),
  });
  await expect(page.locator(".chat-upload-details")).toContainText(
    "direct-note.txt",
  );
  await page.getByRole("button", { name: "Отправить", exact: true }).click();
  await expect(bobPage.getByRole("log")).toContainText("direct-note.txt");
  await page
    .getByRole("button", { name: "Удалить", exact: true })
    .first()
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Да, удалить сообщение", exact: true })
    .click();
  await expect(bobPage.getByRole("log")).toContainText("Сообщение удалено");
  await bobPage.reload();
  await expect(bobPage.getByRole("log")).toContainText("Ответ от Bob");
  await expect(bobPage.locator(".chat-message")).toHaveCount(3);
  await page.goto("/personal");
  await page
    .getByRole("button", { name: "Новое сообщение", exact: true })
    .click();
  await page.getByRole("textbox", { name: "Имя пользователя" }).fill("Bob");
  await page.getByRole("button", { name: "B Bob" }).click();
  await expect(page).toHaveURL(new RegExp(`/personal/${conversationId}$`));
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator(".personal-sidebar")).toBeHidden();
  await expect(
    page.getByRole("textbox", { name: "Сообщение", exact: true }),
  ).toBeVisible();
  const box = await page.locator(".chat-composer").boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(391);
  expect(box!.y + box!.height).toBeLessThanOrEqual(844);
  await page.screenshot({ path: info.outputPath("personal-mobile.png") });
  await page.getByRole("textbox", { name: "Сообщение", exact: true }).focus();
  await page.setViewportSize({ width: 390, height: 500 });
  await expect
    .poll(async () => {
      const composer = await page.locator(".chat-composer").boundingBox();
      return composer!.y + composer!.height;
    })
    .toBeLessThanOrEqual(500);
  await page.setViewportSize({ width: 320, height: 640 });
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
    .toBeLessThanOrEqual(320);
  await page.getByRole("link", { name: "Назад к перепискам" }).click();
  await expect(page.locator(".personal-sidebar")).toBeVisible();
  await expect(page.locator(".personal-content")).toBeHidden();
  await other.close();
});
