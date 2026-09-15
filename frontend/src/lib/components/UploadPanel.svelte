<script lang="ts">
  import { untrack } from 'svelte';
  import Icon from './Icon.svelte';
  import PageNav from './PageNav.svelte';
  import TagAutocompleteInput from './TagAutocompleteInput.svelte';
  import UploadTargetPicker from './UploadTargetPicker.svelte';
  import UploadMediaPreview from './UploadMediaPreview.svelte';
  import UploadViewerDialog from './UploadViewerDialog.svelte';
  import type { FileItem } from '$lib/api/types';
  import { mergeTagCandidateOccurrenceCounts, type TagCandidate } from '$lib/utils/tagSuggestions';
  import { formatBytes, parseTags } from '$lib/utils/format';
  import { uploadShortcutAction } from '$lib/utils/keyboard';
  import { effectiveUploadTargetID, type UploadItem, type UploadTargetOption } from '$lib/state/uploadItems';
  import { filterUploadRows, groupUploadQueueRows, paginateUploadRows, partitionUploadRows, summarizeUploadQueueBatch, type IndexedUploadRow, type UploadQueueBatch } from '$lib/state/uploadPanelRows';
  import { uploadItemCanOpenViewer, uploadViewerScope, type UploadViewerScope } from '$lib/state/uploadViewer';

  let {
    uploadFiles,
    uploadItems,
    uploadTags,
    uploadBusy,
    cancelBusy,
    cancelRequested = false,
    uploadStatus,
    activeUploadJobID,
    targets,
    targetID,
    addedAtStrategy,
    autoUpload,
    tags,
    stagedTagCandidates,
    onTargetInput,
    onFiles,
    onTagsInput,
    onItemTagsInput,
    onItemRemoteTagsLoaded,
    onAddedAtStrategyInput,
    onAutoUploadInput,
    onSubmit,
    onCancel,
    onClear,
    onRemove,
    onViewerUntrack,
    onViewerDelete,
    onViewerTagSearch
  } = $props<{
    uploadFiles: File[];
    uploadItems: UploadItem[];
    uploadTags: string;
    uploadBusy: boolean;
    cancelBusy: boolean;
    cancelRequested?: boolean;
    uploadStatus: string;
    activeUploadJobID: string;
    targets: UploadTargetOption[];
    targetID: string;
    addedAtStrategy: 'queue' | 'reverse_queue' | 'modtime';
    autoUpload: boolean;
    tags: TagCandidate[];
    stagedTagCandidates: TagCandidate[];
    onTargetInput: (value: string) => void;
    onFiles: (files: FileList | File[] | null) => void;
    onTagsInput: (value: string) => void;
    onItemTagsInput: (index: number, tags: string[]) => void;
    onItemRemoteTagsLoaded: (index: number, tags: string[], expectedBaseTags: string[] | undefined) => void;
    onAddedAtStrategyInput: (value: 'queue' | 'reverse_queue' | 'modtime') => void;
    onAutoUploadInput: (value: boolean) => void;
    onSubmit: () => void;
    onCancel: (jobID: string) => void;
    onClear: (scope: 'staged' | 'done') => void;
    onRemove: (index: number) => void;
    onViewerUntrack: (file: FileItem) => void;
    onViewerDelete: (file: FileItem) => void;
    onViewerTagSearch: (tag: string) => void;
  }>();

  let dragActive = $state(false);
  let fileInput: HTMLInputElement | undefined;
  let tagDraft = $state('');
  let itemTagDrafts = $state<Record<number, string>>({});
  let stagedFilter = $state('');
  let stagedPage = $state(0);
  let queuePages = $state<Record<string, number>>({});
  let viewerIndex = $state<number | null>(null);
  let viewerScope = $state<UploadViewerScope>('staged');

  const uploadListPageSize = 100;
  const indexedItems = $derived(uploadItems.map((item: UploadItem, index: number) => ({ item, index })));
  const partitionedRows = $derived.by(() => {
    const rows = indexedItems;
    // Staged-vs-queue membership changes only when the workflow structurally
    // replaces/adds/removes rows. Per-row status/progress changes should not
    // make a 10K queue re-run its category filters.
    return untrack(() => partitionUploadRows(rows));
  });
  const stagedRows = $derived(partitionedRows.staged);
  const filteredStagedRows = $derived(filterUploadRows(stagedRows, stagedFilter));
  const queueRows = $derived(partitionedRows.queue);
  const queueBatches = $derived.by(() => {
    const rows = queueRows;
    return untrack(() => groupUploadQueueRows(rows));
  });
  const completionTags = $derived(mergeTagCandidateOccurrenceCounts(tags, stagedTagCandidates));
  const stagedBytes = $derived(uploadFiles.reduce((sum: number, file: File) => sum + file.size, 0));
  const queueBytes = $derived(queueRows.reduce((sum: number, row: IndexedUploadRow) => sum + row.item.size, 0));
  const initialTags = $derived(parseTags(uploadTags));
  const selectedTargetID = $derived(effectiveUploadTargetID(targetID, targets));
  const stagedPageState = $derived.by(() => paginateUploadRows(filteredStagedRows, stagedPage, uploadListPageSize));
  const stagedPageCount = $derived(stagedPageState.pageCount);
  const stagedVisibleRows = $derived(stagedPageState.rows);

  function queueBatchKey(batch: UploadQueueBatch) {
    return batch.batchID == null ? 'legacy' : String(batch.batchID);
  }

  function queueBatchPage(batch: UploadQueueBatch) {
    return paginateUploadRows(batch.rows, queuePages[queueBatchKey(batch)] ?? 0, uploadListPageSize);
  }

  function setQueueBatchPage(batch: UploadQueueBatch, page: number) {
    queuePages = { ...queuePages, [queueBatchKey(batch)]: Math.max(0, page) };
  }

  function batchLabel(batch: UploadQueueBatch) {
    return batch.batchID == null ? 'Earlier upload' : `Batch ${batch.batchID}`;
  }

  $effect(() => {
    if (stagedPage !== stagedPageState.page) stagedPage = stagedPageState.page;
    if (!queueBatches.length && Object.keys(queuePages).length) queuePages = {};
  });

  function chooseFiles() {
    fileInput?.click();
  }

  function hasFiles(event: DragEvent) {
    return Array.from(event.dataTransfer?.types ?? []).includes('Files');
  }

  function handleDragOver(event: DragEvent) {
    if (!hasFiles(event)) return;
    event.preventDefault();
    dragActive = true;
  }

  function handleDragLeave(event: DragEvent) {
    const nextTarget = event.relatedTarget as Node | null;
    if (nextTarget && (event.currentTarget as HTMLElement).contains(nextTarget)) return;
    dragActive = false;
  }

  function handleDrop(event: DragEvent) {
    if (!hasFiles(event)) return;
    event.preventDefault();
    dragActive = false;
    onFiles(event.dataTransfer?.files ?? null);
  }

  function picked(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    onFiles(input.files);
    input.value = '';
  }

  function commitInitialTag(tagInput: string) {
    onTagsInput(Array.from(new Set([...initialTags, ...parseTags(tagInput)])).join(' '));
    tagDraft = '';
  }

  function removeInitialTag(tag: string) {
    onTagsInput(initialTags.filter((candidate) => candidate !== tag).join(' '));
  }

  function setItemTagDraft(index: number, value: string) {
    itemTagDrafts = { ...itemTagDrafts, [index]: value };
  }

  function commitItemTag(index: number, item: UploadItem, tagInput: string) {
    const current = item.tags ?? [];
    onItemTagsInput(index, Array.from(new Set([...current, ...parseTags(tagInput)])));
    setItemTagDraft(index, '');
  }

  function removeItemTag(index: number, item: UploadItem, tag: string) {
    onItemTagsInput(index, (item.tags ?? []).filter((candidate) => candidate !== tag));
  }

  function openViewer(index: number) {
    const item = uploadItems[index];
    if (!item || !uploadItemCanOpenViewer(item)) return;
    viewerIndex = index;
    viewerScope = uploadViewerScope(item);
  }

  function closeViewer() {
    viewerIndex = null;
  }

  function resetStagedTransientState() {
    closeViewer();
    itemTagDrafts = {};
    stagedFilter = '';
    stagedPage = 0;
  }

  function submitStaged() {
    resetStagedTransientState();
    onSubmit();
  }

  function clearRows(scope: 'staged' | 'done') {
    closeViewer();
    itemTagDrafts = {};
    if (scope === 'staged') {
      stagedFilter = '';
      stagedPage = 0;
    }
    onClear(scope);
  }

  function removeStagedItem(index: number) {
    closeViewer();
    itemTagDrafts = {};
    onRemove(index);
  }

  function setStagedFilter(value: string) {
    stagedFilter = value;
    stagedPage = 0;
  }

  function statusClass(status: string) {
    if (status === 'imported' || status === 'uploaded') return 'ok';
    if (status === 'error') return 'err';
    return '';
  }

  function statusLabel(status: string) {
    return status.replace(/_/g, ' ');
  }

  function handleShortcut(event: KeyboardEvent) {
    if (event.defaultPrevented || stagedRows.length === 0) return;
    if (uploadShortcutAction(event.key, event.target, event.ctrlKey, event.metaKey, event.altKey, event.shiftKey) !== 'submit-upload') return;
    event.preventDefault();
    submitStaged();
  }
</script>

<svelte:window onkeydown={handleShortcut} />

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Upload</div>
      <h1>Import media into your library</h1>
      <p>Files are content-hashed on receipt. Duplicates are detected automatically.</p>
    </div>

    <form class="upload-stack" onsubmit={(event) => { event.preventDefault(); submitStaged(); }}>
      <section class="g-card upload-config-card">
        <div class="field-row">
          <span>Target</span>
          <div class="field-control">
            <UploadTargetPicker targets={targets} selectedID={selectedTargetID} onSelect={onTargetInput} />
          </div>
        </div>

        <div class="field-row">
          <span>Initial tags</span>
          <div class="field-control upload-tags-control">
            {#each initialTags as tag}
              {@const separator = tag.indexOf(':')}
              <span class="g-tag">
                {#if separator > 0}
                  <span class="g-tag-ns">{tag.slice(0, separator)}:</span><span>{tag.slice(separator + 1)}</span>
                {:else}
                  <span>{tag}</span>
                {/if}
                <button class="g-tag-x" type="button" aria-label={`Remove ${tag}`} onclick={() => removeInitialTag(tag)}>
                  <Icon name="close" size={11} />
                </button>
              </span>
            {/each}

            <TagAutocompleteInput
              value={tagDraft}
              tags={completionTags}
              existing={initialTags}
              placeholder="add tag — e.g. subject:portrait"
              ariaLabel="Initial tags"
              onInput={(value) => (tagDraft = value)}
              onCommit={commitInitialTag}
              onRemoveLast={removeInitialTag}
            />
          </div>
        </div>

        <div class="field-row">
          <span>Added time</span>
          <div class="field-control">
            <div class="seg" aria-label="Library added time strategy">
              {#each [{ value: 'queue', label: 'Queue' }, { value: 'reverse_queue', label: 'Reverse queue' }, { value: 'modtime', label: 'File modified' }] as option}
                <button type="button" class={addedAtStrategy === option.value ? 'is-active' : ''} onclick={() => onAddedAtStrategyInput(option.value as 'queue' | 'reverse_queue' | 'modtime')}>{option.label}</button>
              {/each}
            </div>
          </div>
        </div>
      </section>

      <section
        class={`upload-zone ${dragActive ? 'is-drag' : ''}`}
        role="group"
        aria-label="File upload drop zone"
        style="position: relative;"
        ondragover={handleDragOver}
        ondragleave={handleDragLeave}
        ondrop={handleDrop}
      >
        <input bind:this={fileInput} type="file" multiple hidden onchange={picked} />
        <button
          type="button"
          aria-label="Browse files from drop zone"
          style="position: absolute; inset: 0; z-index: 0; border: 0; background: transparent; cursor: pointer;"
          onclick={chooseFiles}
        ></button>
        <div class="seg" aria-label="Upload drop behavior" style="position: absolute; top: 12px; right: 12px; z-index: 2;">
          <button type="button" class={!autoUpload ? 'is-active' : ''} onclick={() => onAutoUploadInput(false)}>Stage first</button>
          <button type="button" class={autoUpload ? 'is-active' : ''} onclick={() => onAutoUploadInput(true)}>Auto-upload</button>
        </div>
        <div class="icon-wrap" style="position: relative; z-index: 1; pointer-events: none;"><Icon name="upload" size={26} /></div>
        <h3 style="position: relative; z-index: 1; pointer-events: none;">Drop files here</h3>
        <p style="position: relative; z-index: 1; pointer-events: none;">or click to browse</p>
        <div class="upload-zone-actions" style="position: relative; z-index: 2;">
          <button class="g-btn g-btn-primary" type="button" onclick={chooseFiles}><Icon name="folder" size={14} /> Choose files…</button>
          <button class="g-btn" type="button" disabled title="Paste URL import coming soon"><Icon name="external" size={14} /> Paste URL</button>
        </div>
      </section>

      {#if stagedRows.length > 0}
        <section>
          <div class="upload-list-head">
            <div class="g-eyebrow">Staged · {stagedRows.length} {stagedRows.length === 1 ? 'file' : 'files'} · {formatBytes(stagedBytes)}</div>
            <div class="upload-list-actions">
              <input
                class="g-input compact upload-filter-input"
                type="search"
                value={stagedFilter}
                placeholder="Filter staged files"
                aria-label="Filter staged files"
                oninput={(event) => setStagedFilter(event.currentTarget.value)}
              />
              <button class="g-btn g-btn-sm" type="button" onclick={() => clearRows('staged')}><Icon name="close" size={12} /> Clear staged</button>
              <button class="g-btn g-btn-primary g-btn-sm" type="submit">
                <Icon name="upload" size={12} /> Upload {stagedRows.length} {stagedRows.length === 1 ? 'file' : 'files'}
              </button>
            </div>
          </div>
          <div class="g-card upload-list-card">
            {#if filteredStagedRows.length === 0}
              <div class="upload-filter-empty">No staged files match this filter.</div>
            {:else}
              <div class="upload-list" data-testid="staged-upload-list">
                {#each stagedVisibleRows as row (row.index)}
                  {@const item = row.item}
                  <div
                    class:is-viewable={uploadItemCanOpenViewer(item)}
                    class="upload-row upload-row-staged"
                    data-testid={`upload-row-${row.index}`}
                  >
                    <button
                      class="upload-row-open-target"
                      type="button"
                      disabled={!uploadItemCanOpenViewer(item)}
                      aria-label={`Preview ${item.name}`}
                      title={uploadItemCanOpenViewer(item) ? `Preview ${item.name}` : 'Preview available after import'}
                      onclick={() => openViewer(row.index)}
                    ></button>
                    <UploadMediaPreview file={item.previewFile} {item} />
                    <div class="upload-item-main">
                      <div class="name">{item.name}</div>
                      <div class="upload-item-tags upload-tags-control" aria-label={`Tags for ${item.name}`}>
                        {#each item.tags ?? [] as tag}
                          {@const separator = tag.indexOf(':')}
                          <span class="g-tag">
                            {#if separator > 0}
                              <span class="g-tag-ns">{tag.slice(0, separator)}:</span><span>{tag.slice(separator + 1)}</span>
                            {:else}
                              <span>{tag}</span>
                            {/if}
                            <button class="g-tag-x" type="button" aria-label={`Remove ${tag} from ${item.name}`} onclick={() => removeItemTag(row.index, item, tag)}>
                              <Icon name="close" size={11} />
                            </button>
                          </span>
                        {/each}
                        <TagAutocompleteInput
                          value={itemTagDrafts[row.index] ?? ''}
                          tags={completionTags}
                          existing={item.tags ?? []}
                          placeholder="add tag"
                          ariaLabel={`Add tag to ${item.name}`}
                          onInput={(value) => setItemTagDraft(row.index, value)}
                          onCommit={(value) => commitItemTag(row.index, item, value)}
                          onRemoveLast={(tag) => removeItemTag(row.index, item, tag)}
                        />
                      </div>
                    </div>
                    <div class="size">{formatBytes(item.size)}</div>
                    <div class="progress is-staged" aria-hidden="true"></div>
                    <div class="status">
                      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" title="Remove from staging" aria-label={`Remove ${item.name} from staging`} onclick={() => removeStagedItem(row.index)}>
                        <Icon name="close" size={11} />
                      </button>
                    </div>
                  </div>
                {/each}
              </div>
            {/if}
            {#if stagedPageCount > 1}
              <div class="upload-list-pager">
                <PageNav
                  page={stagedPage + 1}
                  pageCount={stagedPageCount}
                  ariaLabel="Staged upload pages"
                  embedded
                  onPage={(page) => (stagedPage = page - 1)}
                />
                <span>Showing {stagedVisibleRows.length} at a time</span>
              </div>
            {/if}
          </div>
        </section>
      {/if}

      {#if queueRows.length > 0}
        <section class="upload-queue-section" aria-label={uploadStatus || 'Upload queue'}>
          <div class:has-active-job={uploadBusy || Boolean(activeUploadJobID)} class="upload-list-head upload-queue-head">
            <div class="g-eyebrow">Queue · {queueRows.length} {queueRows.length === 1 ? 'file' : 'files'} · {formatBytes(queueBytes)}</div>
            <div class="upload-list-actions upload-queue-actions">
              {#if uploadBusy || activeUploadJobID}
                <button
                  class="g-btn g-btn-sm"
                  type="button"
                  disabled={cancelBusy || cancelRequested}
                  onclick={() => onCancel(activeUploadJobID)}
                >
                  <Icon name="close" size={12} /> {cancelBusy ? 'Canceling' : cancelRequested ? 'Canceled' : 'Cancel'}
                </button>
              {/if}
              <button class="g-btn g-btn-sm" type="button" disabled={uploadBusy || Boolean(activeUploadJobID)} onclick={() => clearRows('done')}><Icon name="close" size={12} /> Clear done</button>
            </div>
          </div>
          <div class="upload-batches" data-testid="upload-queue-list">
            {#each queueBatches as batch (batch.batchID ?? 'legacy')}
              {@const pageState = queueBatchPage(batch)}
              {@const batchSummary = summarizeUploadQueueBatch(batch)}
              <div class="g-card upload-list-card upload-batch-card" data-testid="upload-queue-batch" data-batch-id={batch.batchID ?? 'legacy'}>
                <div class="upload-list-head upload-batch-head">
                  <div>
                    <div class="g-eyebrow">{batchLabel(batch)} · {batch.rows.length} {batch.rows.length === 1 ? 'file' : 'files'} · {formatBytes(batch.bytes)}</div>
                    <div class="upload-batch-status">{batchSummary.status || 'Waiting'} · {batchSummary.progress}%</div>
                  </div>
                  <div class="progress upload-batch-progress" aria-label={`${batchLabel(batch)} ${batchSummary.progress}%`}>
                    <div style={`width: ${batchSummary.progress}%`}></div>
                  </div>
                </div>
                <div class="upload-list">
                  {#each pageState.rows as row (row.index)}
                    {@const item = row.item}
                    <div
                      class:is-viewable={uploadItemCanOpenViewer(item)}
                      class="upload-row"
                      data-testid={`upload-row-${row.index}`}
                    >
                      <button
                        class="upload-row-open-target"
                        type="button"
                        disabled={!uploadItemCanOpenViewer(item)}
                        aria-label={`Preview ${item.name}`}
                        title={uploadItemCanOpenViewer(item) ? `Preview ${item.name}` : 'Preview available after import'}
                        onclick={() => openViewer(row.index)}
                      ></button>
                      <UploadMediaPreview file={item.previewFile} {item} />
                      <div class="upload-item-main">
                        <div class="name">{item.name}</div>
                        {#if item.error}<div class="upload-error">{item.error}</div>{/if}
                        {#if item.tagSyncError}<div class="upload-error">{item.tagSyncError}</div>{/if}
                        <div class="upload-item-tags upload-tags-control" aria-label={`Tags for ${item.name}`}>
                          {#each item.tags ?? [] as tag}
                            {@const separator = tag.indexOf(':')}
                            <span class="g-tag">
                              {#if separator > 0}
                                <span class="g-tag-ns">{tag.slice(0, separator)}:</span><span>{tag.slice(separator + 1)}</span>
                              {:else}
                                <span>{tag}</span>
                              {/if}
                              <button class="g-tag-x" type="button" aria-label={`Remove ${tag} from ${item.name}`} onclick={() => removeItemTag(row.index, item, tag)}>
                                <Icon name="close" size={11} />
                              </button>
                            </span>
                          {/each}
                          <TagAutocompleteInput
                            value={itemTagDrafts[row.index] ?? ''}
                            tags={completionTags}
                            existing={item.tags ?? []}
                            placeholder="add tag"
                            ariaLabel={`Add tag to ${item.name}`}
                            onInput={(value) => setItemTagDraft(row.index, value)}
                            onCommit={(value) => commitItemTag(row.index, item, value)}
                            onRemoveLast={(tag) => removeItemTag(row.index, item, tag)}
                          />
                        </div>
                      </div>
                      <div class="size">{formatBytes(item.size)}</div>
                      <div class="progress" aria-label={`${statusLabel(item.status)} ${item.progress}%`}>
                        <div style={`width: ${item.progress}%`}></div>
                      </div>
                      <div class={`status ${statusClass(item.status)}`}>{statusLabel(item.status)}</div>
                    </div>
                  {/each}
                </div>
                {#if pageState.pageCount > 1}
                  <div class="upload-list-pager">
                    <PageNav
                      page={pageState.page + 1}
                      pageCount={pageState.pageCount}
                      ariaLabel={`${batchLabel(batch)} upload queue pages`}
                      embedded
                      onPage={(page) => setQueueBatchPage(batch, page - 1)}
                    />
                    <span>Showing {pageState.rows.length} at a time</span>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </section>
      {/if}
    </form>
  </div>
</main>

{#if viewerIndex != null}
  <UploadViewerDialog
    {uploadItems}
    activeIndex={viewerIndex}
    scope={viewerScope}
    tags={completionTags}
    onIndex={(index) => (viewerIndex = index)}
    onClose={closeViewer}
    {onItemTagsInput}
    {onItemRemoteTagsLoaded}
    {onRemove}
    onRemoteUntrack={onViewerUntrack}
    onRemoteDelete={onViewerDelete}
    onTagSearch={onViewerTagSearch}
  />
{/if}

<style>
  .upload-row {
    position: relative;
  }

  .upload-row-open-target {
    position: absolute;
    z-index: 1;
    inset: 0;
    width: 100%;
    border: 0;
    border-radius: inherit;
    padding: 0;
    background: transparent;
    cursor: pointer;
  }

  .upload-row-open-target:disabled {
    cursor: default;
  }

  .upload-row-open-target:focus-visible {
    outline: 2px solid var(--accent-line);
    outline-offset: -2px;
  }

  .upload-row.is-viewable {
    cursor: pointer;
    transition: background 120ms ease;
  }

  .upload-row.is-viewable:hover {
    background: var(--surface-2);
  }

  .upload-row :is(button:not(.upload-row-open-target), input) {
    position: relative;
    z-index: 2;
  }

  .upload-item-main {
    min-width: 0;
  }

  .upload-item-tags {
    margin-top: 6px;
    min-height: 30px;
  }

  .upload-filter-input {
    min-width: 190px;
    padding: 5px 10px;
    font-size: 12px;
  }

  .upload-filter-empty {
    padding: 18px 14px;
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .upload-batches {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .upload-batch-head {
    gap: 14px;
    padding: 12px 14px 0;
  }

  .upload-batch-status {
    margin-top: 4px;
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 11px;
    text-transform: lowercase;
  }

  .upload-batch-progress {
    flex: 1 1 180px;
    max-width: 280px;
    min-width: 140px;
  }

  .upload-list-pager {
    display: flex;
    flex-direction: column;
    align-items: center;
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 11px;
    padding-bottom: 10px;
  }
</style>
