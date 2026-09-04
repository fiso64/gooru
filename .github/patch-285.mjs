import fs from 'node:fs';

function edit(path, replacements) {
  let source = fs.readFileSync(path, 'utf8');
  for (const [before, after] of replacements) {
    if (!source.includes(before)) throw new Error(`missing expected snippet in ${path}: ${before.slice(0, 80)}`);
    source = source.replace(before, after);
  }
  fs.writeFileSync(path, source);
}

edit('frontend/src/lib/components/PreviewDialog.svelte', [
  [
`  function focusTagInput(mode: 'add' | 'remove' = 'add') {
    tagMode = mode;
    document.getElementById(\`tags-\${file.id}\`)?.focus();
  }
`,
`  function focusTagInput(mode: 'add' | 'remove' = 'add') {
    tagMode = mode;
    document.getElementById(\`tags-\${file.id}\`)?.focus();
  }

  function toggleOriginalMedia() {
    if (!originalAvailable) return;
    preferOriginal = !preferOriginal;
    updateViewerSessionPreferences({ preferOriginal });
  }
`
  ],
  [
`    if (key === 'd') {
      event.preventDefault();
      event.stopPropagation();
      downloadLink?.click();
      return;
    }
`,
`    if (key === 'q' && originalAvailable) {
      event.preventDefault();
      event.stopPropagation();
      toggleOriginalMedia();
      return;
    }
    if (key === 'd') {
      event.preventDefault();
      event.stopPropagation();
      downloadLink?.click();
      return;
    }
`
  ],
  [
`    <button class="g-btn g-btn-ghost" type="button" title="Add tag" aria-label="Add tag" onclick={() => focusTagInput('add')}><Icon name="tag" size={16} /></button>`,
`    <button class="g-btn g-btn-ghost" type="button" title="Add tag (T)" aria-label="Add tag" onclick={() => focusTagInput('add')}><Icon name="tag" size={16} /></button>`
  ],
  [
`        title={preferOriginal ? 'Using original media; click to use preview' : 'Load original media'}
        onclick={() => {
          preferOriginal = !preferOriginal;
          updateViewerSessionPreferences({ preferOriginal });
        }}`,
`        title={preferOriginal ? 'Use derived preview (Q)' : 'Use original media (Q)'}
        onclick={toggleOriginalMedia}`
  ],
  [
`title="Download original" aria-label={\`Download \${file.name}\`}`,
`title="Download original (D)" aria-label={\`Download \${file.name}\`}`
  ],
  [
`title="Delete file from disk" aria-label={\`Delete \${file.name} from disk\`}`,
`title="Delete file from disk (Shift+Delete)" aria-label={\`Delete \${file.name} from disk\`}`
  ],
  [
`title="Remove from library without deleting the file" aria-label={\`Untrack \${file.name} from library\`}`,
`title="Remove from library without deleting the file (Delete)" aria-label={\`Untrack \${file.name} from library\`}`
  ]
]);

edit('frontend/src/lib/components/ShortcutsView.svelte', [[
`        { keys: ['u'], description: 'Focus tag input in untag mode' },
        { keys: ['d'], description: 'Download original' },`,
`        { keys: ['u'], description: 'Focus tag input in untag mode' },
        { keys: ['q'], description: 'Toggle original / preview media' },
        { keys: ['d'], description: 'Download original' },`
]]);

edit('frontend/tests/viewer-shortcuts.spec.ts', [
  [
`  await preview.focus();
  await page.keyboard.press('t');
  await expect(page.getByRole('textbox', { name: 'Tags for one.jpg' })).toBeFocused();

  await preview.focus();
  await page.keyboard.press('Delete');`,
`  await preview.focus();
  await page.keyboard.press('t');
  await expect(page.getByRole('textbox', { name: 'Tags for one.jpg' })).toBeFocused();

  await preview.focus();
  const originalToggle = preview.getByRole('button', { name: 'Use original media' });
  await expect(originalToggle).toHaveAttribute('title', 'Use original media (Q)');
  await page.keyboard.press('q');
  const previewToggle = preview.getByRole('button', { name: 'Use derived preview' });
  await expect(previewToggle).toHaveAttribute('aria-pressed', 'true');
  await expect(previewToggle).toHaveAttribute('title', 'Use derived preview (Q)');
  await expect(preview.getByTitle('Add tag (T)')).toBeVisible();
  await expect(preview.getByTitle('Download original (D)')).toBeVisible();
  await expect(preview.getByTitle('Delete file from disk (Shift+Delete)')).toBeVisible();
  await expect(preview.getByTitle('Remove from library without deleting the file (Delete)')).toBeVisible();

  await preview.focus();
  await page.keyboard.press('Delete');`
  ],
  [
`  await expect(shortcuts.getByText('Save current search')).toBeVisible();
  await expect(shortcuts.getByRole('heading', { name: 'Shortcuts' })).toHaveCSS('font-size', '22px');`,
`  await expect(shortcuts.getByText('Save current search')).toBeVisible();
  await expect(shortcuts.getByText('Toggle original / preview media')).toBeVisible();
  await expect(shortcuts.getByRole('heading', { name: 'Shortcuts' })).toHaveCSS('font-size', '22px');`
  ]
]);
