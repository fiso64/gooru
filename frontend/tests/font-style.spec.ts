import { expect, test } from '@playwright/test';

async function mockLoggedOutSession(page: import('@playwright/test').Page) {
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'unauthorized', message: 'login required' } })
    });
  });
}

test('comic font preset loads and renders the bundled comic face', async ({ page }) => {
  await mockLoggedOutSession(page);
  await page.route('**/api/v1/ui-config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ font_style: 'comic', load_full_media_by_default: false })
    });
  });

  await page.goto('/');

  const root = page.locator('.gooru-root');
  await expect(root).toHaveClass(/gooru-type-comic/);
  await expect(page.getByLabel('Username')).toBeVisible();
  const typography = await root.evaluate(async (element) => {
    const faces = await document.fonts.load('14px "Comic Neue"', 'gooru');
    const style = getComputedStyle(element);
    return {
      loadedFaces: faces.length,
      family: style.fontFamily,
      display: style.getPropertyValue('--font-display'),
      ui: style.getPropertyValue('--font-ui')
    };
  });
  expect(typography.loadedFaces).toBeGreaterThan(0);
  expect(typography.family).toContain('Comic Neue');
  expect(typography.display).toContain('Comic Neue');
  expect(typography.ui).toContain('Comic Neue');
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
