import fs from 'node:fs';

const appPath = 'frontend/src/lib/components/AppShell.svelte';
let text = fs.readFileSync(appPath, 'utf8');
const oldMarkup = `          <button
            class="sidebar-mini saved-search-drag"
            type="button"
            title={\`Drag \${saved.name}\`}
            aria-label={\`Drag \${saved.name}\`}
            draggable={!savedSearchReorderBusy}
            disabled={savedSearchReorderBusy}
            ondragstart={(event) => startSavedSearchDrag(event, saved.id)}
            ondragend={() => (draggedSavedSearchID = '')}
          >
            <span aria-hidden="true">⋮⋮</span>
          </button>
          <button class="sidebar-item" type="button" onclick={() => onSavedSearch(saved.query, saved.name)}>
`;
const newMarkup = `          <button
            class="sidebar-item saved-search-drag"
            type="button"
            draggable={!savedSearchReorderBusy}
            disabled={savedSearchReorderBusy}
            ondragstart={(event) => startSavedSearchDrag(event, saved.id)}
            ondragend={() => (draggedSavedSearchID = '')}
            onclick={() => onSavedSearch(saved.query, saved.name)}
          >
`;
if (!text.includes(oldMarkup)) throw new Error('saved-search drag markup did not match expected develop state');
text = text.replace(oldMarkup, newMarkup);
text = text.replace(`  .saved-search-drag {
    cursor: grab;
    flex: 0 0 auto;
  }
`, `  .saved-search-drag {
    cursor: grab;
  }
`);
fs.writeFileSync(appPath, text);

const specPath = 'frontend/tests/saved-search-reorder.spec.ts';
let test = fs.readFileSync(specPath, 'utf8');
const oldTest = `  await page.getByRole('button', { name: 'Drag First' }).dragTo(rows.nth(2));
`;
const newTest = `  await expect(page.getByRole('button', { name: 'Drag First' })).toHaveCount(0);
  const firstEntry = rows.nth(0).locator('.sidebar-item');
  await expect(firstEntry).toHaveAttribute('draggable', 'true');
  await firstEntry.dragTo(rows.nth(2));
`;
if (!test.includes(oldTest)) throw new Error('saved-search browser test did not match expected develop state');
fs.writeFileSync(specPath, test.replace(oldTest, newTest));
