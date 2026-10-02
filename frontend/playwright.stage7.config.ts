import { defineConfig, devices } from "@playwright/test";

// Изолированный opt-in стенд: конфигурация не запускает Vite и не обращается к основному Compose.
export default defineConfig({
  testDir: "./e2e",
  testMatch: "stage-seven-live.spec.ts",
  outputDir: "./test-results/stage7-live",
  workers: 1,
  timeout: 300_000,
  expect: { timeout: 20_000 },
  retries: 0,
  reporter: "list",
  use: {
    baseURL: "https://localhost:15482",
    ignoreHTTPSErrors: true,
    viewport: { width: 1440, height: 1100 },
    screenshot: "only-on-failure",
    trace: "off",
  },
  projects: [
    {
      name: "stage7-chromium",
      use: {
        ...devices["Desktop Chrome"],
        launchOptions: {
          args: [
            "--use-fake-ui-for-media-stream",
            "--use-fake-device-for-media-stream",
          ],
        },
      },
    },
  ],
});
