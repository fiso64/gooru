<script lang="ts">
  import { FileImage } from '@lucide/svelte';
  import type { FileItem } from '$lib/api/types';

  let { file, token, size = 256 } = $props<{ file: FileItem; token: string; size?: number }>();

  let objectURL = $state('');
  let failed = $state(false);
  let loading = $state(false);

  $effect(() => {
    objectURL = '';
    failed = false;
    loading = false;

    if (file.media_kind !== 'image' || !token) return;

    const controller = new AbortController();
    let disposed = false;
    let currentURL = '';
    loading = true;

    fetch(`${file.media_urls.thumbnail}?size=${size}`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: controller.signal
    })
      .then(async (response) => {
        if (!response.ok) throw new Error(`thumbnail request failed: ${response.status}`);
        const blob = await response.blob();
        if (disposed) return;
        currentURL = URL.createObjectURL(blob);
        objectURL = currentURL;
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
      controller.abort();
      if (currentURL) URL.revokeObjectURL(currentURL);
    };
  });
</script>

<div class="relative flex aspect-square items-center justify-center bg-black/30 text-zinc-500">
  {#if objectURL && !failed}
    <img class="h-full w-full object-cover" src={objectURL} alt="" />
  {:else}
    <FileImage size={34} />
  {/if}
  {#if loading}
    <div class="absolute inset-x-0 bottom-0 h-1 overflow-hidden bg-white/10">
      <div class="h-full w-1/2 animate-pulse bg-emerald-300/70"></div>
    </div>
  {/if}
</div>
