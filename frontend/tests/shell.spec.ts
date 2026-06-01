import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: string, name: string, kind: 'photo' | 'video' | 'gif' | 'audio' | 'other' = 'photo') {
  return {
    id,
    content_id: `hash-${id}`,
    name,
    safe_display_path: `library/${name}`,
    size: kind === 'video' ? 104857600 : 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: kind === 'video' ? 'video/mp4' : kind === 'audio' ? 'audio/mpeg' : 'image/jpeg',
    media_kind: kind,
    metadata: kind === 'video' ? { video_width: 1920, video_height: 1080, video_duration: 8 } : kind === 'audio' ? { audio_duration: 12 } : { image_width: 800, image_height: 600 },
    tags: ['rating:safe', 'blue'],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

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
  await page.route('**/api/v1/auth/logout', async (route) => {
    loggedIn = false;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });
}

async function mockShellApis(page: Page) {
  await page.route('**/api/v1/jobs', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 's1', name: 'Safe blue', query: 'rating:safe blue' }] }) });
  });
  await page.route('**/api/v1/upload-targets', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) });
  });
  await page.route('**/api/v1/tags?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ tags: [{ name: 'rating:safe', namespace: 'rating', value: 'safe', count: 3 }, { name: 'blue', count: 2 }] })
    });
  });
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
}

async function signIn(page: Page) {
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
}

test('renders login and authenticated concept shell', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
    });
  });

  await page.goto('/');
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByLabel('Password')).toBeVisible();
  await signIn(page);
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Upload' })).toBeVisible();
  await expect(page.getByPlaceholder('tag, key:value, @tagged')).toBeVisible();
  await expect(page.getByText('Settings')).toBeVisible();
  await expect(page.getByText('No results')).toBeVisible();
});

test('renders direct thumbnails, preview, and csrf tag mutation', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const requests: string[] = [];
  const thumbnailRequests: string[] = [];
  const previewRequests: string[] = [];
  const contentRequests: string[] = [];
  const mutations: Array<{ authorization: string; csrf: string; method: string; body: unknown }> = [];
  await page.route('**/api/v1/files?**', async (route) => {
    requests.push(route.request().headers().authorization ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [fileItem('bG9jOjE', 'sample.jpg')], total_count: 1, library_count: 1, facets: { kind: [{ value: 'photo', count: 1 }] } })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    thumbnailRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#f4d976"/></svg>' });
  });
  await page.route('**/api/v1/files/*/preview', async (route) => {
    previewRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64" fill="#111"/></svg>' });
  });
  await page.route('**/api/v1/files/*/content', async (route) => {
    contentRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({ contentType: 'text/plain', body: 'original' });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    mutations.push({
      authorization: route.request().headers().authorization ?? '',
      csrf: route.request().headers()['x-gooru-csrf'] ?? '',
      method: route.request().method(),
      body: route.request().postDataJSON()
    });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ operation: 'add', selector: { file_ids: ['bG9jOjE'] }, affected_count: 1 }) });
  });

  await page.goto('/');
  await signIn(page);
  await expect.poll(() => thumbnailRequests).toContain('');
  expect(requests).toContain('');

  await page.getByRole('button', { name: 'Preview sample.jpg' }).click();
  const dialog = page.getByRole('dialog', { name: 'sample.jpg' });
  await expect(dialog).toBeVisible();
  await expect.poll(() => previewRequests).toContain('');
  expect(contentRequests).toEqual([]);

  await dialog.getByLabel('Tags for sample.jpg').fill('reviewed');
  await dialog.getByRole('button', { name: 'Add tags to sample.jpg' }).click();
  await expect.poll(() => mutations.length).toBe(1);
  expect(mutations[0]).toMatchObject({ authorization: '', csrf: 'csrf-one', method: 'POST', body: { file_ids: ['bG9jOjE'], tags: ['reviewed'] } });
});

test('loads paginated large libraries with bounded virtualized DOM', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const total = 180;
  const pageSize = 60;
  const allFiles = Array.from({ length: total }, (_, index) => fileItem(`file-${index}`, `large-${String(index).padStart(3, '0')}.jpg`));
  const fileRequests: Array<{ token: string; includeFacets: string | null; signalSeen: boolean }> = [];

  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const token = url.searchParams.get('page_token') ?? '';
    const start = token ? Number(token) : 0;
    fileRequests.push({
      token,
      includeFacets: url.searchParams.get('include_facets'),
      signalSeen: true
    });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: allFiles.slice(start, start + pageSize),
        next_page_token: start + pageSize < total ? String(start + pageSize) : '',
        total_count: total,
        library_count: total,
        facets: url.searchParams.get('include_facets') === 'true' ? { kind: [{ value: 'photo', count: total }] } : undefined
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#f4d976"/></svg>' });
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByText('180 files')).toBeVisible();
  await expect.poll(() => fileRequests.length).toBeGreaterThanOrEqual(1);
  expect(fileRequests[0]).toMatchObject({ token: '', includeFacets: 'true', signalSeen: true });
  expect(await page.locator('.thumb').count()).toBeLessThan(total);

  await expect.poll(async () => {
    await page.getByTestId('infinite-scroll-sentinel').scrollIntoViewIfNeeded();
    return fileRequests.some((request) => request.token === '60');
  }).toBe(true);

  expect(fileRequests[0]).toMatchObject({ token: '', includeFacets: 'true' });
  expect(fileRequests.some((request) => request.token === '60' && request.includeFacets === null)).toBe(true);
  await expect(page.getByText('180 files')).toBeVisible();
  expect(await page.locator('.thumb').count()).toBeLessThan(total);
});

test('video preview uses direct range-capable content route', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const contentRequests: Array<{ authorization: string; cookie: string }> = [];
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [fileItem('video-one', 'sample-video.mp4', 'video')], total_count: 1, library_count: 1, facets: { kind: [{ value: 'video', count: 1 }] } })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"></svg>' });
  });
  await page.route('**/api/v1/files/*/preview', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"></svg>' });
  });
  await page.route('**/api/v1/files/*/content', async (route) => {
    contentRequests.push({ authorization: route.request().headers().authorization ?? '', cookie: route.request().headers().cookie ?? '' });
    await route.fulfill({ contentType: 'video/mp4', body: '' });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: 'Preview sample-video.mp4' }).click();
  await expect(page.getByRole('dialog', { name: 'sample-video.mp4' })).toBeVisible();
  await expect.poll(() => contentRequests.length).toBeGreaterThan(0);
  expect(contentRequests[0]).toMatchObject({ authorization: '', cookie: expect.stringContaining('gooru_session=session-one') });
});

test('audio preview uses native audio content route', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const contentRequests: string[] = [];
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [fileItem('audio-one', 'sample-audio.mp3', 'audio')], total_count: 1, library_count: 1, facets: { kind: [{ value: 'audio', count: 1 }] } })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"></svg>' });
  });
  await page.route('**/api/v1/files/*/preview', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"></svg>' });
  });
  await page.route('**/api/v1/files/*/content', async (route) => {
    contentRequests.push(route.request().headers().authorization ?? '');
    await route.fulfill({ contentType: 'audio/mpeg', body: '' });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: 'Preview sample-audio.mp3' }).click();
  await expect(page.getByRole('dialog', { name: 'sample-audio.mp3' })).toBeVisible();
  await expect(page.locator('audio')).toHaveAttribute('src', /audio-one\/content/);
  await expect.poll(() => contentRequests.length).toBeGreaterThan(0);
});

test('tag index renders real tag counts and navigates to a tag query', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const fileQueries: string[] = [];
  await page.route('**/api/v1/files?**', async (route) => {
    fileQueries.push(new URL(route.request().url()).searchParams.get('query') ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [], total_count: 0, library_count: 3, facets: { kind: [] } })
    });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: /Tags/ }).first().click();
  await expect(page.getByRole('heading', { name: 'Tag index' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'rating: 3' })).toBeVisible();
  await expect(page.getByRole('button', { name: /rating:safe/ })).toBeVisible();
  await page.getByRole('button', { name: /rating:safe/ }).click();
  await expect.poll(() => fileQueries).toContain('rating:safe');
});

test('uploads with job polling and cancellation', async ({ page }) => {
  await mockAuth(page);
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) }));
  let canceled = false;
  await page.route('**/api/v1/jobs', async (route) => {
    if (route.request().method() === 'DELETE') return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ removed: 1 }) });
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: canceled ? [] : [{ id: 'job-one', type: 'upload_import', status: 'running', progress: 0.4, submitted_at: '2026-05-20T00:00:00Z' }] }) });
  });
  await page.route('**/api/v1/uploads', async (route) => {
    await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: 'pending', submitted_at: '2026-05-20T00:00:00Z' }) });
  });
  const cancelRequests: Array<{ csrf: string; method: string }> = [];
  await page.route('**/api/v1/jobs/job-one', async (route) => {
    if (route.request().method() === 'DELETE') {
      cancelRequests.push({ csrf: route.request().headers()['x-gooru-csrf'] ?? '', method: route.request().method() });
      canceled = true;
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: 'canceled', submitted_at: '2026-05-20T00:00:00Z' }) });
    }
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'job-one', type: 'upload_import', status: canceled ? 'canceled' : 'running', progress: canceled ? 1 : 0.5, submitted_at: '2026-05-20T00:00:00Z' }) });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: 'Upload' }).click();
  await page.locator('input[type="file"]').setInputFiles({ name: 'upload.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('upload') });
  await page.getByPlaceholder('collection:inbox @review').fill('incoming');
  await page.getByRole('button', { name: 'Upload 1' }).click();
  await expect(page.getByText('Importing', { exact: true })).toBeVisible();
  const deleteResponse = page.waitForResponse((response) =>
    response.url().endsWith('/api/v1/jobs/job-one') && response.request().method() === 'DELETE'
  );
  await page.getByRole('button', { name: 'Cancel' }).click();
  await deleteResponse;
  await expect.poll(() => cancelRequests).toEqual([{ csrf: 'csrf-one', method: 'DELETE' }]);
});

test('upload result details preserve duplicate and error statuses', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/uploads', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        affected_count: 1,
        files: [
          { name: 'new.jpg', size: 6, target_id: 'default', status: 'imported' },
          { name: 'dupe.jpg', size: 4, target_id: 'default', status: 'duplicate_existing' },
          { name: 'bad.jpg', size: 3, target_id: 'default', status: 'error', error: 'unsupported media' }
        ]
      })
    });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: 'Upload' }).click();
  await page.locator('input[type="file"]').setInputFiles([
    { name: 'new.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('new') },
    { name: 'dupe.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('dupe') },
    { name: 'bad.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('bad') }
  ]);
  await page.getByRole('button', { name: 'Upload 3' }).click();
  await expect(page.getByText('imported', { exact: true })).toBeVisible();
  await expect(page.getByText('duplicate existing', { exact: true })).toBeVisible();
  await expect(page.getByText('unsupported media')).toBeVisible();
});
