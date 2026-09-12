import { defineConfig } from '@playwright/test'

// The bare-instance suite: every spec that brings its own state.
//
// `*.fixture.spec.ts` is excluded here and run by playwright.fixture.config.ts
// against a seeded instance (scripts/w13_frontend_smoke.py up). The exclusion
// is what keeps `npx playwright test` -- the command CI runs and the command a
// developer types -- correct against a plain instance.
export default defineConfig({
  testDir: './tests',
  testIgnore: /\.fixture\.spec\.ts$/,
  workers: 1,
  timeout: 120000,
  use: {
    baseURL: process.env.GODDI_TEST_BASE_URL || 'http://127.0.0.1:16090',
    channel: process.env.GODDI_BROWSER_CHANNEL || undefined,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
})
