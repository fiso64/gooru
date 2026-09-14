<script lang="ts">
  import CancelActiveJobsButton from './CancelActiveJobsButton.svelte';
  import ClearCompletedJobsButton from './ClearCompletedJobsButton.svelte';
  import JobRow from './JobRow.svelte';
  import PageNav from './PageNav.svelte';
  import { listMaintenanceJobs, runMaintenanceJob, type MaintenanceJob } from '$lib/api/operations';
  import { authState } from '$lib/stores/auth';
  import { createJobsQuery } from '$lib/queries/jobs';
  import type { Job } from '$lib/api/types';
  import { errorMessage } from '$lib/utils/format';

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

  let maintenanceJobs = $state<MaintenanceJob[]>([]);
  let selectedMaintenanceJobID = $state('');
  let maintenanceLoading = $state(false);
  let maintenanceRunning = $state(false);
  let maintenanceMessage = $state('');
  let maintenanceError = $state('');

  $effect(() => {
    const user = $authState.user;
    const scope = authScope;
    if (!user) {
      maintenanceJobs = [];
      selectedMaintenanceJobID = '';
      maintenanceLoading = false;
      maintenanceMessage = '';
      maintenanceError = '';
      return;
    }

    let canceled = false;
    maintenanceLoading = true;
    maintenanceMessage = '';
    maintenanceError = '';
    void listMaintenanceJobs()
      .then((result) => {
        if (canceled) return;
        maintenanceJobs = result.items;
        if (!result.items.some((job) => job.id === selectedMaintenanceJobID)) {
          selectedMaintenanceJobID = result.items[0]?.id ?? '';
        }
      })
      .catch((cause) => {
        if (canceled) return;
        maintenanceJobs = [];
        selectedMaintenanceJobID = '';
        maintenanceError = errorMessage(cause);
      })
      .finally(() => {
        if (!canceled) maintenanceLoading = false;
      });

    void scope;
    return () => {
      canceled = true;
    };
  });

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

  async function runSelectedMaintenanceJob() {
    if (!selectedMaintenanceJobID || maintenanceRunning) return;
    maintenanceRunning = true;
    maintenanceMessage = '';
    maintenanceError = '';
    try {
      const result = await runMaintenanceJob(selectedMaintenanceJobID, $authState.csrfToken);
      maintenanceMessage = result.created
        ? `${result.job.name} queued.`
        : `${result.job.name} has no pending work or is already active.`;
      resetPagination();
      await pageQuery.refetch();
    } catch (cause) {
      maintenanceError = errorMessage(cause);
    } finally {
      maintenanceRunning = false;
    }
  }
</script>

<main class="main">
  <div class="page jobs-page">
    <div class="page-header jobs-page-header">
      <div class="g-eyebrow g-eyebrow-accent">Jobs</div>
      <div class="jobs-title-row">
        <h1>Background work</h1>
        <div class="jobs-page-actions">
          <div class="maintenance-actions">
            <select
              class="maintenance-select"
              aria-label="Maintenance job"
              bind:value={selectedMaintenanceJobID}
              disabled={maintenanceLoading || maintenanceRunning || maintenanceJobs.length === 0}
            >
              {#if maintenanceLoading}
                <option value="">Loading maintenance jobs…</option>
              {:else if maintenanceJobs.length === 0}
                <option value="">No maintenance jobs</option>
              {:else}
                {#each maintenanceJobs as job (job.id)}
                  <option value={job.id}>{job.name}</option>
                {/each}
              {/if}
            </select>
            <button
              class="g-btn g-btn-sm"
              type="button"
              onclick={runSelectedMaintenanceJob}
              disabled={maintenanceLoading || maintenanceRunning || !selectedMaintenanceJobID}
            >
              {maintenanceRunning ? 'Running…' : 'Run'}
            </button>
          </div>
          <CancelActiveJobsButton />
          <ClearCompletedJobsButton onCleared={resetPagination} />
        </div>
      </div>
      {#if maintenanceMessage || maintenanceError}
        <div class:maintenance-error={Boolean(maintenanceError)} class="maintenance-status" aria-live="polite">
          {maintenanceError || maintenanceMessage}
        </div>
      {/if}
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

  .jobs-page-actions,
  .maintenance-actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .maintenance-select {
    min-width: 190px;
    max-width: 250px;
    height: 30px;
    padding: 0 28px 0 9px;
    border: 1px solid var(--border-2);
    border-radius: var(--radius-sm);
    background: var(--surface-1);
    color: var(--text-1);
    font: inherit;
    font-size: 12px;
  }

  .maintenance-status {
    margin-top: 8px;
    color: var(--text-3);
    font-size: 11px;
    text-align: right;
  }

  .maintenance-error {
    color: var(--danger);
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

    .maintenance-status {
      text-align: left;
    }
  }

  @media (max-width: 460px) {
    .maintenance-actions {
      width: 100%;
    }

    .maintenance-select {
      min-width: 0;
      flex: 1;
    }
  }
</style>
