import { expect, test, type Page, type Route } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page) {
  let loggedIn = false;
  let serverTotal = 75;
  let gridRequests = 0;
  let metadataRequests = 0;
  let tagsRequests = 0;
  let jobsRequests = 0;

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
  await page.route('**/api/v1/files?**', async (route) => {
    const params = new URL(route.request().url()).searchParams;
    const limit = Number(params.get('limit') ?? 0);
    const includeFacets = params.get('include_facets') === 'true';
    const queryTotal = params.get('query') ? 12 : serverTotal;
    if (limit > 1) gridRequests += 1;
    else metadataRequests += 1;
    // Mirror the real files API: without aggregate metadata total_count is only
    // a pagination lower bound, not the authoritative query result count.
    const lowerBoundTotal = queryTotal > limit ? limit + 1 : queryTotal;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [],
        total_count: includeFacets ? queryTotal : lowerBoundTotal,
        ...(includeFacets ? {
          library_count: serverTotal,
          facets: { kind: queryTotal ? [{ value: 'image', count: queryTotal }] : [] }
        } : {})
      })
    });
  });
  await page.route('**/api/v1/jobs**', async (route) => {
    jobsRequests += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], active_count: 0 }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => {
    tagsRequests += 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        tags: serverTotal ? [{ name: 'fresh', value: 'fresh', namespace: '', count: serverTotal }] : [],
        library_count: serverTotal,
        facets: { kind: serverTotal ? [{ value: 'image', count: serverTotal }] : [] }
      })
    });
  });
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));

  return {
    setServerTotal(value: number) { serverTotal = value; },
    gridRequests: () => gridRequests,
    metadataRequests: () => metadataRequests,
    tagsRequests: () => tagsRequests,
    jobsRequests: () => jobsRequests
  };
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
}

function librarySidebar(page: Page) {
  return page.locator('.sidebar button.sidebar-item').filter({ hasText: 'Library' });
}

test('active uploads refresh authoritative counts without churning grid pages and expose an explicit results refresh', async ({ page }) => {
  const server = await mockApp(page);
  let pendingUpload: Route | undefined;
  await page.route('**/api/v1/uploads', async (route) => {
    pendingUpload = route;
  });

  await signIn(page);
  await expect(librarySidebar(page)).toContainText('75');
  await expect(page.getByTestId('library-header-count')).toHaveText('75 files');

  const search = page.getByLabel('Search library');
  await search.fill('alpha');
  await search.press('Enter');
  await expect(page.getByTestId('library-header-count')).toHaveText('12 matching · 75 files');
  await page.getByLabel('Clear search').click();
  await expect(page.getByTestId('library-header-count')).toHaveText('75 files');

  const initialGridRequests = server.gridRequests();
  const initialMetadataRequests = server.metadataRequests();
  const initialTagsRequests = server.tagsRequests();
  const initialJobsRequests = server.jobsRequests();

  await page.getByRole('button', { name: 'Upload' }).click();
  await page.locator('input[type="file"]').setInputFiles({
    name: 'fresh.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('fresh')
  });
  await page.getByRole('button', { name: /Upload 1 file/ }).click();
  await expect.poll(() => Boolean(pendingUpload)).toBe(true);

  server.setServerTotal(76);
  await librarySidebar(page).click();

  await expect.poll(() => server.tagsRequests(), { timeout: 5_500 }).toBeGreaterThan(initialTagsRequests);
  await expect.poll(() => server.jobsRequests(), { timeout: 5_500 }).toBeGreaterThan(initialJobsRequests);
  await expect.poll(() => server.metadataRequests(), { timeout: 5_500 }).toBeGreaterThan(initialMetadataRequests);
  await expect(librarySidebar(page)).toContainText('76');
  await expect(page.getByTestId('library-header-count')).toHaveText('76 files');
  const refreshButton = page.getByTestId('refresh-upload-results');
  await expect(refreshButton).toHaveText('1 new item · Refresh results');
  const [refreshBackground, accentBackground] = await Promise.all([
    refreshButton.evaluate((node) => getComputedStyle(node).backgroundColor),
    page.evaluate(() => {
      const root = document.querySelector('.gooru-root');
      if (!root) throw new Error('missing gooru root');
      const probe = document.createElement('div');
      probe.style.background = 'var(--accent)';
      root.append(probe);
      const background = getComputedStyle(probe).backgroundColor;
      probe.remove();
      return background;
    })
  ]);
  expect(refreshBackground).toBe(accentBackground);
  expect(server.gridRequests()).toBe(initialGridRequests);

  const gridBeforeExplicitRefresh = server.gridRequests();
  await refreshButton.click();
  await expect.poll(() => server.gridRequests()).toBeGreaterThan(gridBeforeExplicitRefresh);
  await expect(refreshButton).toHaveCount(0);
  await expect(page.getByTestId('library-header-count')).toHaveText('76 files');

  await pendingUpload!.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ files: [{ name: 'fresh.jpg', size: 5, target_id: 'default', status: 'imported' }], affected_count: 1 })
  });

  const tagsBeforeFinal = server.tagsRequests();
  const jobsBeforeFinal = server.jobsRequests();
  await expect.poll(() => server.tagsRequests()).toBeGreaterThan(tagsBeforeFinal);
  await expect.poll(() => server.jobsRequests()).toBeGreaterThan(jobsBeforeFinal);
});
