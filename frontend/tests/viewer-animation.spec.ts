import { mockFileAround } from './helpers/mockFileAround';
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
  const files = [
    fileItem('wide', 'wide.jpg'),
    fileItem('tall', 'tall.jpg'),
    fileItem('third', 'third.jpg'),
    fileItem('fourth', 'fourth.jpg'),
    fileItem('fifth', 'fifth.jpg'),
    fileItem('sixth', 'sixth.jpg')
  ];

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
  await mockFileAround(page, () => files);
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

test('decoded target geometry is installed in the same render that swaps image source', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  await expect(media).toBeVisible();
  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().width / node.getBoundingClientRect().height)).toBeGreaterThan(2);

  await media.evaluate((node) => {
    const state = window as typeof window & { __viewerAspectAtTallSwap?: number; __viewerSwapObserver?: MutationObserver };
    const observer = new MutationObserver(() => {
      if (!(node instanceof HTMLImageElement) || !node.src.includes('/tall/preview')) return;
      const rect = node.getBoundingClientRect();
      state.__viewerAspectAtTallSwap = rect.width / rect.height;
    });
    observer.observe(node, { attributes: true, attributeFilter: ['src', 'style'] });
    state.__viewerSwapObserver = observer;
  });

  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'tall.jpg' })).toBeVisible();
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __viewerAspectAtTallSwap?: number }).__viewerAspectAtTallSwap ?? Number.POSITIVE_INFINITY)).toBeLessThan(1);
  await page.evaluate(() => {
    const state = window as typeof window & { __viewerSwapObserver?: MutationObserver };
    state.__viewerSwapObserver?.disconnect();
  });
});

test('navigation never collapses the displayed image while swapping decoded sources', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  await expect(media).toBeVisible();
  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().width)).toBeGreaterThan(0);
  await media.evaluate((node) => {
    const state = window as typeof window & { __viewerZeroSizeObserved?: boolean; __viewerSizeObserver?: MutationObserver };
    state.__viewerZeroSizeObserved = false;
    const inspect = () => {
      const rect = node.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) state.__viewerZeroSizeObserved = true;
    };
    const observer = new MutationObserver(inspect);
    observer.observe(node, { attributes: true, attributeFilter: ['style', 'src'] });
    state.__viewerSizeObserver = observer;
  });

  await page.getByLabel('Next file').click();
  await expect(page.getByRole('dialog', { name: 'tall.jpg' })).toBeVisible();
  await expect.poll(() => media.getAttribute('src')).toContain('/tall/preview');
  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().height)).toBeGreaterThan(0);
  expect(await page.evaluate(() => (window as typeof window & { __viewerZeroSizeObserved?: boolean }).__viewerZeroSizeObserved)).toBe(false);
  await page.evaluate(() => {
    const state = window as typeof window & { __viewerSizeObserver?: MutationObserver };
    state.__viewerSizeObserver?.disconnect();
  });
});

test('rapid navigation bounds expensive image predecodes', async ({ page }) => {
  await page.addInitScript(() => {
    const originalDecode = HTMLImageElement.prototype.decode;
    const state = window as typeof window & { __viewerDecodeActive?: number; __viewerDecodeMax?: number };
    state.__viewerDecodeActive = 0;
    state.__viewerDecodeMax = 0;
    HTMLImageElement.prototype.decode = function () {
      if (!this.src.includes('/preview')) return originalDecode ? originalDecode.call(this) : Promise.resolve();
      state.__viewerDecodeActive = (state.__viewerDecodeActive ?? 0) + 1;
      state.__viewerDecodeMax = Math.max(state.__viewerDecodeMax ?? 0, state.__viewerDecodeActive);
      return new Promise<void>((resolve) => {
        setTimeout(() => {
          state.__viewerDecodeActive = Math.max(0, (state.__viewerDecodeActive ?? 1) - 1);
          resolve();
        }, 150);
      });
    };
  });

  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();
  await page.waitForTimeout(250);
  await page.evaluate(() => {
    const state = window as typeof window & { __viewerDecodeActive?: number; __viewerDecodeMax?: number };
    state.__viewerDecodeActive = 0;
    state.__viewerDecodeMax = 0;
  });

  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'sixth.jpg' })).toBeVisible();
  await page.waitForTimeout(200);

  // Latest-wins navigation cancels speculative work on every move and only primes one
  // direction-aware neighbor after the committed target presents, so image decode concurrency
  // should never exceed one during this burst.
  expect(await page.evaluate(() => (window as typeof window & { __viewerDecodeMax?: number }).__viewerDecodeMax ?? 0)).toBeLessThanOrEqual(1);
});

test('held navigation keeps image presentation advancing when full image decode is backlogged', async ({ page }) => {
  await page.addInitScript(() => {
    const originalDecode = HTMLImageElement.prototype.decode;
    HTMLImageElement.prototype.decode = function () {
      if (!this.src.includes('/preview')) return originalDecode ? originalDecode.call(this) : Promise.resolve();
      return new Promise<void>(() => {});
    };
  });

  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  await expect(media).toBeVisible();
  await expect(media).toHaveAttribute('src', /\/wide\/preview/);

  await page.evaluate(() => {
    const target = document.querySelector('.viewer-stage');
    if (!(target instanceof HTMLElement)) throw new Error('viewer stage missing');
    for (let i = 0; i < 5; i += 1) {
      target.dispatchEvent(new KeyboardEvent('keydown', {
        key: 'ArrowRight',
        code: 'ArrowRight',
        repeat: i > 0,
        bubbles: true,
        cancelable: true
      }));
    }
  });

  await expect(page.getByRole('dialog', { name: 'sixth.jpg' })).toBeVisible();
  await expect(media).toHaveAttribute('src', /\/sixth\/preview/);
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
