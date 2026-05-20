import { expect, test } from '@playwright/test';

test('renders the library shell', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByPlaceholder('Paste server token')).toBeVisible();
  await expect(page.getByPlaceholder('tag, key:value, @tagged')).toBeVisible();
});

test('renders authenticated thumbnail results', async ({ page }) => {
  const requests: string[] = [];
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
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#34d399"/></svg>'
    });
  });

  await page.goto('/');
  await page.getByPlaceholder('Paste server token').fill('secret');
  await page.getByRole('button', { name: 'Save' }).click();

  await expect(page.getByRole('heading', { name: 'sample.jpg' })).toBeVisible();
  await expect(page.getByText('rating:safe')).toBeVisible();
  expect(requests).toContain('Bearer secret');
});
