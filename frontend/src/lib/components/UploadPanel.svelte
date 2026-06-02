<script lang="ts">
  import Icon from './Icon.svelte';
  import JobStatus from './JobStatus.svelte';
  import { formatBytes } from '$lib/utils/format';
  import type { UploadItem } from '$lib/state/uploadItems';

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
    conflictPolicy,
    autoUpload,
    onTargetInput,
    onFiles,
    onTagsInput,
    onConflictInput,
    onAutoUploadInput,
    onSubmit,
    onCancel,
    onClear,
    onRemove
  } = $props<{
    uploadFiles: File[];
    uploadItems: UploadItem[];
    uploadTags: string;
    uploadBusy: boolean;
    cancelBusy: boolean;
    cancelRequested?: boolean;
    uploadStatus: string;
    activeUploadJobID: string;
    targets: Array<{ id: string; name: string }>;
    targetID: string;
    conflictPolicy: string;
    autoUpload: boolean;
    onTargetInput: (value: string) => void;
    onFiles: (files: FileList | File[] | null) => void;
    onTagsInput: (value: string) => void;
    onConflictInput: (value: string) => void;
    onAutoUploadInput: (value: boolean) => void;
    onSubmit: () => void;
    onCancel: (jobID: string) => void;
    onClear: () => void;
    onRemove: (index: number) => void;
  }>();

  let dragActive = $state(false);
  let fileInput: HTMLInputElement | undefined;

  const stagedItems = $derived(uploadItems.filter((item: UploadItem) => item.status === 'staged'));
  const queueItems = $derived(uploadItems.filter((item: UploadItem) => item.status !== 'staged'));
  const stagedBytes = $derived(uploadFiles.reduce((sum: number, file: File) => sum + file.size, 0));
  const queueBytes = $derived(queueItems.reduce((sum: number, item: UploadItem) => sum + item.size, 0));

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
    if (event.currentTarget !== event.target) return;
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

  function itemIcon(item: UploadItem) {
    if (item.type?.startsWith('video/')) return 'video';
    if (item.type?.startsWith('audio/')) return 'audio';
    if (item.type === 'image/gif') return 'gif';
    if (item.name.match(/\.(zip|tar|gz)$/i)) return 'folder';
    return 'photo';
  }

  function statusClass(status: string) {
    if (status === 'imported' || status === 'uploaded') return 'ok';
    if (status === 'error') return 'err';
    return '';
  }

  function statusLabel(status: string) {
    return status.replace(/_/g, ' ');
  }
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Upload</div>
      <h1>Import media into your library</h1>
      <p>Files are content-hashed on receipt. Duplicates are detected automatically. Initial tags can be applied here.</p>
    </div>

    <form class="upload-stack" onsubmit={(event) => { event.preventDefault(); onSubmit(); }}>
      <section class="g-card upload-config-card">
        <label class="field-row">
          <span>Target</span>
          <select class="g-input" value={targetID} onchange={(event) => onTargetInput(event.currentTarget.value)}>
            {#each targets as target}
              <option value={target.id}>{target.name}</option>
            {:else}
              <option value="">Default target</option>
            {/each}
          </select>
        </label>

        <label class="field-row">
          <span>Initial tags</span>
          <div class="field-control upload-tags-control">
            <input class="g-input" value={uploadTags} oninput={(event) => onTagsInput(event.currentTarget.value)} placeholder="collection:inbox @review" />
            <button class="g-btn g-btn-ghost g-btn-sm" type="button" disabled title="Tag picker coming soon"><Icon name="plus" size={12} /> Add tag</button>
          </div>
        </label>

        <div class="field-row">
          <span>On conflict</span>
          <div class="field-control">
            <div class="seg" aria-label="Upload conflict policy">
              {#each [{ value: 'skip', label: 'Skip' }, { value: 'rename', label: 'Rename' }, { value: 'replace', label: 'Replace' }] as option}
                <button type="button" class={conflictPolicy === option.value ? 'is-active' : ''} onclick={() => onConflictInput(option.value)}>{option.label}</button>
              {/each}
            </div>
          </div>
        </div>

        <div class="field-row">
          <span>On drop</span>
          <div class="field-control">
            <div class="seg" aria-label="Upload drop behavior">
              <button type="button" class={!autoUpload ? 'is-active' : ''} onclick={() => onAutoUploadInput(false)}>Stage first</button>
              <button type="button" class={autoUpload ? 'is-active' : ''} onclick={() => onAutoUploadInput(true)}>Auto-upload</button>
            </div>
          </div>
        </div>
      </section>

      <section
        class={`upload-zone ${dragActive ? 'is-drag drag-active' : ''}`}
        role="button"
        tabindex="0"
        onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); chooseFiles(); } }}
        onclick={chooseFiles}
        ondragover={handleDragOver}
        ondragleave={handleDragLeave}
        ondrop={handleDrop}
      >
        <input bind:this={fileInput} type="file" multiple class="sr-only" onchange={picked} />
        <div class="icon-wrap"><Icon name="upload" size={26} /></div>
        <h3>Drop files here</h3>
        <p>or click to browse</p>
        <div class="upload-zone-actions">
          <button class="g-btn g-btn-primary" type="button" onclick={(event) => { event.stopPropagation(); chooseFiles(); }}><Icon name="folder" size={14} /> Choose files…</button>
          <button class="g-btn" type="button" disabled title="Paste URL import coming soon" onclick={(event) => event.stopPropagation()}><Icon name="external" size={14} /> Paste URL</button>
        </div>
      </section>

      {#if stagedItems.length > 0}
        <section>
          <div class="upload-list-head">
            <div class="g-eyebrow">Staged · {stagedItems.length} {stagedItems.length === 1 ? 'file' : 'files'} · {formatBytes(stagedBytes)}</div>
            <div class="upload-list-actions">
              <button class="g-btn g-btn-sm" type="button" onclick={onClear}><Icon name="close" size={12} /> Clear staged</button>
              <button class="g-btn g-btn-primary g-btn-sm" type="submit" disabled={uploadBusy || Boolean(activeUploadJobID)}>
                <Icon name="upload" size={12} /> Upload {stagedItems.length} {stagedItems.length === 1 ? 'file' : 'files'}
              </button>
            </div>
          </div>
          <div class="g-card upload-list-card">
            <div class="upload-list">
              {#each stagedItems as item, index}
                <div class="upload-row upload-row-staged">
                  <div class="thumb-tile"><Icon name={itemIcon(item)} size={18} /></div>
                  <div><div class="name">{item.name}</div></div>
                  <div class="size">{formatBytes(item.size)}</div>
                  <div class="progress is-staged" aria-hidden="true"></div>
                  <div class="status">
                    <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" title="Remove from staging" onclick={() => onRemove(index)}>
                      <Icon name="close" size={11} />
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          </div>
        </section>
      {/if}

      {#if queueItems.length > 0}
        <section>
          <div class="upload-list-head">
            <div class="g-eyebrow">Uploads · {queueItems.length} {queueItems.length === 1 ? 'file' : 'files'} · {formatBytes(queueBytes)}</div>
            {#if activeUploadJobID}
              <JobStatus jobID={activeUploadJobID} status={uploadStatus} {cancelBusy} {cancelRequested} onCancel={onCancel} />
            {:else if uploadStatus}
              <p class="status-note">{uploadStatus}</p>
            {/if}
          </div>
          <div class="g-card upload-list-card">
            <div class="upload-list">
              {#each queueItems as item}
                <div class="upload-row">
                  <div class="thumb-tile"><Icon name={itemIcon(item)} size={18} /></div>
                  <div>
                    <div class="name">{item.name}</div>
                    {#if item.error}<div class="upload-error">{item.error}</div>{/if}
                  </div>
                  <div class="size">{formatBytes(item.size)}</div>
                  <div class="progress" aria-label={`${statusLabel(item.status)} ${item.progress}%`}>
                    <div style={`width: ${item.progress}%`}></div>
                  </div>
                  <div class={`status ${statusClass(item.status)}`}>{statusLabel(item.status)}</div>
                </div>
              {/each}
            </div>
          </div>
        </section>
      {/if}
    </form>
  </div>
</main>
