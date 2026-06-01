<script lang="ts">
  import AuthenticatedThumbnail from './AuthenticatedThumbnail.svelte';
  import TagEditor from './TagEditor.svelte';
  import { formatBytes, parseTags } from '$lib/utils/format';
  import type { FileItem } from '$lib/api/types';

  let {
    file,
    tagDraft,
    tagBusy,
    tagError,
    onOpen,
    onTagInput,
    onMutateTags
  } = $props<{
    file: FileItem;
    tagDraft: string;
    tagBusy: boolean;
    tagError: string;
    onOpen: (file: FileItem) => void;
    onTagInput: (fileID: string, value: string) => void;
    onMutateTags: (file: FileItem, operation: 'add' | 'set' | 'remove') => void;
  }>();
</script>

<article class="group flex min-h-[21.5rem] flex-col overflow-hidden rounded-md border border-white/10 bg-white/[0.04] transition hover:border-emerald-300/50 hover:bg-white/[0.07] sm:min-h-[24rem] lg:min-h-[26rem]">
  <button
    class="block w-full text-left"
    type="button"
    aria-label={`Preview ${file.name}`}
    onclick={() => onOpen(file)}
  >
    <AuthenticatedThumbnail {file} size={256} />
  </button>
  <div class="space-y-2 p-3">
    <h2 class="truncate text-sm font-semibold text-zinc-100" title={file.name}>{file.name}</h2>
    <div class="flex items-center justify-between gap-2 text-xs text-zinc-400">
      <span>{file.media_kind}</span>
      <span>{formatBytes(file.size)}</span>
    </div>
    <div class="flex min-h-6 flex-wrap gap-1">
      {#each file.tags.slice(0, 3) as tag}
        <span class="max-w-full truncate rounded border border-white/10 bg-black/20 px-1.5 py-0.5 text-[11px] text-zinc-300">{tag}</span>
      {/each}
    </div>
    <TagEditor
      fileID={file.id}
      fileName={file.name}
      draft={tagDraft}
      busy={tagBusy}
      error={tagError}
      canSubmit={Boolean(parseTags(tagDraft).length)}
      onInput={(value) => onTagInput(file.id, value)}
      onMutate={(operation) => onMutateTags(file, operation)}
    />
  </div>
</article>
