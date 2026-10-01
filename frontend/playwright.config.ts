import { defineConfig, devices } from "@playwright/test";

const port = process.env.MEET_LIVE_TEST_URL ? 5175 : 5174;
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: "./e2e",
  outputDir: process.env.MEET_LIVE_TEST_URL
    ? "./test-results/live"
    : "./test-results/ui",
  workers: process.env.CI ? 2 : 4,
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: "list",
  use: {
    baseURL,
    reducedMotion: "reduce",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 1440, height: 1000 },
        launchOptions: process.env.MEET_MEDIA_CONFERENCE
          ? {
              args: [
                "--use-fake-ui-for-media-stream",
                "--use-fake-device-for-media-stream",
              ],
            }
          : undefined,
      },
    },
    {
      name: "firefox",
      use: {
        ...devices["Desktop Firefox"],
        viewport: { width: 1440, height: 1000 },
      },
    },
  ],
  webServer: {
    command: `npm run dev -- --port ${port}`,
    url: baseURL,
    reuseExistingServer: !process.env.CI && !process.env.MEET_LIVE_TEST_URL,
  },
});
