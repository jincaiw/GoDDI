import { defineConfig } from '@playwright/test'

// The specs that need a *seeded* instance -- `*.fixture.spec.ts`.
//
// Two families of spec live in this suite and they make opposite assumptions
// about the world they run in. Most of them write what they assert on and are
// only correct on a table nobody else has touched (a pagination case that
// expects page 2 to hold exactly one row is a count of the whole table, not of
// its own rows). The fixture family is the other way round: it renders
// surfaces that need an address with a DNS record, a materialised /24 and a
// sparse /15 to exist before the browser opens, and it writes without cleaning
// up. Run them in one instance and one family fails the other.
//
// So each gets its own instance and its own config, and the split is by file
// suffix rather than by a hand-written list of filenames: a new spec lands in
// the bare config by default, and moving it to the seeded one is a rename.
// A list would have the failure mode this suite exists to prevent -- a spec
// that no job runs and that therefore always passes.
//
// The seeded instance is scripts/w13_frontend_smoke.py up: it builds the
// binary, starts it and creates the fixtures the specs name.
export default defineConfig({
  testDir: './tests',
  testMatch: /\.fixture\.spec\.ts$/,
  workers: 1,
  timeout: 120000,
  use: {
    baseURL: process.env.GODDI_TEST_BASE_URL || 'http://127.0.0.1:16090',
    channel: process.env.GODDI_BROWSER_CHANNEL || undefined,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
})
