import { expect, test, type Page, type Route } from '@playwright/test';

const session = {
  user: { id: 'usr_private', username: 'private-user', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-private'
};

type URLState = {
  query?: string;
  kind?: string;
  sort?: string;
  order?: string;
  file_id?: string;
};

async function mockProtectedApp(page: Page) {
  const states = new Map<string, URLState>();
  const fileSearches: Array<{ url: string; method: string; body: Record<string, unknown> }> = [];
  const suggestionSearches: Array<{ url: string; method: string; body: Record<string, unknown> }> = [];
  let stateSequence = 0;

  const json = (route: Route, body: unknown, status = 200) =>
    route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;

    if (path === '/api/v1/auth/me') return json(route, session);
    if (path === '/api/v1/ui-config') {
      return json(route, { protected_mode: true, opaque_url_state: true });
    }
    if (path === '/api/v1/files/search') {
      const body = request.postDataJSON() as Record<string, unknown>;
      fileSearches.push({ url: request.url(), method: request.method(), body });
      return json(route, { files: [], total_count: 0, library_count: 0, facets: { kind: [] } });
    }
    if (path === '/api/v1/search/suggestions') {
      const body = request.postDataJSON() as Record<string, unknown>;
      suggestionSearches.push({ url: request.url(), method: request.method(), body });
      return json(route, { items: [] });
    }
    if (path === '/api/v1/ui-state' && request.method() === 'POST') {
      const state = request.postDataJSON() as URLState;
      const token = `opaque-${++stateSequence}`;
      states.set(token, state);
      return json(route, { token }, 201);
    }
    if (path.startsWith('/api/v1/ui-state/') && request.method() === 'GET') {
      const token = decodeURIComponent(path.slice('/api/v1/ui-state/'.length));
      const state = states.get(token);
      if (token === 'opaque-1') await new Promise((resolve) => setTimeout(resolve, 120));
      return state ? json(route, state) : json(route, { error: { code: 'not_found', message: 'not found' } }, 404);
    }
    if (path === '/api/v1/saved-searches') return json(route, { items: [] });
    if (path === '/api/v1/upload-targets') return json(route, { items: [] });
    if (path === '/api/v1/tags') {
      return json(route, { tags: [], library_count: 0, facets: { kind: [] } });
    }

    return json(route, { error: { code: 'unexpected_test_request', message: `${request.method()} ${path}` } }, 404);
  });

  return { states, fileSearches, suggestionSearches };
}

async function commitSearchToken(page: Page, token: string) {
  const search = page.getByLabel('Search library');
  await search.fill(token);
  await search.press('Enter');
}

async function expectCommittedTokens(page: Page, expected: string[]) {
  const pills = page.locator('.searchbar-pill');
  await expect(pills).toHaveCount(expected.length);
  await expect.poll(async () => (await pills.allTextContents()).map((text) => text.trim())).toEqual(expected);
}

test('protected mode keeps free-form search out of request URLs and restores opaque browser history', async ({ page }) => {
  const traffic = await mockProtectedApp(page);
  const firstToken = 'person:alice';
  const secondToken = 'namespace:secret-private-value';
  const firstQuery = firstToken;
  const secondQuery = `${firstToken} ${secondToken}`;

  await page.goto('/');
  const search = page.getByLabel('Search library');
  await expect(search).toBeVisible();

  await commitSearchToken(page, firstToken);
  await expect.poll(() => traffic.fileSearches.some((entry) => entry.body.query === firstQuery)).toBe(true);
  await expect.poll(() => new URL(page.url()).searchParams.get('state') ?? '').toMatch(/^opaque-/);

  const firstURL = page.url();
  expect(firstURL).not.toContain('alice');
  expect(firstURL).not.toContain('private');
  expect(traffic.fileSearches.every((entry) => entry.method === 'POST')).toBe(true);
  expect(traffic.fileSearches.every((entry) => !entry.url.includes('alice') && !entry.url.includes('private'))).toBe(true);
  expect(traffic.suggestionSearches.every((entry) => entry.method === 'POST')).toBe(true);
  expect(traffic.suggestionSearches.every((entry) => !entry.url.includes('alice') && !entry.url.includes('private'))).toBe(true);

  await page.reload();
  await expectCommittedTokens(page, [firstToken]);
  expect(page.url()).toBe(firstURL);

  await commitSearchToken(page, secondToken);
  await expect.poll(() => traffic.fileSearches.some((entry) => entry.body.query === secondQuery)).toBe(true);
  await expect.poll(() => page.url() !== firstURL && (new URL(page.url()).searchParams.get('state') ?? '').startsWith('opaque-')).toBe(true);
  const secondURL = page.url();
  expect(secondURL).not.toContain('secret');
  expect(secondURL).not.toContain('private');
  await expectCommittedTokens(page, [firstToken, secondToken]);

  await page.goBack();
  await expectCommittedTokens(page, [firstToken]);
  expect(page.url()).toBe(firstURL);

  await page.goForward();
  await expectCommittedTokens(page, [firstToken, secondToken]);
  expect(page.url()).toBe(secondURL);

  // A slow restore for the older history entry must not overwrite a newer one.
  await page.goBack({ waitUntil: 'commit' });
  await page.goForward({ waitUntil: 'commit' });
  await expectCommittedTokens(page, [firstToken, secondToken]);
  await page.waitForTimeout(180);
  await expectCommittedTokens(page, [firstToken, secondToken]);
  expect(page.url()).toBe(secondURL);
});
