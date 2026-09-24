import { mockFileAround } from './helpers/mockFileAround';
import { expect, test, type Page, type Route } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const dimensions = {
  a: [900, 1400],
  b: [1600, 900],
  c: [700, 1400],
  d: [1800, 900]
} as const;

function fileItem(id: keyof typeof dimensions) {
  const [width, height] = dimensions[id];
  return {
    id,
    content_id: `hash-${id}`,
    name: `${id}.jpg`,
    safe_display_path: `library/${id}.jpg`,
    size: 4096,
    added_at: '2026-05-20T00:00:00Z',
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: width, image_height: height },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

function imageBody(id: keyof typeof dimensions) {
  const [width, height] = dimensions[id];
  const fill = id === 'a' ? '#ff2200' : id === 'b' ? '#00aa33' : id === 'c' ? '#6633ff' : '#0099ff';
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}"><rect width="${width}" height="${height}" fill="${fill}"/></svg>`;
}

async function fulfillImage(route: Route, id: keyof typeof dimensions) {
  await route.fulfill({ contentType: 'image/svg+xml', body: imageBody(id) });
}

async function mockApp(page: Page) {
  const files = (['a', 'b', 'c', 'd'] as const).map(fileItem);
  let loggedIn = false;
  const previewRequests: string[] = [];
  const contentRequests: string[] = [];
  const gates = new Map<string, () => void>();
  const waits = new Map<string, Promise<void>>();
  for (const id of ['b', 'c', 'd'] as const) {
    waits.set(id, new Promise<void>((resolve) => gates.set(id, resolve)));
  }

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await mockFileAround(page, () => files);
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } }) }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));
  await page.route('**/api/v1/files/*/preview', async (route) => {
    const id = route.request().url().match(/\/files\/([^/]+)\/preview/)?.[1] as keyof typeof dimensions;
    previewRequests.push(id);
    const wait = waits.get(id);
    if (wait) await wait;
    await fulfillImage(route, id);
  });
  await page.route('**/api/v1/files/*/content', async (route) => {
    const id = route.request().url().match(/\/files\/([^/]+)\/content/)?.[1] as keyof typeof dimensions;
    contentRequests.push(id);
    await fulfillImage(route, id);
  });

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  return {
    previewRequests,
    contentRequests,
    release: (id: 'b' | 'c' | 'd') => gates.get(id)?.()
  };
}

test('rapid navigation keeps committed pixels until only the latest target can present', async ({ page }) => {
  const app = await mockApp(page);
  await page.getByRole('button', { name: 'Preview a.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  const freeze = page.locator('.viewer-image-freeze');
  await expect(media).toBeVisible();
  await expect.poll(async () => media.evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBe(900);
  const committed = await media.boundingBox();
  expect(committed).not.toBeNull();

  await page.evaluate(() => {
    const benchmark = (window as Window & { __gooruViewerBenchmark?: { enable: () => void } }).__gooruViewerBenchmark;
    if (!benchmark) throw new Error('viewer benchmark controls unavailable');
    benchmark.enable();
  });

  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'd.jpg' })).toBeVisible();
  await expect(media).toHaveAttribute('src', /\/d\/preview/);
  await expect(freeze).toBeVisible();
  await expect(media).toHaveCSS('visibility', 'hidden');

  const frozen = await freeze.boundingBox();
  expect(frozen).not.toBeNull();
  expect(Math.abs(frozen!.width - committed!.width)).toBeLessThan(0.5);
  expect(Math.abs(frozen!.height - committed!.height)).toBeLessThan(0.5);

  app.release('b');
  app.release('c');
  await page.waitForTimeout(100);
  await expect(freeze).toBeVisible();
  await expect(media).toHaveCSS('visibility', 'hidden');

  app.release('d');
  await expect(freeze).toBeHidden();
  await expect(media).toHaveCSS('visibility', 'visible');
  await expect.poll(async () => media.evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBe(1800);
  await expect(page.getByRole('dialog', { name: 'd.jpg' })).toBeVisible();

  const metrics = await page.evaluate(() => {
    const benchmark = (window as Window & { __gooruViewerBenchmark?: { snapshot: () => { presentations: number; pending: number; requestToPresentMs: { p50: number } | null } } }).__gooruViewerBenchmark;
    if (!benchmark) throw new Error('viewer benchmark controls unavailable');
    return benchmark.snapshot();
  });
  expect(metrics.presentations).toBe(1);
  expect(metrics.pending).toBe(0);
  expect(metrics.requestToPresentMs?.p50).toBeGreaterThanOrEqual(0);
});

test('speculative neighbor preload waits for presentation and follows original-media mode', async ({ page }) => {
  const app = await mockApp(page);
  await page.getByRole('button', { name: 'Preview a.jpg' }).click();
  const media = page.locator('.viewer-visual-media');
  await expect.poll(async () => media.evaluate((node) => (node as HTMLImageElement).naturalWidth)).toBe(900);

  // Let the preview-mode neighbor preload start so the mode switch can prove it is cancelled
  // rather than racing with the request-accounting reset below.
  await expect.poll(() => app.previewRequests.includes('b')).toBe(true);
  app.previewRequests.length = 0;
  app.contentRequests.length = 0;

  await page.getByRole('button', { name: 'Use original media' }).click();
  await expect(media).toHaveAttribute('src', /\/a\/content/);
  await expect.poll(() => app.contentRequests.includes('b')).toBe(true);
  expect(app.previewRequests.includes('b')).toBe(false);
});
