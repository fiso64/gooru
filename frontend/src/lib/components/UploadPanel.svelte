<script lang="ts">
  import Icon from './Icon.svelte';
  import JobStatus from './JobStatus.svelte';
  import { formatBytes } from '$lib/utils/format';

  let {
    uploadFiles,
    uploadTags,
    uploadBusy,
    cancelBusy,
    uploadStatus,
    activeUploadJobID,
    targets,
    targetID,
    onTargetInput,
    onFiles,
    onTagsInput,
    onSubmit,
    onCancel,
    onClear
  } = $props<{
    uploadFiles: File[];
    uploadTags: string;
    uploadBusy: boolean;
    cancelBusy: boolean;
    uploadStatus: string;
    activeUploadJobID: string;
    targets: Array<{ id: string; name: string }>;
    targetID: string;
    onTargetInput: (value: string) => void;
    onFiles: (files: FileList | null) => void;
    onTagsInput: (value: string) => void;
    onSubmit: () => void;
    onCancel: () => void;
    onClear: () => void;
  }>();

  function stagedSize(files: File[]) {
    return files.reduce((sum: number, file: File) => sum + file.size, 0);
  }
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Upload</div>
      <h1>Import media into your library</h1>
      <p>Files are sent to a configured upload target with optional initial tags. Paste URL import is intentionally hidden until the backend supports it.</p>
    </div>

    <form class="upload-layout" onsubmit={(event) => { event.preventDefault(); onSubmit(); }}>
      <section class="g-card upload-config">
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
          <input class="g-input" value={uploadTags} oninput={(event) => onTagsInput(event.currentTarget.value)} placeholder="collection:inbox @review" />
        </label>
      </section>

      <label class="upload-zone">
        <input type="file" multiple onchange={(event) => onFiles(event.currentTarget.files)} />
        <span class="icon-wrap"><Icon name="upload" size={26} /></span>
        <strong>Drop files here</strong>
        <span>or click to browse</span>
      </label>

      {#if uploadFiles.length}
        <section class="g-card upload-list-wrap">
          <div class="list-head">
            <span class="g-eyebrow">Staged · {uploadFiles.length} files · {formatBytes(stagedSize(uploadFiles))}</span>
            <button class="g-btn g-btn-sm" type="button" onclick={onClear}><Icon name="close" size={12} /> Clear staged</button>
          </div>
          <div class="upload-list">
            {#each uploadFiles as file}
              <div class="upload-row">
                <div class="thumb-tile"><Icon name={file.type.startsWith('video/') ? 'video' : 'photo'} size={18} /></div>
                <div class="name">{file.name}</div>
                <div class="size">{formatBytes(file.size)}</div>
                <div class="progress is-staged"></div>
                <div class="status">staged</div>
              </div>
            {/each}
          </div>
        </section>
      {/if}

      <div class="upload-actions">
        <button class="g-btn g-btn-primary" type="submit" disabled={!uploadFiles.length || uploadBusy || Boolean(activeUploadJobID)}>
          <Icon name="upload" size={14} />
          {uploadBusy ? uploadStatus : activeUploadJobID ? 'Import running' : `Upload ${uploadFiles.length || ''}`.trim()}
        </button>
        {#if activeUploadJobID}
          <JobStatus jobID={activeUploadJobID} status={uploadStatus} {cancelBusy} onCancel={onCancel} />
        {/if}
        {#if uploadStatus && !uploadBusy && !activeUploadJobID}
          <p class="status-note">{uploadStatus}</p>
        {/if}
      </div>
    </form>
  </div>
</main>
