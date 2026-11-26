import { defineConfig } from '@playwright/test'
import { existsSync } from 'node:fs'

export default defineConfig({
  testDir: './e2e',
  globalSetup: './e2e/setup.ts',
  timeout: 120000,
  expect: { timeout: 15000 },
  workers: 1,
  retries: 0,
  use: {
    browserName: 'chromium',
    channel: process.platform === 'darwin' && existsSync('/Applications/Google Chrome.app') ? 'chrome' : undefined,
    viewport: { width: 1440, height: 930 },
    locale: 'zh-CN',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    acceptDownloads: true,
  },
  reporter: [['list']],
})
