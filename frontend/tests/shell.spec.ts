import { expect, test } from '@playwright/test';

test('renders the library shell', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByPlaceholder('Paste server token')).toBeVisible();
  await expect(page.getByPlaceholder('tag, key:value, @tagged')).toBeVisible();
});

test('renders authenticated thumbnail results', async ({ page }) => {
  const requests: string[] = [];
  const thumbnailRequests: string[] = [];
  const mutations: Array<{ authorization: string; method: string; body: unknown }> = [];
  await page.route('**/api/v1/files?**', async (route) => {
    requests.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [
          {
            id: 'bG9jOjE',
            content_id: 'hash-one',
            name: 'sample.jpg',
            size: 2048,
            modified_time: '2026-05-20T00:00:00Z',
            media_type: 'image/jpeg',
            media_kind: 'image',
            tags: ['rating:safe', 'blue'],
            media_urls: {
              thumbnail: '/api/v1/files/bG9jOjE/thumbnail',
              preview: '/api/v1/files/bG9jOjE/preview',
              content: '/api/v1/files/bG9jOjE/content'
            }
          }
        ]
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail?**', async (route) => {
    thumbnailRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#34d399"/></svg>'
    });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    mutations.push({
      authorization: route.request().headers().authorization ?? '',
      method: route.request().method(),
      body: route.request().postDataJSON()
    });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        operation: 'add',
        selector: { file_ids: ['bG9jOjE'] },
        matched_files: 1,
        affected_count: 1
      })
    });
  });

  await page.goto('/');
  await page.getByPlaceholder('Paste server token').fill('secret');
  await page.getByRole('button', { name: 'Save' }).click();

  await expect(page.getByRole('heading', { name: 'sample.jpg' })).toBeVisible();
  await expect(page.getByText('rating:safe')).toBeVisible();
  expect(requests).toContain('Bearer secret');
  await expect.poll(() => thumbnailRequests).toContain('Bearer secret');

  await page.getByLabel('Tags for sample.jpg').fill('reviewed');
  await page.getByRole('button', { name: 'Add tags to sample.jpg' }).click();
  await expect.poll(() => mutations.length).toBe(1);
  expect(mutations[0]).toMatchObject({
    authorization: 'Bearer secret',
    method: 'POST',
    body: { file_ids: ['bG9jOjE'], tags: ['reviewed'] }
  });
});

test('uploads with job polling and cancellation', async ({ page }) => {
  const uploads: Array<{ authorization: string; prefer: string }> = [];
  const jobGets: string[] = [];
  const jobDeletes: string[] = [];
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [] })
    });
  });
  await page.route('**/api/v1/uploads', async (route) => {
    uploads.push({
      authorization: route.request().headers().authorization ?? '',
      prefer: route.request().headers().prefer ?? ''
    });
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: 'pending' })
    });
  });
  await page.route('**/api/v1/jobs/job-one', async (route) => {
    if (route.request().method() === 'DELETE') {
      jobDeletes.push(route.request().headers().authorization ?? '');
      await route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: 'canceled' })
      });
      return;
    }
    jobGets.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: 'running' })
    });
  });

  await page.goto('/');
  await page.getByPlaceholder('Paste server token').fill('secret');
  await page.getByRole('button', { name: 'Save' }).click();
  await page.locator('input[type="file"]').setInputFiles({
    name: 'upload.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('image')
  });
  await page.getByRole('button', { name: 'Import 1' }).click();

  await expect.poll(() => uploads).toEqual([{ authorization: 'Bearer secret', prefer: 'respond-async' }]);
  await expect.poll(() => jobGets).toContain('Bearer secret');
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect.poll(() => jobDeletes).toContain('Bearer secret');
  await expect(page.getByText('Canceled')).toBeVisible();
});
