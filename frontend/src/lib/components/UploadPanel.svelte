<script lang="ts">
  import { Upload } from '@lucide/svelte';
  import JobStatus from './JobStatus.svelte';

  let {
    uploadFiles,
    uploadTags,
    uploadBusy,
    cancelBusy,
    uploadStatus,
    activeUploadJobID,
    onFiles,
    onTagsInput,
    onSubmit,
    onCancel
  } = $props<{
    uploadFiles: File[];
    uploadTags: string;
    uploadBusy: boolean;
    cancelBusy: boolean;
    uploadStatus: string;
    activeUploadJobID: string;
    onFiles: (files: FileList | null) => void;
    onTagsInput: (value: string) => void;
    onSubmit: () => void;
    onCancel: () => void;
  }>();
</script>

<form class="rounded-md border border-white/10 bg-white/[0.03] p-3" onsubmit={(event) => { event.preventDefault(); onSubmit(); }}>
  <div class="mb-2 flex items-center gap-2 text-xs uppercase text-zinc-500">
    <Upload size={14} />
    <span>Upload</span>
  </div>
  <input
    class="block w-full text-xs text-zinc-300 file:mr-3 file:rounded file:border-0 file:bg-white/10 file:px-2 file:py-1 file:text-xs file:text-zinc-100"
    type="file"
    multiple
    onchange={(event) => onFiles(event.currentTarget.files)}
  />
  <input
    class="mt-2 w-full rounded border border-white/10 bg-black/20 px-2 py-1.5 text-xs text-zinc-100 outline-none placeholder:text-zinc-500"
    value={uploadTags}
    oninput={(event) => onTagsInput(event.currentTarget.value)}
    placeholder="initial tags"
  />
  <button
    class="mt-2 w-full rounded-md border border-emerald-400/30 bg-emerald-500/15 px-3 py-2 text-sm font-semibold text-emerald-100 transition hover:bg-emerald-500/25 disabled:cursor-not-allowed disabled:opacity-60"
    type="submit"
    disabled={!uploadFiles.length || uploadBusy || Boolean(activeUploadJobID)}
  >
    {uploadBusy ? uploadStatus : activeUploadJobID ? 'Import running' : `Import ${uploadFiles.length || ''}`.trim()}
  </button>
  {#if activeUploadJobID}
    <JobStatus jobID={activeUploadJobID} status={uploadStatus} {cancelBusy} onCancel={onCancel} />
  {/if}
  {#if uploadStatus && !uploadBusy && !activeUploadJobID}
    <p class="mt-2 text-xs text-zinc-300">{uploadStatus}</p>
  {/if}
</form>
