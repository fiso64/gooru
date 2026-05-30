<script lang="ts">
  import MediaCard from './MediaCard.svelte';
  import { errorMessage } from '$lib/utils/format';
  import type { VirtualGrid } from '$lib/state/ui';
  import type { FileItem } from '$lib/api/types';

  let {
    authToken,
    isLoading,
    isError,
    error,
    files,
    virtual,
    hasNextPage,
    isFetchingNextPage,
    loadMoreSentinel = $bindable<HTMLDivElement | undefined>(),
    tagDrafts,
    tagBusy,
    tagErrors,
    onOpen,
    onTagInput,
    onMutateTags
  } = $props<{
    authToken: string;
    isLoading: boolean;
    isError: boolean;
    error: unknown;
    files: FileItem[];
    virtual: VirtualGrid;
    hasNextPage: boolean;
    isFetchingNextPage: boolean;
    loadMoreSentinel?: HTMLDivElement;
    tagDrafts: Record<string, string>;
    tagBusy: Record<string, boolean>;
    tagErrors: Record<string, string>;
    onOpen: (file: FileItem) => void;
    onTagInput: (fileID: string, value: string) => void;
    onMutateTags: (file: FileItem, operation: 'add' | 'set' | 'remove') => void;
  }>();
</script>

{#if !authToken}
  <div class="flex min-h-[22rem] items-center justify-center rounded-md border border-dashed border-white/15 bg-white/[0.03] p-8 text-center text-zinc-300">
    <p class="max-w-md text-sm leading-6">Enter the token configured for `gooru serve` to browse this library.</p>
  </div>
{:else if isLoading}
  <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
    {#each Array(12) as _}
      <div class="aspect-[4/5] rounded-md border border-white/10 bg-white/[0.05]"></div>
    {/each}
  </div>
{:else if isError}
  <div class="rounded-md border border-red-400/30 bg-red-500/10 p-4 text-sm text-red-100">
    {errorMessage(error)}
  </div>
{:else if !files.length}
  <div class="flex min-h-[22rem] items-center justify-center rounded-md border border-dashed border-white/15 bg-white/[0.03] p-8 text-center text-zinc-300">
    <p class="max-w-md text-sm leading-6">No files matched this query.</p>
  </div>
{:else}
  <div class="mb-3 flex items-center justify-between gap-3 text-sm text-zinc-400">
    <span>{files.length} files loaded</span>
    {#if hasNextPage}
      <span>{isFetchingNextPage ? 'Loading more' : 'Scroll for more results'}</span>
    {/if}
  </div>
  <div class="relative" style={`height: ${virtual.totalHeight}px;`}>
    <div
      class="absolute inset-x-0 top-0 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8"
      data-testid="virtual-media-grid"
      style={`transform: translateY(${virtual.offsetTop}px);`}
    >
      {#each virtual.files as file (file.id)}
        <MediaCard
          {file}
          token={authToken}
          tagDraft={tagDrafts[file.id] ?? ''}
          tagBusy={Boolean(tagBusy[file.id])}
          tagError={tagErrors[file.id] ?? ''}
          onOpen={onOpen}
          onTagInput={onTagInput}
          onMutateTags={onMutateTags}
        />
      {/each}
    </div>
  </div>
  {#if hasNextPage || isFetchingNextPage}
    <div bind:this={loadMoreSentinel} class="mt-4 flex min-h-12 items-center justify-center text-sm text-zinc-400" data-testid="infinite-scroll-sentinel">
      {isFetchingNextPage ? 'Loading more results' : 'More results available'}
    </div>
  {/if}
{/if}
