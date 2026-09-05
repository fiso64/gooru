<script lang="ts">
  import JobRow from './JobRow.svelte';
  import { authState } from '$lib/stores/auth';
  import { createJobsQuery } from '$lib/queries/jobs';
  import type { Job } from '$lib/api/types';

  let {
    jobs,
    authScope,
    onCancel,
    onClearCompleted
  } = $props<{
    jobs: Job[];
    authScope: number;
    onCancel: (job: Job) => void;
    onClearCompleted: () => void;
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
  const hasCompleted = $derived(pageJobs.some((job: Job) => job.status === 'completed' || job.status === 'failed' || job.status === 'canceled'));
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

  function clearCompleted() {
    pageTokens = [''];
    pageIndex = 0;
    onClearCompleted();
  }
</script>

<main class="main">
  <div class="page jobs-page">
    <div class="page-header jobs-page-header">
      <div class="g-eyebrow g-eyebrow-accent">Jobs</div>
      <h1>Background work</h1>
      <p>Completed jobs remain visible for 1 hour.</p>
      {#if hasCompleted}
        <button class="g-btn g-btn-ghost g-btn-sm jobs-clear" type="button" onclick={clearCompleted}>Clear completed</button>
      {/if}
    </div>

    <div class="g-card jobs-card" aria-busy={pageQuery.isFetching}>
      {#each pageJobs as job (job.id)}
        <JobRow {job} onCancel={onCancel} />
      {:else}
        <div class="jobs-empty">No jobs have been recorded.</div>
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

  .jobs-clear {
    position: absolute;
    right: 0;
    bottom: 0;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.12s;
  }

  .jobs-page-header:hover .jobs-clear,
  .jobs-page-header:focus-within .jobs-clear {
    opacity: 1;
    pointer-events: auto;
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
