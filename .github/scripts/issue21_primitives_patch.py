from pathlib import Path
import re


def sub_once(path: str, pattern: str, replacement: str, label: str) -> None:
    file = Path(path)
    text = file.read_text()
    updated, count = re.subn(pattern, replacement, text, count=1, flags=re.S)
    if count != 1:
        raise SystemExit(f"{label}: expected one replacement, got {count}")
    file.write_text(updated)


def replace_once(path: str, old: str, new: str, label: str) -> None:
    file = Path(path)
    text = file.read_text()
    if old not in text:
        raise SystemExit(f"{label}: source block not found")
    file.write_text(text.replace(old, new, 1))


# The concept uses real 500/700 IBM Plex weights. The validation workflow
# downloads those exact self-hosted files from a pinned upstream revision.
tokens = Path('frontend/src/lib/styles/tokens.css')
text = tokens.read_text()
_, rest = text.split(':root {', 1)
tokens.write_text("""@font-face {
  font-family: 'IBM Plex Sans';
  src: url('$lib/assets/fonts/IBMPlexSans-Regular.ttf') format('truetype');
  font-weight: 400;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Sans';
  src: url('$lib/assets/fonts/IBMPlexSans-Medium.ttf') format('truetype');
  font-weight: 500;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Sans';
  src: url('$lib/assets/fonts/IBMPlexSans-SemiBold.ttf') format('truetype');
  font-weight: 600;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Sans';
  src: url('$lib/assets/fonts/IBMPlexSans-Bold.ttf') format('truetype');
  font-weight: 700;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Mono';
  src: url('$lib/assets/fonts/IBMPlexMono-Regular.ttf') format('truetype');
  font-weight: 400;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Mono';
  src: url('$lib/assets/fonts/IBMPlexMono-Medium.ttf') format('truetype');
  font-weight: 500;
  font-display: swap;
}

@font-face {
  font-family: 'IBM Plex Mono';
  src: url('$lib/assets/fonts/IBMPlexMono-SemiBold.ttf') format('truetype');
  font-weight: 600;
  font-display: swap;
}

@font-face {
  font-family: 'Instrument Serif';
  src: url('$lib/assets/fonts/InstrumentSerif-Regular.ttf') format('truetype');
  font-weight: 400;
  font-display: swap;
}

:root {""" + rest)

# Port the concept's utility/focus rules directly rather than compensating for
# the old approximation in every component.
Path('frontend/src/lib/styles/base.css').write_text(""".gooru-root *,
.gooru-root *::before,
.gooru-root *::after {
  box-sizing: border-box;
}

.gooru-root button {
  font: inherit;
  color: inherit;
}

.gooru-root input,
.gooru-root textarea,
.gooru-root select {
  font: inherit;
  color: inherit;
}

.gooru-root ::selection {
  background: var(--accent);
  color: var(--accent-ink);
}

.g-mono {
  font-family: var(--font-mono);
  font-feature-settings: 'ss01';
}

.g-display {
  font-family: var(--font-display);
  font-weight: 400;
  letter-spacing: -0.01em;
}

.g-eyebrow {
  font-family: var(--font-mono);
  font-size: 10.5px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--text-3);
}

.g-eyebrow-accent {
  color: var(--accent);
}

.gooru-root :focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
  border-radius: 4px;
}

.g-btn:focus,
.g-input:focus,
.g-tag:focus {
  outline: none;
}

.g-btn:focus-visible,
.g-tag:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}
""")

# Sidebar section heads are deliberately smaller/dimmer than generic concept
# eyebrows; the prior selector accidentally made every eyebrow inherit that
# sidebar-only treatment.
replace_once(
    'frontend/src/lib/styles/layout.css',
    ".sidebar-section-head,\n.g-eyebrow {\n  font-family: var(--font-mono);\n  font-size: 10px;\n  letter-spacing: 0.14em;\n  text-transform: uppercase;\n  color: var(--text-4);\n}\n\n.g-eyebrow-accent {\n  color: var(--accent);\n}\n",
    ".sidebar-section-head {\n  font-family: var(--font-mono);\n  font-size: 10px;\n  letter-spacing: 0.14em;\n  text-transform: uppercase;\n  color: var(--text-4);\n}\n",
    'sidebar eyebrow split',
)

components = Path('frontend/src/lib/styles/components.css')
text = components.read_text()
text, count = re.subn(
    r"\.g-btn \{.*?(?=\.g-card \{)",
    """.g-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border-radius: var(--r-3);
  background: var(--surface-2);
  border: 1px solid var(--border);
  color: var(--text);
  cursor: pointer;
  transition: background 0.12s, border-color 0.12s, color 0.12s, transform 0.04s;
  font-size: 13px;
  font-weight: 500;
  letter-spacing: 0.005em;
  white-space: nowrap;
}

.g-btn:hover {
  background: var(--surface-3);
  border-color: var(--border-strong);
}

.g-btn:active {
  transform: translateY(0.5px);
}

.g-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.g-btn-primary {
  background: var(--accent);
  color: var(--accent-ink);
  border-color: var(--accent);
}

.g-btn-primary:hover {
  background: oklch(from var(--accent) calc(l - 0.04) c h);
  border-color: oklch(from var(--accent) calc(l - 0.04) c h);
}

.g-btn-ghost {
  background: transparent;
  border-color: transparent;
}

.g-btn-ghost:hover {
  background: var(--surface-2);
  border-color: var(--border);
}

.g-btn-sm {
  padding: 5px 10px;
  font-size: 12px;
  gap: 6px;
}

.g-btn-icon {
  padding: 7px;
}

.g-btn-icon.g-btn-sm {
  padding: 5px;
}

.g-input {
  width: 100%;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r-3);
  padding: 9px 12px;
  color: var(--text);
  font-size: 14px;
  outline: none;
  transition: border-color 0.12s, background 0.12s, box-shadow 0.12s;
}

.g-input::placeholder {
  color: var(--text-4);
}

.g-input:focus {
  border-color: var(--accent-line);
  background: var(--bg-2);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.g-input.compact {
  width: auto;
}

.seg {
  display: inline-flex;
  padding: 3px;
  background: var(--surface-2);
  border-radius: var(--r-3);
  border: 1px solid var(--border);
}

.seg button {
  appearance: none;
  border: 0;
  background: transparent;
  padding: 5px 12px;
  font-size: 12px;
  font-weight: 500;
  border-radius: 4px;
  cursor: pointer;
  color: var(--text-2);
  font-family: var(--font-ui);
}

.seg button.active,
.seg button.is-active {
  background: var(--surface-3);
  color: var(--text);
}

.seg button:hover:not(.active):not(.is-active) {
  color: var(--text);
}

""",
    text,
    count=1,
    flags=re.S,
)
if count != 1:
    raise SystemExit(f'button/input/seg primitives: expected one replacement, got {count}')

text, count = re.subn(
    r"\.g-card \{.*?(?=\.page \{)",
    """.g-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--r-4);
}

.g-divider {
  height: 1px;
  background: var(--border);
  border: 0;
}

.g-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 8px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-radius: 3px;
  font-family: var(--font-mono);
  font-size: 11.5px;
  color: var(--text-2);
  white-space: nowrap;
  cursor: pointer;
  transition: background 0.1s, border-color 0.1s, color 0.1s;
}

.g-tag:hover {
  background: var(--surface-3);
  color: var(--text);
}

.g-tag .ns,
.g-tag-ns {
  color: var(--accent);
  font-weight: 500;
}

.g-tag-count {
  color: var(--text-4);
  font-variant-numeric: tabular-nums;
  font-size: 10.5px;
}

.g-tag-active {
  background: var(--accent-soft);
  border-color: var(--accent-line);
  color: var(--accent);
}

.g-tag-active .ns,
.g-tag-active .g-tag-ns {
  color: var(--accent);
}

""",
    text,
    count=1,
    flags=re.S,
)
if count != 1:
    raise SystemExit(f'card/tag primitives: expected one replacement, got {count}')

old_grid = ".grid {\n  display: grid;\n  gap: 4px;\n  padding: 16px;\n  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));\n  align-content: start;\n}"
new_grid = ".grid {\n  display: grid;\n  gap: var(--grid-gap, 5px);\n  padding: var(--grid-pad, 16px);\n  grid-template-columns: repeat(auto-fill, minmax(var(--grid-cell, 180px), 1fr));\n  align-content: start;\n}"
if old_grid not in text:
    raise SystemExit('concept grid block not found')
text = text.replace(old_grid, new_grid, 1)

old_badge = "  font-family: var(--font-mono);\n  font-size: 10px;\n  color: #fff;\n}"
new_badge = "  font-family: var(--font-mono);\n  font-size: 10px;\n  color: #fff;\n  font-weight: 500;\n}"
if old_badge not in text:
    raise SystemExit('thumbnail badge block not found')
text = text.replace(old_badge, new_badge, 1)

old_auth = ".auth-submit {\n  width: 100%;\n  height: 36px;\n}"
new_auth = ".auth-submit {\n  width: 100%;\n  height: 36px;\n  justify-content: center;\n}"
if old_auth not in text:
    raise SystemExit('auth submit block not found')
text = text.replace(old_auth, new_auth, 1)
components.write_text(text)

replace_once(
    'frontend/src/lib/state/ui.ts',
    'const gridGap = 4;',
    'const gridGap = 5;',
    'virtual grid gap',
)

# Persist the design decision and a computed-style/browser regression so the
# old approximate primitives cannot silently return.
docs = Path('docs/issue-21-concept-discrepancies.md')
text = docs.read_text()
addition = '39. Shared design primitives and self-hosted font weights: B. Buttons, inputs, segmented controls, cards, tags, focus rings, and default gallery geometry must use the concept source values directly; the exact IBM Plex 500/700 faces used by those rules are self-hosted rather than browser-synthesized.'
if addition not in text:
    text = text.rstrip() + '\n' + addition + '\n'
docs.write_text(text)

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
if "test('uses exact concept primitives and font weights'" not in text:
    text += r'''

test('uses exact concept primitives and font weights', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [fileItem('concept-one', 'concept.jpg')], total_count: 1, library_count: 1, facets: { kind: [{ value: 'photo', count: 1 }] } })
    });
  });

  await page.goto('/');
  const input = page.getByLabel('Username');
  const inputStyle = await input.evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, radius: style.borderRadius, fontSize: style.fontSize };
  });
  expect(inputStyle).toEqual({ padding: '9px 12px', radius: '6px', fontSize: '14px' });

  const fontFaces = await page.evaluate(() => Array.from(document.fonts).map((face) => ({
    family: face.family.replace(/["']/g, ''),
    weight: face.weight
  })));
  expect(fontFaces).toContainEqual({ family: 'IBM Plex Sans', weight: '500' });
  expect(fontFaces).toContainEqual({ family: 'IBM Plex Sans', weight: '700' });
  expect(fontFaces).toContainEqual({ family: 'IBM Plex Mono', weight: '500' });
  expect(fontFaces).toContainEqual({ family: 'IBM Plex Mono', weight: '600' });

  await signIn(page);
  const sortButton = page.getByTitle('Sort direction');
  const buttonStyle = await sortButton.evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, radius: style.borderRadius, fontSize: style.fontSize, weight: style.fontWeight, gap: style.gap };
  });
  expect(buttonStyle).toEqual({ padding: '5px 10px', radius: '6px', fontSize: '12px', weight: '500', gap: '6px' });

  const segment = page.getByRole('button', { name: 'Modified' });
  const segmentStyle = await segment.evaluate((node) => {
    const style = getComputedStyle(node);
    return { padding: style.padding, radius: style.borderRadius, fontSize: style.fontSize, weight: style.fontWeight };
  });
  expect(segmentStyle).toEqual({ padding: '5px 12px', radius: '4px', fontSize: '12px', weight: '500' });

  const grid = page.getByTestId('virtual-media-grid');
  const gridStyle = await grid.evaluate((node) => {
    const style = getComputedStyle(node);
    return { gap: style.gap, padding: style.padding };
  });
  expect(gridStyle).toEqual({ gap: '5px', padding: '16px' });
});
'''
spec.write_text(text)
