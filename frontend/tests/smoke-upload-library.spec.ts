import { expect, test } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const fileCount = Number(process.env.GOORU_SMOKE_FILE_COUNT ?? '2000');
const datasetDir = process.env.GOORU_SMOKE_DATASET_DIR;
const artifactDir = process.env.GOORU_SMOKE_ARTIFACT_DIR;
const username = process.env.GOORU_SMOKE_USERNAME ?? 'smoke-admin';
const password = process.env.GOORU_SMOKE_PASSWORD ?? '';
if (!password) throw new Error('GOORU_SMOKE_PASSWORD is required');
const tag = process.env.GOORU_SMOKE_TAG ?? `e2e:smoke-${Date.now()}`;
const operationTimeout = Number(process.env.GOORU_SMOKE_TIMEOUT_MS ?? String(45 * 60 * 1000));

if (!datasetDir) throw new Error('GOORU_SMOKE_DATASET_DIR is required');
if (!artifactDir) throw new Error('GOORU_SMOKE_ARTIFACT_DIR is required');

type Timing = {
  started_at: string;
  finished_at: string;
  duration_ns: string;
  duration_ms: number;
};

type TimingReport = {
  tag: string;
  file_count: number;
  upload?: Timing;
  delete_from_disk?: Timing;
};

type RemovalOperation = {
  status: 'pending' | 'running' | 'completed' | 'failed' | 'canceled';
  error_message?: string;
};

const timingReport: TimingReport = { tag, file_count: fileCount };

function elapsed(startedAt: Date, startNs: bigint): Timing {
  const finishedAt = new Date();
  const duration = process.hrtime.bigint() - startNs;
  return {
    started_at: startedAt.toISOString(),
    finished_at: finishedAt.toISOString(),
    duration_ns: duration.toString(),
    duration_ms: Number(duration) / 1_000_000
  };
}

function saveTimings() {
  fs.mkdirSync(artifactDir!, { recursive: true });
  fs.writeFileSync(path.join(artifactDir!, 'timings.json'), `${JSON.stringify(timingReport, null, 2)}\n`);
  const lines = [
    `tag=${timingReport.tag}`,
    `file_count=${timingReport.file_count}`,
    timingReport.upload ? `upload_duration_ns=${timingReport.upload.duration_ns}` : 'upload_duration_ns=not-completed',
    timingReport.upload ? `upload_duration_ms=${timingReport.upload.duration_ms.toFixed(3)}` : 'upload_duration_ms=not-completed',
    timingReport.delete_from_disk ? `delete_from_disk_duration_ns=${timingReport.delete_from_disk.duration_ns}` : 'delete_from_disk_duration_ns=not-completed',
    timingReport.delete_from_disk ? `delete_from_disk_duration_ms=${timingReport.delete_from_disk.duration_ms.toFixed(3)}` : 'delete_from_disk_duration_ms=not-completed'
  ];
  fs.writeFileSync(path.join(artifactDir!, 'timings.txt'), `${lines.join('\n')}\n`);
}

function datasetFiles(): string[] {
  const manifest = JSON.parse(fs.readFileSync(path.join(datasetDir!, 'manifest.json'), 'utf8')) as {
    count: number;
    files: Array<{ name: string }>;
  };
  expect(manifest.count).toBe(fileCount);
  expect(manifest.files).toHaveLength(fileCount);
  return manifest.files.map((entry) => path.join(datasetDir!, entry.name));
}

function successfulUploadCount(summary: string): number {
  if (!summary) return -1;
  let total = 0;
  for (const part of summary.split(' / ')) {
    const match = part.match(/^(\d+) (imported|uploaded)$/);
    if (!match) return -1;
    total += Number(match[1]);
  }
  return total;
}

async function waitForRemovalOperation(page: import('@playwright/test').Page, operationID: string) {
  const deadline = Date.now() + operationTimeout;
  for (;;) {
    const response = await page.request.get(`/api/v1/operations/${encodeURIComponent(operationID)}`);
    if (!response.ok()) {
      throw new Error(`Unable to read file removal status (HTTP ${response.status()}).`);
    }
    const operation = await response.json() as RemovalOperation;
    if (operation.status === 'completed') return;
    if (operation.status === 'failed') throw new Error(operation.error_message || 'File removal failed.');
    if (operation.status === 'canceled') throw new Error('File removal was canceled.');
    if (Date.now() >= deadline) throw new Error(`File removal operation ${operationID} did not complete before timeout.`);
    await page.waitForTimeout(100);
  }
}

async function verifyFirstPageThumbnails(page: import('@playwright/test').Page, expectedCount: number) {
  const viewport = page.getByTestId('library-viewport');
  const grid = page.getByTestId('virtual-media-grid');
  const cards = grid.locator('.thumb');
  const seen = new Set<string>();
  await viewport.evaluate((node) => node.scrollTo({ top: 0 }));

  for (let step = 0; step < expectedCount * 4 && seen.size < expectedCount; step += 1) {
    await expect.poll(async () => {
      return await cards.evaluateAll((renderedCards) => {
        const viewportNode = document.querySelector('[data-testid="library-viewport"]');
        if (!(viewportNode instanceof HTMLElement)) return false;
        const viewportRect = viewportNode.getBoundingClientRect();
        const visibleCards = renderedCards.filter((card) => {
          const rect = card.getBoundingClientRect();
          return rect.bottom > viewportRect.top && rect.top < viewportRect.bottom;
        });
        return visibleCards.length > 0 && visibleCards.every((card) => {
          const img = card.querySelector('img');
          return img instanceof HTMLImageElement && img.complete && img.naturalWidth > 0 && img.naturalHeight > 0;
        });
      });
    }, { timeout: operationTimeout, message: 'every visible first-page card should have a loadable thumbnail' }).toBe(true);

    const names = await cards.evaluateAll((renderedCards) => {
      const viewportNode = document.querySelector('[data-testid="library-viewport"]');
      if (!(viewportNode instanceof HTMLElement)) return [];
      const viewportRect = viewportNode.getBoundingClientRect();
      return renderedCards.flatMap((card) => {
        const rect = card.getBoundingClientRect();
        if (rect.bottom <= viewportRect.top || rect.top >= viewportRect.bottom) return [];
        const img = card.querySelector('img');
        return img instanceof HTMLImageElement && img.alt ? [img.alt] : [];
      });
    });
    for (const name of names) seen.add(name);

    const scroll = await viewport.evaluate((node) => ({
      top: node.scrollTop,
      height: node.clientHeight,
      scrollHeight: node.scrollHeight
    }));
    if (scroll.top + scroll.height >= scroll.scrollHeight - 2) break;
    await viewport.evaluate((node) => node.scrollBy({ top: Math.max(1, Math.floor(node.clientHeight * 0.25)) }));
  }

  expect(seen.size, 'every item on the first library page should produce a loadable thumbnail').toBe(expectedCount);
  await viewport.evaluate((node) => node.scrollTo({ top: 0 }));
}

type Page = import('@playwright/test').Page;

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('button', { name: 'Upload', exact: true })).toBeVisible();
}

async function uploadTaggedFiles(page: Page, files: string[], uploadTag: string, chunkSize?: number): Promise<Timing> {
  const segmentIndices: number[] = [];
  const reservedSegmentCounts: number[] = [];
  let normalMultipartRequests = 0;
  const trackUploadRequest = (request: import('@playwright/test').Request) => {
    if (request.method() !== 'POST' || new URL(request.url()).pathname !== '/api/v1/uploads') return;
    const headers = request.headers();
    if (headers['x-gooru-upload-reserve'] === 'true' && headers['x-gooru-upload-segment-count']) {
      reservedSegmentCounts.push(Number(headers['x-gooru-upload-segment-count']));
    }
    if (headers['x-gooru-upload-segment-index'] !== undefined) {
      segmentIndices.push(Number(headers['x-gooru-upload-segment-index']));
    } else if (headers['x-gooru-upload-reserve'] !== 'true') {
      normalMultipartRequests += 1;
    }
  };

  if (chunkSize !== undefined) {
    page.on('request', trackUploadRequest);
    await page.evaluate((forcedChunkSize) => {
      (window as unknown as { __gooruUploadChunkSize?: number }).__gooruUploadChunkSize = forcedChunkSize;
    }, chunkSize);
  }

  try {
    await page.getByRole('button', { name: 'Upload', exact: true }).click();
    const clearDone = page.getByRole('button', { name: 'Clear done' });
    if (await clearDone.isVisible()) await clearDone.click();

    const tagInput = page.getByLabel('Initial tags');
    await tagInput.fill(uploadTag);
    await tagInput.press('Enter');
    await expect(page.getByRole('button', { name: `Remove ${uploadTag}` })).toBeVisible();

    await page.locator('input[type="file"]').setInputFiles(files);
    const uploadButton = page.getByRole('button', { name: `Upload ${files.length} files` });
    await expect(uploadButton).toBeEnabled({ timeout: operationTimeout });

    const uploadStartedAt = new Date();
    const uploadStartNs = process.hrtime.bigint();
    await uploadButton.click();
    const uploadQueue = page.locator('section.upload-queue-section');
    await expect(uploadQueue).toBeVisible({ timeout: operationTimeout });
    if (chunkSize === undefined) {
      await expect.poll(async () => {
        return successfulUploadCount((await uploadQueue.getAttribute('aria-label')) ?? '');
      }, { timeout: operationTimeout, message: `all ${files.length} uploads should reach a successful terminal state` }).toBe(files.length);
    } else {
      const segmentCount = Math.ceil(files.length / chunkSize);
      const expectedRequests = segmentCount > 1
        ? { reservedSegmentCounts: [segmentCount], segmentIndices: Array.from({ length: segmentCount }, (_, index) => index), normalMultipartRequests: 0 }
        : { reservedSegmentCounts: [], segmentIndices: [], normalMultipartRequests: 1 };
      await expect.poll(() => ({
        reservedSegmentCounts: [...reservedSegmentCounts],
        segmentIndices: [...segmentIndices],
        normalMultipartRequests
      }), { timeout: operationTimeout, message: `chunk size ${chunkSize} should use the expected multipart request shape` }).toEqual(expectedRequests);
    }

    return elapsed(uploadStartedAt, uploadStartNs);
  } finally {
    if (chunkSize !== undefined) {
      page.off('request', trackUploadRequest);
      await page.evaluate(() => {
        delete (window as unknown as { __gooruUploadChunkSize?: number }).__gooruUploadChunkSize;
      });
    }
  }
}

async function browseAndDeleteTaggedFiles(page: Page, uploadTag: string, expectedCount: number, verifyThumbnails = false): Promise<Timing> {
  await page.locator('.sidebar button.sidebar-item').filter({ hasText: 'Library' }).click();
  const search = page.getByLabel('Search library');
  await search.fill(uploadTag);
  await search.press('Enter');

  const formattedCount = expectedCount.toLocaleString('en-US');
  await expect(page.getByTestId('library-header-count')).toHaveText(`${formattedCount} matching · ${formattedCount} files`, { timeout: operationTimeout });

  const refreshResults = page.getByRole('button', { name: /new items · Refresh results$/ });
  if (await refreshResults.isVisible()) await refreshResults.click();

  const grid = page.getByTestId('virtual-media-grid');
  await expect(grid.locator('.thumb')).not.toHaveCount(0, { timeout: operationTimeout });
  if (verifyThumbnails) await verifyFirstPageThumbnails(page, Math.min(expectedCount, 100));

  await grid.locator('.thumb-checkbox').first().click();
  const selectAll = page.getByRole('button', { name: new RegExp(`Select all ${formattedCount}$`) });
  await selectAll.click();
  await expect(page.locator('.selection-summary')).toContainText(`${expectedCount} selected`, { timeout: operationTimeout });

  await page.getByRole('button', { name: 'Delete', exact: true }).click();
  await expect(page.getByRole('dialog')).toContainText(`Permanently delete ${expectedCount} selected file`);
  const confirmDelete = page.getByRole('button', { name: 'Delete files', exact: true });
  await expect(confirmDelete).toBeEnabled();

  const deleteStartedAt = new Date();
  const deleteStartNs = process.hrtime.bigint();
  const removalAdmissionPromise = page.waitForResponse((response) => {
    const request = response.request();
    return request.method() === 'DELETE' && new URL(response.url()).pathname === '/api/v1/files';
  });
  await confirmDelete.click();
  const removalAdmission = await removalAdmissionPromise;
  expect(removalAdmission.ok(), 'bulk delete admission should succeed').toBe(true);
  const removalPayload = await removalAdmission.json() as { operation_id?: string };
  expect(removalPayload.operation_id, 'bulk delete should return an async operation id').toBeTruthy();
  await waitForRemovalOperation(page, removalPayload.operation_id!);
  const deletionTiming = elapsed(deleteStartedAt, deleteStartNs);

  // Deletion is asynchronous and the live grid is not required to refresh itself.
  // Verify persistence only after a full refresh, then re-apply the initial-tag search.
  await page.reload();
  const refreshedSearch = page.getByLabel('Search library');
  await refreshedSearch.fill(uploadTag);
  await refreshedSearch.press('Enter');
  await expect(page.getByRole('heading', { name: 'No results' })).toBeVisible({ timeout: operationTimeout });
  await expect(page.locator('[data-testid="virtual-media-grid"] .thumb')).toHaveCount(0);
  await expect(page.getByTestId('library-header-count')).toHaveText('0 matching · 0 files', { timeout: operationTimeout });

  return deletionTiming;
}

test.describe.configure({ mode: 'serial' });
test.setTimeout(operationTimeout + 5 * 60 * 1000);

test('upload, browse, thumbnail, and delete a stable mixed-media corpus', async ({ page }) => {
  saveTimings();
  try {
    await signIn(page);
    const files = datasetFiles();

    timingReport.upload = await uploadTaggedFiles(page, files, tag);
    saveTimings();

    timingReport.delete_from_disk = await browseAndDeleteTaggedFiles(page, tag, fileCount, true);
    saveTimings();
  } finally {
    saveTimings();
  }
});

test.only('upload the same 20 files with chunk size 20, then chunk size 4', async ({ page }) => {
  await signIn(page);
  const files = datasetFiles().slice(0, 20);
  expect(files).toHaveLength(20);

  for (const chunkSize of [20, 4] as const) {
    await test.step(`upload 20 files with chunk size ${chunkSize}`, async () => {
      const chunkTag = `${tag}:chunk-${chunkSize}`;
      await uploadTaggedFiles(page, files, chunkTag, chunkSize);
      await browseAndDeleteTaggedFiles(page, chunkTag, files.length);
    });
  }
});
