<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { FileImage, KeyRound, RefreshCw, Search } from '@lucide/svelte';
  import { writable } from 'svelte/store';
  import { ApiClient, ApiError } from '$lib/api/client';
  import { authToken } from '$lib/stores/auth';
  import type { FileItem } from '$lib/api/types';

  const searchDraft = writable('');
  const submittedSearch = writable('');
  const pageLimit = 36;
  const filesQuery = createQuery(() => ({
    queryKey: ['files', $submittedSearch],
    enabled: Boolean($authToken),
    queryFn: () => new ApiClient($authToken).listFiles({ query: $submittedSearch, limit: pageLimit })
  }));

  let tokenDraft = $state('');

  $effect(() => {
    tokenDraft = $authToken;
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
</script>

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
          {#if filesQuery.data?.files?.length}
            <div class="rounded-md border border-white/10 bg-white/[0.03] p-3">
              <div class="text-xs uppercase text-zinc-500">Kinds</div>
              <div class="mt-2 text-zinc-200">{selectedKind(filesQuery.data.files)}</div>
            </div>
          {/if}
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
        {:else if !filesQuery.data?.files.length}
          <div class="flex min-h-[22rem] items-center justify-center rounded-md border border-dashed border-white/15 bg-white/[0.03] p-8 text-center text-zinc-300">
            <p class="max-w-md text-sm leading-6">No files matched this query.</p>
          </div>
        {:else}
          <div class="mb-3 flex items-center justify-between gap-3 text-sm text-zinc-400">
            <span>{filesQuery.data.files.length} files loaded</span>
            {#if filesQuery.data.next_page_token}
              <span>More results available</span>
            {/if}
          </div>
          <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8">
            {#each filesQuery.data.files as file (file.id)}
              <article class="group overflow-hidden rounded-md border border-white/10 bg-white/[0.04] transition hover:border-emerald-300/50 hover:bg-white/[0.07]">
                <div class="flex aspect-square items-center justify-center bg-black/30 text-zinc-500">
                  <FileImage size={34} />
                </div>
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
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    </section>
  </div>
</main>
