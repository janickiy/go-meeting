import { expect, test, type Page } from "@playwright/test";

const user = {
  id: "parity-self",
  email: "anna@example.test",
  displayName: "Анна Морозова",
  createdAt: "2026-09-14T10:00:00Z",
  updatedAt: "2026-09-14T10:00:00Z",
};
const peer = { id: "parity-peer", displayName: "Мария Орлова" };
const direct = {
  id: "parity-direct",
  type: "direct",
  peer,
  createdAt: "2026-10-09T09:00:00Z",
  lastMessageAt: "2026-10-09T09:42:00Z",
  lastMessageId: "msg4",
  preview: "Обновлю повестку до вечера.",
  unreadCount: 2,
  notificationsEnabled: true,
  historyClearedThrough: 0,
};
const group = {
  id: "parity-group",
  type: "group",
  name: "Команда продукта",
  description: "Обсуждаем продукт и готовимся к встречам.",
  createdBy: user.id,
  createdAt: direct.createdAt,
  updatedAt: direct.createdAt,
  memberCount: 3,
  myRole: "owner",
  avatarVersion: null,
  lastMessageAt: "2026-10-09T09:28:00Z",
  lastMessageId: "msg3",
  lastSender: peer,
  preview: "Обсудим завтра на встрече",
  unreadCount: 5,
};
const folders = [
  {
    id: "parity-folder",
    name: "Продуктовая команда",
    conversationCount: 2,
    conferenceCount: 0,
    itemCount: 2,
    position: 0,
    createdAt: direct.createdAt,
    updatedAt: direct.createdAt,
  },
  {
    id: "parity-folder-design",
    name: "Дизайн и исследования",
    conversationCount: 0,
    conferenceCount: 0,
    itemCount: 0,
    position: 1,
    createdAt: direct.createdAt,
    updatedAt: direct.createdAt,
  },
];

/** Изолирует реальные React-страницы от любых данных и изменений рабочего сервера. */
async function fixture(page: Page) {
  await page.routeWebSocket("**/api/v1/ws**", () => {});
  await page.addInitScript(() =>
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "parity-only",
        expiresAt: Date.now() + 3_600_000,
      }),
    ),
  );
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const json = (data: unknown) => route.fulfill({ json: data });
    if (["/auth/session", "/auth/refresh"].includes(path))
      return json({
        status: "success",
        accessToken: "parity-only",
        expiresIn: 3600,
        user,
      });
    if (path === "/auth/me") return json({ status: "success", user });
    if (path === "/capabilities")
      return json({ capabilities: { recordingModes: ["composite"] } });
    if (path === "/notifications/events")
      return route.fulfill({
        contentType: "text/event-stream",
        body: ": fixture\n\n",
      });
    if (path === "/ws-ticket")
      return json({ ticket: "isolated", expiresAt: "2099-01-01T00:00:00Z" });
    if (path === "/notifications") return json({ items: [], unreadCount: 0 });
    if (path === "/users") return json({ items: [peer] });
    if (path === "/conversations")
      return json({ items: [direct, group], unreadCount: 7, nextCursor: null });
    const item = path.includes(group.id) ? group : direct;
    if (/^\/conversations\/[^/]+$/.test(path)) return json({ item });
    if (path.endsWith("/members"))
      return json({
        items: [
          {
            id: user.id,
            displayName: user.displayName,
            role: "owner",
            online: true,
          },
          { ...peer, role: "admin", online: true },
          { id: "ivan", displayName: "Иван Ким", role: "member", online: null },
        ],
      });
    if (path.endsWith("/messages"))
      return json({
        items: [
          {
            id: "msg1",
            text: "Привет! Подготовила структуру следующей встречи. Посмотри, пожалуйста, когда будет время.",
            senderId: peer.id,
            senderName: peer.displayName,
            attachments: [],
            createdAt: "2026-10-09T09:34:00Z",
            updatedAt: "2026-10-09T09:34:00Z",
            deletedAt: null,
            version: 1,
            conversationId: item.id,
            sequence: "1",
          },
          {
            id: "msg2",
            text: "",
            senderId: peer.id,
            senderName: peer.displayName,
            attachments: [
              {
                id: "file",
                filename: "Структура встречи.pdf",
                size: 438272,
                contentType: "application/pdf",
              },
            ],
            createdAt: "2026-10-09T09:35:00Z",
            updatedAt: "2026-10-09T09:35:00Z",
            deletedAt: null,
            version: 1,
            conversationId: item.id,
            sequence: "2",
          },
          {
            id: "msg3",
            text: "Спасибо! Посмотрела — всё понятно. Добавим в конце 10 минут на вопросы? 👍",
            senderId: user.id,
            senderName: user.displayName,
            attachments: [],
            replyPreview: {
              senderName: peer.displayName,
              text: "Подготовила структуру следующей встречи.",
            },
            createdAt: "2026-10-09T09:40:00Z",
            updatedAt: "2026-10-09T09:40:00Z",
            deletedAt: null,
            version: 1,
            conversationId: item.id,
            sequence: "3",
          },
          {
            id: "msg4",
            text: "Да, хорошая идея. Обновлю повестку до вечера.",
            senderId: peer.id,
            senderName: peer.displayName,
            attachments: [],
            createdAt: "2026-10-09T09:42:00Z",
            updatedAt: "2026-10-09T09:42:00Z",
            deletedAt: null,
            version: 1,
            conversationId: item.id,
            sequence: "4",
          },
        ],
        unreadCount: 0,
        nextCursor: null,
      });
    if (path.endsWith("/read"))
      return json({ item: { unreadCount: 0, lastReadMessageId: "msg4" } });
    if (path === "/folders") return json({ items: folders });
    if (path === "/folders/parity-folder") return json({ item: folders[0] });
    if (path === "/folders/parity-folder/items" || path === "/folder-items")
      return json({
        items: [direct, group].map((item) => ({
          type: "conversation",
          item,
          inFolder: true,
        })),
        nextCursor: null,
      });
    return route.fulfill({
      status: 501,
      json: { message: `Изолированный макет не разрешает ${path}` },
    });
  });
}

for (const viewport of [
  { name: "desktop", width: 1440, height: 1000 },
  { name: "tablet", width: 834, height: 1112 },
  { name: "mobile", width: 390, height: 844 },
]) {
  test(`макеты общения и папок: ${viewport.name}`, async ({ page }, info) => {
    await page.setViewportSize(viewport);
    await fixture(page);
    const capture = async (name: string) => {
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth + 1,
        ),
      ).toBe(true);
      await page.screenshot({
        path: info.outputPath(`${name}-${viewport.name}.png`),
        fullPage: true,
      });
    };
    await page.goto("/personal");
    await expect(
      page
        .locator(".personal-sidebar")
        .getByRole("heading", { name: "Личные" }),
    ).toBeVisible();
    if (viewport.name === "mobile") {
      await expect(
        page.getByRole("navigation", { name: "Быстрая навигация" }),
      ).toBeVisible();
    }
    await capture("personal");
    await page
      .locator(".personal-sidebar")
      .getByRole("button", { name: "Новый чат", exact: true })
      .click();
    await page
      .getByRole("menuitem", { name: "Создать группу", exact: true })
      .click();
    await expect(
      page.getByRole("dialog", { name: "Создать группу", exact: true }),
    ).toBeVisible();
    await capture("group-create");
    await page.keyboard.press("Escape");
    await page.goto("/personal/parity-direct");
    await expect(page.getByTestId("chat-message-msg4")).toBeVisible();
    if (viewport.name === "mobile") {
      await expect(
        page.getByRole("navigation", { name: "Быстрая навигация" }),
      ).toBeHidden();
      const thread = await page.locator(".personal-page").boundingBox();
      expect(thread!.y + thread!.height).toBeCloseTo(viewport.height, 0);
      const attachment = page.locator(".chat-attachment").first();
      await expect(attachment).toHaveCSS("flex-direction", "row");
      const icon = await attachment.locator(".chat-file-icon").boundingBox();
      const copy = await attachment.locator(".chat-file-copy").boundingBox();
      expect(icon!.x + icon!.width).toBeLessThan(copy!.x);
    }
    await capture("direct-chat");
    if (viewport.name === "mobile") {
      await page.getByRole("link", { name: "Назад к перепискам" }).click();
      await expect(page).toHaveURL(/\/personal$/);
      await expect(
        page.getByRole("navigation", { name: "Быстрая навигация" }),
      ).toBeVisible();
      await expect(page.locator(".personal-sidebar")).toBeVisible();
    }
    await page.goto("/personal/parity-group");
    await expect(page.getByTestId("chat-message-msg4")).toBeVisible();
    if (viewport.name === "mobile") {
      await expect(
        page.getByRole("navigation", { name: "Быстрая навигация" }),
      ).toBeHidden();
    }
    await capture("group-chat");
    await page
      .getByRole("button", { name: "Информация о группе", exact: true })
      .click();
    await expect(
      page.getByRole("list", { name: "Участники группы" }),
    ).toContainText("Иван Ким");
    if (viewport.name === "mobile") {
      const dialog = await page
        .getByRole("dialog", { name: "Информация о группе" })
        .boundingBox();
      expect(dialog!.width).toBe(viewport.width);
      expect(dialog!.height).toBe(viewport.height);
    }
    await capture("group-info");
    await page.keyboard.press("Escape");
    await page.goto("/folders");
    await expect(
      page.getByRole("heading", { name: "Папки", exact: true }),
    ).toBeVisible();
    await capture("folders");
    await page
      .getByRole("button", { name: "Новая папка", exact: true })
      .click();
    await expect(
      page.getByRole("dialog", { name: "Новая папка", exact: true }),
    ).toBeVisible();
    await capture("folder-create");
    await page.keyboard.press("Escape");
    await page.goto("/folders/parity-folder");
    await expect(
      page.getByRole("heading", { name: "Продуктовая команда", exact: true }),
    ).toBeVisible();
    await capture("folder-detail");
    await page.getByRole("button", { name: "Добавить", exact: true }).click();
    await expect(
      page.getByRole("checkbox", { name: "Мария Орлова", exact: true }),
    ).toBeVisible();
    await capture("folder-add");
    await page.keyboard.press("Escape");
    await page
      .getByRole("button", {
        name: "Действия с папкой: Продуктовая команда",
        exact: true,
      })
      .click();
    await page
      .getByRole("menuitem", { name: "Удалить папку", exact: true })
      .click();
    await expect(
      page.getByRole("dialog", { name: "Удалить папку", exact: true }),
    ).toBeVisible();
    await capture("folder-delete");
  });
}
