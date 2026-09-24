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
  await expect(page.getByLabel('Search library')).toBeVisible();
  await expect(page.getByText('Settings')).toBeVisible();
  await expect(page.getByText('No results')).toBeVisible();
});

test('returns to login when an authenticated API request receives 401', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'unauthorized', message: 'session expired' } })
    });
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByLabel('Username')).toBeVisible();
  await expect(page.getByLabel('Password')).toBeVisible();
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
  await dialog.getByLabel('Tags for sample.jpg').press('Enter');
  await expect.poll(() => mutations.length).toBe(1);
  expect(mutations[0]).toMatchObject({ authorization: '', csrf: 'csrf-one', method: 'POST', body: { file_ids: ['bG9jOjE'], tags: ['reviewed'] } });
});

test('loads paginated large libraries with bounded virtualized DOM', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const total = 600;
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
        previous_page_token: start > 0 ? String(Math.max(0, start - pageSize)) : '',
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
  await expect(page.getByText('600 files')).toBeVisible();
  await expect.poll(() => fileRequests.length).toBeGreaterThanOrEqual(1);
  expect(fileRequests[0]).toMatchObject({ token: '', includeFacets: 'true', signalSeen: true });
  expect(await page.locator('.thumb').count()).toBeLessThan(total);

  await page.locator('.main').evaluate((node) => { node.scrollTop = 10_000; node.dispatchEvent(new Event('scroll')); });
  await expect.poll(() => fileRequests.some((request) => request.token === '60')).toBe(true);

  for (let i = 0; i < 18; i += 1) {
    await page.locator('.main').evaluate((node, step) => { node.scrollTop = step * 2_500; node.dispatchEvent(new Event('scroll')); }, i + 1);
    await page.waitForTimeout(75);
    if (await page.getByRole('button', { name: 'Preview large-560.jpg' }).count()) break;
  }
  await expect(page.getByRole('button', { name: 'Preview large-560.jpg' })).toHaveCount(1);

  expect(fileRequests[0]).toMatchObject({ token: '', includeFacets: 'true' });
  expect(fileRequests.some((request) => request.token === '60' && request.includeFacets === null)).toBe(true);
  await expect(page.getByText('600 files')).toBeVisible();
  expect(await page.locator('.thumb').count()).toBeLessThan(total);

  await page.locator('.main').evaluate((node) => { node.scrollTop = 0; node.dispatchEvent(new Event('scroll')); });
  await expect(page.getByRole('button', { name: 'Preview large-000.jpg' })).toBeVisible();
});

test('debounces search suggestions while preserving typed draft', async ({ page }) => {
  await mockAuth(page);
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  const suggestionQueries: string[] = [];
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    suggestionQueries.push(new URL(route.request().url()).searchParams.get('q') ?? '');
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ name: 'rating:safe', count: 1 }] }) });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByLabel('Search library').pressSequentially('safe', { delay: 10 });
  await expect(page.getByLabel('Search library')).toHaveValue('safe');
  await expect.poll(() => suggestionQueries).toEqual(['safe']);
  await expect(page.getByText('rating:safe')).toBeVisible();
});

test('search bar commits token pills and keyboard autocomplete', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const fileQueries: string[] = [];
  await page.route('**/api/v1/files?**', async (route) => {
    fileQueries.push(new URL(route.request().url()).searchParams.get('query') ?? '');
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  await page.goto('/');
  await signIn(page);
  const search = page.getByLabel('Search library');

  await search.pressSequentially('rat', { delay: 10 });
  await expect(page.getByText('Suggestions', { exact: true })).toBeVisible();
  await search.press('Enter');
  await expect(search).toHaveValue('rating:');
  await search.pressSequentially('safe', { delay: 10 });
  await search.press('Enter');
  await expect(page.locator('.searchbar-pill').filter({ hasText: 'rating:safe' })).toBeVisible();
  await expect.poll(() => fileQueries).toContain('rating:safe');

  await search.pressSequentially('e', { delay: 10 });
  await search.press('ArrowDown');
  await search.press('Enter');
  await expect.poll(() => fileQueries).toContain('rating:safe blue');
  await expect(page.getByLabel('Remove blue')).toBeVisible();

  await page.getByLabel('Remove rating:safe').click();
  await expect.poll(() => fileQueries).toContain('blue');
  await search.press('Backspace');
  await expect.poll(() => fileQueries).toContain('');

  await search.pressSequentially('-rating:safe', { delay: 10 });
  await search.press('Tab');
  await expect(page.locator('.searchbar-pill.neg')).toContainText('rating:safe');
  await expect.poll(() => fileQueries).toContain('-rating:safe');

  await page.getByLabel('Clear search').click();
  await expect.poll(() => fileQueries).toContain('');
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
  await expect(page.getByRole('heading', { name: '2 tags across 3 files' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'rating', exact: true })).toBeVisible();
  const ratingSafe = page.getByRole('main').getByRole('button', { name: /rating:safe/ });
  await expect(ratingSafe).toBeVisible();
  await ratingSafe.click();
  await expect.poll(() => fileQueries).toContain('rating:safe');
});

test('uploads with durable operation polling and cancellation', async ({ page }) => {
  await mockAuth(page);
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) }));
  let canceled = false;
  await page.route('**/api/v1/operations?**', async (route) => {
    const ids = new URL(route.request().url()).searchParams.getAll('id');
    const operation = {
      id: 'job-one',
      kind: 'upload_import',
      status: canceled ? 'canceled' : 'running',
      progress_total: 10,
      progress_completed: canceled ? 10 : 4,
      progress_failed: 0,
      created_at: '2026-05-20T00:00:00Z'
    };
    const visible = !canceled && (!ids.length || ids.includes(operation.id));
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: visible ? [operation] : [] }) });
  });
  await page.route('**/api/v1/uploads', async (route) => {
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-one', kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-05-20T00:00:00Z' })
    });
  });
  const cancelRequests: Array<{ csrf: string; method: string }> = [];
  await page.route('**/api/v1/operations/job-one', async (route) => {
    if (route.request().method() === 'DELETE') {
      cancelRequests.push({ csrf: route.request().headers()['x-gooru-csrf'] ?? '', method: route.request().method() });
      canceled = true;
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ id: 'job-one', kind: 'upload_import', status: 'canceled', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', finished_at: '2026-05-20T00:01:00Z' })
      });
    }
    return route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-one', kind: 'upload_import', status: canceled ? 'canceled' : 'running', progress_total: 10, progress_completed: canceled ? 10 : 5, progress_failed: 0, created_at: '2026-05-20T00:00:00Z' })
    });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: 'Upload' }).click();
  await page.locator('input[type="file"]').setInputFiles({ name: 'upload.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('upload') });
  await page.getByLabel('Initial tags').fill('incoming');
  await page.getByLabel('Initial tags').press('Enter');
  await page.getByRole('button', { name: 'Upload 1' }).click();
  await expect(page.getByText('importing', { exact: true })).toBeVisible();
  await expect(page.getByText(/Queue · 1 file/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Pause all' })).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Clear done' })).toBeDisabled();
  const deleteResponse = page.waitForResponse((response) =>
    response.url().endsWith('/api/v1/operations/job-one') && response.request().method() === 'DELETE'
  );
  await page.locator('.upload-queue-head').hover();
  await page.getByRole('button', { name: 'Cancel' }).click();
  await deleteResponse;
  await expect.poll(() => cancelRequests).toEqual([{ csrf: 'csrf-one', method: 'DELETE' }]);
});

test('stages uploads from drag and drop', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: 'Upload' }).click();
  const dataTransfer = await page.evaluateHandle(() => {
    const transfer = new DataTransfer();
    transfer.items.add(new File(['drop'], 'dropped.jpg', { type: 'image/jpeg' }));
    return transfer;
  });
  await page.locator('.upload-zone').dispatchEvent('dragover', { dataTransfer });
  await expect(page.locator('.upload-zone')).toHaveClass(/is-drag/);
  await page.locator('.upload-zone').dispatchEvent('drop', { dataTransfer });
  await expect(page.getByText('dropped.jpg')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Upload 1' })).toBeVisible();
});

test('upload result details preserve duplicate and error statuses', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  const uploadResults = [
    { name: 'new.jpg', size: 6, target_id: 'default', status: 'imported' },
    { name: 'dupe.jpg', size: 4, target_id: 'default', status: 'duplicate_existing' },
    { name: 'bad.jpg', size: 3, target_id: 'default', status: 'error', error: 'unsupported media' }
  ] as const;
  let uploadResultIndex = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    const result = uploadResults[uploadResultIndex++];
    if (!result) return route.fulfill({ status: 500, body: 'unexpected upload request' });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ affected_count: result.status === 'imported' ? 1 : 0, files: [result] })
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


test('bulk selection supports concept untag action', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const files = [fileItem('one', 'one.jpg'), fileItem('two', 'two.jpg')];
  const mutations: Array<{ method: string; csrf: string; body: unknown }> = [];

  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files, total_count: 2, library_count: 2, facets: { kind: [{ value: 'photo', count: 2 }] } })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    mutations.push({
      method: route.request().method(),
      csrf: route.request().headers()['x-gooru-csrf'] ?? '',
      body: route.request().postDataJSON()
    });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ operation: 'remove', selector: { file_ids: ['one'] }, affected_count: 1 })
    });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('checkbox', { name: 'Select one.jpg' }).click();
  await expect(page.locator('.thumb.is-selected')).toHaveCount(1);
  await expect(page.getByText('1 of 2 selected')).toBeVisible();
  await page.locator('.selection-bar .sb-actions button').filter({ hasText: 'Untag' }).click();
  await expect(page.getByRole('heading', { name: 'Untag selected files' })).toBeVisible();
  await page.getByRole('textbox', { name: 'Tags', exact: true }).fill('blue');
  await page.getByRole('button', { name: 'Remove tags' }).click();

  await expect.poll(() => mutations.length).toBe(1);
  expect(mutations[0]).toMatchObject({ method: 'DELETE', csrf: 'csrf-one', body: { file_ids: ['one'], tags: ['blue'], verbose: false } });
  await expect(page.locator('.selection-bar')).toHaveCount(0);
});

test('lightbox supports per-tag removal and confirmed untrack', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const mutations: Array<{ method: string; csrf: string; body: unknown }> = [];
  const untracks: Array<{ csrf: string; body: unknown }> = [];

  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [fileItem('bG9jOjE', 'sample.jpg')], total_count: 1, library_count: 1, facets: { kind: [{ value: 'photo', count: 1 }] } })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' });
  });
  await page.route('**/api/v1/files/*/preview', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64" />' });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    mutations.push({ method: route.request().method(), csrf: route.request().headers()['x-gooru-csrf'] ?? '', body: route.request().postDataJSON() });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ operation: 'remove', selector: { file_ids: ['bG9jOjE'] }, affected_count: 1 }) });
  });
  await page.route('**/api/v1/files/bG9jOjE', async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback();
    untracks.push({ csrf: route.request().headers()['x-gooru-csrf'] ?? '', body: route.request().postDataJSON() });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'bG9jOjE', untracked: true }) });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: 'Preview sample.jpg' }).click();
  const dialog = page.getByRole('dialog', { name: 'sample.jpg' });

  await dialog.getByRole('button', { name: 'Remove blue' }).click();
  await expect.poll(() => mutations.length).toBe(1);
  expect(mutations[0]).toMatchObject({ method: 'DELETE', csrf: 'csrf-one', body: { file_ids: ['bG9jOjE'], tags: ['blue'], verbose: false } });

  await dialog.getByRole('button', { name: 'Untrack sample.jpg from library' }).click();
  await expect(page.getByRole('heading', { name: 'Remove from library' })).toBeVisible();
  await page.getByRole('button', { name: 'Remove', exact: true }).click();
  await expect.poll(() => untracks.length).toBe(1);
  expect(untracks[0]).toMatchObject({ csrf: 'csrf-one', body: { mode: 'untrack' } });
  await expect(page.getByRole('dialog', { name: 'sample.jpg' })).toHaveCount(0);
});


test('Tags index matches concept grouping and filtering', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 9, library_count: 9, facets: { kind: [] } }) });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByText('Tags', { exact: true }).first().click();
  await expect(page.getByRole('heading', { name: '2 tags across 9 files' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'tags', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'rating', exact: true })).toBeVisible();

  const filter = page.getByLabel('Filter tags');
  await filter.fill('safe');
  await expect(page.locator('.tagscloud-item')).toHaveCount(1);
  await expect(page.locator('.tagscloud-item').filter({ hasText: 'rating:safe' })).toBeVisible();

  await page.locator('.tagscloud-item').filter({ hasText: 'rating:safe' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.locator('.searchbar-pill').filter({ hasText: 'rating:safe' })).toBeVisible();
});


test('Jobs drawer preserves route and shares real job actions with the page', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  let canceled = false;
  const cancels: Array<{ csrf: string; method: string }> = [];

  await page.route('**/api/v1/operations**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/operations/job-run') && route.request().method() === 'DELETE') {
      cancels.push({ csrf: route.request().headers()['x-gooru-csrf'] ?? '', method: route.request().method() });
      canceled = true;
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ id: 'job-run', kind: 'upload_import', status: 'canceled', progress_total: 10, progress_completed: 4, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', finished_at: '2026-05-20T00:02:00Z' })
      });
    }
    if (url.pathname.endsWith('/operations')) {
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 'job-run', kind: 'upload_import', status: canceled ? 'canceled' : 'running', progress_total: 10, progress_completed: 4, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z' },
            { id: 'job-done', kind: 'tag_mutation', status: 'completed', progress_total: 1, progress_completed: 1, progress_failed: 0, created_at: '2026-05-19T23:00:00Z', started_at: '2026-05-19T23:01:00Z', finished_at: '2026-05-19T23:02:00Z' }
          ]
        })
      });
    }
    return route.fallback();
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  await page.locator('.topbar-right').getByRole('button', { name: 'Jobs' }).click();
  const drawer = page.getByRole('dialog', { name: 'Jobs' });
  await expect(drawer).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(drawer.getByText('Import media')).toBeVisible();
  await expect(drawer.getByRole('button', { name: 'Pause all coming soon' })).toBeDisabled();
  await drawer.locator('.job-row').filter({ hasText: 'Import media' }).hover();
  await drawer.getByRole('button', { name: 'Cancel Import media' }).click();
  await expect.poll(() => cancels).toEqual([{ csrf: 'csrf-one', method: 'DELETE' }]);

  await drawer.getByRole('button', { name: 'Close jobs' }).click();
  await expect(page.getByRole('dialog', { name: 'Jobs' })).toHaveCount(0);

  await page.locator('.sidebar').getByRole('button', { name: /Jobs/ }).click();
  await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();
  await expect(page.getByText('Tag edit', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Clear completed' })).toHaveCount(0);
});


test('Shortcuts matches the concept and question mark opens it outside text entry', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  await page.locator('.main').click({ position: { x: 8, y: 8 } });
  await page.keyboard.press('Shift+/');
  await expect(page.getByRole('heading', { name: 'Shortcuts' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Navigation' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Viewer' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Selection' })).toBeVisible();
  await expect(page.getByText('Focus search')).toBeVisible();
  await expect(page.getByText('Next file or comic page')).toBeVisible();
  await expect(page.getByText('Select all files in the current view')).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('heading', { name: 'Shortcuts' })).toHaveCount(0);

  await page.locator('.sidebar').getByRole('button', { name: /Library/ }).click();
  const search = page.getByLabel('Search library');
  await search.focus();
  await page.keyboard.press('Shift+/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Shortcuts' })).toHaveCount(0);
});


test('Settings preserves concept structure and changes password through CSRF', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  const changes: Array<{ csrf: string; body: unknown }> = [];
  await page.route('**/api/v1/auth/change-password', async (route) => {
    changes.push({
      csrf: route.request().headers()['x-gooru-csrf'] ?? '',
      body: route.request().postDataJSON()
    });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });

  await page.goto('/');
  await signIn(page);
  await page.locator('.sidebar').getByRole('button', { name: 'Settings' }).click();

  await expect(page.getByRole('heading', { name: 'Server & library settings' })).toBeVisible();
  for (const heading of ['Account', 'Appearance', 'Server', 'Library', 'Media processing']) {
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible();
  }
  await expect(page.getByText('Auth tokens', { exact: true })).toHaveCount(0);
  await expect(page.getByLabel('Listen address')).toBeDisabled();
  await expect(page.getByLabel('Appearance settings unavailable').getByLabel('Grid density')).toBeDisabled();
  await expect(page.getByLabel('Library settings').getByText('Configured in gooru.yaml')).toBeVisible();

  await page.getByRole('button', { name: 'Change…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Change password' });
  await dialog.getByLabel('Current password').fill('old-secret');
  await dialog.getByLabel('New password', { exact: true }).fill('new-secret');
  await dialog.getByLabel('Confirm new password').fill('new-secret');
  await dialog.getByRole('button', { name: 'Change password' }).click();

  await expect.poll(() => changes).toEqual([{ csrf: 'csrf-one', body: { current_password: 'old-secret', new_password: 'new-secret' } }]);
  await expect(page.getByRole('status')).toHaveText('Password updated.');
});


test('login matches concept without fabricating build metadata', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  let loginRequests = 0;
  await page.route('**/api/v1/auth/login', async (route) => {
    loginRequests += 1;
    await route.fallback();
  });

  await page.goto('/');
  const username = page.getByLabel('Username');
  await expect(username).toBeFocused();
  await expect(page.locator('.login-v2-id-meta')).toContainText('server');
  await expect(page.locator('.login-v2-id-meta')).toContainText('ready');
  await expect(page.locator('.login-v2-id-meta')).toContainText('gpl-3.0');
  await expect(page.getByText('v0.4.2', { exact: true })).toHaveCount(0);
  await expect(page.getByText('4f7a91d', { exact: true })).toHaveCount(0);
  await expect(page.getByText('gooru user create-admin', { exact: true })).toBeVisible();
  await expect(page.getByText('docs', { exact: true })).toHaveAttribute('aria-disabled', 'true');

  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('alert')).toHaveText(/username and password required/);
  expect(loginRequests).toBe(0);

  await username.fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect.poll(() => loginRequests).toBe(1);
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
});


test('uses exact concept primitives and font weights', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [fileItem('concept-one', 'concept.jpg')], total_count: 1, library_count: 1, facets: { kind: [{ value: 'photo', count: 1 }] } })
    });
  });

  await page.goto('/');
  const input = page.getByLabel('Username');
  const inputStyle = await input.evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, radius: style.borderRadius, fontSize: style.fontSize };
  });
  expect(inputStyle).toEqual({ padding: '9px 12px', radius: '4px', fontSize: '14px' });
  await input.evaluate((node) => node.blur());
  await expect.poll(() => input.evaluate((node) => getComputedStyle(node).borderRadius)).toBe('6px');

  const fontFaces = await page.evaluate(() => Array.from(document.fonts).map((face) => ({
    family: face.family.replace(/["']/g, ''),
    weight: face.weight
  })));
  expect(fontFaces).toContainEqual({ family: 'IBM Plex Sans', weight: '500' });
  expect(fontFaces).toContainEqual({ family: 'IBM Plex Sans', weight: '700' });
  expect(fontFaces).toContainEqual({ family: 'IBM Plex Mono', weight: '500' });
  expect(fontFaces).toContainEqual({ family: 'IBM Plex Mono', weight: '600' });

  await signIn(page);
  const sortButton = page.getByTitle('Sort direction');
  const buttonStyle = await sortButton.evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, radius: style.borderRadius, fontSize: style.fontSize, weight: style.fontWeight, gap: style.gap };
  });
  expect(buttonStyle).toEqual({ padding: '5px 10px', radius: '6px', fontSize: '14px', weight: '400', gap: '6px' });

  const segment = page.getByRole('button', { name: 'Added' });
  const segmentStyle = await segment.evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, radius: style.borderRadius, fontSize: style.fontSize, weight: style.fontWeight };
  });
  expect(segmentStyle).toEqual({ padding: '5px 12px', radius: '4px', fontSize: '12px', weight: '500' });

  const grid = page.getByTestId('virtual-media-grid');
  const gridStyle = await grid.evaluate((node) => {
    const style = getComputedStyle(node);
    return { gap: style.gap, padding: style.padding };
  });
  expect(gridStyle).toEqual({ gap: '5px', padding: '16px' });
});


test('matches concept Lightbox geometry and navigation', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const first = fileItem('lightbox-one', 'first-very-long-preview-name.jpg');
  const second = fileItem('lightbox-two', 'second.jpg');
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [first, second], total_count: 2, library_count: 2, facets: { kind: [{ value: 'photo', count: 2 }] } })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#444"/></svg>' });
  });
  await page.route('**/api/v1/files/*/preview', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64" fill="#222"/></svg>' });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: `Preview ${first.name}` }).click();

  let dialog = page.getByRole('dialog', { name: first.name });
  await expect(dialog).toBeVisible();
  const shellStyle = await dialog.evaluate((node) => {
    const style = getComputedStyle(node);
    return { position: style.position, backdrop: style.backdropFilter };
  });
  expect(shellStyle.position).toBe('absolute');
  expect(shellStyle.backdrop).toBe('blur(20px)');

  const asideStyle = await dialog.locator('.lightbox-aside').evaluate((node) => {
    const style = getComputedStyle(node);
    return { top: style.paddingTop, right: style.paddingRight, bottom: style.paddingBottom, left: style.paddingLeft };
  });
  expect(asideStyle).toEqual({ top: '18px', right: '18px', bottom: '16px', left: '18px' });

  const titleStyle = await dialog.getByRole('heading', { name: first.name }).evaluate((node) => {
    const style = getComputedStyle(node);
    return { fontSize: style.fontSize, wordBreak: style.wordBreak };
  });
  expect(titleStyle).toEqual({ fontSize: '22px', wordBreak: 'break-all' });
  await expect.poll(() => dialog.locator('.lightbox-meta').evaluate((node) => getComputedStyle(node).rowGap)).toBe('4px');
  await expect.poll(() => dialog.getByRole('button', { name: 'Add tag' }).evaluate((node) => getComputedStyle(node).justifyContent)).toBe('center');

  const prevTransform = await dialog.getByRole('button', { name: 'Previous file' }).evaluate((node) => {
    const matrix = new DOMMatrix(getComputedStyle(node).transform);
    return { a: matrix.a, b: matrix.b, c: matrix.c, d: matrix.d };
  });
  expect(prevTransform).toEqual({ a: 1, b: 0, c: 0, d: 1 });

  await dialog.getByRole('button', { name: 'Add tag' }).click();
  await expect(dialog.getByLabel(`Tags for ${first.name}`)).toBeFocused();
  await dialog.getByLabel(`Tags for ${first.name}`).evaluate((node) => (node as HTMLInputElement).blur());

  await page.keyboard.press('ArrowRight');
  dialog = page.getByRole('dialog', { name: second.name });
  await expect(dialog).toBeVisible();
  await page.keyboard.press('k');
  dialog = page.getByRole('dialog', { name: first.name });
  await expect(dialog).toBeVisible();
  await page.keyboard.press('j');
  dialog = page.getByRole('dialog', { name: second.name });
  await expect(dialog).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
});


test('matches concept Tags index styling and routes tag clicks', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [], total_count: 3, library_count: 3, facets: { kind: [] } })
    });
  });

  await page.goto('/');
  await signIn(page);
  await page.locator('.sidebar .sidebar-item').filter({ hasText: 'Tags' }).click();

  const heading = page.getByRole('heading', { name: '2 tags across 3 files' });
  await expect(heading).toBeVisible();
  const headingTracking = await heading.evaluate((node) => parseFloat(getComputedStyle(node).letterSpacing));
  expect(headingTracking).toBeCloseTo(-0.38, 2);

  const safeTile = page.locator('.tagscloud-item').filter({ hasText: 'rating:safe' });
  await expect(safeTile).toBeVisible();
  const tileStyle = await safeTile.evaluate((node) => {
    const style = getComputedStyle(node);
    return {
      alignItems: style.alignItems,
      padding: style.padding,
      fontFamily: style.fontFamily,
      fontSize: style.fontSize
    };
  });
  expect(tileStyle.alignItems).toBe('center');
  expect(tileStyle.padding).toBe('8px 12px');
  expect(tileStyle.fontFamily).toContain('IBM Plex Mono');
  expect(tileStyle.fontSize).toBe('12.5px');
  await expect.poll(() => safeTile.locator('.ns').evaluate((node) => getComputedStyle(node).fontWeight)).toBe('500');
  await expect.poll(() => safeTile.locator('.count').evaluate((node) => getComputedStyle(node).fontSize)).toBe('11px');

  const filter = page.getByLabel('Filter tags');
  await filter.fill('blue');
  await expect(safeTile).toHaveCount(0);
  const blueTile = page.locator('.tagscloud-item').filter({ hasText: 'blue' });
  await expect(blueTile).toBeVisible();

  await blueTile.click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByLabel('Remove blue')).toBeVisible();
});


test('matches concept Library gallery and selection states', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const photo = fileItem('library-photo', 'portrait.jpg');
  const video = fileItem('library-video', 'clip.mp4', 'video');
  const gif = fileItem('library-gif', 'loop.gif', 'gif');
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [photo, video, gif],
        total_count: 3,
        library_count: 3,
        facets: { kind: [{ value: 'photo', count: 1 }, { value: 'video', count: 1 }, { value: 'gif', count: 1 }] }
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="#333"/></svg>' });
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByText('3 files', { exact: true })).toBeVisible();

  const grid = page.getByTestId('virtual-media-grid');
  const gridStyle = await grid.evaluate((node) => {
    const style = getComputedStyle(node);
    return { gap: style.gap, padding: style.padding, columns: style.gridTemplateColumns };
  });
  expect(gridStyle.gap).toBe('5px');
  expect(gridStyle.padding).toBe('16px');
  expect(gridStyle.columns).not.toBe('none');

  const photoCard = page.getByRole('button', { name: 'Preview portrait.jpg' }).locator('..');
  const meta = photoCard.locator('.thumb-meta');
  await expect.poll(() => meta.evaluate((node) => getComputedStyle(node).alignItems)).toBe('flex-end');
  await expect.poll(() => meta.evaluate((node) => getComputedStyle(node).opacity)).toBe('0');
  await photoCard.hover();
  await expect.poll(() => meta.evaluate((node) => getComputedStyle(node).opacity)).toBe('1');
  await expect.poll(() => photoCard.locator('.thumb-overlay').evaluate((node) => getComputedStyle(node).opacity)).toBe('1');

  const videoCard = page.getByRole('button', { name: 'Preview clip.mp4' }).locator('..');
  await expect(videoCard.locator('.thumb-badge')).toContainText('0:08');
  await expect.poll(() => videoCard.locator('.thumb-badge').evaluate((node) => getComputedStyle(node).fontWeight)).toBe('500');

  const select = page.getByRole('checkbox', { name: 'Select portrait.jpg' });
  const checkboxTransition = await select.evaluate((node) => getComputedStyle(node).transitionProperty);
  expect(checkboxTransition).toContain('opacity');
  expect(checkboxTransition).toContain('background');
  expect(checkboxTransition).toContain('border-color');
  await select.click();
  await expect(page.getByRole('checkbox', { name: 'Deselect portrait.jpg' })).toHaveAttribute('aria-checked', 'true');

  const selectedCard = page.getByRole('button', { name: 'Preview portrait.jpg' }).locator('..');
  const selectedRing = await selectedCard.evaluate((node) => getComputedStyle(node, '::after').boxShadow);
  expect(selectedRing).toContain('inset');

  const selectionBar = page.locator('.selection-bar');
  await expect(selectionBar).toContainText('1 of 3 selected');
  const selectionStyle = await selectionBar.evaluate((node) => {
    const style = getComputedStyle(node);
    return { position: style.position, padding: style.padding, fontSize: style.fontSize, fontWeight: style.fontWeight };
  });
  expect(selectionStyle).toEqual({ position: 'sticky', padding: '10px 24px', fontSize: '12px', fontWeight: '500' });
  await expect(selectionBar.getByRole('button', { name: 'Export' })).toBeDisabled();
  await selectionBar.getByRole('button', { name: 'Clear selection' }).click();
  await expect(selectionBar).toHaveCount(0);
});


test('matches concept utility views while exposing only real capabilities', async ({ page }) => {
  await mockAuth(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/operations', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: [
          { id: 'utility-run', kind: 'upload_import', status: 'running', progress_total: 10, progress_completed: 4, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z' },
          { id: 'utility-done', kind: 'bulk_tag', status: 'completed', progress_total: 1, progress_completed: 1, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z', finished_at: '2026-05-20T00:02:00Z' }
        ]
      })
    });
  });

  await page.goto('/');
  await signIn(page);

  // Settings keeps the concept page/section frame while capability-limited
  // controls remain visible but disabled and removed bearer auth stays absent.
  await page.locator('.sidebar').getByRole('button', { name: 'Settings' }).click();
  await expect(page.getByRole('heading', { name: 'Server & library settings' })).toBeVisible();
  const settingsPage = page.locator('.page');
  const pageStyle = await settingsPage.evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, maxWidth: style.maxWidth, gap: style.gap };
  });
  expect(pageStyle).toEqual({ padding: '32px 40px 80px', maxWidth: '1100px', gap: '28px' });
  const accountSection = page.locator('.settings-section').filter({ hasText: 'Account' }).first();
  const sectionStyle = await accountSection.evaluate((node) => {
    const style = getComputedStyle(node);
    return { columns: style.gridTemplateColumns, gap: style.gap, padding: style.padding };
  });
  expect(sectionStyle.columns).toMatch(/^220px /);
  expect(sectionStyle.gap).toBe('32px');
  expect(sectionStyle.padding).toBe('28px 0px');
  await expect(page.getByLabel('Appearance settings unavailable').getByRole('slider')).toBeDisabled();
  await expect(page.getByLabel('Server settings').getByLabel('Public URL')).toBeDisabled();
  await expect(page.getByText('Auth tokens', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Change…' })).toBeEnabled();
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeEnabled();

  // Shortcuts preserves the exact help grid; unsupported commands are the
  // documented A-state rather than pretending to work.
  await page.locator('.sidebar').getByRole('button', { name: 'Shortcuts' }).click();
  await expect(page.getByRole('heading', { name: 'Shortcuts' })).toBeVisible();
  const shortcutGrid = page.locator('.shortcut-grid');
  const shortcutGridStyle = await shortcutGrid.evaluate((node) => {
    const style = getComputedStyle(node);
    return { gap: style.gap, columns: style.gridTemplateColumns };
  });
  expect(shortcutGridStyle.gap).toBe('28px 42px');
  expect(shortcutGridStyle.columns).not.toBe('none');
  const focusSearchShortcut = page.locator('.shortcut-row').filter({ hasText: 'Focus search' });
  await expect(focusSearchShortcut).toBeVisible();
  expect(await focusSearchShortcut.evaluate((node) => getComputedStyle(node).opacity)).toBe('1');
  const launcherShortcut = page.locator('.shortcut-row').filter({ hasText: 'Show shortcuts' });
  await expect(launcherShortcut).toBeVisible();
  expect(await launcherShortcut.evaluate((node) => getComputedStyle(node).opacity)).toBe('1');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('heading', { name: 'Shortcuts' })).toHaveCount(0);

  // The top-bar Jobs drawer and shared row preserve the concept geometry while
  // pause-all stays visibly disabled and live job data replaces mock counters.
  await page.getByRole('button', { name: 'Jobs' }).first().click();
  const drawer = page.getByRole('dialog', { name: 'Jobs' });
  await expect(drawer).toBeVisible();
  const drawerStyle = await drawer.evaluate((node) => {
    const style = getComputedStyle(node);
    return { position: style.position, top: style.top, right: style.right, width: style.width, borderRadius: style.borderRadius };
  });
  expect(drawerStyle.position).toBe('absolute');
  expect(drawerStyle.top).toBe('56px');
  expect(drawerStyle.right).toBe('14px');
  expect(drawerStyle.width).toBe('360px');
  await expect(drawer.getByRole('button', { name: 'Pause all coming soon' })).toBeDisabled();
  const row = drawer.locator('.job-row').first();
  const rowStyle = await row.evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, gap: style.gap, display: style.display };
  });
  expect(rowStyle).toEqual({ padding: '14px 16px', gap: '7px', display: 'flex' });
  await expect.poll(() => row.locator('.job-progress').evaluate((node) => getComputedStyle(node).height)).toBe('4px');
});


test('matches exact Upload staging surface and releases local previews', async ({ page }) => {
  await page.addInitScript(() => {
    const original = URL.revokeObjectURL.bind(URL);
    const state = window as typeof window & { __gooruRevoked?: string[] };
    state.__gooruRevoked = [];
    URL.revokeObjectURL = (url: string) => {
      state.__gooruRevoked?.push(url);
      original(url);
    };
  });
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  await page.goto('/');
  await signIn(page);
  await page.locator('.sidebar').getByRole('button', { name: 'Upload' }).click();
  await expect(page.getByRole('heading', { name: 'Import media into your library' })).toBeVisible();

  const stackStyle = await page.locator('.upload-stack').evaluate((node) => ({ gap: getComputedStyle(node).gap }));
  expect(stackStyle.gap).toBe('28px');
  const configStyle = await page.locator('.upload-config-card').evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, gap: style.gap };
  });
  expect(configStyle).toEqual({ padding: '18px', gap: '16px' });
  const zoneStyle = await page.locator('.upload-zone').evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, radius: style.borderRadius };
  });
  expect(zoneStyle.padding).toBe('48px 24px');
  await expect(page.getByText(/max 5 GB/)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Paste URL', exact: true })).toBeDisabled();

  const tagInput = page.getByLabel('Initial tags');
  await expect(tagInput).toBeVisible();
  await tagInput.focus();
  await expect(tagInput).toBeFocused();
  await tagInput.fill('collection:may-2026 @review');
  await tagInput.press('Enter');
  await expect(page.locator('.upload-tags-control .g-tag')).toHaveCount(1);
  await expect(page.locator('.upload-tags-control')).toContainText('collection:may-2026');
  await expect(page.locator('.upload-tags-control')).not.toContainText('@review');
  await page.getByRole('button', { name: 'Remove collection:may-2026' }).click();
  await expect(page.locator('.upload-tags-control .g-tag')).toHaveCount(0);

  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zs1sAAAAASUVORK5CYII=', 'base64');
  await page.locator('input[type="file"]').setInputFiles({ name: 'stage.png', mimeType: 'image/png', buffer: png });
  const stagedRow = page.locator('.upload-row-staged').filter({ hasText: 'stage.png' });
  await expect(stagedRow).toBeVisible();
  const preview = stagedRow.locator('.thumb-tile img');
  await expect(preview).toHaveAttribute('src', /^blob:/);
  const previewURL = await preview.getAttribute('src');
  expect(previewURL).toBeTruthy();

  await stagedRow.getByRole('button', { name: 'Remove stage.png from staging' }).click();
  await expect(stagedRow).toHaveCount(0);
  await expect.poll(() => page.evaluate(() => (window as typeof window & { __gooruRevoked?: string[] }).__gooruRevoked ?? [])).toContain(previewURL!);
});


test('shows the Comics kind only when CBZ files exist and filters with ext:cbz', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const queries: string[] = [];
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const query = url.searchParams.get('query') ?? '';
    queries.push(query);
    const isComicQuery = query === 'ext:cbz';
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: isComicQuery ? [fileItem('comic', 'book.cbz', 'other')] : [fileItem('photo', 'sample.jpg')],
        total_count: isComicQuery ? 1 : 2,
        library_count: 2,
        facets: url.searchParams.get('include_facets') === 'true' ? { kind: [{ value: 'photo', count: 1 }] } : undefined
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' });
  });

  await page.goto('/');
  await signIn(page);
  const comics = page.getByRole('button', { name: /Comics/ });
  await expect(comics).toBeVisible();
  await expect(comics).toContainText('1');

  await comics.click();
  await expect.poll(() => queries.filter((query) => query === 'ext:cbz').length).toBeGreaterThanOrEqual(2);
  await expect(comics).toHaveClass(/active/);
});

test('hides the Comics kind when no CBZ files exist', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [],
        total_count: 0,
        library_count: 0,
        facets: url.searchParams.get('include_facets') === 'true' ? { kind: [] } : undefined
      })
    });
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByRole('button', { name: /Comics/ })).toHaveCount(0);
});

test('sidebar kind filters stay visible, mutually exclusive, and keep counts stable', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const fileQueries: string[] = [];
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const query = url.searchParams.get('query') ?? '';
    fileQueries.push(query);
    const baseFacets = { kind: [
      { value: 'photo', count: 7 },
      { value: 'video', count: 2 },
      { value: 'gif', count: 1 }
    ] };
    const total = query.includes('ext:cbz') ? 3
      : query.includes('type:photo') ? 7
      : query.includes('type:video') ? 2
      : query.includes('type:gif') ? 1
      : 10;
    const filteredKind = query.match(/type:(photo|video|gif)/)?.[1];
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [],
        total_count: total,
        library_count: 10,
        facets: filteredKind
          ? { kind: [{ value: filteredKind, count: total }] }
          : baseFacets
      })
    });
  });

  await page.goto('/');
  await signIn(page);
  const search = page.getByLabel('Search library');
  await search.fill('technology');
  await search.press('Enter');

  const photos = page.getByRole('button', { name: /Photos/ });
  const videos = page.getByRole('button', { name: /Videos/ });
  const gifs = page.getByRole('button', { name: /GIFs/ });
  await expect(photos.getByText('7')).toBeVisible();
  await expect(videos.getByText('2')).toBeVisible();
  await expect(gifs.getByText('1')).toBeVisible();

  await videos.click();
  await expect(page.locator('.searchbar-pill')).toHaveText(['technology', 'type:video']);
  await expect(photos.getByText('7')).toBeVisible();
  await expect(videos.getByText('2')).toBeVisible();
  await expect(gifs.getByText('1')).toBeVisible();

  await photos.click();
  await expect(page.locator('.searchbar-pill')).toHaveText(['technology', 'type:photo']);
  await expect(page.locator('.searchbar-pill')).not.toContainText(['type:video']);

  await gifs.click();
  await expect(page.locator('.searchbar-pill')).toHaveText(['technology', 'type:gif']);
  await expect(page.locator('.searchbar-pill')).not.toContainText(['type:photo']);

  const comics = page.getByRole('button', { name: /Comics/ });
  await expect(comics).toBeVisible();
  await comics.click();
  await expect(page.locator('.searchbar-pill')).toHaveText(['technology', 'ext:cbz']);
  await expect(page.locator('.searchbar-pill')).not.toContainText(['type:gif']);

  await page.getByRole('button', { name: 'Safe blue', exact: true }).click();
  await expect(page.locator('.searchbar-pill')).toHaveText(['rating:safe', 'blue']);

  await page.getByRole('button', { name: /^blue\s+2$/ }).click();
  await expect(page.locator('.searchbar-pill')).toHaveText(['blue']);
  expect(fileQueries).toContain('technology type:video');
  expect(fileQueries).toContain('technology');
});
