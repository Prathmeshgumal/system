import { defineConfig } from '@playwright/test'

// End-to-end tests: real browsers against the real API + Vite dev server.
// `make e2e` runs them; Playwright starts the API and Vite unless they're already
// running (e.g. under `make dev`). First time: `pnpm exec playwright install chromium`.
export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'retain-on-failure',
    // Use an already-installed Chromium instead of Playwright's own, if set.
    launchOptions: { executablePath: process.env.PW_CHROMIUM_PATH || undefined },
  },
  webServer: [
    {
      command: 'go run ./cmd/api',
      cwd: '../backend',
      url: 'http://localhost:8080/readyz',
      reuseExistingServer: true,
      timeout: 120_000,
    },
    {
      command: 'pnpm dev --port 5173 --strictPort',
      url: 'http://localhost:5173',
      reuseExistingServer: true,
    },
  ],
})
