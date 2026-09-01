from pathlib import Path


docs = Path('docs/issue-21-concept-discrepancies.md')
text = docs.read_text()
addition = '38. Login version/build metadata: C. The concept hard-codes `v0.4.2` / `4f7a91d`, but the real unauthenticated health endpoint exposes only `status`; the identity strip keeps the exact three-part geometry while showing honest server readiness instead of fabricated release/build values.'
if addition not in text:
    text = text.rstrip() + '\n' + addition + '\n'
docs.write_text(text)

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
if "test('login matches concept without fabricating build metadata'" not in text:
    text += r'''

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
'''
spec.write_text(text)
