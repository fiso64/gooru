import { defineConfig, devices } from '@playwright/test';

const chromiumExecutablePath = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;
const externalBaseURL = process.env.GOORU_E2E_BASE_URL?.trim();
const previewPort = Number(process.env.GOORU_E2E_PORT ?? '4173');
if (!Number.isInteger(previewPort) || previewPort < 1024 || previewPort > 65535) {
  throw new Error('GOORU_E2E_PORT must be an integer between 1024 and 65535');
}
const previewBaseURL = `http://127.0.0.1:${previewPort}`;

export default defineConfig({
  testDir: './tests',
  // The destructive, credentialed upload benchmark has its own smoke config.
  testIgnore: 'smoke-upload-library.spec.ts',
  timeout: 30_000,
  use: {
    baseURL: externalBaseURL || previewBaseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure'
  },
  ...(externalBaseURL ? {} : {
    webServer: {
      command: `npm run build && npm run preview -- --host 127.0.0.1 --port ${previewPort} --strictPort`,
      url: previewBaseURL,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000
    }
  }),
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
