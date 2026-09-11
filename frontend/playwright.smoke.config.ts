import { defineConfig, devices } from '@playwright/test';

const chromiumExecutablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;
const timeout = Number(process.env.GOORU_SMOKE_TIMEOUT_MS ?? String(45 * 60 * 1000));

export default defineConfig({
  testDir: './tests',
  testMatch: 'smoke-upload-library.spec.ts',
  timeout: timeout + 5 * 60 * 1000,
  expect: { timeout: 30_000 },
  outputDir: process.env.GOORU_SMOKE_PLAYWRIGHT_OUTPUT ?? 'test-results/smoke',
  use: {
    baseURL: process.env.GOORU_SMOKE_BASE_URL ?? 'http://127.0.0.1:5678',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure'
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        ...(chromiumExecutablePath ? { launchOptions: { executablePath: chromiumExecutablePath } } : {})
      }
    }
  ]
});
