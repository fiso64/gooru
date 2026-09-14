from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}")
    p.write_text(text.replace(old, new, 1))


workflow = 'frontend/src/lib/state/uploadWorkflow.svelte.ts'
replace_once(workflow,
'''  markUploadItemTagSyncErrorInPlace,
  markUploadItemTagsSyncedInPlace,
  queuedItem,
''',
'''  markUploadItemTagSyncAppliedInPlace,
  markUploadItemTagSyncErrorInPlace,
  queuedItem,
''')
replace_once(workflow,
'''  stagedUploadItems,
  uploadProgressItem,
  uploadSummaryFromCounts,
  type UploadAddedAtStrategy,
  type UploadItem,
''',
'''  stagedUploadItems,
  uploadItemTagSyncDelta,
  uploadProgressItem,
  uploadSummaryFromCounts,
  type UploadAddedAtStrategy,
  type UploadItem,
  type UploadItemTagSyncOperation,
''')
replace_once(workflow,
'''  function markItemTagsSynced(index: number, syncedTags: string[]) {
    markUploadItemTagsSyncedInPlace(items, index, syncedTags);
  }

  function markItemTagSyncError(index: number, message: string) {
''',
'''  function itemTagSyncDelta(index: number) {
    const current = items[index];
    return current ? uploadItemTagSyncDelta(current) : { add: [], remove: [] };
  }

  function markItemTagSyncApplied(index: number, operation: UploadItemTagSyncOperation, tags: string[]) {
    markUploadItemTagSyncAppliedInPlace(items, index, operation, tags);
  }

  function markItemTagSyncError(index: number, message: string) {
''')
replace_once(workflow,
'''    const batchID = ++nextBatchID;
    const nextItems = [...items];
    for (const itemIndex of batchItemIndices) {
      const current = nextItems[itemIndex];
      if (current) nextItems[itemIndex] = { ...current, batchID, status: 'uploading', progress: 0, error: '' };
    }
    items = nextItems;
    files = [];
    const controller = new AbortController();
    activeSubmissionControllers.add(controller);
    activeSubmissions += 1;
    statusCounts = countUploadStatuses(items);

    const parsedTags = parseTags(tags);
''',
'''    const parsedTags = parseTags(tags);
    const batchID = ++nextBatchID;
    const nextItems = [...items];
    for (const itemIndex of batchItemIndices) {
      const current = nextItems[itemIndex];
      if (current) {
        const submittedTags = [...(current.tags ?? parsedTags)];
        nextItems[itemIndex] = {
          ...current,
          batchID,
          tagSyncBaseTags: submittedTags,
          tagSyncPending: false,
          tagSyncError: '',
          status: 'uploading',
          progress: 0,
          error: ''
        };
      }
    }
    items = nextItems;
    files = [];
    const controller = new AbortController();
    activeSubmissionControllers.add(controller);
    activeSubmissions += 1;
    statusCounts = countUploadStatuses(items);

''')
replace_once(workflow,
'''    setItemTags,
    markItemTagsSynced,
    markItemTagSyncError,
''',
'''    setItemTags,
    itemTagSyncDelta,
    markItemTagSyncApplied,
    markItemTagSyncError,
''')

app = 'frontend/src/lib/components/AuthenticatedApp.svelte'
replace_once(app,
'''  async function reconcileUploadItemTags(index: number) {
    const item = upload.items[index];
    if (!item?.tagSyncPending || !item.remoteFileID || uploadTagSyncRuns.has(item)) return;
    const syncedTags = [...(item.tags ?? [])];
    const remoteFileID = item.remoteFileID;
    uploadTagSyncRuns.add(item);
    try {
      await tagMutation.mutateAsync({ operation: 'set', body: { file_ids: [remoteFileID], tags: syncedTags } });
      const currentIndex = upload.items.indexOf(item);
      if (currentIndex >= 0) upload.markItemTagsSynced(currentIndex, syncedTags);
    } catch (error) {
      const currentIndex = upload.items.indexOf(item);
      if (currentIndex >= 0) upload.markItemTagSyncError(currentIndex, errorMessage(error));
    } finally {
      uploadTagSyncRuns.delete(item);
      const currentIndex = upload.items.indexOf(item);
      if (currentIndex >= 0 && item.tagSyncPending && item.remoteFileID && !item.tagSyncError) {
        void reconcileUploadItemTags(currentIndex);
      }
    }
  }
''',
'''  async function reconcileUploadItemTags(index: number) {
    const item = upload.items[index];
    if (!item?.tagSyncPending || !item.remoteFileID || uploadTagSyncRuns.has(item)) return;
    const delta = upload.itemTagSyncDelta(index);
    const operation = delta.add.length ? 'add' : delta.remove.length ? 'remove' : null;
    if (!operation) {
      upload.markItemTagSyncApplied(index, 'add', []);
      return;
    }
    const changedTags = operation === 'add' ? delta.add : delta.remove;
    const remoteFileID = item.remoteFileID;
    uploadTagSyncRuns.add(item);
    try {
      await tagMutation.mutateAsync({ operation, body: { file_ids: [remoteFileID], tags: changedTags } });
      const currentIndex = upload.items.indexOf(item);
      if (currentIndex >= 0) upload.markItemTagSyncApplied(currentIndex, operation, changedTags);
    } catch (error) {
      const currentIndex = upload.items.indexOf(item);
      if (currentIndex >= 0) upload.markItemTagSyncError(currentIndex, errorMessage(error));
    } finally {
      uploadTagSyncRuns.delete(item);
      const currentIndex = upload.items.indexOf(item);
      if (currentIndex >= 0 && item.tagSyncPending && item.remoteFileID && !item.tagSyncError) {
        void reconcileUploadItemTags(currentIndex);
      }
    }
  }
''')

item_test = 'frontend/src/lib/state/uploadItems.test.ts'
replace_once(item_test,
'''  itemsFromResult,
  replaceUploadItemInPlace,
  retargetStagedUploadItems,
  setUploadItemTagsInPlace,
''',
'''  itemsFromResult,
  markUploadItemTagSyncAppliedInPlace,
  markUploadItemTagSyncErrorInPlace,
  replaceUploadItemInPlace,
  retargetStagedUploadItems,
  setUploadItemTagsInPlace,
''')
replace_once(item_test,
'''  stagedUploadItems,
  uploadProgressItem,
  uploadSummaryFromCounts,
''',
'''  stagedUploadItems,
  uploadItemTagSyncDelta,
  uploadProgressItem,
  uploadSummaryFromCounts,
''')
replace_once(item_test,
'''  it('preserves a row tag snapshot when an import result replaces transport state', () => {
    const items = stagedUploadItems([uploadFile('first.jpg')], 'primary', 123, ['person:alice']);
    const result = itemsFromResult({
      files: [{
        name: 'first.jpg',
        size: 10,
        target_id: 'primary',
        status: 'imported'
      }]
    } as never, items);

    expect(result[0].tags).toEqual(['person:alice']);
  });
''',
'''  it('preserves a row tag snapshot when an import result replaces transport state', () => {
    const items = stagedUploadItems([uploadFile('first.jpg')], 'primary', 123, ['person:alice']);
    items[0].tagSyncBaseTags = ['person:alice'];
    const result = itemsFromResult({
      files: [{
        name: 'first.jpg',
        size: 10,
        target_id: 'primary',
        status: 'imported'
      }]
    } as never, items);

    expect(result[0].tags).toEqual(['person:alice']);
    expect(result[0].tagSyncBaseTags).toEqual(['person:alice']);
  });

  it('computes only the add/remove delta from the submitted baseline', () => {
    const item = queueItem(0);
    item.status = 'imported';
    item.tags = ['submitted', 'new'];
    item.tagSyncBaseTags = ['submitted', 'removed'];

    expect(uploadItemTagSyncDelta(item)).toEqual({ add: ['new'], remove: ['removed'] });
  });

  it('advances successful operations independently while preserving concurrent edits', () => {
    const items = [queueItem(0)];
    items[0].status = 'imported';
    items[0].tags = ['keep', 'add'];
    items[0].tagSyncBaseTags = ['keep', 'remove'];
    items[0].tagSyncPending = true;

    markUploadItemTagSyncAppliedInPlace(items, 0, 'add', ['add']);
    expect(items[0].tagSyncBaseTags).toEqual(['keep', 'remove', 'add']);
    expect(items[0].tagSyncPending).toBe(true);

    setUploadItemTagsInPlace(items, 0, ['keep', 'add', 'late']);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: ['late'], remove: ['remove'] });

    markUploadItemTagSyncAppliedInPlace(items, 0, 'remove', ['remove']);
    expect(items[0].tagSyncBaseTags).toEqual(['keep', 'add']);
    expect(items[0].tagSyncPending).toBe(true);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: ['late'], remove: [] });
  });

  it('retains a successful partial baseline when a later operation fails', () => {
    const items = [queueItem(0)];
    items[0].status = 'imported';
    items[0].tags = ['keep', 'add'];
    items[0].tagSyncBaseTags = ['keep', 'remove'];
    items[0].tagSyncPending = true;

    markUploadItemTagSyncAppliedInPlace(items, 0, 'add', ['add']);
    markUploadItemTagSyncErrorInPlace(items, 0, 'remove failed');
    expect(items[0].tagSyncBaseTags).toEqual(['keep', 'remove', 'add']);
    expect(items[0].tagSyncPending).toBe(false);
    expect(items[0].tagSyncError).toBe('remove failed');

    setUploadItemTagsInPlace(items, 0, ['keep', 'add']);
    expect(items[0].tagSyncError).toBe('');
    expect(items[0].tagSyncPending).toBe(true);
    expect(uploadItemTagSyncDelta(items[0])).toEqual({ add: [], remove: ['remove'] });
  });
''')

chunk_test = 'frontend/src/lib/state/uploadWorkflow.chunking.test.ts'
replace_once(chunk_test,
'''  it('keeps one visible job whether a selection uses one request or several', async () => {
''',
'''  it('captures submitted tags at synchronous admission and preserves later edits as a delta', async () => {
    const workflow = createUploadWorkflow();
    workflow.tags = 'submitted common';
    workflow.select([uploadFile(0)]);

    let release!: () => void;
    const response = new Promise<BackgroundOperation>((resolve) => {
      release = () => resolve(pendingJob('job-baseline', 1));
    });
    const submission = workflow.submit(() => response);

    expect(workflow.items[0]).toMatchObject({
      status: 'uploading',
      tags: ['submitted', 'common'],
      tagSyncBaseTags: ['submitted', 'common'],
      tagSyncPending: false
    });

    workflow.setItemTags(0, ['submitted', 'later']);
    expect(workflow.itemTagSyncDelta(0)).toEqual({ add: ['later'], remove: ['common'] });
    expect(workflow.items[0].tagSyncPending).toBe(true);

    release();
    await submission;
    expect(workflow.items[0].tagSyncBaseTags).toEqual(['submitted', 'common']);
    expect(workflow.itemTagSyncDelta(0)).toEqual({ add: ['later'], remove: ['common'] });
  });

  it('keeps one visible job whether a selection uses one request or several', async () => {
''')
