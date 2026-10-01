import { defineConfig, devices } from "@playwright/test";

// Opt in only against the already running local Compose stack; no user profile.
export default defineConfig({
  testDir: "./e2e",
  testMatch: "stage-five-collaboration.spec.ts",
  outputDir: "./test-results/stage5-docker",
  workers: 1,
  retries: 0,
  reporter: "list",
  timeout: 240000,
  expect: { timeout: 20000 },
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
