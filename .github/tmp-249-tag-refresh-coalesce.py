from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    '''      await tagMutation.mutateAsync({ operation, body: { file_ids: [remoteFileID], tags: changedTags } });\n''',
    '''      await new ApiClient($authState.csrfToken).mutateTags(operation, { file_ids: [remoteFileID], tags: changedTags });\n''',
)
replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    '''      const currentIndex = upload.items.indexOf(item);\n      if (currentIndex >= 0 && item.tagSyncPending && item.remoteFileID && !item.tagSyncError) {\n        void reconcileUploadItemTags(currentIndex);\n      }\n''',
    '''      const currentIndex = upload.items.indexOf(item);\n      const needsAnotherPass = currentIndex >= 0 && item.tagSyncPending && item.remoteFileID && !item.tagSyncError;\n      if (needsAnotherPass) {\n        void reconcileUploadItemTags(currentIndex);\n      } else if (uploadTagSyncRuns.size === 0) {\n        // Per-item upload tag reconciliation updates local row state itself. Refresh the\n        // broad library/tag queries once after the current reconciliation wave instead\n        // of once per item/delta through the general-purpose tag mutation.\n        void refreshUploadQueries(queryClient).catch(() => undefined);\n      }\n''',
)

replace_once(
    "frontend/tests/upload-viewer.spec.ts",
    '''  tagMutations?: TagMutation[];\n  tagMutationGate?: Promise<void>;\n  fileRemovals?: FileRemoval[];\n''',
    '''  tagMutations?: TagMutation[];\n  tagMutationGate?: Promise<void>;\n  fileListRequests?: string[];\n  tagListRequests?: string[];\n  fileRemovals?: FileRemoval[];\n''',
)
replace_once(
    "frontend/tests/upload-viewer.spec.ts",
    '''  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));\n''',
    '''  await page.route('**/api/v1/files?**', async (route) => {\n    options.fileListRequests?.push(route.request().url());\n    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });\n  });\n''',
)
replace_once(
    "frontend/tests/upload-viewer.spec.ts",
    '''  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({\n    contentType: 'application/json',\n    body: JSON.stringify({\n      tags: [\n        { name: 'artist:alice', count: 8 },\n        { name: 'artist:alina', count: 5 },\n        { name: 'artist:alex', count: 3 },\n        { name: 'artist:amelia', count: 2 }\n      ]\n    })\n  }));\n''',
    '''  await page.route('**/api/v1/tags?**', async (route) => {\n    options.tagListRequests?.push(route.request().url());\n    await route.fulfill({\n      contentType: 'application/json',\n      body: JSON.stringify({\n        tags: [\n          { name: 'artist:alice', count: 8 },\n          { name: 'artist:alina', count: 5 },\n          { name: 'artist:alex', count: 3 },\n          { name: 'artist:amelia', count: 2 }\n        ]\n      })\n    });\n  });\n''',
)

p = Path("frontend/tests/upload-viewer.spec.ts")
text = p.read_text()
anchor = '''test('completed row stays stable while a direct tag save settles', async ({ page }) => {\n'''
if text.count(anchor) != 1:
    raise SystemExit(f"expected one test anchor, found {text.count(anchor)}")
new_test = '''test('concurrent post-upload tag saves coalesce broad library refreshes', async ({ page }) => {\n  let releaseTagMutation: (() => void) | undefined;\n  const tagMutationGate = new Promise<void>((resolve) => { releaseTagMutation = resolve; });\n  const tagMutations: TagMutation[] = [];\n  const fileListRequests: string[] = [];\n  const tagListRequests: string[] = [];\n  await mockUploadApp(page, { completeAsDuplicate: true, tagMutations, tagMutationGate, fileListRequests, tagListRequests });\n\n  const chooser = page.locator('input[type="file"]');\n  await chooser.setInputFiles({ name: 'duplicate.png', mimeType: 'image/png', buffer: png });\n  await page.getByRole('button', { name: 'Upload 1 file' }).click();\n  await expect(page.getByText('duplicate existing', { exact: true })).toBeVisible();\n  await chooser.setInputFiles({ name: 'duplicate.png', mimeType: 'image/png', buffer: png });\n  await page.getByRole('button', { name: 'Upload 1 file' }).click();\n  const previews = page.getByRole('button', { name: 'Preview duplicate.png' });\n  await expect(previews).toHaveCount(2);\n\n  fileListRequests.length = 0;\n  tagListRequests.length = 0;\n\n  await previews.nth(0).click();\n  const dialog = page.getByRole('dialog', { name: 'duplicate.png' });\n  let remoteTagRemove = dialog.getByRole('button', { name: 'Remove remote:existing' });\n  await expect(remoteTagRemove).toBeVisible();\n  await remoteTagRemove.click();\n  await expect.poll(() => tagMutations.length).toBe(1);\n\n  await page.keyboard.press('ArrowRight');\n  remoteTagRemove = dialog.getByRole('button', { name: 'Remove remote:existing' });\n  await expect(remoteTagRemove).toBeEnabled();\n  await remoteTagRemove.click();\n  await expect.poll(() => tagMutations.length).toBe(2);\n  expect(fileListRequests).toHaveLength(0);\n  expect(tagListRequests).toHaveLength(0);\n\n  releaseTagMutation?.();\n  await expect.poll(() => fileListRequests.length).toBe(1);\n  await expect.poll(() => tagListRequests.length).toBe(1);\n  expect(tagMutations).toEqual([\n    { method: 'DELETE', body: { file_ids: ['file-existing'], tags: ['remote:existing'], verbose: false } },\n    { method: 'DELETE', body: { file_ids: ['file-existing'], tags: ['remote:existing'], verbose: false } }\n  ]);\n});\n\n'''
p.write_text(text.replace(anchor, new_test + anchor, 1))
