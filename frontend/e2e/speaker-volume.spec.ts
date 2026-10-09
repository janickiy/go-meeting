import { expect, test, type Locator, type Page } from "@playwright/test";

/** prepare создаёт изолированную комнату с настоящими синтетическими потоками.
 * @args page — тестовая страница браузера.
 * @return локальная и удалённая плитки; API и физические устройства не используются.
 */
async function prepare(page: Page) {
  await page.goto("/login");
  await page.evaluate(async () => {
    const path = "/e2e/helpers/speaker-fixture.tsx";
    const fixture = await import(path);
    fixture.install({ volumeControls: true });
  });
  await page.getByRole("button", { name: "Запустить тестовые потоки" }).click();
  const local = page.getByTestId("local-media");
  const remote = page
    .getByTestId("remote-media")
    .filter({ has: page.locator("video") });
  await expect(local).toHaveCount(1);
  await expect(remote).toHaveCount(1);
  return { local, remote };
}

/** readMeter считывает одновременно числовой сигнал и реальное оформление плитки.
 * @args tile — плитка участника.
 * @return уровень, прозрачность рамки и доля заполненного фона микрофона.
 */
async function readMeter(tile: Locator) {
  return tile.evaluate((element) => {
    const microphone = element.querySelector(".media-tile-microphone")!;
    const border = getComputedStyle(element, "::after");
    const fill = getComputedStyle(microphone, "::before");
    const microphoneHeight = microphone.getBoundingClientRect().height;
    return {
      level: Number(element.getAttribute("data-audio-level")),
      opacity: Number(border.opacity),
      fill: parseFloat(fill.height) / microphoneHeight,
      animation: border.animationName,
      microphoneAnimation: getComputedStyle(microphone).animationName,
      borderColor: border.borderTopColor,
      microphoneFillColor: fill.backgroundColor,
    };
  });
}

/** steadyLevel ждёт стабилизации измерителя после изменения громкости.
 * @args page — страница; tile — плитка; button — название уровня генератора.
 * @return отображённый нормализованный уровень.
 */
async function steadyLevel(page: Page, tile: Locator, button: string) {
  await page.getByRole("button", { name: button, exact: true }).click();
  await expect(tile).toHaveAttribute("data-speaking", "true");
  // Несколько окон Web Audio нужны для выхода из предыдущей амплитуды.
  await page.waitForTimeout(350);
  return (await readMeter(tile)).level;
}

test("громкость синхронно меняет рамку и заполнение микрофона без перезапуска видео", async ({
  page,
}, info) => {
  await page.emulateMedia({ reducedMotion: "no-preference" });
  const { local, remote } = await prepare(page);
  const video = await remote.locator("video").elementHandle();
  const originalStream = await video!.evaluateHandle(
    (element) => (element as HTMLVideoElement).srcObject,
  );
  const levels: number[] = [];
  for (const [index, button] of [
    "Тихий звук",
    "Средний звук",
    "Громкий звук",
  ].entries()) {
    const level = await steadyLevel(page, remote, button);
    levels.push(level);
    expect(level).toBeGreaterThan(0);
    expect(level).toBeLessThanOrEqual(1);
    for (const tile of [local, remote]) {
      const meter = await readMeter(tile);
      expect(meter.opacity).toBeCloseTo(meter.level, 1);
      expect(meter.fill).toBeCloseTo(meter.level, 1);
      expect(meter.animation).toBe("none");
      expect(meter.microphoneAnimation).toBe("none");
      expect(meter.borderColor).toBe("rgb(66, 216, 139)");
      expect(meter.microphoneFillColor).toBe("rgb(66, 216, 139)");
    }
    await expect(page.locator(".media-tile-screen")).toHaveAttribute(
      "data-audio-level",
      "0",
    );
    await page.screenshot({
      path: info.outputPath(`speaker-volume-${index + 1}.png`),
      fullPage: true,
    });
  }
  expect(levels[1]).toBeGreaterThan(levels[0] + 0.1);
  expect(levels[2]).toBeGreaterThan(levels[1] + 0.1);
  expect(
    await video!.evaluate(
      (element, stream) =>
        element.isConnected &&
        (element as HTMLVideoElement).srcObject === stream,
      originalStream,
    ),
  ).toBe(true);

  const silenceDelay = await remote.evaluate((element) => {
    return new Promise<number>((resolve, reject) => {
      const start = performance.now();
      const timeout = setTimeout(() => {
        observer.disconnect();
        reject(new Error("Индикатор не погас после тишины"));
      }, 1000);
      const observer = new MutationObserver(() => {
        if (element.getAttribute("data-speaking") !== "false") return;
        observer.disconnect();
        clearTimeout(timeout);
        resolve(performance.now() - start);
      });
      observer.observe(element, { attributes: true });
      const button = [...document.querySelectorAll("button")].find(
        (candidate) => candidate.textContent === "Тишина",
      );
      button!.click();
    });
  });
  expect(silenceDelay).toBeLessThanOrEqual(250);
  await expect(remote).toHaveAttribute("data-audio-level", "0");
  await expect(local).toHaveAttribute("data-audio-level", "0");
  await expect(remote.locator(".media-tile-microphone")).not.toHaveClass(
    /media-microphone-speaking/,
  );
});

test("измеритель работает с сокращённой анимацией и сразу гаснет при выключении микрофона или связи", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const { local, remote } = await prepare(page);
  const quiet = await steadyLevel(page, remote, "Тихий звук");
  const loud = await steadyLevel(page, remote, "Громкий звук");
  expect(loud).toBeGreaterThan(quiet + 0.2);
  await page
    .getByRole("button", { name: "Переключить микрофон Бориса" })
    .click();
  await expect(remote).toHaveAttribute("data-audio-level", "0");
  await expect(remote).toHaveAttribute("data-speaking", "false");
  await expect(local).toHaveAttribute("data-speaking", "true");
  await expect(remote.getByLabel("Микрофон выключен")).toBeVisible();
  await page
    .getByRole("button", { name: "Переключить микрофон Бориса" })
    .click();
  await expect(remote).toHaveAttribute("data-speaking", "true");
  await page.getByRole("button", { name: "Отключить связь" }).click();
  for (const tile of [local, remote]) {
    await expect(tile).toHaveAttribute("data-audio-level", "0");
    await expect(tile).toHaveAttribute("data-speaking", "false");
  }
});
