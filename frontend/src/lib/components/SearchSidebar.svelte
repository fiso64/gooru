<script lang="ts">
  import { RefreshCw, Search } from '@lucide/svelte';
  import UploadPanel from './UploadPanel.svelte';

  let {
    authSaved,
    searchDraft,
    loadedKinds,
    uploadFiles,
    uploadTags,
    uploadBusy,
    cancelBusy,
    uploadStatus,
    activeUploadJobID,
    onSearchInput,
    onSearchSubmit,
    onRefresh,
    onUploadFiles,
    onUploadTagsInput,
    onUploadSubmit,
    onUploadCancel
  } = $props<{
    authSaved: boolean;
    searchDraft: string;
    loadedKinds: string;
    uploadFiles: File[];
    uploadTags: string;
    uploadBusy: boolean;
    cancelBusy: boolean;
    uploadStatus: string;
    activeUploadJobID: string;
    onSearchInput: (value: string) => void;
    onSearchSubmit: () => void;
    onRefresh: () => void;
    onUploadFiles: (files: FileList | null) => void;
    onUploadTagsInput: (value: string) => void;
    onUploadSubmit: () => void;
    onUploadCancel: () => void;
  }>();
</script>

<form class="space-y-3" onsubmit={(event) => { event.preventDefault(); onSearchSubmit(); }}>
  <label>
    <span class="mb-1 block text-xs font-medium uppercase text-zinc-400">Search</span>
    <span class="flex items-center gap-2 rounded-md border border-white/10 bg-black/30 px-3 py-2 shadow-inner shadow-black/20">
      <Search size={16} class="shrink-0 text-zinc-400" />
      <input
        class="min-w-0 flex-1 bg-transparent text-sm text-zinc-100 outline-none placeholder:text-zinc-500"
        value={searchDraft}
        oninput={(event) => onSearchInput(event.currentTarget.value)}
        placeholder="tag, key:value, @tagged"
      />
    </span>
  </label>
  <div class="flex gap-2">
    <button class="rounded-md border border-white/10 bg-white/10 px-3 py-2 text-sm font-semibold text-white transition hover:bg-white/15" type="submit">
      Search
    </button>
    <button
      class="rounded-md border border-white/10 bg-transparent p-2 text-zinc-300 transition hover:bg-white/10"
      type="button"
      title="Refresh results"
      aria-label="Refresh results"
      onclick={onRefresh}
    >
      <RefreshCw size={17} />
    </button>
  </div>
</form>

<div class="mt-6 space-y-3 text-sm text-zinc-400">
  <div class="rounded-md border border-white/10 bg-white/[0.03] p-3">
    <div class="text-xs uppercase text-zinc-500">Status</div>
    <div class="mt-2 text-zinc-200">{authSaved ? 'Token saved' : 'Token required'}</div>
  </div>
  {#if loadedKinds}
    <div class="rounded-md border border-white/10 bg-white/[0.03] p-3">
      <div class="text-xs uppercase text-zinc-500">Kinds</div>
      <div class="mt-2 text-zinc-200">{loadedKinds}</div>
    </div>
  {/if}
  <UploadPanel
    {uploadFiles}
    {uploadTags}
    {uploadBusy}
    {cancelBusy}
    {uploadStatus}
    {activeUploadJobID}
    onFiles={onUploadFiles}
    onTagsInput={onUploadTagsInput}
    onSubmit={onUploadSubmit}
    onCancel={onUploadCancel}
  />
</div>
