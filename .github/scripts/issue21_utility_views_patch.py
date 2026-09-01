from pathlib import Path

docs = Path('docs/issue-21-concept-discrepancies.md')
text = docs.read_text()
entry = '43. Settings, Jobs, and Shortcuts rendered utility surfaces: B for preserving the concept page/section/drawer/row geometry around real supported behavior; A controls remain visibly disabled/greyed, and C prototype-only auth/server facts remain absent rather than fabricated.'
if entry not in text:
    docs.write_text(text.rstrip() + '\n' + entry + '\n')

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
if "test('matches concept utility views while exposing only real capabilities'" not in text:
    text += r'''

test('matches concept utility views while exposing only real capabilities', async ({ page }) => {
  await mockAuth(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/jobs', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: [
          { id: 'utility-run', type: 'upload_import', status: 'running', progress: 0.4, submitted_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z' },
          { id: 'utility-done', type: 'bulk_tag', status: 'completed', progress: 1, submitted_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z', completed_at: '2026-05-20T00:02:00Z' }
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
  await expect(page.getByLabel('Appearance settings coming soon').getByRole('slider')).toBeDisabled();
  await expect(page.getByLabel('Server settings coming soon').getByLabel('Public URL')).toBeDisabled();
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
  expect(shortcutGridStyle.gap).toBe('24px 40px');
  expect(shortcutGridStyle.columns).not.toBe('none');
  const futureShortcut = page.locator('.shortcut-row').filter({ hasText: 'Focus search' });
  await expect.poll(() => futureShortcut.evaluate((node) => getComputedStyle(node).opacity)).toBe('0.42');
  const launcherShortcut = page.locator('.shortcut-row').filter({ hasText: 'Show this cheatsheet' });
  expect(await launcherShortcut.evaluate((node) => getComputedStyle(node).opacity)).toBe('1');

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
  expect(rowStyle).toEqual({ padding: '12px 14px', gap: '6px', display: 'flex' });
  await expect.poll(() => row.locator('.job-progress').evaluate((node) => getComputedStyle(node).height)).toBe('4px');
});
'''
spec.write_text(text)
