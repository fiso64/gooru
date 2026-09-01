from pathlib import Path


def replace_once(path: str, old: str, new: str, label: str) -> None:
    file = Path(path)
    text = file.read_text()
    if old not in text:
        raise SystemExit(f"{label}: source block not found")
    file.write_text(text.replace(old, new, 1))


css = 'frontend/src/lib/styles/components.css'

replace_once(
    css,
    """.page-header h1 {
  font-family: var(--font-display);
  font-size: 38px;
  font-weight: 400;
  letter-spacing: 0;
  margin: 0;
}
""",
    """.page-header h1 {
  font-family: var(--font-display);
  font-size: 38px;
  font-weight: 400;
  letter-spacing: -0.01em;
  margin: 0;
}
""",
    'page header typography',
)

replace_once(
    css,
    """.settings-section-head h2 {
  font-family: var(--font-display);
  font-size: 22px;
  font-weight: 400;
  margin: 0 0 6px;
  letter-spacing: 0;
}
""",
    """.settings-section-head h2 {
  font-family: var(--font-display);
  font-size: 22px;
  font-weight: 400;
  margin: 0 0 6px;
  letter-spacing: -0.005em;
}
""",
    'settings section typography',
)

replace_once(
    css,
    """.tagscloud-item {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 9px 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-3);
  background: var(--surface);
  color: var(--text-2);
  cursor: pointer;
}

.tagscloud-item .count {
  color: var(--text-4);
  font-family: var(--font-mono);
}

.tagscloud-item .ns {
  color: var(--accent);
}
""",
    """.tagscloud-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 12px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r-3);
  font-family: var(--font-mono);
  font-size: 12.5px;
  cursor: pointer;
  transition: background .12s, border-color .12s, color .12s;
}

/* The concept tile is a div. The Svelte port uses a semantic button, so the
   concept's global .gooru-root button font shorthand would otherwise override
   the tile typography. Keep the accessible element while rendering the same
   mono 12.5px tile as the concept. */
button.tagscloud-item {
  font-family: var(--font-mono);
  font-size: 12.5px;
}

.tagscloud-item:hover {
  background: var(--surface-2);
  border-color: var(--border-strong);
}

.tagscloud-item .ns {
  color: var(--accent);
  font-weight: 500;
}

.tagscloud-item .count {
  color: var(--text-4);
  font-size: 11px;
}
""",
    'Tags index tiles',
)

docs = Path('docs/issue-21-concept-discrepancies.md')
text = docs.read_text()
entry = '41. Tags index and shared page typography: B. Keep the real backend tag counts/filter/search routing, but use the concept page-heading tracking and Tags tile padding, mono type, namespace emphasis, count size, and hover treatment directly.'
if entry not in text:
    docs.write_text(text.rstrip() + '\n' + entry + '\n')

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
if "test('matches concept Tags index styling and routes tag clicks'" not in text:
    text += r'''

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
'''
spec.write_text(text)
