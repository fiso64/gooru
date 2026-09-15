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
  let maintenanceLoading = $state(false);
  let maintenanceMenuOpen = $state(false);
  let maintenanceStartingIDs = $state<string[]>([]);
  let maintenanceError = $state('');

  $effect(() => {
    const user = $authState.user;
    const scope = authScope;
    const visibleJobs = jobs;
    if (!user) {
      maintenanceJobs = [];
      maintenanceLoading = false;
      maintenanceMenuOpen = false;
      maintenanceStartingIDs = [];
      maintenanceError = '';
      return;
    }

    let canceled = false;
    maintenanceLoading = true;
    maintenanceError = '';
    void listMaintenanceJobs()
      .then((result) => {
        if (canceled) return;
        maintenanceJobs = result.items;
      })
      .catch((cause) => {
        if (canceled) return;
        maintenanceJobs = [];
        maintenanceError = errorMessage(cause);
      })
      .finally(() => {
        if (!canceled) maintenanceLoading = false;
      });

    void scope;
    void visibleJobs;
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

  function maintenanceJobBusy(job: MaintenanceJob) {
    return job.running || maintenanceStartingIDs.includes(job.id);
  }

  async function runMaintenance(job: MaintenanceJob) {
    if (maintenanceJobBusy(job)) return;
    maintenanceMenuOpen = false;
    maintenanceError = '';
    maintenanceStartingIDs = [...maintenanceStartingIDs, job.id];
    try {
      const result = await runMaintenanceJob(job.id, $authState.csrfToken);
      maintenanceJobs = maintenanceJobs.map((item) => item.id === result.job.id ? result.job : item);
      resetPagination();
      await pageQuery.refetch();
      const catalog = await listMaintenanceJobs();
      maintenanceJobs = catalog.items;
    } catch (cause) {
      maintenanceError = errorMessage(cause);
    } finally {
      maintenanceStartingIDs = maintenanceStartingIDs.filter((id) => id !== job.id);
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
          <details class="maintenance-menu" bind:open={maintenanceMenuOpen}>
            <summary
              class="g-btn g-btn-sm"
              role="button"
              aria-haspopup="menu"
              aria-expanded={maintenanceMenuOpen}
            >Run job</summary>
            <div class="maintenance-menu-popover" aria-label="Runnable maintenance jobs">
              {#if maintenanceLoading}
                <div class="maintenance-menu-empty">Loading…</div>
              {:else if maintenanceJobs.length === 0}
                <div class="maintenance-menu-empty">No runnable jobs</div>
              {:else}
                {#each maintenanceJobs as job (job.id)}
                  <button
                    class="maintenance-menu-item"
                    type="button"
                    disabled={maintenanceJobBusy(job)}
                    title={job.description}
                    onclick={() => runMaintenance(job)}
                  >
                    <span>{job.name}</span>
                    {#if maintenanceJobBusy(job)}<span class="maintenance-job-state">Running</span>{/if}
                  </button>
                {/each}
              {/if}
            </div>
          </details>
          <CancelActiveJobsButton />
          <ClearCompletedJobsButton onCleared={resetPagination} />
        </div>
      </div>
      {#if maintenanceError}
        <div class="maintenance-error" role="alert">{maintenanceError}</div>
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

  .jobs-page-actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .maintenance-menu {
    position: relative;
  }

  .maintenance-menu > summary {
    list-style: none;
    cursor: pointer;
    user-select: none;
  }

  .maintenance-menu > summary::-webkit-details-marker {
    display: none;
  }

  .maintenance-menu > summary::after {
    content: '▾';
    margin-left: 6px;
    font-size: 9px;
  }

  .maintenance-menu-popover {
    position: absolute;
    z-index: 20;
    top: calc(100% + 6px);
    right: 0;
    min-width: 240px;
    padding: 5px;
    border: 1px solid var(--border);
    border-radius: var(--r-2);
    background: var(--surface);
    box-shadow: 0 10px 30px color-mix(in srgb, black 18%, transparent);
  }

  .maintenance-menu-item {
    width: 100%;
    min-height: 34px;
    padding: 7px 9px;
    border: 0;
    border-radius: var(--r-2);
    background: transparent;
    color: var(--text);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    font: inherit;
    font-size: 12px;
    text-align: left;
    cursor: pointer;
  }

  .maintenance-menu-item:hover:not(:disabled),
  .maintenance-menu-item:focus-visible:not(:disabled) {
    background: var(--surface-2);
  }

  .maintenance-menu-item:disabled {
    color: var(--text-3);
    cursor: default;
  }

  .maintenance-job-state,
  .maintenance-menu-empty {
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 10.5px;
  }

  .maintenance-job-state {
    flex: 0 0 auto;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .maintenance-menu-empty {
    padding: 8px 9px;
    white-space: nowrap;
  }

  .maintenance-error {
    margin-top: 8px;
    color: var(--danger);
    font-size: 11px;
    text-align: right;
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

    .maintenance-menu-popover {
      right: auto;
      left: 0;
    }

    .maintenance-error {
      text-align: left;
    }
  }
</style>
