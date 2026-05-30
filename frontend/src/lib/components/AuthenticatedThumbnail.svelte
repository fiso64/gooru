<script lang="ts">
  import { File, FileImage, FileVideo } from '@lucide/svelte';
  import { authenticatedMediaCache, type MediaLease } from '$lib/media/authenticated';
  import type { FileItem } from '$lib/api/types';

  let { file, token, size = 256 } = $props<{ file: FileItem; token: string; size?: number }>();

  let objectURL = $state('');
  let failed = $state(false);
  let loading = $state(false);

  $effect(() => {
    objectURL = '';
    failed = false;
    loading = false;

    if (!['image', 'video'].includes(file.media_kind) || !token) return;

    let disposed = false;
    let lease: MediaLease | undefined;
    loading = true;

    // TODO: replace authenticated blob URLs with direct <img src> media URLs once cookie-session auth lands.
    authenticatedMediaCache
      .load(`${file.media_urls.thumbnail}?size=${size}`, token)
      .then((loaded) => {
        if (disposed) {
          loaded.release();
          return;
        }
        lease = loaded;
        objectURL = loaded.url;
      })
      .catch((error) => {
        if (!disposed && error instanceof Error && error.name !== 'AbortError') {
          failed = true;
        }
      })
      .finally(() => {
        if (!disposed) loading = false;
      });

    return () => {
      disposed = true;
      lease?.release();
    };
  });
</script>

<div class="relative flex aspect-square items-center justify-center bg-zinc-950 text-zinc-500">
  {#if objectURL && !failed}
    <img class="h-full w-full object-cover" src={objectURL} alt="" />
  {:else if file.media_kind === 'video'}
    <FileVideo size={34} />
  {:else if file.media_kind === 'image'}
    <FileImage size={34} />
  {:else}
    <File size={34} />
  {/if}
  {#if loading}
    <div class="absolute inset-x-0 bottom-0 h-1 overflow-hidden bg-white/10">
      <div class="h-full w-1/2 animate-pulse bg-emerald-300/70"></div>
    </div>
  {/if}
</div>
