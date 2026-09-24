import type { Page } from '@playwright/test';

// E2E fixtures must model the *complete* server listing even when the mocked
// library grid only exposes one page. Resolve neighbors from the supplied
// ordered result set, not from whichever grid page happens to be visible.
export async function mockFileAround<T extends { id: string }>(page: Page, getFiles: (request: { query: string }) => readonly T[]) {
  await page.route('**/api/v1/files/around', (route) => {
    const { file_id, count = 5, query = '' } = route.request().postDataJSON() as { file_id: string; count?: number; query?: string };
    // Match the API contract: legacy grid fixtures often omit this required DTO field.
    // Preserve explicit unsupported-media values when a fixture provides them.
    const files = getFiles({ query }).map((file) => ({ viewer_support: 'supported', ...file }));
    const index = files.findIndex((item) => item.id === file_id);
    if (index < 0) return route.fulfill({ status: 404, json: { error: { code: 'not_found', message: 'file is not in listing' } } });
    const length = Math.min(count, files.length - 1);
    return route.fulfill({ json: {
      before: Array.from({ length }, (_, offset) => files[(index - offset - 1 + files.length) % files.length]),
      after: Array.from({ length }, (_, offset) => files[(index + offset + 1) % files.length])
    } });
  });
}
