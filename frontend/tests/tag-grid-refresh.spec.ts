import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(index: number) {
  const id = `file-${index}`;
  return {
    id,
    content_id: `hash-${id}`,
    name: `perf-${index}.jpg`,
    safe_display_path: `library/perf-${index}.jpg`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: 800, image_height: 600 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

test('tagging after deep infinite scroll does not refetch retained file pages', async ({ page }) => {
  const files = Array.from({ length: 240 }, (_, index) => fileItem(index));
  const requestedOffsets: number[] = [];
  const tagRequests: Array<{ method: string; body: { file_ids?: string[]; tags?: string[]; verbose?: boolean } }> = [];

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
  }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({})
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [] })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const token = url.searchParams.get('page_token') ?? '';
    let offset = /^\d+$/.test(token) ? Number(token) : 0;
    if (token && !/^\d+$/.test(token)) {
      const decoded = atob(token.replace(/-/g, '+').replace(/_/g, '/'));
      offset = Number(decoded.match(/^offset:(\d+)$/)?.[1] ?? '0');
    }
    requestedOffsets.push(offset);
    const pageFiles = files.slice(offset, offset + 60);
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: pageFiles,
        total_count: files.length,
        library_count: files.length,
        facets: { kind: [{ value: 'photo', count: files.length }] },
        next_page_token: offset + 60 < files.length ? String(offset + 60) : undefined,
        previous_page_token: offset > 0 ? String(Math.max(0, offset - 60)) : undefined
      })
    });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    if (!['POST', 'PUT', 'DELETE'].includes(route.request().method())) return route.fallback();
    const body = route.request().postDataJSON() as { file_ids?: string[]; tags?: string[]; verbose?: boolean };
    tagRequests.push({ method: route.request().method(), body });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        operation: 'add',
        selector: { file_ids: body.file_ids ?? [] },
        matched_files: body.file_ids?.length ?? 0,
        affected_count: body.file_ids?.length ?? 0
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>'
  }));

  await page.goto('/');
  await expect(page.getByText('240 files')).toBeVisible();
  await page.getByRole('checkbox', { name: 'Select perf-0.jpg' }).click();
  await expect(page.getByText('1 selected')).toBeVisible();

  const main = page.locator('.main');
  for (let attempt = 0; attempt < 16 && !requestedOffsets.includes(180); attempt += 1) {
    await main.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
      node.dispatchEvent(new Event('scroll'));
    });
    await page.waitForTimeout(50);
  }
  await expect.poll(() => new Set(requestedOffsets).size).toBe(4);
  await page.waitForTimeout(150);
  const requestsBeforeTagging = requestedOffsets.length;

  await page.locator('.sb-actions button').filter({ hasText: /^\s*Tag…\s*$/ }).click();
  const dialog = page.getByRole('dialog', { name: 'Tag selected files', exact: true });
  const input = dialog.getByRole('textbox', { name: 'Tags' });
  await input.fill('reviewed');
  await input.press('Control+Enter');

  await expect.poll(() => tagRequests.length).toBe(1);
  expect(tagRequests[0]).toEqual({
    method: 'POST',
    body: { file_ids: ['file-0'], tags: ['reviewed'], verbose: false }
  });
  await expect(dialog).toHaveCount(0);
  await page.waitForTimeout(250);
  expect(requestedOffsets).toHaveLength(requestsBeforeTagging);
});


test('restored deep viewer does not page the background infinite grid forward', async ({ page }) => {
  const files = Array.from({ length: 360 }, (_, index) => fileItem(index));
  const requestedOffsets: number[] = [];

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
  }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({})
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [], library_count: files.length })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('page_token') ?? '0');
    requestedOffsets.push(offset);
    const pageFiles = files.slice(offset, offset + 60);
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: pageFiles,
        total_count: files.length,
        library_count: files.length,
        facets: offset === 0 ? { kind: [{ value: 'photo', count: files.length }] } : undefined,
        next_page_token: offset + 60 < files.length ? String(offset + 60) : undefined,
        previous_page_token: offset > 0 ? String(Math.max(0, offset - 60)) : undefined
      })
    });
  });
  await page.route('**/api/v1/files/file-*', async (route) => {
    const path = new URL(route.request().url()).pathname;
    const match = path.match(/^\/api\/v1\/files\/file-(\d+)$/);
    if (!match) return route.fallback();
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(fileItem(Number(match[1])))
    });
  });
  const imageBody = '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600"><rect width="800" height="600"/></svg>';
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: imageBody
  }));
  await page.route('**/api/v1/files/*/preview*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: imageBody
  }));

  await page.goto('/');
  await expect(page.getByText('360 files')).toBeVisible();
  const main = page.locator('.main');
  for (let attempt = 0; attempt < 20 && !requestedOffsets.includes(300); attempt += 1) {
    await main.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
      node.dispatchEvent(new Event('scroll'));
    });
    await page.waitForTimeout(50);
  }
  await expect.poll(() => requestedOffsets.includes(300)).toBe(true);
  await expect(page.getByRole('button', { name: 'Preview perf-330.jpg' })).toBeVisible();
  await page.getByRole('button', { name: 'Preview perf-330.jpg' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect.poll(() => new URL(page.url()).searchParams.get('file')).toBe('file-330');
  await expect.poll(() => new URL(page.url()).searchParams.get('page')).toBe('6');

  requestedOffsets.splice(0);
  await page.reload();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect.poll(() => requestedOffsets[0]).toBe(300);
  await page.waitForTimeout(300);
  expect([...new Set(requestedOffsets)]).toEqual([300]);

  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect.poll(() => new URL(page.url()).searchParams.get('page')).toBeNull();
});
