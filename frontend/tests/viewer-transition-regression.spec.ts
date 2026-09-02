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

test('presented image node and geometry survive while a different-aspect target loads', async ({ page }) => {
  const { releaseTall } = await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();

  const media = page.locator('.viewer-incoming-media');
  await expect(media).toBeVisible();
  const before = await media.boundingBox();
  expect(before).not.toBeNull();
  expect(before!.width / before!.height).toBeGreaterThan(2);
  await media.evaluate((node) => {
    (window as typeof window & { __presentedViewerNode?: Element }).__presentedViewerNode = node;
  });

  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'tall.jpg' })).toBeVisible();
  await expect(media).toHaveAttribute('src', /\/tall\/preview/);

  // #162 replaced the actually-painted node with a fresh outgoing <img>. On slow
  // images that clone can itself need decode/presentation and expose a blank frame.
  // Keep the same real node mounted instead, with its old geometry frozen.
  expect(await page.evaluate(() => document.querySelector('.viewer-incoming-media') === (window as typeof window & { __presentedViewerNode?: Element }).__presentedViewerNode)).toBe(true);
  await expect(page.locator('.viewer-outgoing-media')).toHaveCount(0);

  const during = await media.boundingBox();
  expect(during).not.toBeNull();
  expect(Math.abs(during!.width - before!.width)).toBeLessThan(0.5);
  expect(Math.abs(during!.height - before!.height)).toBeLessThan(0.5);

  await page.waitForTimeout(75);
  const stillDuring = await media.boundingBox();
  expect(stillDuring).not.toBeNull();
  expect(Math.abs(stillDuring!.width - before!.width)).toBeLessThan(0.5);
  expect(Math.abs(stillDuring!.height - before!.height)).toBeLessThan(0.5);

  releaseTall();
  await expect.poll(async () => {
    const box = await media.boundingBox();
    return box ? box.width / box.height : Number.POSITIVE_INFINITY;
  }).toBeLessThan(1);
  expect(await page.evaluate(() => document.querySelector('.viewer-incoming-media') === (window as typeof window & { __presentedViewerNode?: Element }).__presentedViewerNode)).toBe(true);
});
