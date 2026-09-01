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
    """.lightbox {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: grid;
  grid-template-columns: 320px 1fr 60px;
  background: oklch(0.10 0.005 70 / 0.94);
  backdrop-filter: blur(20px);
}
""",
    """.lightbox {
  position: absolute;
  inset: 0;
  background: oklch(0.10 0.005 70 / 0.92);
  -webkit-backdrop-filter: blur(20px);
  backdrop-filter: blur(20px);
  display: grid;
  grid-template-columns: 320px 1fr 60px;
  z-index: 20;
}
""",
    'lightbox shell',
)

replace_once(
    css,
    """.lightbox-aside {
  border-right: 1px solid var(--border);
  background: var(--bg);
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  overflow-y: auto;
}
""",
    """.lightbox-aside {
  border-right: 1px solid var(--border);
  background: var(--bg);
  padding: 18px 18px 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  overflow-y: auto;
}
""",
    'lightbox aside',
)

replace_once(
    css,
    """.lightbox-name {
  font-family: var(--font-display);
  font-size: 24px;
  font-weight: 400;
  word-break: break-word;
  line-height: 1.1;
  margin: 0;
}
""",
    """.lightbox-name {
  font-family: var(--font-display);
  font-size: 22px;
  letter-spacing: -0.005em;
  word-break: break-all;
  line-height: 1.15;
  margin: 0;
}
""",
    'lightbox name',
)

replace_once(
    css,
    """.lightbox-meta {
  display: grid;
  grid-template-columns: max-content 1fr;
  column-gap: 16px;
  row-gap: 5px;
  font-family: var(--font-mono);
  font-size: 11.5px;
}

.lightbox-meta dt {
  color: var(--text-4);
  text-transform: uppercase;
  letter-spacing: 0.1em;
  font-size: 10px;
}

.lightbox-meta dd {
  color: var(--text);
  margin: 0;
  word-break: break-word;
}

.lightbox-meta dd.hash {
  font-size: 10.5px;
  color: var(--text-2);
}
""",
    """.lightbox-meta {
  display: grid;
  grid-template-columns: max-content 1fr;
  column-gap: 16px;
  row-gap: 4px;
  font-family: var(--font-mono);
  font-size: 11.5px;
}

.lightbox-meta dt {
  color: var(--text-4);
  text-transform: uppercase;
  letter-spacing: 0.1em;
  font-size: 10px;
  align-self: center;
}

.lightbox-meta dd {
  color: var(--text);
  margin: 0;
}

.lightbox-meta dd.path {
  word-break: break-all;
}

.lightbox-meta dd.hash {
  font-size: 10.5px;
  color: var(--text-2);
  word-break: break-all;
}
""",
    'lightbox metadata',
)

replace_once(
    css,
    """.lightbox-tag-group {
  display: grid;
  gap: 4px;
}

.lightbox-tag-group-head {
  display: flex;
  justify-content: space-between;
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--text-4);
}
""",
    """.lightbox-tag-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.lightbox-tag-group-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-family: var(--font-mono);
  font-size: 10px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--text-4);
}
""",
    'lightbox tag groups',
)

replace_once(
    css,
    """.lightbox-tag-input input {
  min-width: 0;
  flex: 1;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text);
}
""",
    """.lightbox-tag-input input {
  min-width: 0;
  flex: 1;
  background: transparent;
  border: 0;
  outline: none;
  font: inherit;
  color: var(--text);
}
""",
    'lightbox tag input',
)

replace_once(
    css,
    """.lightbox-rail .g-btn {
  width: 36px;
  height: 36px;
  padding: 0;
}
""",
    """.lightbox-rail .g-btn {
  width: 36px;
  height: 36px;
  padding: 0;
  justify-content: center;
}
""",
    'lightbox rail buttons',
)

replace_once(
    css,
    """.lightbox-nav-arrow {
  position: absolute;
  top: 50%;
  width: 44px;
  height: 44px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid var(--border);
  color: var(--text-2);
  display: grid;
  place-items: center;
  cursor: pointer;
}

.lightbox-nav-arrow.prev {
  left: 12px;
  transform: translateY(-50%) rotate(45deg);
}

.lightbox-nav-arrow.next {
  right: 12px;
  transform: translateY(-50%) rotate(-135deg);
}
""",
    """.lightbox-nav-arrow {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  width: 44px;
  height: 44px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid var(--border);
  color: var(--text-2);
  display: grid;
  place-items: center;
  cursor: pointer;
  transition: background .12s, color .12s, transform .12s;
}

.lightbox-nav-arrow:hover {
  background: rgba(255, 255, 255, 0.10);
  color: var(--text);
}

.lightbox-nav-arrow.prev {
  left: 12px;
}

.lightbox-nav-arrow.next {
  right: 12px;
}
""",
    'lightbox navigation arrows',
)

# Document the visual/source decision. The real video/audio controls remain the
# B-class backend-connected extension layered into the exact concept frame.
docs = Path('docs/issue-21-concept-discrepancies.md')
text = docs.read_text()
entry = '40. Lightbox exact frame and real media behavior: B. Port the concept overlay/panel/metadata/tag/rail/navigation rendering directly while retaining the real photo/video/audio sources, video controls, tag mutations, download/open-original links, safe-content policy, and untrack workflow.'
if entry not in text:
    docs.write_text(text.rstrip() + '\n' + entry + '\n')

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
if "test('matches concept Lightbox geometry and navigation'" not in text:
    text += r'''

test('matches concept Lightbox geometry and navigation', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  const first = fileItem('lightbox-one', 'first-very-long-preview-name.jpg');
  const second = fileItem('lightbox-two', 'second.jpg');
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [first, second], total_count: 2, library_count: 2, facets: { kind: [{ value: 'photo', count: 2 }] } })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#444"/></svg>' });
  });
  await page.route('**/api/v1/files/*/preview', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64" fill="#222"/></svg>' });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: `Preview ${first.name}` }).click();

  let dialog = page.getByRole('dialog', { name: first.name });
  await expect(dialog).toBeVisible();
  const shellStyle = await dialog.evaluate((node) => {
    const style = getComputedStyle(node);
    return { position: style.position, backdrop: style.backdropFilter };
  });
  expect(shellStyle.position).toBe('absolute');
  expect(shellStyle.backdrop).toBe('blur(20px)');

  const asideStyle = await dialog.locator('.lightbox-aside').evaluate((node) => {
    const style = getComputedStyle(node);
    return { top: style.paddingTop, right: style.paddingRight, bottom: style.paddingBottom, left: style.paddingLeft };
  });
  expect(asideStyle).toEqual({ top: '18px', right: '18px', bottom: '16px', left: '18px' });

  const titleStyle = await dialog.getByRole('heading', { name: first.name }).evaluate((node) => {
    const style = getComputedStyle(node);
    return { fontSize: style.fontSize, wordBreak: style.wordBreak };
  });
  expect(titleStyle).toEqual({ fontSize: '22px', wordBreak: 'break-all' });
  await expect.poll(() => dialog.locator('.lightbox-meta').evaluate((node) => getComputedStyle(node).rowGap)).toBe('4px');
  await expect.poll(() => dialog.getByRole('button', { name: 'Add tag' }).evaluate((node) => getComputedStyle(node).justifyContent)).toBe('center');

  const prevTransform = await dialog.getByRole('button', { name: 'Previous file' }).evaluate((node) => {
    const matrix = new DOMMatrix(getComputedStyle(node).transform);
    return { a: matrix.a, b: matrix.b, c: matrix.c, d: matrix.d };
  });
  expect(prevTransform).toEqual({ a: 1, b: 0, c: 0, d: 1 });

  await dialog.getByRole('button', { name: 'Add tag' }).click();
  await expect(dialog.getByLabel(`Tags for ${first.name}`)).toBeFocused();
  await dialog.getByLabel(`Tags for ${first.name}`).evaluate((node) => (node as HTMLInputElement).blur());

  await page.keyboard.press('ArrowRight');
  dialog = page.getByRole('dialog', { name: second.name });
  await expect(dialog).toBeVisible();
  await page.keyboard.press('k');
  dialog = page.getByRole('dialog', { name: first.name });
  await expect(dialog).toBeVisible();
  await page.keyboard.press('j');
  dialog = page.getByRole('dialog', { name: second.name });
  await expect(dialog).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
});
'''
spec.write_text(text)
