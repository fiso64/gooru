import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function file(id: string, name: string) {
  return {
    id,
    content_id: `hash-${id}`,
    name,
    safe_display_path: `library/${name}`,
    size: 2048,
    modified_time: '2026-09-18T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: 800, image_height: 600 },
    tags: id === 'cat' ? ['subject:cat'] : [],
    can_delete: true,
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockShell(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({})
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
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
    body: JSON.stringify({
      tags: [{ name: 'subject:cat', namespace: 'subject', value: 'cat', count: 1 }],
      library_count: 2,
      facets: { kind: [{ value: 'photo', count: 2 }] }
    })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], meta_tags: [] })
  }));
  await page.route('**/api/v1/operations?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], total_count: 0, active_count: 0 })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"/>'
  }));
}

test('tag navigation never renders rows from the previous library view', async ({ page }) => {
  await mockShell(page);

  let filteredRequested = false;
  let releaseFiltered!: () => void;
  const filteredGate = new Promise<void>((resolve) => {
    releaseFiltered = resolve;
  });

  await page.route('**/api/v1/files?**', async (route) => {
    const query = new URL(route.request().url()).searchParams.get('query') ?? '';
    if (query === 'subject:cat') {
      filteredRequested = true;
      await filteredGate;
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          files: [file('cat', 'cat.jpg')],
          total_count: 1,
          library_count: 2,
          facets: { kind: [{ value: 'photo', count: 1 }] }
        })
      });
      return;
    }

    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [file('old', 'old.jpg')],
        total_count: 2,
        library_count: 2,
        facets: { kind: [{ value: 'photo', count: 2 }] }
      })
    });
  });

  await page.goto('/');
  await expect(page.getByRole('img', { name: 'old.jpg' })).toBeVisible();

  await page.getByRole('navigation', { name: 'Primary navigation' })
    .getByRole('button', { name: 'Tags', exact: true })
    .click();
  await expect(page.getByRole('heading', { name: /tags across/ })).toBeVisible();
  await expect(page.getByRole('img', { name: 'old.jpg' })).toHaveCount(0);

  await page.getByRole('button', { name: /subject:cat/ }).click();
  await expect.poll(() => filteredRequested).toBe(true);

  await expect(page.getByRole('img', { name: 'old.jpg' })).toHaveCount(0);
  await expect(page.locator('.skeleton-grid')).toBeVisible();

  releaseFiltered();
  await expect(page.getByRole('img', { name: 'cat.jpg' })).toBeVisible();
  await expect(page.getByRole('img', { name: 'old.jpg' })).toHaveCount(0);
});
