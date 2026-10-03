import { defineConfig, devices } from "@playwright/test";

// Приёмочный тест запускается явно для уже работающего локального стенда Compose.
// Сервер разработки, профиль браузера пользователя и физические устройства не используются.
// Запуск из корня репозитория: MEET_STAGE4_DOCKER=true npx --prefix frontend playwright
// test -c frontend/playwright.recording-docker.config.ts
export default defineConfig({
  testDir: "./e2e",
  testMatch: "composite-recording.spec.ts",
  outputDir: "./test-results/stage4-docker",
  workers: 1,
  retries: 0,
  reporter: "list",
  timeout: 240000,
  expect: { timeout: 15000 },
  use: {
    ...devices["Desktop Chrome"],
    viewport: { width: 1440, height: 1100 },
    ignoreHTTPSErrors: true,
    screenshot: "only-on-failure",
    trace: "off",
    launchOptions: {
      args: [
        "--use-fake-ui-for-media-stream",
        "--use-fake-device-for-media-stream",
      ],
    },
  },
});
