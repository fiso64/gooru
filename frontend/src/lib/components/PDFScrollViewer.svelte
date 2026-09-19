<script lang="ts">
  import { onMount } from 'svelte';

  let {
    url,
    fileName,
    onReady,
    onError
  }: {
    url: string;
    fileName: string;
    onReady: () => void;
    onError: (message: string) => void;
  } = $props();

  const batchSize = 6;
  let pageCount = $state(0);
  let shownCount = $state(0);
  let pagePrefix = $state('');
  let error = $state('');
  let sentinel = $state<HTMLDivElement | undefined>();
  let firstPagePresented = false;
  // The viewport, rather than the PDF document itself, owns scrolling.

  onMount(() => {
    const controller = new AbortController();
    async function loadManifest() {
      try {
        const response = await fetch(url, { credentials: 'same-origin', signal: controller.signal, cache: 'no-store' });
        if (!response.ok) throw new Error(`PDF viewer could not load the document (${response.status}).`);
        const manifest: { page_count?: unknown; page_url_prefix?: unknown } = await response.json();
        const count = manifest.page_count;
        const prefix = manifest.page_url_prefix;
        if (!Number.isInteger(count) || typeof count !== 'number' || count < 1 || count > 10000 ||
            typeof prefix !== 'string' || prefix !== `${url}?page=`) {
          throw new Error('PDF viewer received an invalid document manifest.');
        }
        if (controller.signal.aborted) return;
        pageCount = count;
        pagePrefix = prefix;
        shownCount = Math.min(batchSize, count);
      } catch (reason) {
        if (controller.signal.aborted) return;
        error = reason instanceof Error ? reason.message : 'PDF viewer could not load the document.';
        onError(error);
      }
    }
    void loadManifest();
    return () => controller.abort();
  });

  $effect(() => {
    const target = sentinel;
    if (!target || !pageCount || shownCount >= pageCount) return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) {
        shownCount = Math.min(pageCount, shownCount + batchSize);
      }
    }, { root: target.closest('.pdf-scroll-viewport'), rootMargin: '800px' });
    observer.observe(target);
    return () => observer.disconnect();
  });

  function presented(page: number) {
    if (page !== 1 || firstPagePresented) return;
    firstPagePresented = true;
    onReady();
  }

  function failed(page: number) {
    const message = `Could not render PDF page ${page}.`;
    if (page === 1) onError(message);
    error = message;
  }
</script>

<div class="pdf-scroll-viewport" role="region" aria-label={`PDF document: ${fileName}`} tabindex="0">
  {#if error}<p role="alert" class="pdf-scroll-status">{error}</p>{/if}
  {#if !pageCount && !error}
    <p role="status" class="pdf-scroll-status">Loading PDF document…</p>
  {:else if pageCount}
    <div class="pdf-scroll-pages">
      {#each Array.from({ length: shownCount }, (_, index) => index + 1) as page (page)}
        <figure class="pdf-scroll-page">
          <img
            src={`${pagePrefix}${page}`}
            alt={`${fileName}, page ${page} of ${pageCount}`}
            loading={page === 1 ? 'eager' : 'lazy'}
            decoding="async"
            onload={() => presented(page)}
            onerror={() => failed(page)}
          />
          <figcaption>Page {page} of {pageCount}</figcaption>
        </figure>
      {/each}
      {#if shownCount < pageCount}
        <div bind:this={sentinel} class="pdf-scroll-next" role="status">Loading more pages…</div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .pdf-scroll-viewport { position: absolute; inset: 0; overflow: auto; overscroll-behavior: contain; padding: 20px 56px; background: #292929; }
  .pdf-scroll-pages { display: flex; flex-direction: column; align-items: center; gap: 18px; }
  .pdf-scroll-page { margin: 0; width: min(100%, 1200px); display: flex; flex-direction: column; align-items: center; gap: 6px; }
  .pdf-scroll-page img { display: block; max-width: 100%; height: auto; background: white; box-shadow: 0 2px 12px #0007; }
  .pdf-scroll-page figcaption { font-size: 12px; color: #eee; }
  .pdf-scroll-status, .pdf-scroll-next { color: #fff; text-align: center; }
  @media (max-width: 600px) { .pdf-scroll-viewport { padding: 12px 16px; } }
</style>
