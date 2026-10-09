import { expect, test, type Page } from "@playwright/test";
import axe from "axe-core";
import type { DirectConversation, User } from "../src/types";

const user: User = {
  id: "direct-actions-self",
  email: "self@example.test",
  displayName: "Александр",
  createdAt: "2026-10-09T00:00:00Z",
  updatedAt: "2026-10-09T00:00:00Z",
};
const initial: DirectConversation = {
  id: "direct-actions",
  type: "direct",
  peer: { id: "direct-actions-peer", displayName: "Василий" },
  createdAt: "2026-10-09T00:00:00Z",
  lastMessageAt: "2026-10-09T00:10:00Z",
  lastMessageId: "old-message",
  preview: "Материалы встречи",
  unreadCount: 0,
  notificationsEnabled: true,
  historyClearedThrough: 0,
};

/** Подменяет все API-запросы страницы, чтобы браузерные проверки не изменяли реальные переписки. */
async function fixture(page: Page) {
  let item = { ...initial };
  let hidden = false;
  const writes: string[] = [];
  const unexpected: string[] = [];
  await page.routeWebSocket("**/api/v1/ws**", () => {});
  await page.addInitScript(() =>
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "direct-actions-test",
        expiresAt: Date.now() + 1_800_000,
      }),
    ),
  );
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    const method = request.method();
    const reply = (body: unknown, status = 200) =>
      route.fulfill({ status, json: body });
    if (["/auth/session", "/auth/refresh"].includes(path))
      return reply({
        status: "success",
        accessToken: "direct-actions-test",
        expiresIn: 3600,
        user,
      });
    if (path === "/auth/me") return reply({ status: "success", user });
    if (path === "/capabilities")
      return reply({
        status: "success",
        buildVersion: "test",
        capabilities: {
          liveCaptions: false,
          transcription: false,
          aiSummary: false,
          semanticSearch: false,
          meetingAnalytics: false,
          recordingModes: ["composite"],
        },
      });
    if (path === "/ws-ticket")
      return reply({
        ticket: "isolated-chat-layout",
        expiresAt: "2099-01-01T00:00:00Z",
      });
    if (path === "/notifications/events")
      return route.fulfill({
        contentType: "text/event-stream",
        body: ": isolated\n\n",
      });
    if (path === "/notifications")
      return reply({
        status: "success",
        items: [],
        unreadCount: 0,
        nextCursor: null,
      });
    if (path === "/folders") return reply({ status: "success", items: [] });
    if (path === "/users") return reply({ items: [item.peer] });
    if (path === "/conversations/direct" && method === "POST") {
      writes.push("direct");
      return reply({ status: "success", item });
    }
    if (path === "/conversations")
      return reply({
        status: "success",
        items: hidden ? [] : [item],
        unreadCount: 0,
        nextCursor: null,
      });
    if (path === "/conversations/direct-actions" && method === "GET")
      return hidden
        ? reply({ message: "Чат скрыт" }, 404)
        : reply({ status: "success", item });
    if (path === "/conversations/direct-actions/preferences") {
      writes.push("preferences");
      item = {
        ...item,
        notificationsEnabled: request.postDataJSON().notificationsEnabled,
      };
      return reply({ status: "success", item });
    }
    if (path === "/conversations/direct-actions/clear-history") {
      writes.push("clear");
      item = {
        ...item,
        historyClearedThrough: 4,
        preview: "",
        lastMessageId: null,
        lastMessageAt: null,
      };
      return reply({ status: "success", item });
    }
    if (path === "/conversations/direct-actions/hide") {
      writes.push("hide");
      hidden = true;
      return reply({
        status: "success",
        hidden: true,
        historyClearedThrough: 4,
      });
    }
    if (path === "/conversations/direct-actions/messages")
      return reply({
        status: "success",
        unreadCount: 0,
        lastReadMessageId: null,
        nextCursor: null,
        items: item.historyClearedThrough
          ? []
          : [
              {
                id: "old-message",
                sequence: "4",
                conversationId: item.id,
                senderId: item.peer.id,
                senderName: item.peer.displayName,
                text: "Материалы встречи",
                createdAt: item.createdAt,
                updatedAt: item.createdAt,
                deletedAt: null,
                version: 1,
                attachments: [],
              },
            ],
      });
    if (
      [
        "/conversations/direct-actions/chat/read",
        "/conversations/direct-actions/read",
      ].includes(path)
    )
      return reply({
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: null },
      });
    unexpected.push(`${method} ${path}`);
    return reply(
      { message: "Неизвестный запрос заблокирован изолированной проверкой" },
      501,
    );
  });
  return { writes, unexpected };
}

for (const size of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`меню личного чата, информация и подтверждения: ${size.name}`, async ({
    page,
  }, info) => {
    await page.setViewportSize(size);
    const state = await fixture(page);
    await page.goto("/personal/direct-actions");
    const header = page.locator(".personal-header");
    const profile = header.getByRole("button", {
      name: "Информация о пользователе: Василий",
    });
    await expect(profile).toBeVisible();
    await expect(
      header.getByText("Личная переписка", { exact: true }),
    ).toBeVisible();
    const message = page.getByTestId("chat-message-old-message");
    await expect(
      message.getByRole("button", { name: /Действия с сообщением:/ }),
    ).toBeVisible();
    await expect(message.getByRole("menuitem")).toHaveCount(0);
    if (size.name === "desktop") {
      const searchBox = (await page.locator(".personal-search").boundingBox())!;
      const tabsBox = (await page.locator(".personal-filters").boundingBox())!;
      expect(searchBox.y + searchBox.height).toBeLessThanOrEqual(tabsBox.y);
      expect(
        (await page.locator(".personal-sidebar").boundingBox())!.width,
      ).toBeCloseTo(288, 0);
      await expect(page.locator(".personal-list-footer")).toBeVisible();
    }
    await page.screenshot({
      path: info.outputPath(`direct-chat-parity-${size.name}.png`),
      fullPage: true,
    });
    await profile.locator(".personal-avatar").click();
    const userInfo = page.getByRole("dialog", {
      name: "Информация о пользователе",
    });
    await expect(userInfo).toBeVisible();
    await expect(
      userInfo.getByText("direct-actions-peer", { exact: true }),
    ).toBeVisible();
    const bounds = (await userInfo.boundingBox())!;
    expect(bounds.x).toBeGreaterThanOrEqual(0);
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(size.width + 1);
    await page.screenshot({
      path: info.outputPath(`direct-user-info-${size.name}.png`),
      fullPage: true,
    });
    await page.addScriptTag({ content: axe.source });
    const violations = await page.evaluate(
      async () =>
        (
          await (window as typeof window & { axe: typeof axe }).axe.run(
            document.querySelector('[role="dialog"]')!,
          )
        ).violations,
    );
    expect(violations.map((violation) => violation.id)).toEqual([]);
    await page.keyboard.press("Escape");
    await expect(profile).toBeFocused();
    const trigger = header.getByRole("button", {
      name: "Действия с перепиской: Василий",
    });
    await trigger.click();
    const menu = page.getByRole("menu", {
      name: "Действия с перепиской: Василий",
    });
    await expect(menu.getByRole("menuitem")).toHaveCount(5);
    await page.screenshot({
      path: info.outputPath(`direct-actions-menu-${size.name}.png`),
      fullPage: true,
    });
    await menu.getByRole("menuitem", { name: "Без уведомлений" }).click();
    await expect(
      header.getByText("Без уведомлений", { exact: true }),
    ).toBeVisible();
    await trigger.click();
    await menu.getByRole("menuitem", { name: "Включить уведомления" }).click();
    await expect(
      header.getByText("Без уведомлений", { exact: true }),
    ).toHaveCount(0);
    await page
      .getByRole("textbox", { name: "Сообщение", exact: true })
      .fill("Черновик");
    await trigger.click();
    await menu.getByRole("menuitem", { name: "Очистить историю" }).click();
    const clear = page.getByRole("dialog", { name: "Очистить историю?" });
    await expect(clear.getByRole("button", { name: "Отмена" })).toBeFocused();
    await page.screenshot({
      path: info.outputPath(`direct-clear-${size.name}.png`),
      fullPage: true,
    });
    await clear
      .getByRole("button", { name: "Очистить историю", exact: true })
      .click();
    await expect(clear).toHaveCount(0);
    await expect(
      page.getByRole("textbox", { name: "Сообщение", exact: true }),
    ).toHaveValue("");
    await expect(page.getByTestId("chat-message-old-message")).toHaveCount(0);
    await trigger.click();
    await menu.getByRole("menuitem", { name: "Удалить чат" }).click();
    const hide = page.getByRole("dialog", { name: "Удалить чат?" });
    await page.screenshot({
      path: info.outputPath(`direct-delete-${size.name}.png`),
      fullPage: true,
    });
    await hide
      .getByRole("button", { name: "Удалить чат", exact: true })
      .click();
    await expect(page).toHaveURL(/\/personal$/);
    await expect(page.getByText("Василий", { exact: true })).toHaveCount(0);
    expect(state.writes).toEqual([
      "preferences",
      "preferences",
      "clear",
      "hide",
    ]);
    expect(state.unexpected).toEqual([]);
  });
}

test("новый личный чат открывается через поиск, без создания повторного диалога", async ({
  page,
}, info) => {
  const state = await fixture(page);
  await page.goto("/personal");
  await page
    .locator(".personal-sidebar")
    .getByRole("button", { name: "Новый чат" })
    .click();
  await page.getByRole("menuitem", { name: "Личный чат", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Новый чат", exact: true });
  await dialog
    .getByRole("searchbox", { name: "Найти пользователя" })
    .fill("Вас");
  await expect(
    dialog.getByRole("button", { name: "Василий Начать переписку" }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("new-direct-chat.png"),
    fullPage: true,
  });
  await dialog
    .getByRole("button", { name: "Василий Начать переписку" })
    .click();
  await expect(page).toHaveURL(/\/personal\/direct-actions$/);
  await expect(dialog).toHaveCount(0);
  expect(state.writes).toEqual(["direct"]);
  expect(state.unexpected).toEqual([]);
});
