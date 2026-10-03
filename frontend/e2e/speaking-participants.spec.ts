import { expect, test } from "@playwright/test";

test("настоящий Web Audio подсвечивает микрофоны и плитки без камеры, не включая экран", async ({
  page,
}, info) => {
  await page.goto("/login");
  await page.evaluate(async () => {
    const path = "/e2e/helpers/speaker-fixture.tsx";
    const fixture = await import(path);
    fixture.install();
  });
  await page.getByRole("button", { name: "Запустить тестовые потоки" }).click();
  await expect(page.getByTestId("local-media")).toHaveCount(1);
  const local = page.getByTestId("local-media");
  const remote = page
    .getByTestId("remote-media")
    .filter({ has: page.locator("video") });
  const screen = page.locator(".media-tile-screen");
  await page.getByRole("button", { name: "Звук", exact: true }).click();
  await expect(local).toHaveAttribute("data-speaking", "true");
  await expect(remote).toHaveAttribute("data-speaking", "true");
  await expect(page.getByLabel("Говорит: Борис")).toHaveClass(
    /media-microphone-speaking/,
  );
  await expect(screen).toHaveAttribute("data-speaking", "false");
  await page.screenshot({
    path: info.outputPath("speaking-participants.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Тишина", exact: true }).click();
  await expect(local).toHaveAttribute("data-speaking", "false");
  await expect(remote).toHaveAttribute("data-speaking", "false");
  await page.getByRole("button", { name: "Звук", exact: true }).click();
  await expect(remote).toHaveAttribute("data-speaking", "true");
  await page
    .getByRole("button", { name: "Переключить микрофон Бориса" })
    .click();
  await expect(remote).toHaveAttribute("data-speaking", "false");
  await expect(local).toHaveAttribute("data-speaking", "true");
  await page
    .getByRole("button", { name: "Переключить микрофон Бориса" })
    .click();
  await expect(remote).toHaveAttribute("data-speaking", "true");
  await page.getByRole("button", { name: "Отключить связь" }).click();
  await expect(local).toHaveAttribute("data-speaking", "false");
  await expect(remote).toHaveAttribute("data-speaking", "false");
});
