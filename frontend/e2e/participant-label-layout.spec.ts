import { expect, test, type Locator, type Page } from "@playwright/test";

const longName = "Александра Оченьдлиннаяфамилия ".repeat(12);

/** Монтирует настоящие плитки без API, потоков и захвата устройств. */
async function installFixture(page: Page) {
  await page.route("**/api/v1/**", (route) =>
    route.fulfill({ status: 404, json: { message: "Изолированная проверка" } }),
  );
  await page.goto("/login");
  await page.evaluate(async () => {
    const path = "/e2e/helpers/speaker-fixture.tsx";
    const fixture = await import(path);
    fixture.install({ screenSharing: true, participantRoles: true });
  });
}

/** Проверяет компактность, центрирование и отсутствие выхода за плитку. */
async function expectCenteredLabel(tile: Locator, shortName: boolean) {
  const label = tile.locator(".media-tile-label");
  await expect(tile).toBeVisible();
  await expect(label).toBeVisible();
  const tileBox = await tile.boundingBox();
  const labelBox = await label.boundingBox();
  expect(tileBox).not.toBeNull();
  expect(labelBox).not.toBeNull();
  expect(
    Math.abs(
      labelBox!.x + labelBox!.width / 2 - (tileBox!.x + tileBox!.width / 2),
    ),
  ).toBeLessThan(1);
  expect(labelBox!.width).toBeLessThanOrEqual(
    Math.min(320, tileBox!.width - 32) + 1,
  );
  expect(labelBox!.x).toBeGreaterThan(tileBox!.x + 15);
  expect(labelBox!.x + labelBox!.width).toBeLessThan(
    tileBox!.x + tileBox!.width - 15,
  );
  const bottomGap =
    tileBox!.y + tileBox!.height - labelBox!.y - labelBox!.height;
  expect(bottomGap).toBeGreaterThan(2);
  expect(bottomGap).toBeLessThanOrEqual(12);
  if (shortName) {
    expect(labelBox!.width).toBeLessThan(180);
    const name = label.locator(":scope > span:first-child");
    expect(
      await name.evaluate(
        (element) => element.scrollWidth <= element.clientWidth + 1,
      ),
    ).toBe(true);
  }
}

/** Проверяет значки роли и микрофона, включая их границы при длинном имени. */
async function expectBadges(tile: Locator, role: string) {
  const label = tile.locator(".media-tile-label");
  const badges = label.locator(".media-tile-badges");
  await expect(badges).toHaveCSS("flex-shrink", "0");
  await expect(badges.getByLabel(role, { exact: true })).toBeVisible();
  await expect(
    badges.getByLabel("Микрофон включён", { exact: true }),
  ).toBeVisible();
  const labelBox = await label.boundingBox();
  for (const icon of [
    badges.getByLabel(role, { exact: true }),
    badges.locator(".media-tile-microphone"),
  ]) {
    const iconBox = await icon.boundingBox();
    expect(iconBox).not.toBeNull();
    expect(iconBox!.width).toBeGreaterThanOrEqual(12);
    expect(iconBox!.height).toBeGreaterThanOrEqual(12);
    expect(iconBox!.x).toBeGreaterThanOrEqual(labelBox!.x);
    expect(iconBox!.x + iconBox!.width).toBeLessThanOrEqual(
      labelBox!.x + labelBox!.width,
    );
    expect(iconBox!.y).toBeGreaterThanOrEqual(labelBox!.y);
    expect(iconBox!.y + iconBox!.height).toBeLessThanOrEqual(
      labelBox!.y + labelBox!.height,
    );
  }
}

/** Меняет только текст для проверки CSS, не вмешиваясь в компоненты и потоки. */
async function makeNameLong(tile: Locator) {
  const name = tile.locator(".media-tile-label > span:first-child");
  await name.evaluate((element, text) => {
    element.textContent = text;
  }, longName);
  await expect(name).toHaveCSS("text-overflow", "ellipsis");
  await expect(name).toHaveCSS("white-space", "nowrap");
  await expect(name).toHaveCSS("overflow", "hidden");
  expect(
    await name.evaluate((element) => element.scrollWidth > element.clientWidth),
  ).toBe(true);
}

for (const viewport of [
  { width: 1440, height: 1000 },
  { width: 390, height: 844 },
  { width: 844, height: 390 },
]) {
  test(`плашки имени компактны и центрированы: ${viewport.width}×${viewport.height}`, async ({
    page,
  }, info) => {
    await page.setViewportSize(viewport);
    await installFixture(page);
    const local = page.locator(".media-tile-local:not(.media-tile-screen)");
    const remote = page
      .locator(".media-tile:not(.media-tile-local):not(.media-tile-screen)")
      .filter({ has: page.getByLabel("Соорганизатор", { exact: true }) });
    for (const [tile, role] of [
      [local, "Организатор"],
      [remote, "Соорганизатор"],
    ] as const) {
      await expectCenteredLabel(tile, true);
      await expectBadges(tile, role);
      await makeNameLong(tile);
      await expectCenteredLabel(tile, false);
      await expectBadges(tile, role);
    }
    await page.screenshot({
      path: info.outputPath("participant-label-long-name.png"),
      fullPage: true,
    });
    for (const action of ["Показать свой экран", "Показать экран Бориса"]) {
      await page.getByRole("button", { name: action, exact: true }).click();
      const screen = page.locator(".media-grid-sharing .media-tile-screen");
      await expectCenteredLabel(screen, true);
      await expect(screen.locator(".media-tile-badges")).toHaveCount(0);
      await makeNameLong(screen);
      await expectCenteredLabel(screen, false);
      await page.getByRole("button", { name: "Остановить показ" }).click();
    }
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
  });
}
