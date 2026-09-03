import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockLoggedOutSession(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'unauthorized', message: 'login required' } })
    });
  });
}

async function mockAuthenticatedLibrary(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
  }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [], library_count: 0, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));
}

async function measuredTextWidth(page: Page, family: string) {
  return await page.evaluate((fontFamily) => {
    const span = document.createElement('span');
    span.textContent = 'Library';
    Object.assign(span.style, {
      position: 'absolute',
      visibility: 'hidden',
      whiteSpace: 'nowrap',
      fontFamily,
      fontSize: '32px',
      fontWeight: '400',
      letterSpacing: '-0.32px'
    });
    document.body.appendChild(span);
    const width = span.getBoundingClientRect().width;
    span.remove();
    return width;
  }, family);
}

test('comic font preset reaches the actual library heading with the bundled face', async ({ page }) => {
  await mockAuthenticatedLibrary(page);
  await page.route('**/api/v1/ui-config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ font_style: 'comic', load_full_media_by_default: false })
    });
  });

  await page.goto('/');

  const root = page.locator('.gooru-root');
  const heading = page.locator('.library-head h1');
  await expect(root).toHaveClass(/gooru-type-comic/);
  await expect(heading).toHaveText('Library');
  await page.evaluate(() => document.fonts.ready);

  const typography = await heading.evaluate(async (element) => {
    const style = getComputedStyle(element);
    const faces = await document.fonts.load(`${style.fontWeight} ${style.fontSize} "Comic Neue"`, element.textContent ?? 'Library');
    return {
      loadedFaces: faces.length,
      family: style.fontFamily,
      width: element.getBoundingClientRect().width
    };
  });
  const comicWidth = await measuredTextWidth(page, '"Comic Neue"');
  const modernWidth = await measuredTextWidth(page, '"IBM Plex Sans"');

  expect(typography.loadedFaces).toBeGreaterThan(0);
  expect(typography.family).toContain('Comic Neue');
  expect(Math.abs(typography.width - comicWidth)).toBeLessThan(0.75);
  expect(Math.abs(typography.width - modernWidth)).toBeGreaterThan(1);
  await expect(page.locator('.gooru-logo-comic')).toBeVisible();
  await expect(page.locator('.gooru-logo img')).toBeHidden();
});

test('modern preset replaces the editorial display face without changing the UI face', async ({ page }) => {
  await mockLoggedOutSession(page);
  await page.route('**/api/v1/ui-config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ font_style: 'modern', load_full_media_by_default: false })
    });
  });

  await page.goto('/');

  const root = page.locator('.gooru-root');
  await expect(root).toHaveClass(/gooru-type-modern/);
  const variables = await root.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      display: style.getPropertyValue('--font-display'),
      ui: style.getPropertyValue('--font-ui')
    };
  });
  expect(variables.display).toContain('IBM Plex Sans');
  expect(variables.ui).toContain('IBM Plex Sans');
  await expect(page.locator('.gooru-logo img')).toBeVisible();
});
