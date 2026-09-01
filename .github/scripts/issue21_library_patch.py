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
    """  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-family: var(--font-mono);
""",
    """  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  font-family: var(--font-mono);
""",
    'thumbnail metadata alignment',
)

replace_once(
    css,
    """  background: rgba(0, 0, 0, 0.72);
  backdrop-filter: blur(8px);
  border-radius: 3px;
""",
    """  background: rgba(0, 0, 0, 0.72);
  -webkit-backdrop-filter: blur(8px);
  backdrop-filter: blur(8px);
  border-radius: 3px;
""",
    'thumbnail badge backdrop',
)

replace_once(
    css,
    """  background: rgba(0, 0, 0, 0.55);
  border: 1.5px solid rgba(255, 255, 255, 0.55);
  backdrop-filter: blur(8px);
  display: grid;
  place-items: center;
  opacity: 0;
  cursor: pointer;
""",
    """  background: rgba(0, 0, 0, 0.55);
  border: 1.5px solid rgba(255, 255, 255, 0.55);
  -webkit-backdrop-filter: blur(8px);
  backdrop-filter: blur(8px);
  display: grid;
  place-items: center;
  opacity: 0;
  transition: opacity .12s, background .12s, border-color .12s;
  cursor: pointer;
""",
    'thumbnail checkbox transition',
)

docs = Path('docs/issue-21-concept-discrepancies.md')
text = docs.read_text()
entry = '42. Library gallery and selection states: B. Keep bounded pagination/virtualization and real file actions, while rendering the concept gallery header, density geometry, thumbnail hover metadata/badges/selection ring, and selection toolbar directly; unsupported Export remains visibly disabled (A).'
if entry not in text:
    docs.write_text(text.rstrip() + '\n' + entry + '\n')

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
if "test('matches concept Library gallery and selection states'" not in text:
    text += r'''

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
'''
spec.write_text(text)
