import { defineConfig } from '@playwright/test';

// Проверяет только согласованный удалённый стенд, без dev server и статических подмен.
export default defineConfig({
  testDir:'./e2e', testMatch:'frontend-live-smoke.spec.ts',
  outputDir:'./test-results/remote', workers:1, retries:0, timeout:240_000,
  reporter:'list', use:{ignoreHTTPSErrors:false, trace:'off', screenshot:'off'},
  projects:[
    {name:'chromium',use:{browserName:'chromium',launchOptions:{args:['--use-fake-ui-for-media-stream','--use-fake-device-for-media-stream']}}},
    {name:'firefox',use:{browserName:'firefox',launchOptions:{firefoxUserPrefs:{'media.navigator.streams.fake':true,'media.navigator.permission.disabled':true}}}},
  ],
});
