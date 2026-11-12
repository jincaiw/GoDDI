import { defineConfig } from '@playwright/test'

// The bare-instance suite: every spec that brings its own state.
//
// `*.fixture.spec.ts` is excluded here and run by playwright.fixture.config.ts
// against a seeded instance (scripts/w13_frontend_smoke.py up). The exclusion
// is what keeps `npx playwright test` -- the command CI runs and the command a
// developer types -- correct against a plain instance.
//
// console.spec.ts and workflows.spec.ts are excluded for a different reason:
// they were written against the pre-TDesign console and fail wholesale
// against the migrated UI. Their coverage was mapped and re-pinned elsewhere
// (see the STALE headers at the top of each file and
// docs/audit/IMPLEMENTATION-STATUS.md §1av/§1aw). Excluding them here keeps
// the bare suite green until someone rewrites them against TDesign -- the
// header comment in each file is the starting point for that work.
export default defineConfig({
  testDir: './tests',
  testIgnore: /(\.fixture\.spec\.ts|console\.spec\.ts|workflows\.spec\.ts)$/,
  workers: 1,
  timeout: 120000,
  use: {
    baseURL: process.env.GODDI_TEST_BASE_URL || 'http://127.0.0.1:16090',
    channel: process.env.GODDI_BROWSER_CHANNEL || undefined,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
})
