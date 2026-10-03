import { test, expect, type Page } from "@playwright/test";

test.skip(
  !process.env.MEET_LIVE_TEST_URL,
  "Use the opt-in Go integration harness, which creates an isolated PostgreSQL database.",
);
test("real Go API: register, create, invite, join, leave, rejoin and finish", /**
 * Проверяет регистрацию, создание встречи, приглашение, присоединение, выход, повторный вход и завершение через реальный Go API.
 *
 * @args
 *   - объект параметров: page — изолированная страница Playwright; browser — браузер Playwright с отдельными тестовыми контекстами.
 *   - info — контекст запуска для диагностических вложений.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ page, browser }, info) => {
  const stamp = `${Date.now()}-${info.project.name}`;
  const password = "abcdefgh";
  /**
   * register отправляет данные регистрации с нормализацией необязательного отображаемого имени.
   *
   * @args
   *   - target (Page) — целевой браузерный объект или ресурс.
   *   - name (string) — отображаемое имя пользователя для инициалов.
   *
   * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
   */
  async function register(target: Page, name: string) {
    await target.goto("/register");
    await target
      .getByLabel("Email", { exact: true })
      .fill(`${name}-${stamp}@example.test`);
    await target.getByLabel("Пароль", { exact: true }).fill(password);
    await target.getByLabel(/Как к вам обращаться/).fill(name);
    await target
      .getByRole("button", { name: "Зарегистрироваться", exact: true })
      .click();
    await expect(
      target.getByRole("heading", { name: "Аккаунт создан!" }),
    ).toBeVisible();
    await target.getByRole("link", { name: "Перейти в приложение" }).click();
  }
  await register(page, "Организатор");
  await page.getByRole("link", { name: "Новая конференция" }).click();
  await page.getByLabel("Название конференции").fill(`Frontend E2E ${stamp}`);
  await page
    .getByRole("button", { name: "Создать конференцию", exact: true })
    .click();
  const invitation = await page.getByLabel("Ссылка-приглашение").inputValue();
  expect(new URL(invitation).pathname).toMatch(/^\/i\/[A-Za-z0-9_-]{32}$/);
  await page.getByRole("link", { name: "Перейти в конференцию" }).click();
  const conferenceURL = page.url();
  await page.getByRole("button", { name: "Начать конференцию" }).click();
  await expect(
    page.getByRole("button", { name: "Завершить конференцию", exact: true }),
  ).toBeVisible();
  const otherContext = await browser.newContext({
    baseURL: process.env.MEET_LIVE_TEST_URL,
  });
  try {
    const other = await otherContext.newPage();
    await register(other, "Участник");
    await other.goto(invitation);
    await other
      .getByRole("button", { name: "Проверить устройства и войти" })
      .click();
    await other.getByRole("button", { name: "Войти во встречу" }).click();
    await expect(other).toHaveURL(conferenceURL);
    await expect(
      other.getByRole("button", { name: "Покинуть конференцию" }),
    ).toBeVisible();
    await expect(
      other.getByRole("button", { name: "Завершить конференцию", exact: true }),
    ).toHaveCount(0);
    await other.getByRole("button", { name: "Покинуть конференцию" }).click();
    await expect(
      other.getByRole("button", { name: "Присоединиться", exact: true }),
    ).toBeVisible();
    await other
      .getByRole("button", { name: "Присоединиться", exact: true })
      .click();
    await other.getByRole("button", { name: "Войти во встречу" }).click();
    await expect(
      other.getByRole("button", { name: "Покинуть конференцию" }),
    ).toBeVisible();
    await page.reload();
    await expect(page.getByText("Участники 2")).toBeVisible();
    await page
      .getByRole("button", { name: "Завершить конференцию", exact: true })
      .click();
    await page.getByRole("button", { name: "Да, завершить" }).click();
    await expect(
      page.getByRole("heading", { name: "Встреча закрыта" }),
    ).toBeVisible();
    await other.reload();
    await expect(
      other.getByRole("heading", { name: "Встреча закрыта" }),
    ).toBeVisible();
    await other.goto(invitation);
    await expect(
      other.getByText("Организатор уже закрыл эту конференцию."),
    ).toBeVisible();
    await expect(
      other.getByRole("button", { name: "Проверить устройства и войти" }),
    ).toHaveCount(0);
  } finally {
    await otherContext.close();
  }
});
