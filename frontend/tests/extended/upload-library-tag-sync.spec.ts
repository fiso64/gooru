import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(tags: string[]) {
  return {
    id: 'one',
    content_id: 'hash-one',
    name: 'one.jpg',
    safe_display_path: 'library/one.jpg',
    size: 5,
    added_at: '2026-09-17T00:00:00Z',
    modified_time: '2026-09-17T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: 32, image_height: 32 },
    tags,
    media_urls: {
      thumbnail: '/api/v1/files/one/thumbnail',
      preview: '/api/v1/files/one/preview',
      content: '/api/v1/files/one/content',
      download: '/api/v1/files/one/download'
    }
  };
}

async function mockApp(page: Page) {
  let loggedIn = false;
  let serverTags = ['source:upload'];
  let fileListRequests = 0;

  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
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
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      tags: serverTags.map((name) => ({ name, value: name, namespace: name.includes(':') ? name.split(':', 1)[0] : '', count: 1 })),
      library_count: 1,
      facets: { kind: [{ value: 'image', count: 1 }] }
    })
  }));
  await page.route('**/api/v1/operations?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], total_count: 0, active_count: 0 })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], meta_tags: [] })
  }));
  await page.route('**/api/v1/files?**', async (route) => {
    fileListRequests += 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [fileItem(serverTags)],
        total_count: 1,
        library_count: 1,
        facets: { kind: [{ value: 'image', count: 1 }] }
      })
    });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    const request = route.request();
    const body = request.postDataJSON() as { tags?: string[] };
    if (request.method() === 'POST') {
      serverTags = Array.from(new Set([...serverTags, ...(body.tags ?? [])]));
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ matched_files: 1, changed_files: 1 })
    });
  });
  await page.route('**/api/v1/uploads', async (route) => {
    if (route.request().headers()['x-gooru-upload-reserve'] === 'true') {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ id: 'upload-one', status: 'pending' })
      });
      return;
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [{ id: 'one', name: 'one.jpg', size: 5, target_id: 'default', status: 'duplicate_existing' }],
        affected_count: 1
      })
    });
  });
  await page.route('**/api/v1/files/one/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>'
  }));
  await page.route('**/api/v1/files/one/preview', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>'
  }));
  await page.route('**/api/v1/files/one/content', async (route) => route.fulfill({ status: 404, contentType: 'text/plain', body: 'fixture unavailable' }));

  return { fileListRequests: () => fileListRequests };
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('library tag edits flow back into an existing upload row without opening the upload viewer', async ({ page }) => {
  const server = await mockApp(page);
  await signIn(page);

  await page.getByRole('button', { name: 'Upload', exact: true }).click();
  const initialTags = page.getByLabel('Initial tags');
  await initialTags.fill('source:upload');
  await initialTags.press('Enter');
  await page.locator('input[type="file"]').setInputFiles({
    name: 'one.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('image')
  });
  await page.getByRole('button', { name: 'Upload 1 file' }).click();

  const uploadRow = page.getByTestId('upload-row-0');
  await expect(uploadRow).toContainText('duplicate existing');
  await expect(uploadRow.getByRole('button', { name: 'Remove source:upload from one.jpg' })).toBeVisible();
  await expect(uploadRow.getByRole('button', { name: 'Remove library:edit from one.jpg' })).toHaveCount(0);

  await page.locator('.sidebar button.sidebar-item').filter({ hasText: 'Library' }).click();
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const dialog = page.getByRole('dialog', { name: 'one.jpg' });
  const tagInput = page.getByLabel('Tags for one.jpg');
  const fileListsBeforeEdit = server.fileListRequests();
  await tagInput.fill('library:edit');
  await tagInput.press('Enter');
  await expect(dialog.getByRole('button', { name: 'Search for library:edit' })).toBeVisible();
  await expect.poll(() => server.fileListRequests()).toBeGreaterThan(fileListsBeforeEdit);

  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await page.getByRole('button', { name: 'Upload', exact: true }).click();

  await expect(uploadRow.getByRole('button', { name: 'Remove library:edit from one.jpg' })).toBeVisible();
});
