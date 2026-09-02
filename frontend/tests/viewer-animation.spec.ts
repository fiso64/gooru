import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: string, name: string) {
  return {
    id,
    content_id: `hash-${id}`,
    name,
    safe_display_path: `library/${name}`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: id === 'wide' ? 1200 : 400, image_height: id === 'wide' ? 400 : 900 },
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
  let loggedIn = false;
  const files = [fileItem('wide', 'wide.jpg'), fileItem('tall', 'tall.jpg')];

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
    const wide = route.request().url().includes('/wide/');
    const width = wide ? 1200 : 400;
    const height = wide ? 400 : 900;
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
}

test('navigation between differently sized images does not tween width or height', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  await expect(media).toBeVisible();
  await expect.poll(() => media.evaluate((node) => getComputedStyle(node).transitionProperty)).not.toMatch(/(?:^|,\s*)(?:width|height)(?:,|$)/);

  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'tall.jpg' })).toBeVisible();
  await expect(media).toBeVisible();
  expect(await media.evaluate((node) => getComputedStyle(node).transitionProperty)).not.toMatch(/(?:^|,\s*)(?:width|height)(?:,|$)/);
});

test('rotation keeps the requested direction when crossing the 0/360 boundary', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();
  const stage = page.locator('.viewer-stage');
  const media = page.locator('.viewer-visual-media');
  await stage.focus();

  for (let i = 0; i < 4; i += 1) await page.keyboard.press('r');
  await expect(media).toHaveAttribute('style', /rotate\(360deg\)/);

  for (let i = 0; i < 8; i += 1) await page.keyboard.press('l');
  await expect(media).toHaveAttribute('style', /rotate\(-360deg\)/);
});
