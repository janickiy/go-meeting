import axe from "axe-core";
import { expect, test, type Locator, type Page } from "@playwright/test";
import type { NotificationPreferences, User } from "../src/types";

const createdAt = "2026-10-05T10:00:00Z";
const firstUser: User = {
  id: "account-modal-first",
  email: "account-modal@example.test",
  displayName: "Тестовый участник",
  createdAt,
  updatedAt: createdAt,
};
const secondUser: User = {
  ...firstUser,
  id: "account-modal-second",
  email: "another-account@example.test",
  displayName: "Другой участник",
};
const appearanceKey = (userId: string) =>
  `go-recorder.appearance.v1:${encodeURIComponent(userId)}`;

type FixtureState = {
  user: User;
  requests: string[];
  profileWrites: unknown[];
  notificationWrites: NotificationPreferences[];
  unexpected: string[];
};

/** Полностью подменяет API настроек и не позволяет неизвестным запросам попасть на сервер.
 * @args page — изолированная браузерная страница теста.
 * @return Состояние фикстуры с журналом запросов и текущей тестовой учётной записью.
 */
async function accountFixture(page: Page): Promise<FixtureState> {
  const state: FixtureState = {
    user: { ...firstUser },
    requests: [],
    profileWrites: [],
    notificationWrites: [],
    unexpected: [],
  };
  let preferences: NotificationPreferences = {
    invitation: true,
    reminder: true,
    recording: true,
    summary: true,
    email: false,
    push: false,
  };
  await page.addInitScript(() => {
    sessionStorage.setItem(
      "meet.session.v1",
      JSON.stringify({
        token: "isolated-account-settings-test",
        expiresAt: Date.now() + 1_800_000,
      }),
    );
    Object.defineProperty(navigator.mediaDevices, "enumerateDevices", {
      configurable: true,
      value: async () => [
        {
          kind: "audioinput",
          deviceId: "fixture-microphone",
          groupId: "fixture-audio",
          label: "Тестовый микрофон",
        },
        {
          kind: "videoinput",
          deviceId: "fixture-camera",
          groupId: "fixture-video",
          label: "Тестовая камера",
        },
      ],
    });
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: async () => {
        throw new DOMException(
          "Media is disabled in settings QA",
          "NotAllowedError",
        );
      },
    });
  });
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api\/v1/, "");
    const method = request.method();
    const entry = `${method} ${path}`;
    state.requests.push(entry);
    const reply = (body: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    if (path === "/auth/me" && method === "GET")
      return reply({ status: "success", user: state.user });
    if (path === "/auth/me" && method === "PATCH") {
      const body = request.postDataJSON() as { displayName: string };
      state.profileWrites.push(body);
      state.user = { ...state.user, displayName: body.displayName };
      return reply({ status: "success", user: state.user });
    }
    if (path === "/capabilities" && method === "GET")
      return reply({
        status: "success",
        buildVersion: "isolated-account-settings",
        capabilities: {
          liveCaptions: false,
          transcription: false,
          aiSummary: false,
          semanticSearch: false,
          meetingAnalytics: false,
          recordingModes: ["composite"],
        },
      });
    if (path === "/notifications/events" && method === "GET")
      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: ": isolated account-settings fixture\n\n",
      });
    if (path === "/notifications" && method === "GET")
      return reply({
        status: "success",
        items: [],
        nextCursor: null,
        unreadCount: 0,
      });
    if (path === "/me/conferences" && method === "GET")
      return reply({ status: "success", items: [], nextCursor: null });
    if (path === "/integrations/capabilities" && method === "GET")
      return reply({
        email: "noop",
        push: "noop",
        calendar: "noop",
        calendarOAuthConfigured: false,
        mockConnectAllowed: false,
      });
    if (path === "/notifications/preferences" && method === "GET")
      return reply(preferences);
    if (path === "/notifications/preferences" && method === "PUT") {
      preferences = request.postDataJSON() as NotificationPreferences;
      state.notificationWrites.push(preferences);
      return reply(preferences);
    }
    state.unexpected.push(entry);
    return reply(
      { message: "Unknown request blocked by isolated settings QA" },
      501,
    );
  });
  return state;
}

/** Возвращает единственный диалог настроек с его доступным заголовком.
 * @args page — браузерная страница теста.
 * @return Локатор модального окна настроек аккаунта.
 */
function settingsDialog(page: Page): Locator {
  return page.getByRole("dialog", { name: "Настройки аккаунта", exact: true });
}

/** Открывает вкладку по доступному имени и проверяет выбранное состояние.
 * @args dialog — окно настроек; name — название вкладки.
 * @return Завершение переключения вкладки.
 */
async function selectTab(dialog: Locator, name: string) {
  const tab = dialog.getByRole("tab", { name, exact: true });
  await tab.click();
  await expect(tab).toHaveAttribute("aria-selected", "true");
}

/** Проверяет видимую область диалога без горизонтального переполнения.
 * @args page — браузерная страница; dialog — окно настроек.
 * @return Завершение проверки геометрии страницы, окна и его панелей.
 */
async function assertFitsViewport(page: Page, dialog: Locator) {
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    )
    .toBe(true);
  const bounds = await dialog.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.y).toBeGreaterThanOrEqual(0);
  const viewport = page.viewportSize()!;
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width + 1);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height + 1);
  await expect
    .poll(() =>
      dialog.evaluate(
        (element) => element.scrollWidth <= element.clientWidth + 1,
      ),
    )
    .toBe(true);
}

/** Запускает проверки доступности, включая контраст, только для открытого окна.
 * @args page — браузерная страница с модальным окном.
 * @return Нарушения правил WCAG вместе с видимыми проблемными элементами.
 */
async function auditDialog(page: Page) {
  await page.evaluate(axe.source);
  return page.evaluate(async () => {
    const runtime = window as unknown as {
      axe: {
        run: (
          element: HTMLElement,
          options: unknown,
        ) => Promise<{
          violations: {
            id: string;
            nodes: {
              html: string;
              failureSummary?: string;
              any?: { data: unknown }[];
            }[];
          }[];
        }>;
      };
    };
    const dialog = document.querySelector<HTMLElement>(
      '[role="dialog"][aria-modal="true"]',
    );
    if (!dialog) throw new Error("Account-settings modal is not open");
    const result = await runtime.axe.run(dialog, {
      runOnly: {
        type: "tag",
        values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
      },
    });
    return result.violations.map(({ id, nodes }) => ({
      id,
      elements: nodes.map(({ html, failureSummary, any }) => ({
        html,
        failureSummary,
        details: any?.map(({ data }) => data),
      })),
    }));
  });
}

test("settings links open an account modal over the current route and return focus", async ({
  page,
}) => {
  const fixture = await accountFixture(page);
  await page.goto("/history?qa=account-modal");
  await expect(
    page.getByRole("heading", { name: "История встреч", exact: true }),
  ).toBeVisible();
  const originalURL = page.url();
  const sidebarLink = page
    .getByRole("navigation", { name: "Основная навигация" })
    .getByRole("link", { name: "Настройки", exact: true });
  await sidebarLink.click();
  const dialog = settingsDialog(page);
  await expect(dialog).toBeVisible();
  expect(page.url()).toBe(originalURL);
  await expect(
    dialog.getByRole("tab", { name: "Профиль", exact: true }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(
    dialog.getByRole("tab", { name: "Интеграции", exact: true }),
  ).toHaveCount(0);
  await expect(
    dialog.getByRole("tab", { name: "Безопасность", exact: true }),
  ).toHaveCount(0);
  await expect(page.locator("body")).toHaveCSS("overflow", "hidden");
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(sidebarLink).toBeFocused();
  expect(page.url()).toBe(originalURL);

  await page.getByLabel("Меню профиля", { exact: true }).click();
  const profileLink = page.getByRole("link", {
    name: "Настройки профиля",
    exact: true,
  });
  await profileLink.click();
  await expect(dialog).toBeVisible();
  expect(page.url()).toBe(originalURL);
  await page.locator(".modal-backdrop").click({ position: { x: 2, y: 2 } });
  await expect(dialog).toHaveCount(0);
  await expect(page.getByLabel("Меню профиля", { exact: true })).toBeFocused();
  expect(fixture.unexpected).toEqual([]);
});

test("mobile settings replaces the navigation drawer without losing the route or opener focus", async ({
  page,
}) => {
  const fixture = await accountFixture(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/history?qa=mobile-settings");
  const originalURL = page.url();
  const opener = page.getByRole("button", {
    name: "Открыть меню",
    exact: true,
  });
  await opener.click();
  const drawer = page.getByRole("dialog", { name: "Меню Meetrix" });
  await expect(drawer).toBeVisible();
  await drawer.getByRole("link", { name: "Настройки", exact: true }).click();
  const dialog = settingsDialog(page);
  await expect(dialog).toBeVisible();
  await expect(drawer).toHaveCount(0);
  expect(page.url()).toBe(originalURL);
  await assertFitsViewport(page, dialog);
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(opener).toBeFocused();
  await expect(opener).toHaveAttribute("aria-expanded", "false");
  await expect(page.locator("body")).not.toHaveCSS("overflow", "hidden");
  expect(page.url()).toBe(originalURL);
  expect(fixture.unexpected).toEqual([]);
});

for (const directPath of ["/app/settings", "/settings"]) {
  test(`direct ${directPath} shows a settings modal with a dashboard fallback`, async ({
    page,
  }) => {
    const fixture = await accountFixture(page);
    await page.goto(directPath);
    const dialog = settingsDialog(page);
    await expect(dialog).toBeVisible();
    await dialog
      .getByRole("button", { name: "Закрыть окно", exact: true })
      .click();
    await expect(dialog).toHaveCount(0);
    await expect(page).toHaveURL(/\/app$/);
    await expect(
      page.getByRole("heading", { name: /Добро пожаловать|Привет/ }),
    ).toBeVisible();
    expect(fixture.unexpected).toEqual([]);
  });
}

test("profile, device preferences, and notifications remain functional without integration or security tabs", async ({
  page,
}) => {
  const fixture = await accountFixture(page);
  await page.goto("/app/settings");
  const dialog = settingsDialog(page);
  await expect(dialog).toBeVisible();
  await dialog
    .getByRole("textbox", { name: "Имя для встреч" })
    .fill("  Обновлённый участник  ");
  await dialog
    .getByRole("button", { name: "Сохранить имя", exact: true })
    .click();
  await expect(dialog.getByRole("status")).toContainText("Имя сохранено.");
  expect(fixture.profileWrites).toEqual([
    { displayName: "Обновлённый участник" },
  ]);
  await expect(
    dialog.getByRole("textbox", { name: "Email", exact: true }),
  ).toHaveAttribute("readonly");
  await selectTab(dialog, "Аудио");
  await dialog
    .getByRole("combobox", { name: "Микрофон", exact: true })
    .selectOption("fixture-microphone");
  await dialog
    .getByRole("switch", {
      name: "Подключаться с выключенным микрофоном",
      exact: true,
    })
    .check();
  await selectTab(dialog, "Профиль");
  await selectTab(dialog, "Аудио");
  await expect(
    dialog.getByRole("combobox", { name: "Микрофон", exact: true }),
  ).toHaveValue("fixture-microphone");
  await expect(
    dialog.getByRole("switch", {
      name: "Подключаться с выключенным микрофоном",
      exact: true,
    }),
  ).toBeChecked();
  await selectTab(dialog, "Уведомления");
  await dialog
    .getByRole("checkbox", { name: "Напоминания о встречах", exact: true })
    .uncheck();
  await dialog
    .getByRole("button", { name: "Сохранить настройки", exact: true })
    .click();
  await expect(dialog.getByRole("status")).toContainText(
    "Настройки сохранены.",
  );
  expect(fixture.notificationWrites).toContainEqual(
    expect.objectContaining({ reminder: false }),
  );
  await expect(dialog.getByRole("checkbox", { name: /Email/ })).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: /Подключить.*календарь/ }),
  ).toHaveCount(0);
  expect(fixture.requests).not.toContain("GET /integrations/calendars");
  expect(fixture.unexpected).toEqual([]);
});

test("appearance contains only theme and percentage text-size controls, persists, and isolates accounts", async ({
  page,
}, info) => {
  const fixture = await accountFixture(page);
  await page.emulateMedia({ colorScheme: "dark" });
  await page.goto("/app/settings");
  const dialog = settingsDialog(page);
  await selectTab(dialog, "Оформление");
  await expect(
    dialog.getByRole("group", { name: "Тема", exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByRole("heading", { name: "Внешний вид", exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByRole("radio", { name: "Светлая тема", exact: true }),
  ).toBeChecked();
  await expect(dialog.getByRole("radio")).toHaveCount(2);
  await expect(dialog.getByRole("combobox")).toHaveCount(1);
  const size = dialog.getByRole("combobox", {
    name: "Размер текста",
    exact: true,
  });
  await expect(size).toHaveValue("100");
  expect(await size.locator("option").allTextContents()).toEqual([
    "75%",
    "90%",
    "100% (по умолчанию)",
    "110%",
    "125%",
    "150%",
    "200%",
  ]);
  const originalFont = await dialog
    .getByRole("heading", { name: "Внешний вид", exact: true })
    .evaluate((element) =>
      Number.parseFloat(getComputedStyle(element).fontSize),
    );
  await page.screenshot({
    path: info.outputPath("account-settings-desktop-light-100.png"),
  });
  await dialog.getByRole("radio", { name: "Тёмная тема", exact: true }).check();
  await size.selectOption("125");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect
    .poll(() =>
      dialog
        .getByRole("heading", { name: "Внешний вид", exact: true })
        .evaluate((element) =>
          Number.parseFloat(getComputedStyle(element).fontSize),
        ),
    )
    .toBeCloseTo(originalFont * 1.25, 1);
  const saved = await page.evaluate(
    (key) => JSON.parse(localStorage.getItem(key) || "null"),
    appearanceKey(firstUser.id),
  );
  expect(saved).toEqual({ version: 1, theme: "dark", textSize: 125 });
  await page.screenshot({
    path: info.outputPath("account-settings-desktop-dark-125.png"),
  });
  await page.reload();
  await selectTab(dialog, "Оформление");
  await expect(
    dialog.getByRole("radio", { name: "Тёмная тема", exact: true }),
  ).toBeChecked();
  await expect(size).toHaveValue("125");
  fixture.user = { ...secondUser };
  await page.reload();
  await selectTab(dialog, "Оформление");
  await expect(
    dialog.getByRole("radio", { name: "Светлая тема", exact: true }),
  ).toBeChecked();
  await expect(size).toHaveValue("100");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  fixture.user = { ...firstUser };
  await page.reload();
  await selectTab(dialog, "Оформление");
  await expect(
    dialog.getByRole("radio", { name: "Тёмная тема", exact: true }),
  ).toBeChecked();
  await expect(size).toHaveValue("125");
  expect(fixture.unexpected).toEqual([]);
});

test("malformed appearance preferences safely restore light theme and 100% text", async ({
  page,
}) => {
  const fixture = await accountFixture(page);
  await page.goto("/app/settings");
  for (const stored of [
    "not-json",
    JSON.stringify({ version: 999, theme: "dark", textSize: 200 }),
    JSON.stringify({ version: 1, theme: "unsupported", textSize: 999 }),
  ]) {
    await page.evaluate(
      ({ key, stored }) => localStorage.setItem(key, stored),
      { key: appearanceKey(firstUser.id), stored },
    );
    await page.reload();
    const dialog = settingsDialog(page);
    await selectTab(dialog, "Оформление");
    await expect(
      dialog.getByRole("radio", { name: "Светлая тема", exact: true }),
    ).toBeChecked();
    await expect(
      dialog.getByRole("combobox", { name: "Размер текста", exact: true }),
    ).toHaveValue("100");
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  }
  expect(fixture.unexpected).toEqual([]);
});

test("keyboard focus includes the text-size dropdown and stays inside the account dialog", async ({
  page,
}) => {
  const fixture = await accountFixture(page);
  await page.goto("/app/settings");
  const dialog = settingsDialog(page);
  await selectTab(dialog, "Оформление");
  const close = dialog.getByRole("button", {
    name: "Закрыть окно",
    exact: true,
  });
  const size = dialog.getByRole("combobox", {
    name: "Размер текста",
    exact: true,
  });
  await size.focus();
  await page.keyboard.press("Tab");
  await expect(close).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(size).toBeFocused();
  expect(fixture.unexpected).toEqual([]);
});

test("light and dark account settings are legible and fit a mobile viewport at 200% text", async ({
  page,
}, info) => {
  test.setTimeout(90_000);
  const fixture = await accountFixture(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/app/settings");
  const dialog = settingsDialog(page);
  await selectTab(dialog, "Оформление");
  await dialog
    .getByRole("combobox", { name: "Размер текста", exact: true })
    .selectOption("200");
  const auditFailures: unknown[] = [];
  for (const [theme, label] of [
    ["light", "Светлая тема"],
    ["dark", "Тёмная тема"],
  ] as const) {
    await dialog.getByRole("radio", { name: label, exact: true }).check();
    await assertFitsViewport(page, dialog);
    for (const tabName of [
      "Профиль",
      "Аудио",
      "Видео",
      "Уведомления",
      "Оформление",
    ]) {
      await selectTab(dialog, tabName);
      await assertFitsViewport(page, dialog);
      const panel = dialog.getByRole("tabpanel", {
        name: tabName,
        exact: true,
      });
      await panel.evaluate((element) => {
        element.scrollTop = 0;
      });
      const topViolations = await auditDialog(page);
      if (topViolations.length)
        auditFailures.push({
          theme,
          tabName,
          position: "top",
          violations: topViolations,
        });
      await panel.evaluate((element) => {
        element.scrollTop = element.scrollHeight;
      });
      const bottomViolations = await auditDialog(page);
      if (bottomViolations.length)
        auditFailures.push({
          theme,
          tabName,
          position: "bottom",
          violations: bottomViolations,
        });
      await panel.evaluate((element) => {
        element.scrollTop = 0;
      });
      await page.screenshot({
        path: info.outputPath(
          `account-settings-mobile-${theme}-200-${tabName}.png`,
        ),
        fullPage: false,
      });
    }
    await page.screenshot({
      path: info.outputPath(`account-settings-mobile-${theme}-200.png`),
      fullPage: false,
    });
  }
  expect(auditFailures, JSON.stringify(auditFailures)).toEqual([]);
  expect(fixture.unexpected).toEqual([]);
});

async function syntheticDevices(page: Page) {
  await page.addInitScript(() => {
    const qa = {
      captures: [] as MediaStreamConstraints[],
      stops: 0,
      sinks: [] as string[],
    };
    (window as unknown as { deviceQA: typeof qa }).deviceQA = qa;
    Object.defineProperty(navigator.mediaDevices, "enumerateDevices", {
      configurable: true,
      value: async () => [
        {
          kind: "audioinput",
          deviceId: "fixture-microphone",
          label: "Микрофон MacBook Pro (Built-in)",
        },
        {
          kind: "audiooutput",
          deviceId: "fixture-speaker",
          label: "Динамики MacBook Pro (Built-in)",
        },
        {
          kind: "audiooutput",
          deviceId: "fixture-headphones",
          label: "Наушники USB",
        },
        {
          kind: "videoinput",
          deviceId: "fixture-camera",
          label: "Камера MacBook Pro",
        },
      ],
    });
    Object.defineProperty(HTMLMediaElement.prototype, "setSinkId", {
      configurable: true,
      value: async (id: string) => {
        qa.sinks.push(id);
      },
    });
    Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
      configurable: true,
      value: async (constraints: MediaStreamConstraints) => {
        qa.captures.push(constraints);
        let stream: MediaStream;
        let cleanup = () => {};
        if (constraints.video) {
          const canvas = document.createElement("canvas");
          canvas.width = 960;
          canvas.height = 540;
          const context = canvas.getContext("2d")!;
          context.fillStyle = "#223857";
          context.fillRect(0, 0, 960, 540);
          context.fillStyle = "#a7c7ef";
          context.font = "32px sans-serif";
          context.textAlign = "center";
          context.fillText("Предпросмотр камеры", 480, 275);
          stream = canvas.captureStream(5);
        } else {
          const context = new AudioContext();
          const source = context.createOscillator();
          const gain = context.createGain();
          gain.gain.value = 0.15;
          const output = context.createMediaStreamDestination();
          source.connect(gain);
          gain.connect(output);
          source.start();
          void context.resume();
          stream = output.stream;
          cleanup = () => {
            source.stop();
            void context.close();
          };
        }
        for (const track of stream.getTracks()) {
          const stop = track.stop.bind(track);
          track.stop = () => {
            if (track.readyState !== "ended") {
              qa.stops++;
              cleanup();
            }
            stop();
          };
        }
        return stream;
      },
    });
  });
}

test("audio level, device outputs and noise suppression work", async ({
  page,
}, info) => {
  const fixture = await accountFixture(page);
  await syntheticDevices(page);
  await page.goto("/app/settings");
  const dialog = settingsDialog(page);
  await selectTab(dialog, "Аудио");
  await dialog
    .getByRole("button", { name: "Проверить микрофон", exact: true })
    .click();
  await expect
    .poll(async () =>
      Number(
        await dialog
          .getByRole("meter", { name: "Уровень микрофона" })
          .getAttribute("aria-valuenow"),
      ),
    )
    .toBeGreaterThan(0);
  await dialog
    .getByRole("combobox", { name: "Микрофон", exact: true })
    .selectOption("fixture-microphone");
  await dialog
    .getByRole("switch", { name: "Шумоподавление", exact: true })
    .uncheck();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (
            window as unknown as {
              deviceQA: { captures: MediaStreamConstraints[] };
            }
          ).deviceQA.captures.at(-1)?.audio,
      ),
    )
    .toMatchObject({
      noiseSuppression: false,
      deviceId: { exact: "fixture-microphone" },
    });
  await dialog
    .getByRole("combobox", { name: "Динамик", exact: true })
    .selectOption("fixture-headphones");
  await dialog
    .getByRole("combobox", { name: "Источник звука уведомлений", exact: true })
    .selectOption("fixture-speaker");
  await dialog
    .getByRole("button", { name: "Проверить: динамик", exact: true })
    .click();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as unknown as { deviceQA: { sinks: string[] } }).deviceQA
            .sinks,
      ),
    )
    .toContain("fixture-headphones");
  await expect(
    dialog.getByRole("button", { name: "Проверить: динамик", exact: true }),
  ).toBeVisible();
  await dialog
    .getByRole("button", { name: "Проверить: звук приглашения", exact: true })
    .click();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as unknown as { deviceQA: { sinks: string[] } }).deviceQA
            .sinks,
      ),
    )
    .toContain("fixture-speaker");
  await page.screenshot({
    path: info.outputPath("audio-settings-desktop.png"),
  });
  await dialog
    .getByRole("button", { name: "Остановить проверку микрофона" })
    .click();
  await expect(
    dialog.getByRole("meter", { name: "Уровень микрофона" }),
  ).toHaveAttribute("aria-valuenow", "0");
  await selectTab(dialog, "Профиль");
  await selectTab(dialog, "Аудио");
  await expect(
    dialog.getByRole("switch", { name: "Шумоподавление", exact: true }),
  ).not.toBeChecked();
  expect(fixture.unexpected).toEqual([]);
});

test("camera preview is released, video switches persist, both panels fit mobile", async ({
  page,
}, info) => {
  const fixture = await accountFixture(page);
  await syntheticDevices(page);
  await page.goto("/app/settings");
  const dialog = settingsDialog(page);
  await selectTab(dialog, "Видео");
  await expect
    .poll(() =>
      dialog
        .getByLabel("Предпросмотр камеры", { exact: true })
        .evaluate((element) => (element as HTMLVideoElement).videoWidth),
    )
    .toBe(960);
  await dialog
    .getByRole("switch", {
      name: "Подключаться с выключенной камерой",
      exact: true,
    })
    .check();
  await dialog.getByRole("switch", { name: /Видеть себя на звонке/ }).uncheck();
  await dialog.getByRole("switch", { name: /Скрыть видео участников/ }).check();
  await page.screenshot({
    path: info.outputPath("video-settings-desktop.png"),
  });
  await selectTab(dialog, "Аудио");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (
            window as unknown as {
              deviceQA: { stops: number; captures: unknown[] };
            }
          ).deviceQA.stops ===
          (window as unknown as { deviceQA: { captures: unknown[] } }).deviceQA
            .captures.length,
      ),
    )
    .toBe(true);
  await selectTab(dialog, "Видео");
  await expect(
    dialog.getByRole("switch", { name: /Видеть себя на звонке/ }),
  ).not.toBeChecked();
  await page.setViewportSize({ width: 390, height: 844 });
  await assertFitsViewport(page, dialog);
  await page.screenshot({ path: info.outputPath("video-settings-mobile.png") });
  await selectTab(dialog, "Аудио");
  await assertFitsViewport(page, dialog);
  await page.screenshot({ path: info.outputPath("audio-settings-mobile.png") });
  expect(fixture.unexpected).toEqual([]);
});
