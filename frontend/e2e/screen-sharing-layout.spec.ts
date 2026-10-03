import { expect, test } from "@playwright/test";

for (const viewport of [
  { width: 1800, height: 1100 },
  { width: 390, height: 844 },
  { width: 844, height: 390 },
]) {
  test(`экран заменяет все плитки без остановки звука: ${viewport.width}×${viewport.height}`, async ({
    page,
  }, info) => {
    await page.setViewportSize(viewport);
    await page.goto("/login");
    await page.evaluate(async () => {
      const path = "/e2e/helpers/speaker-fixture.tsx";
      const fixture = await import(path);
      fixture.install({ screenSharing: true });
    });
    await page
      .getByRole("button", { name: "Запустить тестовые потоки" })
      .click();
    const remote = page
      .locator(".media-tile:not(.media-tile-screen)")
      .filter({ has: page.locator("video") });
    const local = page.locator(".media-tile-local:not(.media-tile-screen)");
    await expect(remote).toBeVisible();
    await expect(local).toBeVisible();
    // Сохраняем настоящие DOM-элемент и поток, чтобы обнаружить перепривязку.
    await remote.locator("video").evaluate((video: HTMLVideoElement) => {
      const probe = window as unknown as {
        preservedVideo: HTMLVideoElement;
        preservedStream: MediaProvider | null;
      };
      probe.preservedVideo = video;
      probe.preservedStream = video.srcObject;
    });
    for (const action of ["Показать свой экран", "Показать экран Бориса"]) {
      await page.getByRole("button", { name: action }).click();
      const sharing = page.locator(".media-grid-sharing");
      const display = sharing.locator(".media-tile-screen");
      await expect(display).toBeVisible();
      await expect(local).toBeHidden();
      await expect(remote).toBeHidden();
      await expect(page.getByTestId("participant-placeholder")).toBeHidden();
      await expect(display.locator("video")).toHaveCSS("object-fit", "contain");
      const stageBox = await sharing.boundingBox();
      const screenBox = await display.boundingBox();
      expect(Math.abs(stageBox!.width - screenBox!.width)).toBeLessThan(2);
      expect(Math.abs(stageBox!.height - screenBox!.height)).toBeLessThan(2);
      await expect
        .poll(() =>
          page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        )
        .toBe(true);
      const elapsed = await remote
        .locator("video")
        .evaluate((video: HTMLVideoElement) => video.currentTime);
      await expect
        .poll(() =>
          remote
            .locator("video")
            .evaluate((video: HTMLVideoElement) => video.currentTime),
        )
        .toBeGreaterThan(elapsed + 0.2);
      expect(
        await remote.locator("video").evaluate((video: HTMLVideoElement) => {
          const probe = window as unknown as {
            preservedVideo: HTMLVideoElement;
            preservedStream: MediaProvider | null;
          };
          return (
            video === probe.preservedVideo &&
            video.srcObject === probe.preservedStream &&
            !video.muted &&
            !video.paused
          );
        }),
      ).toBe(true);
      if (action === "Показать экран Бориса")
        await page.screenshot({
          path: info.outputPath("shared-screen.png"),
          fullPage: true,
        });
      await page.getByRole("button", { name: "Остановить показ" }).click();
      await expect(page.locator(".media-grid-sharing")).toHaveCount(0);
      await expect(remote).toBeVisible();
      await expect(local).toBeVisible();
      await expect(page.getByTestId("participant-placeholder")).toBeVisible();
    }
  });
}
