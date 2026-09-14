from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


items = 'frontend/src/lib/state/uploadItems.ts'
replace_once(items,
'''export function markUploadItemTagSyncErrorInPlace(items: UploadItem[], index: number, message: string): void {
''',
'''export function rebaseUploadItemTagsFromRemoteInPlace(items: UploadItem[], index: number, remoteTags: string[]): void {
  const current = items[index];
  if (!current) return;
  const delta = uploadItemTagSyncDelta(current);
  const base = normalizeUploadItemTags(remoteTags);
  const desired = [...base];
  const desiredSet = new Set(desired);
  for (const tag of delta.add) {
    if (!desiredSet.has(tag)) {
      desired.push(tag);
      desiredSet.add(tag);
    }
  }
  if (delta.remove.length) {
    const removed = new Set(delta.remove);
    current.tags = desired.filter((tag) => !removed.has(tag));
  } else {
    current.tags = desired;
  }
  current.tagSyncBaseTags = base;
  const pending = hasUploadItemTagSyncDelta(current);
  if (!pending) current.tagSyncError = '';
  current.tagSyncPending = pending && !current.tagSyncError;
}

export function markUploadItemTagSyncErrorInPlace(items: UploadItem[], index: number, message: string): void {
''')

workflow = 'frontend/src/lib/state/uploadWorkflow.svelte.ts'
replace_once(workflow,
'''  queuedItem,
  replaceUploadItemInPlace,
  retargetStagedUploadItems,
  setUploadItemTagsInPlace,
''',
'''  queuedItem,
  rebaseUploadItemTagsFromRemoteInPlace,
  replaceUploadItemInPlace,
  retargetStagedUploadItems,
  setUploadItemTagsInPlace,
''')
replace_once(workflow,
'''  function markItemTagSyncApplied(index: number, operation: UploadItemTagSyncOperation, tags: string[]) {
    markUploadItemTagSyncAppliedInPlace(items, index, operation, tags);
  }

  function markItemTagSyncError(index: number, message: string) {
''',
'''  function markItemTagSyncApplied(index: number, operation: UploadItemTagSyncOperation, tags: string[]) {
    markUploadItemTagSyncAppliedInPlace(items, index, operation, tags);
  }

  function rebaseItemTagsFromRemote(index: number, remoteTags: string[]) {
    rebaseUploadItemTagsFromRemoteInPlace(items, index, remoteTags);
  }

  function markItemTagSyncError(index: number, message: string) {
''')
replace_once(workflow,
'''    itemTagSyncDelta,
    markItemTagSyncApplied,
    markItemTagSyncError,
''',
'''    itemTagSyncDelta,
    markItemTagSyncApplied,
    rebaseItemTagsFromRemote,
    markItemTagSyncError,
''')

app = 'frontend/src/lib/components/AuthenticatedApp.svelte'
replace_once(app,
'''  function setUploadItemTags(index: number, nextTags: string[]) {
    upload.setItemTags(index, nextTags);
    void reconcileUploadItemTags(index);
  }

  async function submitUpload() {
''',
'''  function setUploadItemTags(index: number, nextTags: string[]) {
    upload.setItemTags(index, nextTags);
    void reconcileUploadItemTags(index);
  }

  function rebaseUploadItemTagsFromRemote(index: number, remoteTags: string[]) {
    upload.rebaseItemTagsFromRemote(index, remoteTags);
    void reconcileUploadItemTags(index);
  }

  async function submitUpload() {
''')
replace_once(app,
'''        onTagsInput={(value) => (upload.tags = value)}
        onItemTagsInput={setUploadItemTags}
        onAddedAtStrategyInput={(value) => (upload.addedAtStrategy = value)}
''',
'''        onTagsInput={(value) => (upload.tags = value)}
        onItemTagsInput={setUploadItemTags}
        onItemRemoteTagsLoaded={rebaseUploadItemTagsFromRemote}
        onAddedAtStrategyInput={(value) => (upload.addedAtStrategy = value)}
''')

panel = 'frontend/src/lib/components/UploadPanel.svelte'
replace_once(panel,
'''    onTagsInput,
    onItemTagsInput,
    onAddedAtStrategyInput,
''',
'''    onTagsInput,
    onItemTagsInput,
    onItemRemoteTagsLoaded,
    onAddedAtStrategyInput,
''')
replace_once(panel,
'''    onTagsInput: (value: string) => void;
    onItemTagsInput: (index: number, tags: string[]) => void;
    onAddedAtStrategyInput: (value: 'queue' | 'reverse_queue' | 'modtime') => void;
''',
'''    onTagsInput: (value: string) => void;
    onItemTagsInput: (index: number, tags: string[]) => void;
    onItemRemoteTagsLoaded: (index: number, tags: string[]) => void;
    onAddedAtStrategyInput: (value: 'queue' | 'reverse_queue' | 'modtime') => void;
''')
replace_once(panel,
'''    onClose={closeViewer}
    {onItemTagsInput}
  />
''',
'''    onClose={closeViewer}
    {onItemTagsInput}
    {onItemRemoteTagsLoaded}
  />
''')

viewer = 'frontend/src/lib/components/UploadViewerDialog.svelte'
replace_once(viewer,
'''    onIndex,
    onClose,
    onItemTagsInput
''',
'''    onIndex,
    onClose,
    onItemTagsInput,
    onItemRemoteTagsLoaded
''')
replace_once(viewer,
'''    onClose: () => void;
    onItemTagsInput: (index: number, tags: string[]) => void;
  }>();
''',
'''    onClose: () => void;
    onItemTagsInput: (index: number, tags: string[]) => void;
    onItemRemoteTagsLoaded: (index: number, tags: string[]) => void;
  }>();
''')
replace_once(viewer,
'''  const currentTags = $derived(tagOverride ?? remoteFile?.tags ?? activeItem?.tags ?? []);
''',
'''  const currentTags = $derived(tagOverride ?? activeItem?.tags ?? remoteFile?.tags ?? []);
''')
replace_once(viewer,
'''      .then((file) => {
        if (!controller.signal.aborted) remoteFile = file;
      })
''',
'''      .then((file) => {
        if (controller.signal.aborted) return;
        remoteFile = file;
        onItemRemoteTagsLoaded(activeIndex, file.tags ?? []);
        tagOverride = undefined;
      })
''')

item_test = 'frontend/src/lib/state/uploadItems.test.ts'
replace_once(item_test,
'''  markUploadItemTagSyncErrorInPlace,
  replaceUploadItemInPlace,
''',
'''  markUploadItemTagSyncErrorInPlace,
  rebaseUploadItemTagsFromRemoteInPlace,
  replaceUploadItemInPlace,
''')
replace_once(item_test,
'''  it('retains a successful partial baseline when a later operation fails', () => {
''',
'''  it('rebases duplicate-existing rows onto full remote tags so pre-existing tags can be removed', () => {
    const items = [queueItem(0)];
    items[0].status = 'duplicate_existing';
    items[0].tags = ['submitted'];
    items[0].tagSyncBaseTags = ['submitted'];

    rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['remote:existing', 'submitted']);
    expect(items[0].tags).toEqual(['remote:existing', 'submitted']);
    expect(items[0].tagSyncBaseTags).toEqual(['remote:existing', 'submitted']);
    expect(items[0].tagSyncPending).toBe(false);

    setUploadItemTagsInPlace(items, 0, ['submitted']);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: [], remove: ['remote:existing'] });
    expect(items[0].tagSyncPending).toBe(true);
  });

  it('preserves outstanding local edits while learning the authoritative remote tag baseline', () => {
    const items = [queueItem(0)];
    items[0].status = 'imported';
    items[0].tags = ['submitted', 'local:add'];
    items[0].tagSyncBaseTags = ['submitted', 'local:remove'];
    items[0].tagSyncPending = true;

    rebaseUploadItemTagsFromRemoteInPlace(items, 0, ['remote:existing', 'submitted', 'local:remove']);
    expect(items[0].tagSyncBaseTags).toEqual(['remote:existing', 'submitted', 'local:remove']);
    expect(items[0].tags).toEqual(['remote:existing', 'submitted', 'local:add']);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: ['local:add'], remove: ['local:remove'] });
    expect(items[0].tagSyncPending).toBe(true);
  });

  it('retains a successful partial baseline when a later operation fails', () => {
''')
