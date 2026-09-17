import { expect, test, type Page } from '@playwright/test';
import { jobsRefreshFallbackIntervalMs, jobsRefreshMinIntervalMs } from '../src/lib/jobsEvents';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const maintenanceJob = {
  id: 'media-metadata-sweep',
  name: 'Extract media metadata',
  description: 'Scan tracked files that are missing media metadata and enqueue durable extraction work.',
  running: false
};

const controlledClockStart = new Date('2026-01-01T00:00:00Z');
const controlledClockPause = new Date('2026-01-01T00:00:10Z');

type OperationEventTestWindow = Window & typeof globalThis & {
  __emitOperationEvent?: () => void;
  __operationEventSourceCount?: number;
};

async function mockAuth(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: loggedIn ? 200 : 401,
      contentType: 'application/json',
      body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
    });
  });
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
}

async function mockShellApis(page: Page, maintenanceJobs = [{ ...maintenanceJob }]) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/maintenance-jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: maintenanceJobs }) }));
}

async function mockOperationEvents(page: Page) {
  await page.addInitScript(() => {
    const testWindow = window as OperationEventTestWindow;
    let currentSource: FakeEventSource | undefined;

    class FakeEventSource {
      private operationsListener: (() => void) | undefined;

      constructor(_url: string) {
        currentSource = this;
        testWindow.__operationEventSourceCount = (testWindow.__operationEventSourceCount ?? 0) + 1;
      }

      addEventListener(type: string, listener: () => void) {
        if (type === 'operations') this.operationsListener = listener;
      }

      close() {
        if (currentSource === this) currentSource = undefined;
      }

      emitOperation() {
        this.operationsListener?.();
      }
    }

    Object.defineProperty(window, 'EventSource', { value: FakeEventSource, configurable: true });
    testWindow.__emitOperationEvent = () => currentSource?.emitOperation();
  });
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('refreshes durable operations when the SSE stream stays silent', async ({ page }) => {
  await page.clock.install({ time: new Date('2026-09-08T08:00:00Z') });
  await mockAuth(page);
  await mockShellApis(page);
  await mockOperationEvents(page);
  let operationVisible = false;
  await page.route('**/api/v1/operations?**', async (route) => {
    const items = operationVisible
      ? [{ id: 'op-cross-process', kind: 'delete_files', status: 'running', progress_total: 2, progress_completed: 1, progress_failed: 0, created_at: '2026-09-08T08:00:00Z' }]
      : [];
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items, active_count: items.length, total_count: items.length })
    });
  });

  await signIn(page);
  await page.getByRole('complementary').getByRole('button', { name: 'Jobs' }).click();
  await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();
  await expect(page.getByText('No background operations have been recorded.')).toBeVisible();
  await expect.poll(() => page.evaluate(() => (window as OperationEventTestWindow).__operationEventSourceCount ?? 0)).toBe(1);

  operationVisible = true;
  await page.clock.fastForward(jobsRefreshFallbackIntervalMs);

  await expect(page.locator('.jobs-card .job-row').filter({ hasText: 'Delete Files' })).toBeVisible();
  await expect(page.locator('.jobs-card .status.running')).toHaveText('running');
});

test('refreshes active operations from one throttled SSE signal stream', async ({ page }) => {
  await page.clock.install({ time: controlledClockStart });
  await mockAuth(page);
  await mockShellApis(page);
  await mockOperationEvents(page);
  let operationRequests = 0;
  await page.route('**/api/v1/operations?**', async (route) => {
    operationRequests += 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [{ id: 'op-1', kind: 'delete_files', status: 'running', progress_total: 2, progress_completed: 1, progress_failed: 0, created_at: '2026-09-08T08:00:00Z' }] })
    });
  });

  await signIn(page);
  await expect.poll(() => operationRequests).toBeGreaterThan(0);
  await expect.poll(() => page.evaluate(() => (window as OperationEventTestWindow).__operationEventSourceCount ?? 0)).toBe(1);
  await page.waitForTimeout(100);
  await page.clock.pauseAt(controlledClockPause);
  const initialRequests = operationRequests;

  await page.evaluate(() => (window as OperationEventTestWindow).__emitOperationEvent?.());
  await expect.poll(() => operationRequests).toBe(initialRequests + 1);
  const firstRefreshRequests = operationRequests;

  await page.evaluate(() => {
    const testWindow = window as OperationEventTestWindow;
    testWindow.__emitOperationEvent?.();
    testWindow.__emitOperationEvent?.();
    testWindow.__emitOperationEvent?.();
  });
  await page.clock.fastForward(jobsRefreshMinIntervalMs - 1);
  expect(operationRequests).toBe(firstRefreshRequests);
  await page.clock.fastForward(1);
  await expect.poll(() => operationRequests).toBe(firstRefreshRequests + 1);
});

test('runs a maintenance job from one dropdown and shows the durable job row', async ({ page }) => {
  const maintenanceJobs = [{ ...maintenanceJob }];
  await mockAuth(page);
  await mockShellApis(page, maintenanceJobs);
  await mockOperationEvents(page);

  let operationVisible = false;
  await page.route('**/api/v1/operations?**', async (route) => {
    const items = operationVisible
      ? [{ id: 'op-metadata', kind: 'media.metadata-sweep', status: 'pending', progress_total: 0, progress_completed: 0, progress_failed: 0, created_at: '2026-09-15T06:00:00Z' }]
      : [];
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items, active_count: items.length, total_count: items.length })
    });
  });

  let invokedPath = '';
  let csrfHeader = '';
  await page.route('**/api/v1/maintenance-jobs/*', async (route) => {
    invokedPath = new URL(route.request().url()).pathname;
    csrfHeader = route.request().headers()['x-gooru-csrf'] ?? '';
    operationVisible = true;
    maintenanceJobs[0] = { ...maintenanceJob, running: true };
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ job: maintenanceJobs[0], created: true })
    });
  });

  await signIn(page);
  await page.getByRole('complementary').getByRole('button', { name: 'Jobs' }).click();
  const runJobButton = page.getByRole('button', { name: 'Run job' });
  await expect(runJobButton).toBeVisible();
  await runJobButton.click();

  const metadataEntry = page.getByRole('button', { name: maintenanceJob.name });
  await expect(metadataEntry).toBeEnabled();
  await metadataEntry.click();

  await expect.poll(() => invokedPath).toBe('/api/v1/maintenance-jobs/media-metadata-sweep');
  expect(csrfHeader).toBe(session.csrf_token);
  await expect(page.locator('.jobs-card .job-row').filter({ hasText: 'Media.metadata-sweep' })).toBeVisible();
  await expect(page.getByText('Extract media metadata queued.')).toHaveCount(0);

  await runJobButton.click();
  const runningEntry = page.getByRole('button', { name: /Extract media metadata.*Running/i });
  await expect(runningEntry).toBeDisabled();
});
