<script lang="ts">
  import { createInfiniteQuery, createQuery } from '@tanstack/svelte-query';
  import { Check, ExternalLink, KeyRound, Minus, Plus, RefreshCw, Search, Upload, X } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import { writable } from 'svelte/store';
  import AuthenticatedThumbnail from '$lib/components/AuthenticatedThumbnail.svelte';
  import { ApiClient, ApiError } from '$lib/api/client';
  import { authToken } from '$lib/stores/auth';
  import type { FileItem, FileListResponse, Job } from '$lib/api/types';

  const searchDraft = writable('');
  const submittedSearch = writable('');
  const pageLimit = 36;
  const filesQuery = createInfiniteQuery<FileListResponse, Error, { pages: FileListResponse[]; pageParams: string[] }, [string, string], string>(() => ({
    queryKey: ['files', $submittedSearch],
    enabled: Boolean($authToken),
    initialPageParam: '',
    queryFn: ({ pageParam }) =>
      new ApiClient($authToken).listFiles({
        query: $submittedSearch,
        limit: pageLimit,
        pageToken: pageParam || undefined
      }),
    getNextPageParam: (lastPage) => lastPage.next_page_token || undefined
  }));

  let tokenDraft = $state('');
  let loadMoreSentinel = $state<HTMLDivElement | undefined>();
  let viewportHeight = $state(900);
  let viewportWidth = $state(1200);
  let scrollY = $state(0);
  let tagDrafts = $state<Record<string, string>>({});
  let tagBusy = $state<Record<string, boolean>>({});
  let tagErrors = $state<Record<string, string>>({});
  let uploadFiles = $state<File[]>([]);
  let uploadTags = $state('');
  let uploadBusy = $state(false);
  let cancelBusy = $state(false);
  let uploadStatus = $state('');
  let activeUploadJobID = $state('');
  let handledUploadJobID = $state('');
  let activeFile = $state<FileItem | null>(null);
  let previewURL = $state('');
  let previewLoading = $state(false);
  let previewError = $state('');

  const uploadJobQuery = createQuery(() => ({
    queryKey: ['job', activeUploadJobID],
    enabled: Boolean($authToken && activeUploadJobID),
    queryFn: () => new ApiClient($authToken).getJob(activeUploadJobID),
    refetchInterval: 700
  }));

  $effect(() => {
    tokenDraft = $authToken;
  });

  onMount(() => {
    const updateViewport = () => {
      viewportHeight = window.innerHeight;
      viewportWidth = window.innerWidth;
      scrollY = window.scrollY;
    };
    updateViewport();
    window.addEventListener('resize', updateViewport);
    window.addEventListener('scroll', updateViewport, { passive: true });
    return () => {
      window.removeEventListener('resize', updateViewport);
      window.removeEventListener('scroll', updateViewport);
    };
  });

  $effect(() => {
    const node = loadMoreSentinel;
    if (!node || !$authToken) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting) && filesQuery.hasNextPage && !filesQuery.isFetchingNextPage) {
          void filesQuery.fetchNextPage();
        }
      },
      { rootMargin: '900px 0px' }
    );
    observer.observe(node);
    return () => observer.disconnect();
  });

  $effect(() => {
    const job = uploadJobQuery.data;
    if (!job || job.id === handledUploadJobID) return;
    uploadStatus = jobStatusText(job);
    if (isTerminalJob(job)) {
      handledUploadJobID = job.id;
      activeUploadJobID = '';
      if (job.status === 'completed') {
        const result = job.result as { files?: unknown[] } | undefined;
        uploadStatus = `Imported ${result?.files?.length ?? 0} file(s)`;
        uploadFiles = [];
        uploadTags = '';
        void filesQuery.refetch();
      }
    }
  });

  $effect(() => {
    if (uploadJobQuery.isError && activeUploadJobID) {
      uploadStatus = errorMessage(uploadJobQuery.error);
      activeUploadJobID = '';
    }
  });

  $effect(() => {
    const file = activeFile;
    const token = $authToken;
    previewURL = '';
    previewError = '';
    if (!file || !token) {
      previewLoading = false;
      return;
    }

    let objectURL = '';
    let canceled = false;
    const controller = new AbortController();
    previewLoading = true;
    fetch(file.media_urls.preview, {
      headers: { Authorization: `Bearer ${token}` },
      signal: controller.signal
    })
      .then(async (response) => {
        if (!response.ok) throw new Error(`preview request failed: ${response.status}`);
        return response.blob();
      })
      .then((blob) => {
        if (canceled) return;
        objectURL = URL.createObjectURL(blob);
        previewURL = objectURL;
      })
      .catch((error) => {
        if (!controller.signal.aborted) previewError = errorMessage(error);
      })
      .finally(() => {
        if (!controller.signal.aborted) previewLoading = false;
      });

    return () => {
      canceled = true;
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  });

  function saveToken() {
    authToken.set(tokenDraft.trim());
  }

  function submitSearch() {
    submittedSearch.set($searchDraft.trim());
  }

  function formatBytes(size: number) {
    return new Intl.NumberFormat(undefined, {
      notation: size >= 1_000_000 ? 'compact' : 'standard',
      maximumFractionDigits: 1
    }).format(size);
  }

  function errorMessage(error: unknown) {
    if (error instanceof ApiError) return error.message;
    if (error instanceof Error) return error.message;
    return 'The library could not be loaded.';
  }

  function selectedKind(files: FileItem[]) {
    const counts = files.reduce<Record<string, number>>((acc, file) => {
      acc[file.media_kind] = (acc[file.media_kind] ?? 0) + 1;
      return acc;
    }, {});
    return Object.entries(counts)
      .sort((a, b) => b[1] - a[1])
      .map(([kind, count]) => `${kind} ${count}`)
      .join(' / ');
  }

  function visibleFiles() {
    return filesQuery.data?.pages.flatMap((page) => page.files) ?? [];
  }

  function gridColumns() {
    if (viewportWidth >= 1536) return 8;
    if (viewportWidth >= 1280) return 6;
    if (viewportWidth >= 1024) return 4;
    if (viewportWidth >= 640) return 3;
    return 2;
  }

  function virtualGrid(files: FileItem[]) {
    const columns = gridColumns();
    const rowHeight = viewportWidth >= 1024 ? 432 : viewportWidth >= 640 ? 392 : 352;
    const overscanRows = 4;
    const totalRows = Math.ceil(files.length / columns);
    const startRow = Math.max(0, Math.floor((scrollY - 260) / rowHeight) - overscanRows);
    const visibleRows = Math.ceil(viewportHeight / rowHeight) + overscanRows * 2;
    const endRow = Math.min(totalRows, startRow + visibleRows);
    const startIndex = startRow * columns;
    const endIndex = Math.min(files.length, endRow * columns);
    return {
      files: files.slice(startIndex, endIndex),
      totalHeight: totalRows * rowHeight,
      offsetTop: startRow * rowHeight
    };
  }

  function parseTags(value: string) {
    return value
      .split(/[\s,]+/)
      .map((tag) => tag.trim())
      .filter(Boolean);
  }

  function isTerminalJob(job: Job) {
    return job.status === 'completed' || job.status === 'failed' || job.status === 'canceled';
  }

  function jobStatusText(job: Job) {
    if (job.status === 'pending') return 'Queued';
    if (job.status === 'running') return 'Importing';
    if (job.status === 'completed') return 'Completed';
    if (job.status === 'canceled') return 'Canceled';
    return job.error ?? 'Upload failed';
  }

  function openPreview(file: FileItem) {
    activeFile = file;
  }

  function closePreview() {
    activeFile = null;
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && activeFile) closePreview();
  }

  function updateTagDraft(fileID: string, value: string) {
    tagDrafts = { ...tagDrafts, [fileID]: value };
  }

  async function mutateFileTags(file: FileItem, operation: 'add' | 'set' | 'remove') {
    const tags = parseTags(tagDrafts[file.id] ?? '');
    if (!tags.length) return;
    tagBusy = { ...tagBusy, [file.id]: true };
    tagErrors = { ...tagErrors, [file.id]: '' };
    try {
      await new ApiClient($authToken).mutateTags(operation, { file_ids: [file.id], tags });
      tagDrafts = { ...tagDrafts, [file.id]: '' };
      await filesQuery.refetch();
    } catch (error) {
      tagErrors = { ...tagErrors, [file.id]: errorMessage(error) };
    } finally {
      tagBusy = { ...tagBusy, [file.id]: false };
    }
  }

  function selectUploads(files: FileList | null) {
    uploadFiles = files ? Array.from(files) : [];
    uploadStatus = '';
  }

  async function submitUpload() {
    if (!uploadFiles.length || uploadBusy || activeUploadJobID || !$authToken) return;
    uploadBusy = true;
    uploadStatus = 'Uploading';
    try {
      const client = new ApiClient($authToken);
      const response = await client.uploadFiles(uploadFiles, parseTags(uploadTags), true);
      if ('id' in response) {
        handledUploadJobID = '';
        activeUploadJobID = response.id;
        uploadStatus = 'Queued';
      } else {
        uploadStatus = `Imported ${response.files.length} file(s)`;
        uploadFiles = [];
        uploadTags = '';
        await filesQuery.refetch();
      }
    } catch (error) {
      uploadStatus = errorMessage(error);
    } finally {
      uploadBusy = false;
    }
  }

  async function cancelUploadJob() {
    if (!activeUploadJobID || cancelBusy || !$authToken) return;
    cancelBusy = true;
    try {
      const job = await new ApiClient($authToken).cancelJob(activeUploadJobID);
      uploadStatus = jobStatusText(job);
      if (isTerminalJob(job)) {
        handledUploadJobID = job.id;
        activeUploadJobID = '';
      } else {
        await uploadJobQuery.refetch();
      }
    } catch (error) {
      uploadStatus = errorMessage(error);
    } finally {
      cancelBusy = false;
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<svelte:head>
  <title>Gooru Library</title>
</svelte:head>

<main class="min-h-screen text-zinc-100">
  <div class="mx-auto flex min-h-screen w-full max-w-[1500px] flex-col px-4 py-4 sm:px-6 lg:px-8">
    <header class="flex flex-col gap-4 border-b border-white/10 pb-4 lg:flex-row lg:items-end lg:justify-between">
      <div class="min-w-0">
        <p class="text-xs font-semibold uppercase text-emerald-300/80">Gooru</p>
        <h1 class="mt-2 text-3xl font-semibold text-white sm:text-4xl">Library</h1>
      </div>

      <form class="grid gap-3 sm:grid-cols-[minmax(14rem,24rem)_auto]" onsubmit={(event) => { event.preventDefault(); saveToken(); }}>
        <label class="min-w-0">
          <span class="mb-1 block text-xs font-medium uppercase text-zinc-400">Bearer token</span>
          <span class="flex items-center gap-2 rounded-md border border-white/10 bg-black/30 px-3 py-2 shadow-inner shadow-black/20">
            <KeyRound size={16} class="shrink-0 text-zinc-400" />
            <input
              class="min-w-0 flex-1 bg-transparent text-sm text-zinc-100 outline-none placeholder:text-zinc-500"
              bind:value={tokenDraft}
              type="password"
              autocomplete="current-password"
              placeholder="Paste server token"
            />
          </span>
        </label>
        <button class="self-end rounded-md border border-emerald-400/40 bg-emerald-500/15 px-4 py-2 text-sm font-semibold text-emerald-100 transition hover:bg-emerald-500/25" type="submit">
          Save
        </button>
      </form>
    </header>

    <section class="grid flex-1 gap-4 py-4 lg:grid-cols-[18rem_minmax(0,1fr)]">
      <aside class="border-b border-white/10 pb-4 lg:border-b-0 lg:border-r lg:pr-4">
        <form class="space-y-3" onsubmit={(event) => { event.preventDefault(); submitSearch(); }}>
          <label>
            <span class="mb-1 block text-xs font-medium uppercase text-zinc-400">Search</span>
            <span class="flex items-center gap-2 rounded-md border border-white/10 bg-black/30 px-3 py-2 shadow-inner shadow-black/20">
              <Search size={16} class="shrink-0 text-zinc-400" />
              <input
                class="min-w-0 flex-1 bg-transparent text-sm text-zinc-100 outline-none placeholder:text-zinc-500"
                bind:value={$searchDraft}
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
              onclick={() => filesQuery.refetch()}
            >
              <RefreshCw size={17} />
            </button>
          </div>
        </form>

        <div class="mt-6 space-y-3 text-sm text-zinc-400">
          <div class="rounded-md border border-white/10 bg-white/[0.03] p-3">
            <div class="text-xs uppercase text-zinc-500">Status</div>
            <div class="mt-2 text-zinc-200">{$authToken ? 'Token saved' : 'Token required'}</div>
          </div>
          {#if visibleFiles().length}
            <div class="rounded-md border border-white/10 bg-white/[0.03] p-3">
              <div class="text-xs uppercase text-zinc-500">Kinds</div>
              <div class="mt-2 text-zinc-200">{selectedKind(visibleFiles())}</div>
            </div>
          {/if}
          <form class="rounded-md border border-white/10 bg-white/[0.03] p-3" onsubmit={(event) => { event.preventDefault(); submitUpload(); }}>
            <div class="mb-2 flex items-center gap-2 text-xs uppercase text-zinc-500">
              <Upload size={14} />
              <span>Upload</span>
            </div>
            <input
              class="block w-full text-xs text-zinc-300 file:mr-3 file:rounded file:border-0 file:bg-white/10 file:px-2 file:py-1 file:text-xs file:text-zinc-100"
              type="file"
              multiple
              onchange={(event) => selectUploads(event.currentTarget.files)}
            />
            <input
              class="mt-2 w-full rounded border border-white/10 bg-black/20 px-2 py-1.5 text-xs text-zinc-100 outline-none placeholder:text-zinc-500"
              bind:value={uploadTags}
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
              <div class="mt-2 rounded border border-emerald-400/20 bg-emerald-500/10 p-2 text-xs text-emerald-50" role="status">
                <div class="flex items-center justify-between gap-2">
                  <span class="min-w-0 truncate">{uploadStatus || 'Queued'}</span>
                  <button
                    class="rounded border border-white/10 px-2 py-1 text-[11px] text-zinc-100 transition hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-60"
                    type="button"
                    disabled={cancelBusy}
                    onclick={cancelUploadJob}
                  >
                    {cancelBusy ? 'Canceling' : 'Cancel'}
                  </button>
                </div>
                <div class="mt-1 truncate text-[11px] text-emerald-100/70">{activeUploadJobID}</div>
              </div>
            {/if}
            {#if uploadStatus && !uploadBusy && !activeUploadJobID}
              <p class="mt-2 text-xs text-zinc-300">{uploadStatus}</p>
            {/if}
          </form>
        </div>
      </aside>

      <section class="min-w-0">
        {#if !$authToken}
          <div class="flex min-h-[22rem] items-center justify-center rounded-md border border-dashed border-white/15 bg-white/[0.03] p-8 text-center text-zinc-300">
            <p class="max-w-md text-sm leading-6">Enter the token configured for `gooru serve` to browse this library.</p>
          </div>
        {:else if filesQuery.isLoading}
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
            {#each Array(12) as _}
              <div class="aspect-[4/5] rounded-md border border-white/10 bg-white/[0.05]"></div>
            {/each}
          </div>
        {:else if filesQuery.isError}
          <div class="rounded-md border border-red-400/30 bg-red-500/10 p-4 text-sm text-red-100">
            {errorMessage(filesQuery.error)}
          </div>
        {:else if !visibleFiles().length}
          <div class="flex min-h-[22rem] items-center justify-center rounded-md border border-dashed border-white/15 bg-white/[0.03] p-8 text-center text-zinc-300">
            <p class="max-w-md text-sm leading-6">No files matched this query.</p>
          </div>
        {:else}
          {@const files = visibleFiles()}
          {@const virtual = virtualGrid(files)}
          <div class="mb-3 flex items-center justify-between gap-3 text-sm text-zinc-400">
            <span>{files.length} files loaded</span>
            {#if filesQuery.hasNextPage}
              <span>{filesQuery.isFetchingNextPage ? 'Loading more' : 'Scroll for more results'}</span>
            {/if}
          </div>
          <div class="relative" style={`height: ${virtual.totalHeight}px;`}>
            <div
              class="absolute inset-x-0 top-0 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8"
              data-testid="virtual-media-grid"
              style={`transform: translateY(${virtual.offsetTop}px);`}
            >
	            {#each virtual.files as file (file.id)}
	              <article class="group flex min-h-[21.5rem] flex-col overflow-hidden rounded-md border border-white/10 bg-white/[0.04] transition hover:border-emerald-300/50 hover:bg-white/[0.07] sm:min-h-[24rem] lg:min-h-[26rem]">
	                <button
	                  class="block w-full text-left"
	                  type="button"
	                  aria-label={`Preview ${file.name}`}
	                  onclick={() => openPreview(file)}
	                >
	                  <AuthenticatedThumbnail {file} token={$authToken} size={256} />
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
                  <div class="border-t border-white/10 pt-2">
                    <label class="sr-only" for={`tags-${file.id}`}>Tags for {file.name}</label>
                    <input
                      id={`tags-${file.id}`}
                      class="w-full rounded border border-white/10 bg-black/20 px-2 py-1.5 text-xs text-zinc-100 outline-none placeholder:text-zinc-500 focus:border-emerald-300/60"
                      value={tagDrafts[file.id] ?? ''}
                      placeholder="tag:value"
                      oninput={(event) => updateTagDraft(file.id, event.currentTarget.value)}
                    />
                    <div class="mt-2 grid grid-cols-3 gap-1">
                      <button
                        class="flex h-8 items-center justify-center rounded border border-white/10 bg-white/10 text-zinc-100 transition hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-50"
                        type="button"
                        title="Add tags"
                        aria-label={`Add tags to ${file.name}`}
                        disabled={!parseTags(tagDrafts[file.id] ?? '').length || tagBusy[file.id]}
                        onclick={() => mutateFileTags(file, 'add')}
                      >
                        <Plus size={15} />
                      </button>
                      <button
                        class="flex h-8 items-center justify-center rounded border border-white/10 bg-white/10 text-zinc-100 transition hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-50"
                        type="button"
                        title="Set tags"
                        aria-label={`Set tags on ${file.name}`}
                        disabled={!parseTags(tagDrafts[file.id] ?? '').length || tagBusy[file.id]}
                        onclick={() => mutateFileTags(file, 'set')}
                      >
                        <Check size={15} />
                      </button>
                      <button
                        class="flex h-8 items-center justify-center rounded border border-white/10 bg-white/10 text-zinc-100 transition hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-50"
                        type="button"
                        title="Remove tags"
                        aria-label={`Remove tags from ${file.name}`}
                        disabled={!parseTags(tagDrafts[file.id] ?? '').length || tagBusy[file.id]}
                        onclick={() => mutateFileTags(file, 'remove')}
                      >
                        <Minus size={15} />
                      </button>
                    </div>
                    {#if tagErrors[file.id]}
                      <p class="mt-2 text-xs text-red-200">{tagErrors[file.id]}</p>
                    {/if}
                  </div>
                </div>
              </article>
            {/each}
            </div>
          </div>
          {#if filesQuery.hasNextPage || filesQuery.isFetchingNextPage}
            <div bind:this={loadMoreSentinel} class="mt-4 flex min-h-12 items-center justify-center text-sm text-zinc-400" data-testid="infinite-scroll-sentinel">
              {filesQuery.isFetchingNextPage ? 'Loading more results' : 'More results available'}
            </div>
          {/if}
        {/if}
	      </section>
	    </section>
	  </div>
    {#if activeFile}
      <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-3 backdrop-blur-sm sm:p-6" role="presentation">
        <button class="absolute inset-0 cursor-default" type="button" aria-label="Close preview" onclick={closePreview}></button>
        <div
	        class="relative z-10 flex max-h-[92vh] w-full max-w-6xl flex-col overflow-hidden rounded-md border border-white/10 bg-zinc-950 shadow-2xl shadow-black/60"
	        role="dialog"
	        aria-modal="true"
	        aria-labelledby="preview-title"
	      >
	        <header class="flex items-center justify-between gap-3 border-b border-white/10 px-4 py-3">
	          <div class="min-w-0">
	            <h2 id="preview-title" class="truncate text-base font-semibold text-white">{activeFile.name}</h2>
	            <p class="mt-1 truncate text-xs text-zinc-400">{activeFile.media_kind} / {formatBytes(activeFile.size)}</p>
	          </div>
	          <div class="flex shrink-0 items-center gap-2">
            <a
              class="inline-flex h-9 w-9 items-center justify-center rounded border border-white/10 text-zinc-200 transition hover:bg-white/10"
              href={activeFile.media_urls.content}
              target="_blank"
              rel="noreferrer"
              title="Open original"
              aria-label={`Open original ${activeFile.name}`}
            >
              <ExternalLink size={16} />
            </a>
	            <button
	              class="inline-flex h-9 w-9 items-center justify-center rounded border border-white/10 text-zinc-200 transition hover:bg-white/10"
	              type="button"
	              title="Close preview"
	              aria-label="Close preview"
	              onclick={closePreview}
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
              <img class="max-h-[72vh] max-w-full object-contain" src={previewURL} alt={activeFile.name} />
            {/if}
	          </div>
	          <aside class="overflow-y-auto border-t border-white/10 p-4 lg:border-l lg:border-t-0">
	            <div class="space-y-4 text-sm">
	              <div>
	                <div class="text-xs uppercase text-zinc-500">Content id</div>
	                <div class="mt-1 break-all font-mono text-xs text-zinc-300">{activeFile.content_id}</div>
	              </div>
	              <div class="grid grid-cols-2 gap-3">
	                <div>
	                  <div class="text-xs uppercase text-zinc-500">Type</div>
	                  <div class="mt-1 text-zinc-200">{activeFile.media_type}</div>
	                </div>
	                <div>
	                  <div class="text-xs uppercase text-zinc-500">Modified</div>
	                  <div class="mt-1 text-zinc-200">{new Date(activeFile.modified_time).toLocaleDateString()}</div>
	                </div>
	              </div>
	              <div>
	                <div class="text-xs uppercase text-zinc-500">Tags</div>
	                <div class="mt-2 flex flex-wrap gap-1">
	                  {#each activeFile.tags as tag}
	                    <span class="rounded border border-white/10 bg-white/[0.05] px-1.5 py-0.5 text-xs text-zinc-300">{tag}</span>
	                  {/each}
	                </div>
	              </div>
	            </div>
	          </aside>
	        </div>
	      </div>
	    </div>
	  {/if}
	</main>
