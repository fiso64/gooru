import type { Page } from '@playwright/test';

export async function setupComicVariantFixture(page: Page) {
  const pic = '<svg xmlns="http://www.w3.org/2000/svg" width="60" height="60"/>';
  const file = {
    id: 'book', content_id: 'hash-book', name: 'book.cbz',
    safe_display_path: 'books/book.cbz', size: 2048, added_at: '2026-05-20T00:00:00Z',
    modified_time: '2026-05-20T00:00:00Z', media_type: 'application/vnd.comicbook+zip',
    media_kind: 'document', metadata: { page_count: 3 }, tags: [],
    media_urls: { thumbnail: '/api/v1/files/book/thumbnail', preview: '/api/v1/files/book/preview',
      content: '/api/v1/files/book/content', download: '/api/v1/files/book/download' }
  };
  const session = { user: { id: 'test', username: 'tester', role: 'admin' },
    capabilities: { upload: true, tag: true, delete: true, admin: true }, csrf_token: 'test' };
  const respond = (value: unknown) => JSON.stringify(value);
  await page.route('**/api/v1/ui-config', r => r.fulfill({ contentType: 'application/json',
    body: respond({ capabilities: ['preview_images'], prefer_lossless_full_image: true }) }));
  await page.route('**/api/v1/auth/me', r => r.fulfill({ contentType: 'application/json', body: respond(session) }));
  await page.route('**/api/v1/saved-searches', r => r.fulfill({ contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/api/v1/upload-targets', r => r.fulfill({ contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/api/v1/tags?**', r => r.fulfill({ contentType: 'application/json', body: '{"tags":[]}' }));
  await page.route('**/api/v1/search/suggestions?**', r => r.fulfill({ contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/api/v1/files?**', r => r.fulfill({ contentType: 'application/json',
    body: respond({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } }) }));
  await page.route('**/api/v1/comics/book', r => r.fulfill({ contentType: 'application/json',
    body: respond({ pages: [0, 1, 2].map(index => ({ index, name: index + '.jpg',
      url: '/api/v1/comics/book/' + index,
      preview: '/api/v1/comics/book/' + index + '?variant=preview',
      lossless: '/api/v1/comics/book/' + index + '?variant=lossless' })) }) }));
  await page.route('**/api/v1/files/book/*', r => r.fulfill({ contentType: 'image/svg+xml', body: pic }));
  await page.route('**/api/v1/comics/book/*', r => r.fulfill({ contentType: 'image/svg+xml', body: pic }));
  await page.goto('/');
}
