from pathlib import Path

path = Path('frontend/src/lib/components/UploadPanel.svelte')
text = path.read_text()
text = text.replace(
    '  let tagInput: HTMLInputElement | undefined;\n',
    '  let tagInput = $state<HTMLInputElement | undefined>();\n',
    1,
)
text = text.replace(
    '    const urls = uploadFiles.map((file) =>\n',
    '    const urls = uploadFiles.map((file: File) =>\n',
    1,
)
text = text.replace(
    '<section class="upload-queue-section">',
    '<section class="upload-queue-section" aria-label={uploadStatus || \'Upload queue\'}>',
    1,
)
text = text.replace(
    '          {#if uploadStatus && !activeUploadJobID && !queueItems.length}<p class="status-note">{uploadStatus}</p>{/if}\n',
    '',
    1,
)
path.write_text(text)

spec = Path('frontend/tests/shell.spec.ts')
text = spec.read_text()
text = text.replace(
    "  await expect(page.getByText('Importing', { exact: true })).toBeVisible();\n",
    "  await expect(page.getByText('importing', { exact: true })).toBeVisible();\n",
    1,
)
text = text.replace(
    "  await expect(page.locator('.upload-zone')).toHaveClass(/drag-active/);\n",
    "  await expect(page.locator('.upload-zone')).toHaveClass(/is-drag/);\n",
    1,
)
text = text.replace(
    "  await expect(page.getByRole('button', { name: 'Paste URL' })).toBeDisabled();\n",
    "  await expect(page.getByRole('button', { name: 'Paste URL', exact: true })).toBeDisabled();\n",
    1,
)
spec.write_text(text)
