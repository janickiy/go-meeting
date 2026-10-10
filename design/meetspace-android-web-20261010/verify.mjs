import { createRequire } from "node:module";
import { mkdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";

// Run from any directory: node design/meetspace-android-web-20261010/verify.mjs
// Start the static server separately: python3 design/meetspace-android-web-20261010/serve.py
// The test uses only local prototype pages and demonstration fixture data.
const require = createRequire(
  new URL("../../frontend/package.json", import.meta.url),
);
const { chromium } = require("playwright");
const base = process.env.MEETSPACE_QA_URL || "http://127.0.0.1:8767";
const outDir = fileURLToPath(new URL(".", import.meta.url));
const screens = [
  "meetings",
  "home",
  "messages",
  "thread",
  "calendar",
  "recordings",
  "recording",
  "prejoin",
  "call",
  "settings",
  "welcome",
  "connect",
  "login",
  "deploy",
  "fingerprint",
  "preflight",
  "install",
  "complete",
  "install-error",
];
const viewports = [
  { width: 360, height: 800 },
  { width: 390, height: 844 },
  { width: 834, height: 1112 },
  { width: 1194, height: 834 },
  { width: 320, height: 700 },
  { width: 600, height: 900 },
];
const themes = ["light", "dark"];
const report = {
  generatedAt: new Date().toISOString(),
  base,
  matrix: [],
  flows: [],
  screenshots: [],
  issues: [],
};
const browser = await chromium.launch({ headless: true });
await mkdir(new URL("previews/", import.meta.url), { recursive: true });
const url = (screen, theme = "light") =>
  `${base}/preview.html?screen=${screen}&theme=${theme}`;
const ready = async (page) => {
  await page.locator("#app > *").waitFor();
  await page.evaluate(() => document.fonts.ready);
};
const stateIs = async (page, screen) =>
  assert.equal(await page.locator("body").getAttribute("data-screen"), screen);

async function matrix(viewport) {
  const page = await browser.newPage({ viewport, locale: "ru-RU" });
  let errors = [],
    missing = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("response", (response) => {
    if (response.status() >= 400)
      missing.push({ url: response.url(), status: response.status() });
  });
  page.on("requestfailed", (request) =>
    missing.push({ url: request.url(), failure: request.failure()?.errorText }),
  );
  for (const theme of themes)
    for (const screen of screens) {
      errors = [];
      missing = [];
      await page.goto(url(screen, theme), { waitUntil: "load" });
      await ready(page);
      const measure = await page.evaluate(() => ({
        screen: document.body.dataset.screen,
        theme: document.documentElement.dataset.theme,
        width: innerWidth,
        bodyWidth: document.body.scrollWidth,
        documentWidth: document.documentElement.scrollWidth,
        horizontalOverflow:
          Math.max(
            document.body.scrollWidth,
            document.documentElement.scrollWidth,
          ) >
          innerWidth + 1,
        brokenImages: [...document.images]
          .filter((img) => !img.complete || !img.naturalWidth)
          .map((img) => img.getAttribute("src")),
        empty: document.querySelector("#app").innerText.trim().length < 20,
      }));
      const entry = {
        screen,
        theme,
        viewport,
        ...measure,
        errors: [...errors],
        missing: [...missing],
      };
      entry.pass =
        !entry.horizontalOverflow &&
        !entry.empty &&
        !errors.length &&
        !missing.length &&
        !measure.brokenImages.length &&
        measure.screen === screen &&
        measure.theme === theme;
      report.matrix.push(entry);
      if (!entry.pass) {
        report.issues.push(entry);
        console.error("MATRIX FAIL", JSON.stringify(entry));
      }
    }
  await page.close();
}

async function flow(name, viewport, steps) {
  const page = await browser.newPage({ viewport, locale: "ru-RU" });
  page.setDefaultTimeout(10000);
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  try {
    await steps(page);
    assert.deepEqual(errors, []);
    report.flows.push({ name, viewport, pass: true });
    console.log(`Flow passed: ${name} ${viewport.width}`);
  } catch (error) {
    const issue = { name, viewport, pass: false, error: error.message, errors };
    report.flows.push(issue);
    report.issues.push(issue);
    console.error("FLOW FAIL", name, error.message);
    await page.screenshot({
      path: `${outDir}previews/failure-${name}-${viewport.width}.png`,
      fullPage: true,
    });
  }
  await page.close();
}

try {
  for (let offset = 0; offset < viewports.length; offset += 2)
    await Promise.all(viewports.slice(offset, offset + 2).map(matrix));
  console.log(
    `Matrix: ${report.matrix.filter((x) => x.pass).length}/${report.matrix.length} passed`,
  );
  for (const viewport of [viewports[1], viewports[3]]) {
    await flow("meetings-filters", viewport, async (page) => {
      await page.goto(url("meetings"));
      await ready(page);
      assert.equal(await page.locator(".meeting-entry").count(), 3);
      await page.getByLabel("Найти встречу", { exact: true }).fill("Дизайн");
      assert.equal(await page.locator(".meeting-entry").count(), 1);
      await page
        .getByLabel("Найти встречу", { exact: true })
        .fill("Нет такой встречи");
      assert(await page.locator(".meetings-empty").isVisible());
      await page.locator("#reset-meeting-filters").click();
      assert.equal(await page.locator(".meeting-entry").count(), 3);
      await page.locator('[data-meeting-tab="active"]').click();
      assert.equal(await page.locator(".meeting-entry").count(), 1);
      await page.locator('[data-meeting-tab="past"]').click();
      assert.equal(await page.locator(".meeting-entry").count(), 2);
      await page.locator('[data-meeting-open="review"]').click();
      await stateIs(page, "recording");
      await page.goto(url("meetings"));
      await ready(page);
      if (viewport.width < 600) {
        await page.locator("#open-meeting-filters").click();
        await page.locator("#meeting-filter-form select").selectOption("mine");
        await page
          .locator(
            '#meeting-filter-form button[type="submit"],#meeting-filter-form button:not([type])',
          )
          .first()
          .click();
      } else
        await page.locator(".meetings-filters select").selectOption("mine");
      assert.equal(await page.locator(".meeting-entry").count(), 2);
      await page.locator('[data-meeting-open="weekly"]').click();
      await stateIs(page, "prejoin");
      assert(
        (await page.locator(".prejoin-info").innerText()).includes(
          "13 октября",
        ),
      );
      if (viewport.width < 600) {
        await page.goto(url("meetings"));
        await ready(page);
        await page.locator('.bottom-nav [data-action="more"]').click();
        await page
          .getByRole("button", { name: "Записи встреч", exact: true })
          .click();
        await stateIs(page, "recordings");
      }
    });
    await flow("home-actions-and-chats", viewport, async (page) => {
      await page.goto(url("home"));
      await ready(page);
      assert.equal(await page.locator(".home-action-card").count(), 4);
      await page.locator("#home-chat-search").fill("Анна");
      assert.equal(await page.locator(".home-chat-row:visible").count(), 1);
      await page.locator('[data-home-filter="personal"]').click();
      assert.equal(await page.locator(".home-chat-row:visible").count(), 1);
      await page.locator("#home-chat-search").fill("Несуществующий чат");
      assert(await page.locator("#home-chat-empty").isVisible());
      await page.locator("#home-chat-search").fill("");
      await page.locator('[data-home-filter="unread"]').click();
      assert.equal(await page.locator(".home-chat-row:visible").count(), 2);
      await page.locator('.home-chat-row[data-chat="anna"]').click();
      await stateIs(page, "thread");
      assert(
        (await page.locator(".thread-header").innerText()).includes(
          "Анна Кузнецова",
        ),
      );
      await page.goto(url("home"));
      await page.locator('.home-action-card[data-action="newChat"]').click();
      await page.locator('#dialog-root [data-chat="max"]').click();
      await stateIs(page, "thread");
      await page.goto(url("home"));
      await page.locator('.home-action-card[data-action="join"]').click();
      await page
        .locator("#join-form input")
        .fill("http://meet.example.org/room/demo");
      await page.locator("#join-form button").click();
      assert(await page.getByRole("dialog").isVisible());
      await page
        .locator("#join-form input")
        .fill("https://meet.example.org/room/demo");
      await page.locator("#join-form button").click();
      await stateIs(page, "prejoin");
      await page.goto(url("home"));
      await page.locator('.home-action-card[data-action="schedule"]').click();
      await page
        .locator('#meeting-form input[name="title"]')
        .fill("Новая главная — обсуждение");
      await page.locator("#meeting-form button").click();
      await stateIs(page, "home");
      await page.evaluate(() => MS.go("calendar"));
      assert(
        (await page.locator(".timeline").innerText()).includes(
          "Новая главная — обсуждение",
        ),
      );
      await page.goto(url("home"));
      await page
        .context()
        .grantPermissions(["clipboard-read", "clipboard-write"]);
      await page.locator('[data-action="copyHomeInvite"]').click();
      await page
        .getByRole("status")
        .getByText("Демо-ссылка на встречу скопирована")
        .waitFor();
      assert.equal(
        await page.evaluate(() => navigator.clipboard.readText()),
        "https://meet.example.org/room/design",
      );
      await page.evaluate(() => {
        Object.defineProperty(navigator, "clipboard", {
          configurable: true,
          value: {
            writeText: async () => {
              throw new Error("Clipboard disabled for fallback test");
            },
          },
        });
      });
      await page.locator('[data-action="copyHomeInvite"]').click();
      assert.equal(
        await page.locator("#home-invite-link").inputValue(),
        "https://meet.example.org/room/design",
      );
      await page.keyboard.press("Escape");
      assert.equal(await page.getByRole("dialog").count(), 0);
    });
    await flow("meeting", viewport, async (page) => {
      await page.goto(url("home"));
      await ready(page);
      await page.locator('.home-action-card[data-action="newMeeting"]').click();
      await page
        .locator('#meeting-form input[name="title"]')
        .fill("Обсуждение главной");
      await page
        .getByRole("button", { name: "Создать и подключиться", exact: true })
        .click();
      await stateIs(page, "prejoin");
      await page.getByLabel("Выключить микрофон", { exact: true }).click();
      assert.equal(
        await page
          .getByLabel("Включить микрофон", { exact: true })
          .getAttribute("aria-pressed"),
        "false",
      );
      await page
        .getByRole("button", { name: "Присоединиться", exact: false })
        .click();
      await stateIs(page, "call");
      await page.getByRole("button", { name: "Выйти", exact: true }).click();
      assert(await page.getByRole("dialog").isVisible());
      await page.getByRole("button", { name: "Остаться", exact: true }).click();
      await stateIs(page, "call");
      await page.getByRole("button", { name: "Выйти", exact: true }).click();
      await page
        .getByRole("button", { name: "Выйти из встречи", exact: true })
        .click();
      await stateIs(page, "home");
    });
    await flow("messages", viewport, async (page) => {
      await page.goto(url("messages"));
      await ready(page);
      await page.getByLabel("Поиск чатов").fill("Анна");
      assert.equal(await page.locator(".conversation:visible").count(), 1);
      await page.getByLabel("Поиск чатов").fill("Несуществующий чат");
      assert(await page.locator("#chat-empty").isVisible());
      await page.getByLabel("Поиск чатов").fill("Анна");
      await page.locator(".conversation:visible").click();
      await stateIs(page, "thread");
      await page
        .getByLabel("Сообщение", { exact: true })
        .fill("Проверка макета — демо");
      await page
        .getByRole("button", { name: "Отправить сообщение", exact: true })
        .click();
      assert(
        await page
          .locator("#thread-log")
          .getByText("Проверка макета — демо", { exact: false })
          .isVisible(),
      );
      assert.equal(
        await page.getByLabel("Сообщение", { exact: true }).inputValue(),
        "",
      );
    });
    await flow("calendar", viewport, async (page) => {
      await page.goto(url("calendar"));
      await ready(page);
      await page.locator('[data-day="13"]').click();
      assert.equal(
        await page.locator('[data-day="13"]').getAttribute("aria-pressed"),
        "true",
      );
      assert.equal(await page.locator(".event").count(), 0);
      await page.getByRole("button", { name: "Сегодня", exact: true }).click();
      assert.equal(await page.locator(".event").count(), 3);
      await page.locator(".event").first().click();
      assert(await page.getByRole("dialog").isVisible());
      await page.getByRole("button", { name: "Закрыть", exact: true }).click();
    });
    await flow("recordings", viewport, async (page) => {
      await page.goto(url("recordings"));
      await ready(page);
      const total = await page.locator(".recording-card:visible").count();
      await page.locator('[data-record-filter="team"]').click();
      assert((await page.locator(".recording-card:visible").count()) < total);
      assert((await page.locator(".recording-card:visible").count()) > 0);
      await page.getByLabel("Найти запись").fill("Несуществующая запись");
      assert(await page.locator("#record-empty").isVisible());
      await page.getByLabel("Найти запись").fill("");
      await page.locator(".recording-card:visible").first().click();
      await stateIs(page, "recording");
      await page
        .getByRole("button", {
          name: "Запустить демо проигрывателя",
          exact: true,
        })
        .click();
      assert(await page.locator("#toast").isVisible());
    });
    await flow("connect-login", viewport, async (page) => {
      await page.goto(url("welcome"));
      await ready(page);
      await page.locator('[data-go="connect"]').click();
      await stateIs(page, "connect");
      await page
        .getByLabel("Адрес сервера", { exact: true })
        .fill("http://invalid.example");
      await page
        .getByRole("button", { name: "Проверить адрес", exact: false })
        .click();
      assert(await page.locator(".on-form-error").isVisible());
      await page
        .getByLabel("Адрес сервера", { exact: true })
        .fill("https://meet.example.org");
      await page
        .getByRole("button", { name: "Проверить адрес", exact: false })
        .click();
      assert(await page.locator("#on-connect-result").isVisible());
      await page
        .getByRole("button", { name: "Перейти ко входу", exact: false })
        .click();
      await stateIs(page, "login");
      await page.locator('input[name="password"]').fill("demo");
      await page
        .getByRole("button", { name: "Войти в MeetSpace", exact: false })
        .click();
      await stateIs(page, "home");
    });
    await flow("installer", viewport, async (page) => {
      await page.goto(url("welcome"));
      await ready(page);
      await page.locator('[data-go="deploy"]').click();
      await stateIs(page, "deploy");
      await page.locator('input[name="password"]').fill("demo");
      await page
        .getByRole("button", { name: "Продолжить", exact: false })
        .click();
      await stateIs(page, "fingerprint");
      assert(await page.locator("#on-trust-next").isDisabled());
      await page.locator("#on-trust").check();
      await page.locator("#on-trust-next").click();
      await stateIs(page, "preflight");
      assert(await page.locator("#on-install-start").isDisabled());
      await page.locator("#on-consent").check();
      await page.locator("#on-install-start").click();
      await stateIs(page, "install");
      await page.locator("#on-stage-next").click();
      await page.locator("#on-simulate-error").click();
      await stateIs(page, "install-error");
      await page.locator("#on-retry").click();
      await stateIs(page, "install");
      assert(
        (await page.locator(".on-install-percent").innerText()).includes("30"),
      );
      for (let i = 0; i < 4; i++) await page.locator("#on-stage-next").click();
      await stateIs(page, "complete");
      assert(
        (await page.locator("body").innerText()).includes(
          "Сервер не развёрнут",
        ),
      );
    });
  }
  await flow("gallery-sync", { width: 1440, height: 1100 }, async (page) => {
    await page.goto(`${base}/index.html?screen=home`);
    await page.frameLocator("#phone-preview").locator("#app > *").waitFor();
    await page.getByLabel("Выбрать экран макета").selectOption("prejoin");
    await page
      .frameLocator("#phone-preview")
      .locator('[data-action="enterCall"]')
      .click();
    await page.waitForFunction(
      () => document.querySelector("#screen-select").value === "call",
    );
    assert.equal(
      await page
        .frameLocator("#tablet-preview")
        .locator("body")
        .getAttribute("data-screen"),
      "call",
    );
    await page.getByLabel("Включить тёмную тему", { exact: true }).click();
    await page.waitForFunction(() =>
      [...document.querySelectorAll("iframe")].every(
        (frame) =>
          frame.contentDocument.documentElement.dataset.theme === "dark",
      ),
    );
    assert.equal(
      await page
        .frameLocator("#phone-preview")
        .locator("html")
        .getAttribute("data-theme"),
      "dark",
    );
    assert.equal(
      await page
        .frameLocator("#tablet-preview")
        .locator("html")
        .getAttribute("data-theme"),
      "dark",
    );
    await page.getByLabel("Выбрать экран макета").selectOption("settings");
    await page
      .frameLocator("#phone-preview")
      .locator('[data-settings-tab="appearance"]')
      .click();
    await page
      .frameLocator("#phone-preview")
      .getByLabel("Тёмная тема", { exact: true })
      .uncheck();
    await page.waitForFunction(
      () =>
        document.body.dataset.theme === "light" &&
        [...document.querySelectorAll("iframe")].every(
          (frame) =>
            frame.contentDocument.documentElement.dataset.theme === "light",
        ),
    );
    await page.getByLabel("Повернуть планшет").click();
    assert.equal(
      await page.locator("#tablet-preview").getAttribute("width"),
      "834",
    );
    assert.equal(
      await page.locator("#tablet-preview").getAttribute("height"),
      "1194",
    );
  });
  const shots = [
    { name: "meetings-phone", screen: "meetings", viewport: viewports[1] },
    { name: "meetings-tablet", screen: "meetings", viewport: viewports[3] },
    { name: "home-phone", screen: "home", viewport: viewports[1] },
    { name: "home-tablet", screen: "home", viewport: viewports[3] },
    { name: "home-portrait", screen: "home", viewport: viewports[2] },
    {
      name: "home-phone-dark",
      screen: "home",
      viewport: viewports[1],
      theme: "dark",
    },
    { name: "call-tablet", screen: "call", viewport: viewports[3] },
    { name: "welcome-phone", screen: "welcome", viewport: viewports[1] },
    { name: "messages-tablet", screen: "messages", viewport: viewports[3] },
    {
      name: "gallery",
      url: `${base}/index.html?screen=home&theme=light&view=compare`,
      viewport: { width: 1600, height: 1200 },
    },
  ];
  for (const shot of shots) {
    const page = await browser.newPage({
      viewport: shot.viewport,
      deviceScaleFactor: 2,
      locale: "ru-RU",
    });
    await page.goto(shot.url || url(shot.screen, shot.theme), {
      waitUntil: "load",
    });
    if (shot.screen) await ready(page);
    else await page.evaluate(() => document.fonts.ready);
    await page.screenshot({
      path: `${outDir}previews/${shot.name}.png`,
      fullPage: false,
    });
    report.screenshots.push(`previews/${shot.name}.png`);
    await page.close();
  }
} catch (error) {
  report.issues.push({ fatal: true, error: error.message });
  console.error("QA RUN FAILED", error.message);
} finally {
  report.summary = {
    matrixPass: report.matrix.filter((x) => x.pass).length,
    matrixTotal: report.matrix.length,
    flowPass: report.flows.filter((x) => x.pass).length,
    flowTotal: report.flows.length,
    issueCount: report.issues.length,
  };
  await writeFile(
    `${outDir}qa-report.json`,
    JSON.stringify(report, null, 2) + "\n",
  );
  await browser.close();
  console.log(JSON.stringify(report.summary));
  if (report.issues.length) process.exitCode = 1;
}
