import { expect, test } from '@playwright/test';

for (const theme of ['booru-light', 'booru-dark'] as const) {
  test(`${theme} keeps every progress bar square`, async ({ page }) => {
    await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ ui_theme: theme })
    }));
    await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ user: null, csrf_token: '' })
    }));

    await page.goto('/');
    await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();

    await page.locator('.gooru-root').evaluate((root) => {
      const native = document.createElement('progress');
      native.dataset.testid = 'native-progress';
      native.max = 100;
      native.value = 50;
      root.append(native);

      for (const className of ['progress', 'job-progress', 'video-progress']) {
        const bar = document.createElement('div');
        bar.className = className;
        bar.dataset.testid = className;
        root.append(bar);
      }
    });

    for (const testID of ['native-progress', 'progress', 'job-progress', 'video-progress']) {
      await expect(page.getByTestId(testID)).toHaveCSS('border-radius', '0px');
    }
  });
}
