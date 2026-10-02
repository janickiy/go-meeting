import { expect, test } from "@playwright/test";
test.skip(
  !process.env.MEET_REALTIME_CONFERENCE,
  "requires the isolated Stage 2 Go harness",
);
test("two browsers: presence, two tabs and reconnect without opening camera or microphone", /**
 * Проверка: two browsers: presence, two tabs and reconnect without opening camera or microphone выполняет тестовый сценарий «two browsers: presence, two tabs and reconnect without opening camera or microphone» и проверяет ожидаемые результаты.
 *
 * @args
 *   - объект параметров: browser — браузер Playwright с отдельными тестовыми контекстами.
 *
 * @returns Promise, который после завершения операции возвращает: значение не возвращается; функция выполняет описанные действия и обновляет нужное состояние.
 */ async ({ browser }) => {
  const alice = await browser.newContext();
  const bob = await browser.newContext();
  try {
    const a = await alice.newPage();
    const b = await bob.newPage();
    const base = "http://127.0.0.1:5175";
    const conference = process.env.MEET_REALTIME_CONFERENCE!;
    for (const [page, email] of [
      [a, "owner@stage2.example"],
      [b, "member@stage2.example"],
    ] as const) {
      await page.goto(`${base}/login`);
      await page.getByLabel("Email").fill(email);
      await page
        .getByLabel("Пароль", { exact: true })
        .fill("stage-one-test-password");
      await page.getByRole("button", { name: "Войти", exact: true }).click();
      await expect(page).toHaveURL(/\/app/);
      await page.goto(`${base}/conferences/${conference}`);
      await expect(page.getByTestId("connection-id")).toBeVisible();
    }
    const bobPresence = a.getByTestId(
      `presence-${process.env.MEET_REALTIME_BOB}`,
    );
    await expect(bobPresence).toContainText("Онлайн · подключений: 1");
    const b2 = await bob.newPage();
    // Auth is deliberately per tab. A second login obtains its own valid JWT.
    await b2.goto(`${base}/login`);
    await b2.getByLabel("Email").fill("member@stage2.example");
    await b2
      .getByLabel("Пароль", { exact: true })
      .fill("stage-one-test-password");
    await b2.getByRole("button", { name: "Войти", exact: true }).click();
    await expect(b2).toHaveURL(/\/app/);
    await b2.goto(`${base}/conferences/${conference}`);
    await expect(b2.getByTestId("connection-id")).toBeVisible();
    await expect(bobPresence).toContainText("подключений: 2");
    await b2.close();
    await expect(bobPresence).toContainText("Онлайн · подключений: 1");
    const oldID = await b.getByTestId("connection-id").textContent();
    await b
      .getByRole("button", { name: "Переподключиться", exact: true })
      .click();
    await expect(b.getByTestId("connection-id")).not.toHaveText(oldID!);
    await expect(a.getByTestId("local-media")).toHaveCount(0);
    await expect(b.getByTestId("local-media")).toHaveCount(0);
    await expect(a.getByTestId("media-status")).toHaveText(
      "Камера и микрофон выключены",
    );
    // Test never grants media permissions or clicks the explicit media start action.
    await b.close();
    await expect(bobPresence).toContainText("Не в сети");
  } finally {
    await alice.close();
    await bob.close();
  }
});
