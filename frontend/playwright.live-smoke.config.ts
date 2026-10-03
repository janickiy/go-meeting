import { defineConfig, devices } from "@playwright/test";

/** Изолированная приёмка локального backend: не запускает Vite и не сохраняет HTTP-токены в трассировки. */
export default defineConfig({
  testDir: "./e2e",
  testMatch: "frontend-live-smoke.spec.ts",
  outputDir: "./test-results/live-frontend",
  timeout: 180_000,
  workers: 1,
  retries: 0,
  use: {
    ...devices["Desktop Chrome"],
    viewport: { width: 1440, height: 1000 },
    ignoreHTTPSErrors: true,
    permissions: ["camera", "microphone"],
    trace: "off",
    screenshot: "only-on-failure",
    launchOptions: {
      args: [
        "--use-fake-ui-for-media-stream",
        "--use-fake-device-for-media-stream",
      ],
    },
  },
});
