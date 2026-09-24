import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page, fileQueries: string[], tagLimits: string[] = []) {
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => {
    fileQueries.push(new URL(route.request().url()).searchParams.get('query') ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
    });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'saved-1', name: 'Favorites', query: 'rating:5', sort: 'name', order: 'asc' }] })
  }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => {
    const params = new URL(route.request().url()).searchParams;
    const limitParam = params.get('limit') ?? '200';
    const limit = Number(limitParam);
    tagLimits.push(limitParam);
    const tags = [
      ...Array.from({ length: 24 }, (_, index) => ({ name: `tag-${String(index + 1).padStart(2, '0')}`, count: index + 1 })),
      { namespace: 'artist', value: 'alice', count: 99 }
    ].reverse();
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ tags: limit > 0 ? tags.slice(0, limit) : tags, library_count: 0, facets: { kind: [] } })
    });
  });
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
}

test('common tags rank visually, persist collapse state, and run a tag search', async ({ page }) => {
  const fileQueries: string[] = [];
  await mockApp(page, fileQueries);
  await page.goto('/');

  let common = page.locator('.common-tags-section');
  await expect(common.getByText('Common tags')).toBeVisible();
  await expect(page.locator('.saved-searches-section + .common-tags-section')).toHaveCount(1);
  const commonToggle = common.locator(':scope > .common-tags-toggle');
  const commonHeader = commonToggle.locator(':scope > .sidebar-section-head');
  const savedHeader = page.locator('.saved-searches-section > .sidebar-section-head');
  await expect(commonToggle).toHaveJSProperty('tagName', 'BUTTON');
  await expect(commonToggle).toHaveAttribute('aria-expanded', 'true');
  await expect(commonHeader).toHaveCount(1);
  const indicator = commonHeader.locator('.common-tags-chevron');
  await expect(indicator).toBeVisible();
  expect(await indicator.evaluate((node) => {
    const box = node.getBoundingClientRect();
    const style = getComputedStyle(node);
    return box.width > 0 && box.height > 0 && style.color !== 'rgba(0, 0, 0, 0)';
  })).toBe(true);
  const commonHeaderStyle = await commonHeader.evaluate((node) => {
    const style = getComputedStyle(node);
    return [style.fontSize, style.fontWeight, style.letterSpacing, style.textTransform, style.color, style.paddingLeft, style.paddingRight];
  });
  const savedHeaderStyle = await savedHeader.evaluate((node) => {
    const style = getComputedStyle(node);
    return [style.fontSize, style.fontWeight, style.letterSpacing, style.textTransform, style.color, style.paddingLeft, style.paddingRight];
  });
  expect(commonHeaderStyle).toEqual(savedHeaderStyle);

  let rows = common.locator('button.common-tag-item');
  await expect(rows).toHaveCount(20);
  await expect(rows.first()).toContainText('artist:alice');
  await expect(rows.first()).toContainText('99');
  await expect(common.getByRole('button', { name: /tag-24 24/ })).toBeVisible();
  await expect(common.getByRole('button', { name: /tag-06 6/ })).toBeVisible();
  await expect(common.getByText('tag-05', { exact: true })).toHaveCount(0);

  await expect(rows.first()).toHaveAttribute('style', /--common-tag-rank: 100%/);
  await expect(rows.last()).toHaveAttribute('style', /--common-tag-rank: 0%/);
  const firstColor = await rows.first().evaluate((node) => getComputedStyle(node).color);
  const lastColor = await rows.last().evaluate((node) => getComputedStyle(node).color);
  expect(firstColor).not.toBe(lastColor);

  await common.getByText('Common tags', { exact: true }).click();
  await expect(commonToggle).toHaveAttribute('aria-expanded', 'false');
  await expect(common.locator('#common-tags-list')).not.toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem('gooru.preference.v1.common-tags.collapsed'))).toBe('true');

  await page.reload();
  common = page.locator('.common-tags-section');
  await expect(common.getByRole('button', { name: 'Expand Common tags' })).toBeVisible();
  await expect(common.locator('#common-tags-list')).not.toBeVisible();

  await common.getByRole('button', { name: 'Expand Common tags' }).click();
  await expect(common.getByRole('button', { name: 'Collapse Common tags' })).toHaveAttribute('aria-expanded', 'true');
  rows = common.locator('button.common-tag-item');
  await expect(rows).toHaveCount(20);
  await expect(rows.first()).toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem('gooru.preference.v1.common-tags.collapsed'))).toBe('false');

  await rows.first().click();
  await expect.poll(() => fileQueries.includes('artist:alice')).toBe(true);
});

test('fetches only sidebar tags until the complete Tags view is opened', async ({ page }) => {
  const fileQueries: string[] = [];
  const tagLimits: string[] = [];
  await mockApp(page, fileQueries, tagLimits);
  await page.goto('/');

  await expect.poll(() => tagLimits.includes('20')).toBe(true);
  expect(tagLimits).not.toContain('0');
  await expect(page.locator('.common-tags-section button.common-tag-item')).toHaveCount(20);

  await page.getByTestId('nav-tab-tags').click();

  await expect.poll(() => tagLimits.includes('0')).toBe(true);
  await expect(page.locator('main .tagscloud-item')).toHaveCount(25);
  await expect(page.locator('main h1')).toHaveText('25 tags across 0 files');
});
