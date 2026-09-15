from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "frontend/src/lib/state/uploadItems.ts",
    '''export function rebaseUploadItemTagsFromRemoteInPlace(items: UploadItem[], index: number, remoteTags: string[]): void {\n  const current = items[index];\n  if (!current) return;\n''',
    '''function uploadItemTagSyncBaseMatches(current: string[] | undefined, expected: string[] | undefined): boolean {\n  if (current === undefined || expected === undefined) return current === expected;\n  const normalizedCurrent = normalizeUploadItemTags(current);\n  const normalizedExpected = normalizeUploadItemTags(expected);\n  return normalizedCurrent.length === normalizedExpected.length\n    && normalizedCurrent.every((tag, index) => tag === normalizedExpected[index]);\n}\n\nexport function rebaseUploadItemTagsFromRemoteInPlace(\n  items: UploadItem[],\n  index: number,\n  remoteTags: string[],\n  expectedBaseTags: string[] | undefined\n): boolean {\n  const current = items[index];\n  if (!current || !uploadItemTagSyncBaseMatches(current.tagSyncBaseTags, expectedBaseTags)) return false;\n''',
)
replace_once(
    "frontend/src/lib/state/uploadItems.ts",
    '''  current.tagSyncPending = pending && !current.tagSyncError;\n}\n\nexport function markUploadItemTagSyncErrorInPlace''',
    '''  current.tagSyncPending = pending && !current.tagSyncError;\n  return true;\n}\n\nexport function markUploadItemTagSyncErrorInPlace''',
)
replace_once(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    '''  function rebaseItemTagsFromRemote(index: number, remoteTags: string[]) {\n    rebaseUploadItemTagsFromRemoteInPlace(items, index, remoteTags);\n  }\n''',
    '''  function rebaseItemTagsFromRemote(index: number, remoteTags: string[], expectedBaseTags: string[] | undefined) {\n    return rebaseUploadItemTagsFromRemoteInPlace(items, index, remoteTags, expectedBaseTags);\n  }\n''',
)
replace_once(
    "frontend/src/lib/components/AuthenticatedApp.svelte",
    '''  function rebaseUploadItemTagsFromRemote(index: number, remoteTags: string[]) {\n    upload.rebaseItemTagsFromRemote(index, remoteTags);\n    void reconcileUploadItemTags(index);\n  }\n''',
    '''  function rebaseUploadItemTagsFromRemote(index: number, remoteTags: string[], expectedBaseTags: string[] | undefined) {\n    if (!upload.rebaseItemTagsFromRemote(index, remoteTags, expectedBaseTags)) return;\n    void reconcileUploadItemTags(index);\n  }\n''',
)
replace_once(
    "frontend/src/lib/components/UploadPanel.svelte",
    '''    onItemRemoteTagsLoaded: (index: number, tags: string[]) => void;\n''',
    '''    onItemRemoteTagsLoaded: (index: number, tags: string[], expectedBaseTags: string[] | undefined) => void;\n''',
)
replace_once(
    "frontend/src/lib/components/UploadViewerDialog.svelte",
    '''<script lang="ts">\n  import Icon from './Icon.svelte';\n''',
    '''<script lang="ts">\n  import { untrack } from 'svelte';\n  import Icon from './Icon.svelte';\n''',
)
replace_once(
    "frontend/src/lib/components/UploadViewerDialog.svelte",
    '''    onItemRemoteTagsLoaded: (index: number, tags: string[]) => void;\n''',
    '''    onItemRemoteTagsLoaded: (index: number, tags: string[], expectedBaseTags: string[] | undefined) => void;\n''',
)
replace_once(
    "frontend/src/lib/components/UploadViewerDialog.svelte",
    '''    const controller = new AbortController();\n    const client = new ApiClient($authState.csrfToken);\n''',
    '''    const requestIndex = activeIndex;\n    const expectedBaseTags = untrack(() => activeItem?.tagSyncBaseTags ? [...activeItem.tagSyncBaseTags] : undefined);\n    const controller = new AbortController();\n    const client = new ApiClient($authState.csrfToken);\n''',
)
replace_once(
    "frontend/src/lib/components/UploadViewerDialog.svelte",
    '''        onItemRemoteTagsLoaded(activeIndex, file.tags ?? []);\n''',
    '''        onItemRemoteTagsLoaded(requestIndex, file.tags ?? [], expectedBaseTags);\n''',
)
replace_once(
    "frontend/src/lib/state/uploadItems.test.ts",
    '''    rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['remote:existing', 'submitted']);\n''',
    '''    rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['remote:existing', 'submitted'], ['submitted']);\n''',
)
replace_once(
    "frontend/src/lib/state/uploadItems.test.ts",
    '''    rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['remote:existing', 'submitted', 'local:remove']);\n''',
    '''    rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['remote:existing', 'submitted', 'local:remove'], ['submitted', 'local:remove']);\n''',
)
replace_once(
    "frontend/src/lib/state/uploadItems.tagSync.test.ts",
    '''  markUploadItemTagSyncErrorInPlace,\n  setUploadItemTagsInPlace,\n''',
    '''  markUploadItemTagSyncErrorInPlace,\n  rebaseUploadItemTagsFromRemoteInPlace,\n  setUploadItemTagsInPlace,\n''',
)
replace_once(
    "frontend/src/lib/state/uploadItems.tagSync.test.ts",
    '''  it('records a failed direct tag update without retry-looping automatically', () => {\n''',
    '''  it('ignores a remote snapshot when the confirmed baseline advanced after the fetch started', () => {\n    const items = [item('imported', 'file-one')];\n    items[0]!.tagSyncBaseTags = ['person:alice'];\n    const expectedBaseTags = [...items[0]!.tagSyncBaseTags];\n\n    setUploadItemTagsInPlace(items, 0, ['person:alice', 'person:bob']);\n    markUploadItemTagSyncAppliedInPlace(items, 0, 'add', ['person:bob']);\n\n    expect(rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['person:alice'], expectedBaseTags)).toBe(false);\n    expect(items[0]).toMatchObject({\n      tags: ['person:alice', 'person:bob'],\n      tagSyncBaseTags: ['person:alice', 'person:bob'],\n      tagSyncPending: false\n    });\n  });\n\n  it('rebases a current remote snapshot while preserving edits that are still pending', () => {\n    const items = [item('imported', 'file-one')];\n    items[0]!.tagSyncBaseTags = ['person:alice'];\n    const expectedBaseTags = [...items[0]!.tagSyncBaseTags];\n\n    setUploadItemTagsInPlace(items, 0, ['person:bob']);\n\n    expect(rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['person:alice'], expectedBaseTags)).toBe(true);\n    expect(items[0]).toMatchObject({\n      tags: ['person:bob'],\n      tagSyncBaseTags: ['person:alice'],\n      tagSyncPending: true\n    });\n  });\n\n  it('records a failed direct tag update without retry-looping automatically', () => {\n''',
)
