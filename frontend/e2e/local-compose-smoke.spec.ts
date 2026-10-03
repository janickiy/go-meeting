import { randomUUID } from "node:crypto";
import { mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { expect, test } from "@playwright/test";

const ui = process.env.MEET_STAGE9_DOCKER_UI || "http://127.0.0.1:5173";
const api = process.env.MEET_STAGE9_DOCKER_API || "http://127.0.0.1:8085";
const localOnly = [ui, api].every((value) => {
  try {
    return ["127.0.0.1", "localhost"].includes(new URL(value).hostname);
  } catch {
    return false;
  }
});

test.skip(
  process.env.MEET_STAGE9_DOCKER_SMOKE !== "1" || !localOnly,
  "Requires explicit local Compose opt-in on loopback addresses.",
);

test("local Compose: prejoin gates entry, history opens, notifications load and admin denies", async ({
  page,
  request,
}, info) => {
  const email = `stage9-${randomUUID().slice(0, 12)}-${info.project.name}@example.test`;
  const title = `Stage9 smoke ${randomUUID().slice(0, 8)}`;
  let conferenceId = "";
  let token = "";
  let finished = false;
  const visual = resolve(import.meta.dirname, "../test-results/stage9-visual");
  await mkdir(visual, { recursive: true });

  try {
    const document = await page.goto(`${ui}/register`);
    expect(document?.headers()["content-security-policy"]).toContain(
      "default-src 'self'",
    );
    await page.getByLabel("Email", { exact: true }).fill(email);
    await page
      .getByLabel("Пароль", { exact: true })
      .fill("stage9-smoke-password");
    await page.getByLabel(/Как к вам обращаться/).fill("Stage9 Smoke");
    await page
      .getByRole("button", { name: "Зарегистрироваться", exact: true })
      .click();
    await expect(
      page.getByRole("heading", { name: "Аккаунт создан!" }),
    ).toBeVisible();
    await page.getByRole("link", { name: "Перейти в приложение" }).click();
    token = await page.evaluate(() => {
      const session = sessionStorage.getItem("meet.session.v1");
      return session ? JSON.parse(session).token : "";
    });
    expect(token).toBeTruthy();
    const capabilities = await request.get(`${api}/api/v1/capabilities`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(capabilities.ok()).toBe(true);
    const { buildVersion } = await capabilities.json();
    if (process.env.MEET_STAGE9_EXPECTED_BUILD_VERSION)
      expect(buildVersion).toBe(process.env.MEET_STAGE9_EXPECTED_BUILD_VERSION);
    else expect(buildVersion).toMatch(/^stage9-local-/);
    console.log(`Stage9 API buildVersion: ${buildVersion}`);

    await page.getByRole("link", { name: "Новая конференция" }).click();
    await page.getByLabel("Название конференции").fill(title);
    await page
      .getByRole("button", { name: "Создать конференцию", exact: true })
      .click();
    await page.getByRole("link", { name: "Перейти в конференцию" }).click();
    conferenceId = new URL(page.url()).pathname.split("/").pop() || "";
    expect(conferenceId).toBeTruthy();

    let joins = 0;
    page.on("request", (sent) => {
      if (
        sent.method() === "POST" &&
        new URL(sent.url()).pathname ===
          `/api/v1/conferences/${conferenceId}/join`
      )
        joins += 1;
    });
    await page
      .getByRole("button", { name: "Присоединиться", exact: true })
      .click();
    await expect(page).toHaveURL(
      new RegExp(`/conferences/${conferenceId}/join$`),
    );
    await expect(page.getByText("Камера выключена")).toBeVisible();
    expect(joins).toBe(0);
    await page.screenshot({
      path: resolve(visual, `${info.project.name}-prejoin-desktop.png`),
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(
      page.getByRole("button", { name: "Войти во встречу" }),
    ).toBeVisible();
    await page.screenshot({
      path: resolve(visual, `${info.project.name}-prejoin-mobile.png`),
      fullPage: true,
    });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole("button", { name: "Войти во встречу" }).click();
    await expect(page).toHaveURL(new RegExp(`/conferences/${conferenceId}$`));
    await expect(
      page.getByRole("button", { name: "Покинуть конференцию" }),
    ).toBeVisible();
    expect(joins).toBe(1);

    await page.getByRole("button", { name: "Начать конференцию" }).click();
    await expect(
      page.getByRole("region", { name: "Активная встреча" }),
    ).toBeVisible();
    await page.screenshot({
      path: resolve(visual, `${info.project.name}-active-meeting.png`),
      fullPage: true,
      mask: [
        page.getByLabel("Ссылка-приглашение"),
        page.locator(".invite-code code"),
      ],
    });
    await page
      .getByRole("button", { name: "Завершить конференцию", exact: true })
      .click();
    await page.getByRole("button", { name: "Да, завершить" }).click();
    await expect(
      page.getByRole("heading", { name: "Встреча закрыта" }),
    ).toBeVisible();
    finished = true;

    await page.goto(`${ui}/history/${conferenceId}?section=overview`);
    await expect(page.getByRole("heading", { name: title })).toBeVisible();
    await expect(
      page.getByRole("tablist", { name: "Материалы встречи" }),
    ).toBeVisible();
    await page.screenshot({
      path: resolve(visual, `${info.project.name}-history-desktop.png`),
      fullPage: true,
    });
    await page.goto(`${ui}/notifications`);
    await expect(
      page.getByRole("heading", { name: "Уведомления" }),
    ).toBeVisible();

    await page.goto(`${ui}/admin`);
    await expect(
      page.getByRole("heading", { name: "Нет доступа к разделу операций" }),
    ).toBeVisible();
    const admin = await request.get(`${api}/api/v1/admin/summary`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(admin.status()).toBe(403);
  } finally {
    if (conferenceId && token && !finished)
      await request
        .post(`${api}/api/v1/conferences/${conferenceId}/finish`, {
          headers: { Authorization: `Bearer ${token}` },
        })
        .catch(() => {});
  }
});
