<script lang="ts">
  import CancelActiveJobsButton from './CancelActiveJobsButton.svelte';
  import ClearCompletedJobsButton from './ClearCompletedJobsButton.svelte';
  import JobRow from './JobRow.svelte';
  import PageNav from './PageNav.svelte';
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
  const pageCount = $derived(Math.max(pageTokens.length, pageIndex + 1 + (hasNextPage ? 1 : 0)));

  function selectPage(page: number) {
    const targetIndex = page - 1;
    if (targetIndex < 0 || targetIndex === pageIndex || pageQuery.isFetching) return;
    if (targetIndex < pageTokens.length) {
      pageIndex = targetIndex;
      return;
    }
    const next = pageQuery.data?.next_page_token;
    if (targetIndex === pageIndex + 1 && next) {
      pageTokens = [...pageTokens.slice(0, pageIndex + 1), next];
      pageIndex = targetIndex;
    }
  }

  function resetPagination() {
    pageTokens = [''];
    pageIndex = 0;
  }
</script>

<main class="main">
  <div class="page jobs-page">
    <div class="page-header jobs-page-header">
      <div>
        <div class="g-eyebrow g-eyebrow-accent">Jobs</div>
        <h1>Background work</h1>
      </div>
      <div class="jobs-page-actions">
        <CancelActiveJobsButton />
        <ClearCompletedJobsButton onCleared={resetPagination} />
      </div>
    </div>

    {#if pageCount > 1}
      <div class="jobs-top-pager">
        <PageNav
          page={pageIndex + 1}
          {pageCount}
          onPage={selectPage}
          disabled={pageQuery.isFetching}
          ariaLabel="Jobs pages"
          testId="jobs-pages-top"
        />
      </div>
    {/if}

    <div class="g-card jobs-card" aria-busy={pageQuery.isFetching}>
      {#each pageJobs as job (job.id)}
        <JobRow {job} onCancel={onCancel} />
      {:else}
        <div class="jobs-empty">No background operations have been recorded.</div>
      {/each}
    </div>

    {#if pageCount > 1}
      <PageNav
        page={pageIndex + 1}
        {pageCount}
        onPage={selectPage}
        disabled={pageQuery.isFetching}
        ariaLabel="Jobs pages"
        testId="jobs-pages-bottom"
      />
    {/if}
  </div>
</main>

<style>
  .jobs-page {
    width: min(100%, 720px);
    margin-inline: 0;
  }

  .jobs-page-header {
    position: relative;
    max-width: none;
    text-align: left;
    display: flex;
    flex-direction: row;
    align-items: flex-end;
    justify-content: space-between;
    gap: 16px;
  }

  .jobs-page-header h1 {
    margin-bottom: 0;
    white-space: nowrap;
  }

  .jobs-page-actions {
    display: flex;
    gap: 8px;
    align-items: center;
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

  :global(.jobs-top-pager .page-nav) {
    margin-top: 0;
    padding: 0 0 14px;
  }

  @media (max-width: 600px) {
    .jobs-page-header {
      align-items: flex-start;
      flex-direction: column;
    }

    .jobs-page-actions {
      flex-wrap: wrap;
    }
  }
</style>
