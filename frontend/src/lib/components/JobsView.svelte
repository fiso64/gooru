<script lang="ts">
  import JobRow from './JobRow.svelte';
  import { authState } from '$lib/stores/auth';
  import { createJobsQuery } from '$lib/queries/jobs';
  import type { Job } from '$lib/api/types';

  let {
    jobs,
    authScope,
    onCancel
  } = $props<{
    jobs: Job[];
    authScope: number;
    onCancel: (job: Job) => void;
  }>();

  let pageIndex = $state(0);
  let pageTokens = $state(['']);
  const pageToken = $derived(pageTokens[pageIndex] ?? '');
  const pageQuery = createJobsQuery(
    () => Boolean($authState.user),
    () => authScope,
    () => 50,
    () => pageToken
  );
  const pageJobs = $derived(pageQuery.data?.items ?? jobs);
  const hasNextPage = $derived(Boolean(pageQuery.data?.next_page_token));

  function nextPage() {
    const next = pageQuery.data?.next_page_token;
    if (!next) return;
    pageTokens = [...pageTokens.slice(0, pageIndex + 1), next];
    pageIndex += 1;
  }

  function previousPage() {
    if (pageIndex > 0) pageIndex -= 1;
  }
</script>

<main class="main">
  <div class="page jobs-page">
    <div class="page-header jobs-page-header">
      <div class="g-eyebrow g-eyebrow-accent">Jobs</div>
      <h1>Background work</h1>
      <p>Durable operation history remains available across restarts.</p>
    </div>

    <div class="g-card jobs-card" aria-busy={pageQuery.isFetching}>
      {#each pageJobs as job (job.id)}
        <JobRow {job} onCancel={onCancel} />
      {:else}
        <div class="jobs-empty">No background operations have been recorded.</div>
      {/each}
    </div>

    {#if pageIndex > 0 || hasNextPage}
      <nav class="jobs-pager" aria-label="Jobs pages">
        <button class="g-btn g-btn-ghost g-btn-sm" type="button" disabled={pageIndex === 0 || pageQuery.isFetching} onclick={previousPage}>Previous</button>
        <span>Page {pageIndex + 1}</span>
        <button class="g-btn g-btn-ghost g-btn-sm" type="button" disabled={!hasNextPage || pageQuery.isFetching} onclick={nextPage}>Next</button>
      </nav>
    {/if}
  </div>
</main>

<style>
  .jobs-page {
    width: min(100%, 600px);
    margin-inline: 0;
  }

  .jobs-page-header {
    position: relative;
    max-width: 56ch;
    text-align: left;
  }

  .jobs-card {
    overflow: hidden;
    width: 100%;
    text-align: left;
  }

  .jobs-empty {
    padding: 28px 16px;
    color: var(--text-3);
    text-align: left;
    font-family: var(--font-mono);
    font-size: 11px;
  }

  .jobs-pager {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    margin-top: 14px;
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 11px;
  }
</style>
