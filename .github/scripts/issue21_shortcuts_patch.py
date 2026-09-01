from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"{label} not found")
    return text.replace(old, new, 1)


app = Path("frontend/src/lib/components/AuthenticatedApp.svelte")
text = app.read_text()
text = replace_once(
    text,
    "  function handleKeydown(event: KeyboardEvent) {\n    library.handleKeydown(event, loadedFiles);\n  }",
    "  function isTypingTarget(target: EventTarget | null) {\n    if (!(target instanceof HTMLElement)) return false;\n    return target.isContentEditable || target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT';\n  }\n\n  function handleKeydown(event: KeyboardEvent) {\n    if (event.key === '?' && !isTypingTarget(event.target)) {\n      event.preventDefault();\n      setRoute('shortcuts');\n      return;\n    }\n    library.handleKeydown(event, loadedFiles);\n  }",
    "global shortcuts handler",
)
app.write_text(text)

docs = Path("docs/issue-21-concept-discrepancies.md")
text = docs.read_text()
addition = "33. Shortcuts `?` launcher: B. The concept page explicitly tells the user to press `?` from anywhere, so that launcher is implemented as a real global shortcut outside text-entry controls. Other concept shortcut rows that are not implemented remain visible but greyed as A rather than falsely advertising behavior."
if addition not in text:
    text = text.rstrip() + "\n" + addition + "\n"
docs.write_text(text)

spec = Path("frontend/tests/shell.spec.ts")
text = spec.read_text()
test_body = r'''

test('Shortcuts matches the concept and question mark opens it outside text entry', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  await page.keyboard.press('Shift+/');
  await expect(page.getByRole('heading', { name: 'Shortcuts' })).toBeVisible();
  await expect(page.getByText('Press').locator('..')).toContainText('from anywhere to open this cheatsheet.');
  await expect(page.getByRole('heading', { name: 'Navigation' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Browsing' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Selection' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Tagging' })).toBeVisible();

  const futureFocusSearch = page.locator('.shortcut-row').filter({ hasText: 'Focus search' });
  await expect(futureFocusSearch).toHaveAttribute('title', 'Coming soon');
  const supportedNext = page.locator('.shortcut-row').filter({ hasText: 'Next file' });
  await expect(supportedNext).not.toHaveAttribute('title', 'Coming soon');

  await page.locator('.sidebar').getByRole('button', { name: /Library/ }).click();
  const search = page.getByLabel('Search library');
  await search.focus();
  await page.keyboard.press('Shift+/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Shortcuts' })).toHaveCount(0);
});
'''
if "test('Shortcuts matches the concept and question mark opens it outside text entry'" not in text:
    text += test_body
spec.write_text(text)
