import { expect, test, type Page } from "@playwright/test";
import type { DirectConversation, User } from "../src/types";

const user: User = {
  id: "presence-self",
  email: "presence@example.test",
  displayName: "Алиса",
  createdAt: "2026-10-09T00:00:00Z",
  updatedAt: "2026-10-09T00:00:00Z",
};
const conversation: DirectConversation = {
  id: "presence-direct",
  type: "direct",
  peer: { id: "presence-peer", displayName: "Борис" },
  createdAt: "2026-10-09T00:00:00Z",
  lastMessageAt: null,
  lastMessageId: null,
  preview: "Личная переписка",
  unreadCount: 0,
  notificationsEnabled: true,
};
type PresenceMode =
  "loading" | "online" | "offline" | "error" | "mismatch" | 403 | 404;

/**
 * Полностью изолирует HTTP и WebSocket: неизвестные API не попадают на реальный сервер.
 * @args page — браузерная страница; mode — первоначальный ответ присутствия.
 * @return Управление ответами, число чтений и список любых незаявленных API-запросов.
 */
async function fixture(page: Page, mode: PresenceMode = "online") {
  const state = {
    mode,
    reads: 0,
    unexpected: [] as string[],
    release: undefined as (() => void) | undefined,
  };
  await page.routeWebSocket("**/api/v1/ws**", () => {});
  await page.addInitScript(() => {
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "isolated-presence-test",
        expiresAt: Date.now() + 1_800_000,
      }),
    );
  });
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    const reply = (body: unknown, status = 200) =>
      route.fulfill({ status, json: body });
    if (["/auth/session", "/auth/refresh"].includes(path))
      return reply({
        status: "success",
        accessToken: "isolated-presence-test",
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
        ticket: "isolated-presence-test",
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
    if (path === "/conversations")
      return reply({
        status: "success",
        items: [conversation],
        unreadCount: 0,
        nextCursor: null,
      });
    if (path === "/conversations/presence-direct")
      return reply({ status: "success", item: conversation });
    if (path === "/conversations/presence-direct/messages")
      return reply({
        status: "success",
        items: [],
        unreadCount: 0,
        nextCursor: null,
        lastReadMessageId: null,
      });
    if (path === "/conversations/presence-direct/chat/read")
      return reply({
        status: "success",
        item: { unreadCount: 0, lastReadMessageId: null },
      });
    if (
      path === "/conversations/presence-direct/peer-presence" &&
      request.method() === "GET"
    ) {
      state.reads++;
      if (state.mode === "loading")
        await new Promise<void>((resolve) => {
          state.release = resolve;
        });
      if (typeof state.mode === "number")
        return reply({ message: "Нет доступа к переписке" }, state.mode);
      if (state.mode === "error")
        return reply({ message: "Сервис недоступен" }, 503);
      return reply({
        status: "success",
        item: {
          conversationId: conversation.id,
          peerId:
            state.mode === "mismatch" ? "foreign-peer" : conversation.peer.id,
          online: state.mode !== "offline",
        },
      });
    }
    state.unexpected.push(`${request.method()} ${path}`);
    return reply(
      { message: "Неизвестный API заблокирован изолированной проверкой" },
      501,
    );
  });
  return state;
}

/**
 * Меняет видимость документа и отправляет настоящий сигнал обработчику фонового опроса.
 * @args page — изолированная страница; visible — видимость вкладки.
 * @return Завершённое событие visibilitychange в браузере.
 */
async function visibility(page: Page, visible: boolean) {
  await page.evaluate((visible) => {
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => (visible ? "visible" : "hidden"),
    });
    document.dispatchEvent(new Event("visibilitychange"));
  }, visible);
}

test("присутствие заголовка и информации: общий запрос, polling, ошибка и остановка после закрытия диалога", async ({
  page,
}) => {
  const state = await fixture(page, "loading");
  await page.goto("/personal/presence-direct");
  await expect(page.locator(".personal-list-footer")).toHaveCount(0);
  await expect(
    page.getByText("Переписки доступны только участникам", { exact: true }),
  ).toHaveCount(0);
  const headerStatus = page.locator(".personal-header").getByRole("status");
  await expect(headerStatus).toHaveText("Проверяем статус…");
  const profile = page
    .locator(".personal-header")
    .getByRole("button", { name: "Информация о пользователе: Борис" });
  await profile.locator(".personal-header-copy strong").click();
  const dialog = page.getByRole("dialog", {
    name: "Информация о пользователе",
  });
  const status = dialog.getByRole("status");
  await expect(status).toHaveText("Проверяем статус…");
  expect(state.reads).toBe(1);
  state.mode = "online";
  state.release!();
  await expect(status).toHaveText("В сети");
  await expect(headerStatus).toHaveText("Онлайн");
  await expect(headerStatus).toHaveClass(/personal-peer-status-online/);
  state.mode = "offline";
  await expect(status).toHaveText("не в сети.");
  await expect(headerStatus).toHaveText("Не в сети");
  await expect(headerStatus).not.toHaveClass(/personal-peer-status-online/);
  await dialog.getByRole("button", { name: "Закрыть", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  const closedReads = state.reads;
  await page.waitForTimeout(1400);
  expect(state.reads).toBeGreaterThan(closedReads);
  expect(state.reads).toBeLessThanOrEqual(closedReads + 2);
  state.mode = "error";
  await expect(headerStatus).toHaveText("Статус недоступен");
  await profile.locator(".personal-avatar").click();
  await expect(status).toHaveText("Статус временно недоступен.");
  await expect(status).not.toHaveClass(/personal-user-presence-online/);
  await page.keyboard.press("Escape");
  await expect(profile).toBeFocused();
  await page.goto("/personal");
  await expect(page.locator(".personal-header")).toHaveCount(0);
  const detailClosedReads = state.reads;
  await page.waitForTimeout(1400);
  expect(state.reads).toBe(detailClosedReads);
  const row = page
    .locator(".personal-conversation-row")
    .filter({ hasText: "Борис" });
  await row.hover();
  await row
    .getByRole("button", { name: "Действия с перепиской: Борис" })
    .click();
  state.mode = "offline";
  await page.getByRole("menuitem", { name: "Информация", exact: true }).click();
  await expect(status).toHaveText("не в сети.");
  expect(state.unexpected).toEqual([]);
});

test("без модального окна заголовок обновляет online/offline и не выдаёт ошибку за offline", async ({
  page,
}) => {
  const state = await fixture(page, "loading");
  await page.goto("/personal/presence-direct");
  const status = page.locator(".personal-header").getByRole("status");
  await expect(status).toHaveText("Проверяем статус…");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  state.mode = "online";
  state.release!();
  await expect(status).toHaveText("Онлайн");
  await expect(status).toHaveClass(/personal-peer-status-online/);
  state.mode = "offline";
  await expect(status).toHaveText("Не в сети");
  await expect(status).not.toHaveClass(/personal-peer-status-online/);
  state.mode = "error";
  await expect(status).toHaveText("Статус недоступен");
  await expect(status).not.toHaveClass(/personal-peer-status-online/);
  expect(state.unexpected).toEqual([]);
});

test("фоновая вкладка не опрашивается, возврат видимости обновляет заголовок и информацию, ошибочный DTO не даёт online", async ({
  page,
}) => {
  const state = await fixture(page);
  await page.goto("/personal/presence-direct");
  const headerStatus = page.locator(".personal-header").getByRole("status");
  await expect(headerStatus).toHaveText("Онлайн");
  await page
    .locator(".personal-header")
    .getByRole("button", { name: "Информация о пользователе: Борис" })
    .click();
  const status = page.getByRole("dialog").getByRole("status");
  await expect(status).toHaveText("В сети");
  await visibility(page, false);
  const hiddenReads = state.reads;
  state.mode = "offline";
  await page.waitForTimeout(1600);
  expect(state.reads).toBe(hiddenReads);
  await visibility(page, true);
  await expect(status).toHaveText("не в сети.");
  await expect(headerStatus).toHaveText("Не в сети");
  state.mode = "mismatch";
  await expect(status).toHaveText("Статус временно недоступен.");
  await expect(headerStatus).toHaveText("Статус недоступен");
  await expect(headerStatus).not.toHaveClass(/personal-peer-status-online/);
  await expect(status).not.toHaveClass(/personal-user-presence-online/);
  expect(state.unexpected).toEqual([]);
});

for (const denied of [403, 404] as const) {
  test(`отказ ${denied} закрывает информацию и диалог, затем прекращает опрос`, async ({
    page,
  }) => {
    const state = await fixture(page);
    await page.goto("/personal/presence-direct");
    await expect(
      page.locator(".personal-header").getByRole("status"),
    ).toHaveText("Онлайн");
    await page
      .locator(".personal-header")
      .getByRole("button", { name: "Информация о пользователе: Борис" })
      .click();
    await expect(
      page.getByRole("dialog", { name: "Информация о пользователе" }),
    ).toBeVisible();
    state.mode = denied;
    await expect(page).toHaveURL(/\/personal$/);
    await expect(
      page.getByRole("dialog", { name: "Информация о пользователе" }),
    ).toHaveCount(0);
    const deniedReads = state.reads;
    await page.waitForTimeout(1500);
    expect(state.reads).toBe(deniedReads);
    expect(state.unexpected).toEqual([]);
  });
}

for (const viewport of [
  { width: 1440, height: 1000 },
  { width: 834, height: 1112 },
  { width: 390, height: 844 },
]) {
  for (const theme of ["light", "dark"] as const) {
    for (const textSize of [100, 200]) {
      test(`шапка, слоган и копирайт: ${viewport.width}px/${theme}/${textSize}%`, async ({
        page,
      }, info) => {
        await page.setViewportSize(viewport);
        const state = await fixture(page);
        await page.addInitScript(
          ({ actor, theme, textSize }) => {
            localStorage.setItem(
              `go-recorder.appearance.v1:${encodeURIComponent(actor)}`,
              JSON.stringify({ version: 1, theme, textSize }),
            );
          },
          { actor: user.id, theme, textSize },
        );
        await page.goto("/personal/presence-direct");
        await expect(page.locator("html")).toHaveAttribute(
          "data-text-size",
          String(textSize),
        );
        await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
        const header = page.locator(".personal-header");
        await expect(header.getByRole("status")).toHaveText("Онлайн");
        await expect(header).not.toContainText("Личная переписка");
        await expect(page.locator(".personal-list-footer")).toHaveCount(0);
        await expect
          .poll(() =>
            page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth,
            ),
          )
          .toBe(true);
        const statusBox = (await header.getByRole("status").boundingBox())!;
        const headerBox = (await header.boundingBox())!;
        expect(statusBox.x).toBeGreaterThanOrEqual(headerBox.x);
        expect(statusBox.x + statusBox.width).toBeLessThanOrEqual(
          headerBox.x + headerBox.width + 1,
        );
        await page.screenshot({
          path: info.outputPath("personal-header-online.png"),
          fullPage: true,
        });
        state.mode = "offline";
        await expect(header.getByRole("status")).toHaveText("Не в сети");
        await page.goto("/personal");
        await expect(page.locator(".personal-page")).toBeVisible();
        await expect(page.locator(".personal-conversation-row")).toHaveCount(1);
        const footer = page.locator(".workspace-footer");
        await footer.scrollIntoViewIfNeeded();
        await expect(footer).toHaveText(
          "© 2026 Яницкий Александр. Все права защищены.",
        );
        await expect(
          footer.getByRole("link", { name: "Яницкий Александр" }),
        ).toHaveAttribute("href", "https://janickiy.com/");
        await expect(
          footer.getByRole("link", { name: "Яницкий Александр" }),
        ).toBeInViewport();
        await footer
          .getByRole("link", { name: "Яницкий Александр" })
          .click({ trial: true });
        await expect
          .poll(() =>
            page.evaluate(
              () => document.documentElement.scrollWidth <= innerWidth,
            ),
          )
          .toBe(true);
        if (viewport.width >= 1024) {
          const tagline = page.locator(".sidebar-tagline");
          await expect(tagline).toBeVisible();
          await expect(tagline).toHaveText(
            "Встречи, чаты и совместная работа в одном месте.",
          );
          const sidebarBox = (await page.locator(".sidebar").boundingBox())!;
          const taglineBox = (await tagline.boundingBox())!;
          expect(taglineBox.x + taglineBox.width).toBeLessThanOrEqual(
            sidebarBox.x + sidebarBox.width + 1,
          );
        }
        await page.screenshot({
          path: info.outputPath("personal-footer-and-brand.png"),
          fullPage: true,
        });
        expect(state.unexpected).toEqual([]);
      });
    }
  }
}
