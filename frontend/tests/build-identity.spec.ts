import { expect, test, type Page } from '@playwright/test';

async function mockLoginPage(page: Page, build: { version: string; revision: string; dirty: boolean; development: boolean }) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ font_style: 'editorial', load_full_media_by_default: false })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: 401,
    contentType: 'application/json',
    body: JSON.stringify({ error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/build', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(build)
  }));
}

test('release login identity links docs to the version tag and source to the exact revision', async ({ page }) => {
  const revision = '0123456789abcdef0123456789abcdef01234567';
  await mockLoginPage(page, { version: '1.2.3', revision, dirty: false, development: false });

  await page.goto('/');
  await expect(page.getByText('v1.2.3', { exact: true })).toBeVisible();
  await expect(page.locator('.login-v2-first-run').getByRole('link', { name: 'docs' })).toHaveAttribute('href', 'https://github.com/fiso64/gooru/blob/v1.2.3/docs/SERVE.md');
  await expect(page.locator('.login-v2-foot-links').getByRole('link', { name: 'docs' })).toHaveAttribute('href', 'https://github.com/fiso64/gooru/tree/v1.2.3/docs');
  await expect(page.getByRole('link', { name: 'source' })).toHaveAttribute('href', `https://github.com/fiso64/gooru/tree/${revision}`);
});

test('development login identity links docs to the exact revision', async ({ page }) => {
  const revision = 'fedcba9876543210fedcba9876543210fedcba98';
  await mockLoginPage(page, { version: '0.1.0', revision, dirty: false, development: true });

  await page.goto('/');
  await expect(page.getByText('v0.1.0', { exact: true })).toBeVisible();
  await expect(page.locator('.login-v2-foot-links').getByRole('link', { name: 'docs' })).toHaveAttribute('href', `https://github.com/fiso64/gooru/tree/${revision}/docs`);
  await expect(page.getByRole('link', { name: 'source' })).toHaveAttribute('href', `https://github.com/fiso64/gooru/tree/${revision}`);
});
