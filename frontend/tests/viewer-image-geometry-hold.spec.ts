import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: 'portrait' | 'landscape' | 'missing' | 'text') {
  const portrait = id === 'portrait';
  const text = id === 'text';
  return {
    id,
    content_id: `hash-${id}`,
    name: `${id}.${text ? 'txt' : 'jpg'}`,
    safe_display_path: `library/${id}.jpg`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: text ? 'text/plain' : 'image/jpeg',
    media_kind: text ? 'other' : 'photo',
    viewer_support: text ? 'unsupported_media_type' : 'supported',
    metadata: text ? {} : { image_width: portrait ? 1273 : 2546, image_height: 1800 },
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
  const files = [fileItem('portrait'), fileItem('landscape'), fileItem('missing'), fileItem('text')];
  let loggedIn = false;
  let unsupportedMediaRequests = 0;
  let releaseLandscape!: () => void;
  const landscapeGate = new Promise<void>((resolve) => { releaseLandscape = resolve; });

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } }) }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>' }));
  await page.route('**/api/v1/files/*/preview', async (route) => {
    if (route.request().url().includes('/text/')) {
      unsupportedMediaRequests += 1;
      await route.fulfill({ contentType: 'text/plain', body: 'present text file' });
      return;
    }
    if (route.request().url().includes('/missing/')) {
      await route.fulfill({ status: 404, body: '' });
      return;
    }
    const landscape = route.request().url().includes('/landscape/');
    if (landscape) await landscapeGate;
    const width = landscape ? 2546 : 1273;
    const height = 1800;
    const fill = landscape ? '#0055ff' : '#ff3300';
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}"><rect width="${width}" height="${height}" fill="${fill}"/></svg>`
    });
  });
  await page.route('**/api/v1/files/*/content', async (route) => {
    if (route.request().url().includes('/text/')) {
      unsupportedMediaRequests += 1;
      await route.fulfill({ contentType: 'text/plain', body: 'present text file' });
      return;
    }
    await route.fulfill({ status: 404, body: '' });
  });

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  return { releaseLandscape, unsupportedMediaRequestCount: () => unsupportedMediaRequests };
}

test('different-aspect source handoff is covered by exact old pixels until target paint', async ({ page }) => {
  const { releaseLandscape } = await mockApp(page);
  await page.getByRole('button', { name: 'Preview portrait.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  const freeze = page.locator('.viewer-image-freeze');
  await expect(media).toBeVisible();
  await expect.poll(async () => {
    const box = await media.boundingBox();
    return box ? box.width / box.height : 0;
  }).toBeLessThan(1);
  const before = await media.boundingBox();
  expect(before).not.toBeNull();
  const beforePresentation = await media.evaluate((node) => {
    const style = getComputedStyle(node);
    return { filter: style.filter, borderRadius: style.borderRadius };
  });
  expect(beforePresentation.filter).toContain('drop-shadow');

  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'landscape.jpg' })).toBeVisible();

  await expect(media).toHaveAttribute('src', /\/landscape\/preview/);
  await expect.poll(async () => {
    const box = await media.boundingBox();
    return box ? box.width / box.height : 0;
  }).toBeGreaterThan(1.3);
  await expect(freeze).toBeVisible();
  await expect(media).toHaveCSS('visibility', 'hidden');
  const frozen = await freeze.boundingBox();
  expect(frozen).not.toBeNull();
  expect(Math.abs(frozen!.width - before!.width)).toBeLessThan(0.5);
  expect(Math.abs(frozen!.height - before!.height)).toBeLessThan(0.5);
  const frozenPresentation = await freeze.evaluate((node) => {
    const style = getComputedStyle(node);
    return { filter: style.filter, borderRadius: style.borderRadius };
  });
  expect(frozenPresentation).toEqual(beforePresentation);

  const frozenPixel = await freeze.evaluate((node) => {
    const canvas = node as HTMLCanvasElement;
    return Array.from(canvas.getContext('2d')!.getImageData(canvas.width / 2, canvas.height / 2, 1, 1).data);
  });
  expect(frozenPixel[0]).toBeGreaterThan(200);
  expect(frozenPixel[2]).toBeLessThan(80);

  await page.waitForTimeout(125);
  await expect(freeze).toBeVisible();
  await expect.poll(async () => freeze.evaluate((node) => getComputedStyle(node).filter)).toBe(beforePresentation.filter);

  releaseLandscape();
  await expect(freeze).toBeHidden();
  await expect(media).toHaveCSS('visibility', 'visible');
  await expect.poll(async () => {
    const box = await media.boundingBox();
    return box ? box.width / box.height : 0;
  }).toBeGreaterThan(1.3);
});


test('failed supported image preview releases the frozen previous presentation and reports missing media', async ({ page }) => {
  const { releaseLandscape } = await mockApp(page);
  await page.getByRole('button', { name: 'Preview portrait.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  const freeze = page.locator('.viewer-image-freeze');
  await expect(media).toBeVisible();

  await page.getByLabel('Next file').click();
  releaseLandscape();
  await expect(page.getByRole('dialog', { name: 'landscape.jpg' })).toBeVisible();
  await expect(freeze).toBeHidden();
  await expect.poll(async () => media.evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBe(2546);

  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'missing.jpg' })).toBeVisible();
  await expect.poll(async () => media.evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBe(0);
  await expect(freeze).toBeHidden();
  await expect(media).toHaveCSS('visibility', 'visible');
  await expect(page.getByRole('alert')).toHaveText('Media file could not be loaded. It may be missing from disk.');
});


test('present unsupported file reports viewer capability without attempting media load', async ({ page }) => {
  const { unsupportedMediaRequestCount } = await mockApp(page);
  await page.getByRole('button', { name: 'Preview text.txt' }).click();

  await expect(page.getByRole('dialog', { name: 'text.txt' })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveText('No viewer is available for this file type (text/plain).');
  await expect(page.locator('.viewer-visual-media')).toHaveCount(0);
  expect(unsupportedMediaRequestCount()).toBe(0);
});
