<script lang="ts">
  import CancelActiveJobsButton from './CancelActiveJobsButton.svelte';
  import ClearCompletedJobsButton from './ClearCompletedJobsButton.svelte';
  import JobRow from './JobRow.svelte';
  import PageNav from './PageNav.svelte';
  import RunMaintenanceJobMenu from './RunMaintenanceJobMenu.svelte';
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

  const pageSize = 50;
  let pageIndex = $state(0);
  const pageToken = $derived(pageIndex === 0 ? '' : String(pageIndex * pageSize));
  const pageQuery = createJobsQuery(
    () => Boolean($authState.user),
    () => authScope,
    () => pageSize,
    () => pageToken
  );
  const pageJobs = $derived(pageQuery.data?.items ?? (pageIndex === 0 ? jobs : []));
  // While a new page query is loading, preserve only the minimum count needed to
  // keep the selected page valid. Reusing previous query data here could expose
  // rows across an auth-scope change.
  const totalCount = $derived(pageQuery.data?.total_count ?? Math.max(pageJobs.length, pageIndex * pageSize + 1));
  const pageCount = $derived(Math.max(1, Math.ceil(totalCount / pageSize)));

  $effect(() => {
    if (pageIndex >= pageCount) pageIndex = Math.max(0, pageCount - 1);
  });

  function selectPage(page: number) {
    const targetIndex = page - 1;
    if (targetIndex < 0 || targetIndex >= pageCount || targetIndex === pageIndex || pageQuery.isFetching) return;
    pageIndex = targetIndex;
  }

  function resetPagination() {
    pageIndex = 0;
  }

  async function handleMaintenanceStarted() {
    resetPagination();
    await pageQuery.refetch();
  }
</script>

<main class="main">
  <div class="page jobs-page">
    <div class="page-header jobs-page-header">
      <div class="g-eyebrow g-eyebrow-accent">Jobs</div>
      <div class="jobs-title-row">
        <h1>Background work</h1>
        <div class="jobs-page-actions">
          <RunMaintenanceJobMenu {jobs} {authScope} onStarted={handleMaintenanceStarted} />
          <CancelActiveJobsButton />
          <ClearCompletedJobsButton onCleared={resetPagination} />
        </div>
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
    width: min(100%, 820px);
    margin-inline: auto;
  }

  .jobs-page-header {
    position: relative;
    max-width: none;
    text-align: left;
    display: block;
  }

  .jobs-title-row {
    display: flex;
    align-items: center;
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

  @media (max-width: 760px) {
    .jobs-title-row {
      align-items: flex-start;
      flex-direction: column;
    }

    .jobs-page-actions {
      flex-wrap: wrap;
    }
  }
</style>
