from pathlib import Path


def rep(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if new in text:
        return
    if old not in text:
        raise SystemExit(f"missing anchor in {path}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1))

# Upload item state owns queue-entry metadata so target changes/removals never recompute it.
rep(
    'frontend/src/lib/state/uploadItems.ts',
    '''export interface UploadItem {\n  name: string;\n  size: number;\n  type: string;\n  targetID?: string;\n  status: UploadItemStatus;\n  progress: number;\n  error?: string;\n}\n\nexport type UploadTargetOption = { id: string; name: string };''',
    '''export type UploadAddedAtStrategy = 'queue' | 'reverse_queue' | 'modtime';\n\nexport interface UploadItem {\n  name: string;\n  size: number;\n  type: string;\n  targetID?: string;\n  queueTimeMs?: number;\n  status: UploadItemStatus;\n  progress: number;\n  error?: string;\n}\n\nexport type UploadTargetOption = { id: string; name: string; added_at_strategy?: UploadAddedAtStrategy };'''
)
rep(
    'frontend/src/lib/state/uploadItems.ts',
    '''export function stagedUploadItems(files: File[], targetID = ''): UploadItem[] {\n  return files.map((file) => ({\n    name: file.name,\n    size: file.size,\n    type: file.type,\n    targetID,\n    status: 'staged',\n    progress: 0\n  }));\n}''',
    '''export function stagedUploadItems(files: File[], targetID = '', queueTimeMs = Date.now()): UploadItem[] {\n  return files.map((file) => ({\n    name: file.name,\n    size: file.size,\n    type: file.type,\n    targetID,\n    queueTimeMs,\n    status: 'staged',\n    progress: 0\n  }));\n}\n\nexport function retargetStagedUploadItems(items: UploadItem[], targetID: string): UploadItem[] {\n  return items.map((item) => item.status === 'staged' ? { ...item, targetID } : item);\n}'''
)
rep(
    'frontend/src/lib/state/uploadItems.ts',
    '''      targetID: file.target_id,\n      status: file.status,''',
    '''      targetID: file.target_id,\n      queueTimeMs: prior?.queueTimeMs,\n      status: file.status,'''
)

# Workflow captures one queue timestamp at selection time and exact relative order before workers start.
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''  queuedItem,\n  stagedUploadItems,\n  uploadingItem,''',
    '''  queuedItem,\n  retargetStagedUploadItems,\n  stagedUploadItems,\n  uploadingItem,'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''  waitingUploadItems,\n  type UploadItem\n} from './uploadItems';''',
    '''  waitingUploadItems,\n  type UploadAddedAtStrategy,\n  type UploadItem\n} from './uploadItems';'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''  let conflictPolicy = $state('rename');\n  let autoUpload = $state(false);''',
    '''  let conflictPolicy = $state('rename');\n  let addedAtStrategy = $state<UploadAddedAtStrategy>('queue');\n  let autoUpload = $state(false);'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''    conflictPolicy = 'rename';\n    autoUpload = false;''',
    '''    conflictPolicy = 'rename';\n    addedAtStrategy = 'queue';\n    autoUpload = false;'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''  function removeAt(index: number) {\n    files = files.filter((_, fileIndex) => fileIndex !== index);\n    items = stagedUploadItems(files, targetID);\n    status = '';\n  }''',
    '''  function removeAt(index: number) {\n    files = files.filter((_, fileIndex) => fileIndex !== index);\n    items = items.filter((_, itemIndex) => itemIndex !== index);\n    status = '';\n  }'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''    files = [...files, ...additions];\n    items = stagedUploadItems(files, targetID);''',
    '''    const queueTimeMs = Date.now();\n    files = [...files, ...additions];\n    items = [...items, ...stagedUploadItems(additions, targetID, queueTimeMs)];'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''  function setTarget(value: string) {\n    targetID = value;\n    if (files.length) items = stagedUploadItems(files, targetID);\n  }''',
    '''  function setTarget(value: string, defaultStrategy?: UploadAddedAtStrategy) {\n    targetID = value;\n    if (defaultStrategy) addedAtStrategy = defaultStrategy;\n    if (files.length) items = retargetStagedUploadItems(items, targetID);\n  }'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''    const batchConflictPolicy = conflictPolicy;\n    let queued = 0;''',
    '''    const batchConflictPolicy = conflictPolicy;\n    const batchAddedAtStrategy = addedAtStrategy;\n    const fallbackQueueTimeMs = Date.now();\n    const batchQueueTimes = items.map((item) => item.queueTimeMs ?? fallbackQueueTimeMs);\n    const batchQueueTotal = batchFiles.length;\n    let queued = 0;'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''            targetID: batchTargetID,\n            conflictPolicy: batchConflictPolicy,\n            onProgress: (progress) => {''',
    '''            targetID: batchTargetID,\n            conflictPolicy: batchConflictPolicy,\n            addedAtStrategy: batchAddedAtStrategy,\n            queueTimeMs: batchQueueTimes[index],\n            queueIndex: index,\n            queueTotal: batchQueueTotal,\n            onProgress: (progress) => {'''
)
rep(
    'frontend/src/lib/state/uploadWorkflow.svelte.ts',
    '''    get conflictPolicy() { return conflictPolicy; },\n    set conflictPolicy(value: string) { conflictPolicy = value; },\n    get autoUpload() { return autoUpload; },''',
    '''    get conflictPolicy() { return conflictPolicy; },\n    set conflictPolicy(value: string) { conflictPolicy = value; },\n    get addedAtStrategy() { return addedAtStrategy; },\n    set addedAtStrategy(value: UploadAddedAtStrategy) { addedAtStrategy = value; },\n    get autoUpload() { return autoUpload; },'''
)

# Query/mutation transport keeps ordering metadata explicit and scalar per one-file worker request.
rep(
    'frontend/src/lib/queries/library.ts',
    '''import type { SavedSearch, SavedSearchRequest, UploadImportResponse, Job } from '$lib/api/types';''',
    '''import type { SavedSearch, SavedSearchRequest, UploadImportResponse, Job } from '$lib/api/types';\nimport type { UploadAddedAtStrategy } from '$lib/state/uploadItems';'''
)
rep(
    'frontend/src/lib/queries/library.ts',
    '''  targetID: string;\n  conflictPolicy: string;\n  onProgress?: (progress: number) => void;''',
    '''  targetID: string;\n  conflictPolicy: string;\n  addedAtStrategy: UploadAddedAtStrategy;\n  queueTimeMs: number;\n  queueIndex: number;\n  queueTotal: number;\n  onProgress?: (progress: number) => void;'''
)
rep(
    'frontend/src/lib/queries/library.ts',
    '''    mutationFn: ({ files, tags, preferAsync, targetID, conflictPolicy, onProgress }) =>\n      new ApiClient(getCSRFToken()).uploadFiles(files, tags, preferAsync, targetID, conflictPolicy, onProgress),''',
    '''    mutationFn: ({ files, tags, preferAsync, targetID, conflictPolicy, addedAtStrategy, queueTimeMs, queueIndex, queueTotal, onProgress }) =>\n      new ApiClient(getCSRFToken()).uploadFiles(files, tags, preferAsync, targetID, conflictPolicy, onProgress, {\n        addedAtStrategy,\n        queueTimeMs: [queueTimeMs],\n        queueIndex: [queueIndex],\n        queueTotal: [queueTotal]\n      }),'''
)

# API client accepts typed ordering metadata without breaking existing progress-call sites.
rep(
    'frontend/src/lib/api/client.ts',
    '''  async uploadFiles(\n    files: File[],''',
    '''  async uploadFiles(\n    files: File[],'''
)
rep(
    'frontend/src/lib/api/client.ts',
    '''    conflictPolicy = 'rename',\n    onProgress?: (progress: number) => void\n  ): Promise<Job | UploadImportResponse> {''',
    '''    conflictPolicy = 'rename',\n    onProgress?: (progress: number) => void,\n    ordering: UploadOrderingMetadata = {}\n  ): Promise<Job | UploadImportResponse> {'''
)
rep(
    'frontend/src/lib/api/client.ts',
    '''    if (conflictPolicy) form.append('conflict_policy', conflictPolicy);\n\n    return uploadMultipart''',
    '''    if (conflictPolicy) form.append('conflict_policy', conflictPolicy);\n    if (ordering.addedAtStrategy) form.append('added_at_strategy', ordering.addedAtStrategy);\n    for (const value of ordering.queueTimeMs ?? []) if (Number.isFinite(value) && value > 0) form.append('queue_time_ms', String(Math.trunc(value)));\n    for (const value of ordering.queueIndex ?? []) if (Number.isInteger(value) && value >= 0) form.append('queue_index', String(value));\n    for (const value of ordering.queueTotal ?? []) if (Number.isInteger(value) && value > 0) form.append('queue_total', String(value));\n\n    return uploadMultipart'''
)
rep(
    'frontend/src/lib/api/client.ts',
    '''interface UploadMultipartOptions {''',
    '''export interface UploadOrderingMetadata {\n  addedAtStrategy?: 'queue' | 'reverse_queue' | 'modtime';\n  queueTimeMs?: number[];\n  queueIndex?: number[];\n  queueTotal?: number[];\n}\n\ninterface UploadMultipartOptions {'''
)

# UI displays the server-owned default and lets users override it for this submission.
rep(
    'frontend/src/lib/components/UploadPanel.svelte',
    '''    conflictPolicy,\n    autoUpload,''',
    '''    conflictPolicy,\n    addedAtStrategy,\n    autoUpload,'''
)
rep(
    'frontend/src/lib/components/UploadPanel.svelte',
    '''    onConflictInput,\n    onAutoUploadInput,''',
    '''    onConflictInput,\n    onAddedAtStrategyInput,\n    onAutoUploadInput,'''
)
rep(
    'frontend/src/lib/components/UploadPanel.svelte',
    '''    conflictPolicy: string;\n    autoUpload: boolean;''',
    '''    conflictPolicy: string;\n    addedAtStrategy: 'queue' | 'reverse_queue' | 'modtime';\n    autoUpload: boolean;'''
)
rep(
    'frontend/src/lib/components/UploadPanel.svelte',
    '''    onConflictInput: (value: string) => void;\n    onAutoUploadInput: (value: boolean) => void;''',
    '''    onConflictInput: (value: string) => void;\n    onAddedAtStrategyInput: (value: 'queue' | 'reverse_queue' | 'modtime') => void;\n    onAutoUploadInput: (value: boolean) => void;'''
)
rep(
    'frontend/src/lib/components/UploadPanel.svelte',
    '''        <div class="field-row">\n          <span>On conflict</span>''',
    '''        <div class="field-row">\n          <span>Added time</span>\n          <div class="field-control">\n            <div class="seg" aria-label="Library added time strategy">\n              {#each [{ value: 'queue', label: 'Queue' }, { value: 'reverse_queue', label: 'Reverse queue' }, { value: 'modtime', label: 'File modified' }] as option}\n                <button type="button" class={addedAtStrategy === option.value ? 'is-active' : ''} onclick={() => onAddedAtStrategyInput(option.value as 'queue' | 'reverse_queue' | 'modtime')}>{option.label}</button>\n              {/each}\n            </div>\n          </div>\n        </div>\n\n        <div class="field-row">\n          <span>On conflict</span>'''
)

# App binds upload-target API defaults to workflow selection without hardcoding config in the frontend.
rep(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    '''  const selectedCount = $derived(library.selectedCount(currentTotalCount));\n\n  $effect(() => {''',
    '''  const selectedCount = $derived(library.selectedCount(currentTotalCount));\n\n  $effect(() => {\n    const targets = uploadTargetsQuery.data?.items ?? [];\n    if (!targets.length || upload.targetID) return;\n    const target = targets[0];\n    upload.setTarget(target.id, target.added_at_strategy ?? 'queue');\n  });\n\n  $effect(() => {'''
)
rep(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    '''  function selectUploadFiles(files: FileList | File[] | null) {''',
    '''  function selectUploadTarget(value: string) {\n    const target = (uploadTargetsQuery.data?.items ?? []).find((candidate) => candidate.id === value);\n    upload.setTarget(value, target?.added_at_strategy ?? 'queue');\n  }\n\n  function selectUploadFiles(files: FileList | File[] | null) {'''
)
rep(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    '''        conflictPolicy={upload.conflictPolicy}\n        autoUpload={upload.autoUpload}''',
    '''        conflictPolicy={upload.conflictPolicy}\n        addedAtStrategy={upload.addedAtStrategy}\n        autoUpload={upload.autoUpload}'''
)
rep(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    '''        onTargetInput={upload.setTarget}\n        onFiles={selectUploadFiles}''',
    '''        onTargetInput={selectUploadTarget}\n        onFiles={selectUploadFiles}'''
)
rep(
    'frontend/src/lib/components/AuthenticatedApp.svelte',
    '''        onConflictInput={(value) => (upload.conflictPolicy = value)}\n        onAutoUploadInput={(value) => (upload.autoUpload = value)}''',
    '''        onConflictInput={(value) => (upload.conflictPolicy = value)}\n        onAddedAtStrategyInput={(value) => (upload.addedAtStrategy = value)}\n        onAutoUploadInput={(value) => (upload.autoUpload = value)}'''
)

# Workflow regression: queue metadata is captured before parallel workers and survives target changes.
p = Path('frontend/src/lib/state/uploadWorkflow.svelte.test.ts')
text = p.read_text()
needle = '''  it('uploads each file independently with real per-file progress', async () => {'''
insert = r'''  it('captures queue metadata before parallel workers and preserves it across target changes', async () => {
    const now = vi.spyOn(Date, 'now');
    now.mockReturnValueOnce(1_700_000_000_000).mockReturnValue(1_800_000_000_000);
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    workflow.setTarget('archive', 'reverse_queue');

    const calls: Array<{ name: string; queueTimeMs: number; queueIndex: number; queueTotal: number; strategy: string }> = [];
    await workflow.submit(async (variables) => {
      calls.push({
        name: variables.files[0].name,
        queueTimeMs: variables.queueTimeMs,
        queueIndex: variables.queueIndex,
        queueTotal: variables.queueTotal,
        strategy: variables.addedAtStrategy
      });
      return pendingJob(`job-${variables.files[0].name}`);
    });

    expect(calls).toEqual([
      { name: 'first.jpg', queueTimeMs: 1_700_000_000_000, queueIndex: 0, queueTotal: 2, strategy: 'reverse_queue' },
      { name: 'second.jpg', queueTimeMs: 1_700_000_000_000, queueIndex: 1, queueTotal: 2, strategy: 'reverse_queue' }
    ]);
    expect(workflow.items.map((item) => item.queueTimeMs)).toEqual([1_700_000_000_000, 1_700_000_000_000]);
    now.mockRestore();
  });

'''
if insert not in text:
    if needle not in text:
        raise SystemExit('workflow test anchor missing')
    text = text.replace(needle, insert + needle, 1)
p.write_text(text)

# Client regression proves the actual multipart boundary carries the submitted values.
rep(
    'frontend/src/lib/api/client.test.ts',
    '''    const result = await client.uploadFiles([file], ['reviewed'], true, '', 'rename', progress);''',
    '''    const result = await client.uploadFiles([file], ['reviewed'], true, '', 'rename', progress, {\n      addedAtStrategy: 'reverse_queue',\n      queueTimeMs: [1_700_000_000_000],\n      queueIndex: [2],\n      queueTotal: [4]\n    });'''
)
rep(
    'frontend/src/lib/api/client.test.ts',
    '''    expect((xhr.body as FormData).get('source_modtime_ms')).toBe(String(sourceModTime));\n    expect(progress.mock.calls.map(([value]) => value)).toEqual([20, 70, 100, 100]);''',
    '''    expect((xhr.body as FormData).get('source_modtime_ms')).toBe(String(sourceModTime));\n    expect((xhr.body as FormData).get('added_at_strategy')).toBe('reverse_queue');\n    expect((xhr.body as FormData).get('queue_time_ms')).toBe('1700000000000');\n    expect((xhr.body as FormData).get('queue_index')).toBe('2');\n    expect((xhr.body as FormData).get('queue_total')).toBe('4');\n    expect(progress.mock.calls.map(([value]) => value)).toEqual([20, 70, 100, 100]);'''
)
