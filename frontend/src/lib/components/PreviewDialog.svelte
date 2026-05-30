<script lang="ts">
  import { ExternalLink, X } from '@lucide/svelte';
  import { formatBytes } from '$lib/utils/format';
  import type { FileItem } from '$lib/api/types';

  let {
    file,
    previewURL,
    previewLoading,
    previewError,
    onClose
  } = $props<{
    file: FileItem;
    previewURL: string;
    previewLoading: boolean;
    previewError: string;
    onClose: () => void;
  }>();
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-3 backdrop-blur-sm sm:p-6" role="presentation">
  <button class="absolute inset-0 cursor-default" type="button" aria-label="Close preview" onclick={onClose}></button>
  <div
    class="relative z-10 flex max-h-[92vh] w-full max-w-6xl flex-col overflow-hidden rounded-md border border-white/10 bg-zinc-950 shadow-2xl shadow-black/60"
    role="dialog"
    aria-modal="true"
    aria-labelledby="preview-title"
  >
    <header class="flex items-center justify-between gap-3 border-b border-white/10 px-4 py-3">
      <div class="min-w-0">
        <h2 id="preview-title" class="truncate text-base font-semibold text-white">{file.name}</h2>
        <p class="mt-1 truncate text-xs text-zinc-400">{file.media_kind} / {formatBytes(file.size)}</p>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <a
          class="inline-flex h-9 w-9 items-center justify-center rounded border border-white/10 text-zinc-200 transition hover:bg-white/10"
          href={file.media_urls.content}
          target="_blank"
          rel="noreferrer"
          title="Open original"
          aria-label={`Open original ${file.name}`}
        >
          <ExternalLink size={16} />
        </a>
        <button
          class="inline-flex h-9 w-9 items-center justify-center rounded border border-white/10 text-zinc-200 transition hover:bg-white/10"
          type="button"
          title="Close preview"
          aria-label="Close preview"
          onclick={onClose}
        >
          <X size={17} />
        </button>
      </div>
    </header>
    <div class="grid min-h-0 flex-1 lg:grid-cols-[minmax(0,1fr)_18rem]">
      <div class="flex min-h-[18rem] items-center justify-center bg-black p-3 sm:p-5">
        {#if previewLoading}
          <div class="h-16 w-16 rounded-md border border-white/10 bg-white/[0.06]"></div>
        {:else if previewError}
          <div class="max-w-md rounded border border-red-400/30 bg-red-500/10 p-4 text-sm text-red-100">{previewError}</div>
        {:else if previewURL}
          <img class="max-h-[72vh] max-w-full object-contain" src={previewURL} alt={file.name} />
        {/if}
      </div>
      <aside class="overflow-y-auto border-t border-white/10 p-4 lg:border-l lg:border-t-0">
        <div class="space-y-4 text-sm">
          <div>
            <div class="text-xs uppercase text-zinc-500">Content id</div>
            <div class="mt-1 break-all font-mono text-xs text-zinc-300">{file.content_id}</div>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <div class="text-xs uppercase text-zinc-500">Type</div>
              <div class="mt-1 text-zinc-200">{file.media_type}</div>
            </div>
            <div>
              <div class="text-xs uppercase text-zinc-500">Modified</div>
              <div class="mt-1 text-zinc-200">{new Date(file.modified_time).toLocaleDateString()}</div>
            </div>
          </div>
          <div>
            <div class="text-xs uppercase text-zinc-500">Tags</div>
            <div class="mt-2 flex flex-wrap gap-1">
              {#each file.tags as tag}
                <span class="rounded border border-white/10 bg-white/[0.05] px-1.5 py-0.5 text-xs text-zinc-300">{tag}</span>
              {/each}
            </div>
          </div>
        </div>
      </aside>
    </div>
  </div>
</div>
