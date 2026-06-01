import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockAuth(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: loggedIn ? 200 : 401,
      contentType: 'application/json',
      body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
    });
  });
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({
      contentType: 'application/json',
      headers: { 'Set-Cookie': 'gooru_session=session-one; Path=/; HttpOnly; SameSite=Lax' },
      body: JSON.stringify(session)
    });
  });
}

async function signIn(page: Page) {
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
}

test('renders the library shell', async ({ page }) => {
  await mockAuth(page);
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByLabel('Password')).toBeVisible();
  await expect(page.getByPlaceholder('tag, key:value, @tagged')).toBeVisible();
});

test('renders authenticated thumbnail results', async ({ page }) => {
  await mockAuth(page);
  const requests: string[] = [];
  const thumbnailRequests: string[] = [];
  const previewRequests: string[] = [];
  const contentRequests: string[] = [];
  const mutations: Array<{ authorization: string; csrf: string; method: string; body: unknown }> = [];
  await page.route('**/api/v1/files?**', async (route) => {
    requests.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [
          {
            id: 'bG9jOjE',
            content_id: 'hash-one',
            name: 'sample.jpg',
            size: 2048,
            modified_time: '2026-05-20T00:00:00Z',
            media_type: 'image/jpeg',
            media_kind: 'image',
            tags: ['rating:safe', 'blue'],
            media_urls: {
              thumbnail: '/api/v1/files/bG9jOjE/thumbnail',
              preview: '/api/v1/files/bG9jOjE/preview',
              content: '/api/v1/files/bG9jOjE/content'
            }
          }
        ]
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail?**', async (route) => {
    thumbnailRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#34d399"/></svg>'
    });
  });
  await page.route('**/api/v1/files/*/preview', async (route) => {
    previewRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64" fill="#064e3b"/><circle cx="48" cy="32" r="20" fill="#34d399"/></svg>'
    });
  });
  await page.context().route('**/api/v1/files/*/content', async (route) => {
    contentRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({ status: 500, body: 'content should not be fetched for preview rendering' });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    mutations.push({
      authorization: route.request().headers().authorization ?? '',
      csrf: route.request().headers()['x-gooru-csrf'] ?? '',
      method: route.request().method(),
      body: route.request().postDataJSON()
    });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        operation: 'add',
        selector: { file_ids: ['bG9jOjE'] },
        matched_files: 1,
        affected_count: 1
      })
    });
  });

  await page.goto('/');
  await signIn(page);

  await expect(page.getByRole('heading', { name: 'sample.jpg' })).toBeVisible();
  await expect(page.getByText('rating:safe')).toBeVisible();
  expect(requests).toContain('');
  await expect.poll(() => thumbnailRequests).toContain('');
  await page.getByRole('button', { name: 'Preview sample.jpg' }).click();
  const dialog = page.getByRole('dialog', { name: 'sample.jpg' });
  await expect(dialog).toBeVisible();
  await expect.poll(() => previewRequests).toContain('');
  expect(contentRequests).toEqual([]);
  await expect(dialog.getByRole('img', { name: 'sample.jpg' })).toBeVisible();
  await expect(dialog.getByRole('link', { name: 'Open original sample.jpg' })).toBeVisible();
  await dialog.getByRole('button', { name: 'Close preview' }).click();
  await expect(dialog).toBeHidden();

  await page.getByLabel('Tags for sample.jpg').fill('reviewed');
  await page.getByRole('button', { name: 'Add tags to sample.jpg' }).click();
  await expect.poll(() => mutations.length).toBe(1);
  expect(mutations[0]).toMatchObject({
    authorization: '',
    csrf: 'csrf-one',
    method: 'POST',
    body: { file_ids: ['bG9jOjE'], tags: ['reviewed'] }
  });
});

test('signing in resets auth-scoped file results', async ({ page }) => {
  await mockAuth(page);
  const requests: Array<{ authorization: string; query: string | null }> = [];
  await page.route('**/api/v1/files?**', async (route) => {
    requests.push({
      authorization: route.request().headers().authorization ?? '',
      query: new URL(route.request().url()).searchParams.get('query')
    });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [
          {
            id: 'session-file',
            content_id: 'session-hash',
            name: 'session.jpg',
            size: 2048,
            modified_time: '2026-05-20T00:00:00Z',
            media_type: 'image/jpeg',
            media_kind: 'image',
            tags: ['session'],
            media_urls: {
              thumbnail: '/api/v1/files/session-file/thumbnail',
              preview: '/api/v1/files/session-file/preview',
              content: '/api/v1/files/session-file/content'
            }
          }
        ]
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail?**', async (route) => {
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#34d399"/></svg>'
    });
  });

  await page.goto('/');
  await expect(page.getByText('Sign in to browse this library.')).toBeVisible();
  await signIn(page);
  await expect(page.getByRole('heading', { name: 'session.jpg' })).toBeVisible();
  expect(requests).toContainEqual({ authorization: '', query: null });
});

test('video preview uses bounded preview route without fetching original content', async ({ page }) => {
  await mockAuth(page);
  const thumbnailRequests: string[] = [];
  const previewRequests: string[] = [];
  const contentRequests: Array<{ authorization: string; cookie: string }> = [];
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [
          {
            id: 'bG9jOjI',
            content_id: 'hash-video',
            name: 'sample-video.mp4',
            size: 104857600,
            modified_time: '2026-05-20T00:00:00Z',
            media_type: 'video/mp4',
            media_kind: 'video',
            tags: ['clip'],
            media_urls: {
              thumbnail: '/api/v1/files/bG9jOjI/thumbnail',
              preview: '/api/v1/files/bG9jOjI/preview',
              content: '/api/v1/files/bG9jOjI/content'
            }
          }
        ]
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail?**', async (route) => {
    thumbnailRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#34d399"/></svg>'
    });
  });
  await page.route('**/api/v1/files/*/preview', async (route) => {
    previewRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64" fill="#064e3b"/></svg>'
    });
  });
  await page.context().route('**/api/v1/files/*/content', async (route) => {
    contentRequests.push({
      authorization: route.request().headers().authorization ?? '',
      cookie: route.request().headers().cookie ?? ''
    });
    await route.fulfill({ contentType: 'text/plain', body: 'original content' });
  });

  await page.goto('/');
  await signIn(page);
  await expect.poll(() => thumbnailRequests).toContain('');
  await page.getByRole('button', { name: 'Preview sample-video.mp4' }).click();

  const dialog = page.getByRole('dialog', { name: 'sample-video.mp4' });
  await expect(dialog).toBeVisible();
  await expect.poll(() => previewRequests).toContain('');
  expect(contentRequests).toEqual([]);
  await expect(dialog.getByRole('img', { name: 'sample-video.mp4' })).toBeVisible();
  const openOriginal = dialog.getByRole('link', { name: 'Open original sample-video.mp4' });
  await expect(openOriginal).toHaveAttribute('href', '/api/v1/files/bG9jOjI/content');

  const popupPromise = page.waitForEvent('popup');
  await openOriginal.click();
  const popup = await popupPromise;
  await expect.poll(() => contentRequests.length).toBe(1);
  expect(contentRequests[0]).toMatchObject({
    authorization: '',
    cookie: expect.stringContaining('gooru_session=session-one')
  });
  await popup.close();
});

test('scrolls through paginated results with virtualized infinite grid', async ({ page }) => {
  await mockAuth(page);
  const requests: Array<{ authorization: string; pageToken: string | null }> = [];
  const makeFile = (index: number) => ({
    id: `file-${index}`,
    content_id: `hash-${index}`,
    name: `file-${String(index).padStart(3, '0')}.jpg`,
    size: 1024 + index,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'application/octet-stream',
    media_kind: 'other',
    tags: [`batch:${Math.ceil(index / 36)}`],
    media_urls: {
      thumbnail: `/api/v1/files/file-${index}/thumbnail`,
      preview: `/api/v1/files/file-${index}/preview`,
      content: `/api/v1/files/file-${index}/content`
    }
  });
  await page.setViewportSize({ width: 1280, height: 720 });
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const pageToken = url.searchParams.get('page_token');
    requests.push({
      authorization: route.request().headers().authorization ?? '',
      pageToken
    });
    const start = pageToken === 'page-2' ? 37 : 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: Array.from({ length: 36 }, (_, offset) => makeFile(start + offset)),
        next_page_token: pageToken === 'page-2' ? undefined : 'page-2'
      })
    });
  });

  await page.goto('/');
  await signIn(page);

  await expect(page.getByRole('heading', { name: 'file-001.jpg' })).toBeVisible();
  expect(requests).toContainEqual({ authorization: '', pageToken: null });

  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await expect.poll(() => requests.some((request) => request.pageToken === 'page-2')).toBe(true);
  await expect(page.getByRole('heading', { name: 'file-040.jpg' })).toBeVisible();
  await expect(page.getByText('72 files loaded')).toBeVisible();
  expect(await page.getByRole('heading', { name: /^file-/ }).count()).toBeLessThan(72);
  await expect(page.getByTestId('virtual-media-grid')).toBeVisible();
});

test('uploads with job polling and cancellation', async ({ page }) => {
  await mockAuth(page);
  const uploads: Array<{ authorization: string; csrf: string; prefer: string }> = [];
  const jobGets: string[] = [];
  const jobDeletes: Array<{ authorization: string; csrf: string }> = [];
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [] })
    });
  });
  await page.route('**/api/v1/uploads', async (route) => {
    uploads.push({
      authorization: route.request().headers().authorization ?? '',
      csrf: route.request().headers()['x-gooru-csrf'] ?? '',
      prefer: route.request().headers().prefer ?? ''
    });
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: 'pending' })
    });
  });
  await page.route('**/api/v1/jobs/job-one', async (route) => {
    if (route.request().method() === 'DELETE') {
      jobDeletes.push({
        authorization: route.request().headers().authorization ?? '',
        csrf: route.request().headers()['x-gooru-csrf'] ?? ''
      });
      await route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: 'canceled' })
      });
      return;
    }
    jobGets.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: 'running' })
    });
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByText('Signed in')).toBeVisible();
  await page.locator('input[type="file"]').setInputFiles({
    name: 'upload.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('image')
  });
  await page.getByRole('button', { name: 'Import 1' }).click();

  await expect.poll(() => uploads).toEqual([{ authorization: '', csrf: 'csrf-one', prefer: 'respond-async' }]);
  await expect.poll(() => jobGets).toContain('');
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect.poll(() => jobDeletes).toContainEqual({ authorization: '', csrf: 'csrf-one' });
  await expect(page.getByText('Canceled')).toBeVisible();
});
