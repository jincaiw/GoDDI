import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  workers: 1,
  timeout: 120000,
  use: {
    baseURL: process.env.GODDI_TEST_BASE_URL || 'http://127.0.0.1:16090',
    channel: process.env.GODDI_BROWSER_CHANNEL || undefined,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
})
