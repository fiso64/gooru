from pathlib import Path

app = Path('frontend/src/lib/components/AppShell.svelte')
text = app.read_text()
old = '''          <button
            class="sidebar-mini saved-search-drag"
            type="button"
            title={`Drag ${saved.name}`}
            aria-label={`Drag ${saved.name}`}
            draggable={!savedSearchReorderBusy}
            disabled={savedSearchReorderBusy}
            ondragstart={(event) => startSavedSearchDrag(event, saved.id)}
            ondragend={() => (draggedSavedSearchID = '')}
          >
            <span aria-hidden="true">⋮⋮</span>
          </button>
          <button class="sidebar-item" type="button" onclick={() => onSavedSearch(saved.query, saved.name)}>
'''
new = '''          <button
            class="sidebar-item saved-search-drag"
            type="button"
            draggable={!savedSearchReorderBusy}
            disabled={savedSearchReorderBusy}
            ondragstart={(event) => startSavedSearchDrag(event, saved.id)}
            ondragend={() => (draggedSavedSearchID = '')}
            onclick={() => onSavedSearch(saved.query, saved.name)}
          >
'''
if old not in text:
    raise SystemExit('saved-search drag markup did not match expected develop state')
text = text.replace(old, new, 1)
text = text.replace('''  .saved-search-drag {
    cursor: grab;
    flex: 0 0 auto;
  }
''', '''  .saved-search-drag {
    cursor: grab;
  }
''', 1)
app.write_text(text)

spec = Path('frontend/tests/saved-search-reorder.spec.ts')
test = spec.read_text()
old_test = "  await page.getByRole('button', { name: 'Drag First' }).dragTo(rows.nth(2));\n"
new_test = "  await expect(page.getByRole('button', { name: 'Drag First' })).toHaveCount(0);\n  const firstEntry = rows.nth(0).locator('.sidebar-item');\n  await expect(firstEntry).toHaveAttribute('draggable', 'true');\n  await firstEntry.dragTo(rows.nth(2));\n"
if old_test not in test:
    raise SystemExit('saved-search browser test did not match expected develop state')
spec.write_text(test.replace(old_test, new_test, 1))
