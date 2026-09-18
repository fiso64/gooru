<script lang="ts">
  import { listMaintenanceJobs, runMaintenanceJob, type MaintenanceJob } from '$lib/api/operations';
  import type { Job } from '$lib/api/types';
  import { authState } from '$lib/stores/auth';
  import { errorMessage } from '$lib/utils/format';

  let {
    jobs,
    authScope,
    onStarted
  } = $props<{
    jobs: Job[];
    authScope: number;
    onStarted: () => Promise<void> | void;
  }>();

  let maintenanceJobs = $state<MaintenanceJob[]>([]);
  let loading = $state(false);
  let menuOpen = $state(false);
  let startingIDs = $state<string[]>([]);
  let error = $state('');
  let refreshSequence = 0;

  // The catalog includes running state, so refresh it whenever the surrounding
  // Jobs view receives a new auth scope or durable-operation snapshot.
  $effect(() => refreshForJobsSnapshot(authScope, jobs));

  function refreshForJobsSnapshot(_scope: number, _jobs: Job[]) {
    if (!$authState.user) {
      refreshSequence += 1;
      maintenanceJobs = [];
      loading = false;
      menuOpen = false;
      startingIDs = [];
      error = '';
      return;
    }
    void refreshCatalog();
  }

  async function refreshCatalog() {
    const sequence = ++refreshSequence;
    loading = true;
    error = '';
    try {
      const result = await listMaintenanceJobs();
      if (sequence === refreshSequence) maintenanceJobs = result.items;
    } catch (cause) {
      if (sequence === refreshSequence) {
        maintenanceJobs = [];
        error = errorMessage(cause);
      }
    } finally {
      if (sequence === refreshSequence) loading = false;
    }
  }

  function jobBusy(job: MaintenanceJob) {
    return job.running || startingIDs.includes(job.id);
  }

  async function start(job: MaintenanceJob) {
    if (jobBusy(job)) return;
    menuOpen = false;
    error = '';
    startingIDs = [...startingIDs, job.id];
    try {
      const result = await runMaintenanceJob(job.id, $authState.csrfToken);
      maintenanceJobs = maintenanceJobs.map((item) => item.id === result.job.id ? result.job : item);
      await onStarted();
      await refreshCatalog();
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      startingIDs = startingIDs.filter((id) => id !== job.id);
    }
  }
</script>

<div class="maintenance-control">
  <details class="maintenance-menu" bind:open={menuOpen}>
    <!-- Explicit role preserves button semantics in Chromium's accessibility tree. -->
    <!-- svelte-ignore a11y_no_redundant_roles -->
    <summary
      class="g-btn g-btn-sm"
      role="button"
      aria-haspopup="menu"
      aria-expanded={menuOpen}
    >Run job</summary>
    <div class="maintenance-menu-popover" aria-label="Runnable maintenance jobs">
      {#if loading}
        <div class="maintenance-menu-empty">Loading…</div>
      {:else if maintenanceJobs.length === 0}
        <div class="maintenance-menu-empty">No runnable jobs</div>
      {:else}
        {#each maintenanceJobs as job (job.id)}
          <button
            class="maintenance-menu-item"
            type="button"
            disabled={jobBusy(job)}
            title={job.description}
            onclick={() => start(job)}
          >
            <span>{job.name}</span>
            {#if jobBusy(job)}<span class="maintenance-job-state">Running</span>{/if}
          </button>
        {/each}
      {/if}
    </div>
  </details>
  {#if error}
    <div class="maintenance-error" role="alert">{error}</div>
  {/if}
</div>

<style>
  .maintenance-control,
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
    position: absolute;
    top: 100%;
    right: 0;
    margin-top: 8px;
    color: var(--danger);
    font-size: 11px;
    text-align: right;
    white-space: nowrap;
  }

  @media (max-width: 760px) {
    .maintenance-menu-popover {
      right: auto;
      left: 0;
    }

    .maintenance-error {
      right: auto;
      left: 0;
      text-align: left;
    }
  }
</style>
