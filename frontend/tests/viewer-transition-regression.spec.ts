import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: 'wide' | 'tall') {
  const wide = id === 'wide';
  return {
    id,
    content_id: `hash-${id}`,
    name: `${id}.jpg`,
    safe_display_path: `library/${id}.jpg`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: wide ? 1200 : 400, image_height: wide ? 400 : 900 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page) {
  const files = [fileItem('wide'), fileItem('tall')];
  let loggedIn = false;
  let releaseTall!: () => void;
  const tallGate = new Promise<void>((resolve) => { releaseTall = resolve; });

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>'
  }));
  await page.route('**/api/v1/files/*/preview', async (route) => {
    const tall = route.request().url().includes('/tall/');
    if (tall) await tallGate;
    const width = tall ? 400 : 1200;
    const height = tall ? 900 : 400;
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}"><rect width="${width}" height="${height}"/></svg>`
    });
  });
  await page.route('**/api/v1/files/*/content', async (route) => route.fulfill({ status: 404, body: '' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  return { releaseTall };
}

test('painted image stays untouched while a different-aspect target loads behind it', async ({ page }) => {
  const { releaseTall } = await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();

  const presented = page.locator('.viewer-incoming-media');
  await expect(presented).toBeVisible();
  await expect(presented).toHaveAttribute('src', /\/wide\/preview/);
  const before = await presented.boundingBox();
  expect(before).not.toBeNull();
  expect(before!.width / before!.height).toBeGreaterThan(2);
  await presented.evaluate((node) => {
    (window as typeof window & { __presentedViewerNode?: Element }).__presentedViewerNode = node;
  });

  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'tall.jpg' })).toBeVisible();

  // The target request starts in the inactive slot while the already-painted node
  // remains completely unchanged above it. This is the boundary #162 violated by
  // replacing the painted node with a newly-created clone of the old source.
  await expect(presented).toHaveAttribute('src', /\/wide\/preview/);
  await expect(page.locator('.viewer-buffer-media')).toHaveAttribute('src', /\/tall\/preview/);
  expect(await page.evaluate(() => document.querySelector('.viewer-incoming-media') === (window as typeof window & { __presentedViewerNode?: Element }).__presentedViewerNode)).toBe(true);

  const during = await presented.boundingBox();
  expect(during).not.toBeNull();
  expect(Math.abs(during!.width - before!.width)).toBeLessThan(0.5);
  expect(Math.abs(during!.height - before!.height)).toBeLessThan(0.5);

  await page.waitForTimeout(75);
  const stillDuring = await presented.boundingBox();
  expect(stillDuring).not.toBeNull();
  expect(Math.abs(stillDuring!.width - before!.width)).toBeLessThan(0.5);
  expect(Math.abs(stillDuring!.height - before!.height)).toBeLessThan(0.5);

  releaseTall();
  await expect(page.locator('.viewer-incoming-media')).toHaveAttribute('src', /\/tall\/preview/);
  await expect.poll(async () => {
    const box = await page.locator('.viewer-incoming-media').boundingBox();
    return box ? box.width / box.height : Number.POSITIVE_INFINITY;
  }).toBeLessThan(1);

  // The old painted node was not destroyed; it is now the reusable inactive slot.
  expect(await page.evaluate(() => document.querySelector('.viewer-buffer-media') === (window as typeof window & { __presentedViewerNode?: Element }).__presentedViewerNode)).toBe(true);
});
