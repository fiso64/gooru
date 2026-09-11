import { expect, test } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const fileCount = Number(process.env.GOORU_SMOKE_FILE_COUNT ?? '2000');
const datasetDir = process.env.GOORU_SMOKE_DATASET_DIR;
const artifactDir = process.env.GOORU_SMOKE_ARTIFACT_DIR;
const username = process.env.GOORU_SMOKE_USERNAME ?? 'smoke-admin';
const password = process.env.GOORU_SMOKE_PASSWORD;
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

test.describe.configure({ mode: 'serial' });
test.setTimeout(operationTimeout + 5 * 60 * 1000);

test('upload, browse, thumbnail, and delete a stable mixed-media corpus', async ({ page }) => {
  saveTimings();
  try {
    await page.goto('/');
    await page.getByLabel('Username').fill(username);
    await page.getByLabel('Password').fill(password);
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByRole('button', { name: 'Upload', exact: true })).toBeVisible();

    await page.getByRole('button', { name: 'Upload', exact: true }).click();
    const tagInput = page.getByLabel('Initial tags');
    await tagInput.fill(tag);
    await tagInput.press('Enter');
    await expect(page.getByRole('button', { name: `Remove ${tag}` })).toBeVisible();

    const files = datasetFiles();
    await page.locator('input[type="file"]').setInputFiles(files);
    const uploadButton = page.getByRole('button', { name: `Upload ${fileCount} files` });
    await expect(uploadButton).toBeEnabled({ timeout: operationTimeout });

    const uploadStartedAt = new Date();
    const uploadStartNs = process.hrtime.bigint();
    await uploadButton.click();
    const clearDone = page.getByRole('button', { name: 'Clear done' });
    await expect(clearDone).toBeEnabled({ timeout: operationTimeout });
    timingReport.upload = elapsed(uploadStartedAt, uploadStartNs);
    saveTimings();

    await page.locator('.sidebar button.sidebar-item').filter({ hasText: 'Library' }).click();
    const search = page.getByLabel('Search library');
    await search.fill(tag);
    await search.press('Enter');

    const formattedCount = fileCount.toLocaleString('en-US');
    await expect(page.getByTestId('library-header-count')).toHaveText(`${formattedCount} matching · ${formattedCount} files`, { timeout: operationTimeout });

    const grid = page.getByTestId('virtual-media-grid');
    await expect(grid.locator('.thumb')).not.toHaveCount(0, { timeout: operationTimeout });
    await expect.poll(async () => {
      return await grid.locator('.thumb img').evaluateAll((images) => images.length > 0 && images.every((image) => {
        const img = image as HTMLImageElement;
        return img.complete && img.naturalWidth > 0 && img.naturalHeight > 0;
      }));
    }, { timeout: operationTimeout, message: 'first-page thumbnails should be generated and load successfully' }).toBe(true);

    await grid.locator('.thumb-checkbox').first().click();
    const selectAll = page.getByRole('button', { name: new RegExp(`Select all ${formattedCount}$`) });
    await selectAll.click();
    await expect(page.locator('.selection-summary')).toContainText(`${formattedCount} selected`, { timeout: operationTimeout });

    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    await expect(page.getByRole('dialog')).toContainText(`Permanently delete ${fileCount} selected file`);
    const confirmDelete = page.getByRole('button', { name: 'Delete files', exact: true });
    await expect(confirmDelete).toBeEnabled();

    const deleteStartedAt = new Date();
    const deleteStartNs = process.hrtime.bigint();
    await confirmDelete.click();
    await expect(page.getByRole('heading', { name: 'No results' })).toBeVisible({ timeout: operationTimeout });
    await expect(page.locator('[data-testid="virtual-media-grid"] .thumb')).toHaveCount(0);
    timingReport.delete_from_disk = elapsed(deleteStartedAt, deleteStartNs);
    saveTimings();

    // This assertion is intentionally made without page.reload(): the mutation must empty the live grid.
    await expect(page.getByTestId('library-header-count')).toHaveText('0 matching · 0 files', { timeout: operationTimeout });
  } finally {
    saveTimings();
  }
});
