import { expect, test, type Page } from '@playwright/test';
import { jobsRefreshMinIntervalMs } from '../src/lib/jobsEvents';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const controlledClockStart = new Date('2026-01-01T00:00:00Z');
const controlledClockPause = new Date('2026-01-01T00:00:20Z');

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

async function mockShellApis(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
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

test('does not keep polling operations on idle screens', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await mockOperationEvents(page);
  let operationRequests = 0;
  await page.route('**/api/v1/operations?**', async (route) => {
    operationRequests += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });

  await signIn(page);
  await page.getByRole('button', { name: 'Tags' }).click();
  await expect(page.getByRole('heading', { name: 'Tags' })).toBeVisible();
  await expect.poll(() => operationRequests).toBeGreaterThan(0);
  await expect.poll(() => page.evaluate(() => (window as OperationEventTestWindow).__operationEventSourceCount ?? 0)).toBe(1);
  await page.waitForTimeout(200);
  const settledRequests = operationRequests;
  await page.waitForTimeout(2300);
  expect(operationRequests).toBe(settledRequests);
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