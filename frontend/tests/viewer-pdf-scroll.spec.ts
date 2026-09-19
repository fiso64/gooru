import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const pdf = {
  id: 'pdf-one',
  content_id: 'hash-pdf-one',
  name: 'report.pdf',
  safe_display_path: 'library/report.pdf',
  size: 2048,
  modified_time: '2026-05-20T00:00:00Z',
  media_type: 'application/pdf',
  media_kind: 'pdf',
  viewer_support: 'supported',
  metadata: { page_count: 8 },
  tags: [],
  media_urls: {
    thumbnail: '/api/v1/files/pdf-one/thumbnail',
    preview: '/api/v1/files/pdf-one/preview',
    pdf: '/api/v1/files/pdf-one/pdf',
    content: '/api/v1/files/pdf-one/content',
    download: '/api/v1/files/pdf-one/download'
  }
};

async function mockPDFLibrary(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ capabilities: ['preview_images'] })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [pdf], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/pdf-one/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>'
  }));
  await page.route('**/api/v1/files/pdf-one/preview', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>'
  }));
  await page.route('**/api/v1/files/pdf-one/pdf*', async (route) => {
    const query = new URL(route.request().url()).searchParams;
    if (!query.has('page')) {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ page_count: 8, page_url_prefix: '/api/v1/files/pdf-one/pdf?page=' })
      });
      return;
    }
    const pageNumber = Number(query.get('page'));
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="600" height="800"><rect width="600" height="800"/><text x="20" y="40">' + pageNumber + '</text></svg>'
    });
  });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('PDF opens as a continuous lazy scrolling document rather than an iframe or comic pager', async ({ page }) => {
  await mockPDFLibrary(page);
  await page.getByRole('button', { name: 'Preview report.pdf' }).click();

  const viewport = page.getByRole('region', { name: 'PDF document: report.pdf' });
  await expect(viewport).toBeVisible();
  await expect(page.locator('.pdf-scroll-page')).toHaveCount(6);
  await expect(page.getByRole('img', { name: 'report.pdf, page 1 of 8' })).toBeVisible();
  await expect(page.locator('.viewer-stage iframe')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Seek comic page' })).toHaveCount(0);

  await viewport.evaluate((element) => { element.scrollTop = element.scrollHeight; });
  await expect(page.locator('.pdf-scroll-page')).toHaveCount(8);
  await viewport.evaluate((element) => { element.scrollTop = element.scrollHeight; });
  await expect(page.getByRole('img', { name: 'report.pdf, page 8 of 8' })).toBeVisible();
});
