<script lang="ts">
  import { matchesShortcut } from '$lib/utils/keyboard';
  import CancelActiveJobsButton from './CancelActiveJobsButton.svelte';
  import ClearCompletedJobsButton from './ClearCompletedJobsButton.svelte';
  import JobRow from './JobRow.svelte';
  import type { Job } from '$lib/api/types';

  let {
    jobs,
    totalCount,
    onViewAll,
    onClose,
    onCancel
  } = $props<{
    jobs: Job[];
    totalCount: number;
    onViewAll: () => void;
    onClose: () => void;
    onCancel: (job: Job) => void;
  }>();

  let drawerElement: HTMLDivElement | undefined;

  function handleWindowClick(event: MouseEvent) {
    const target = event.target;
    if (!(target instanceof Node)) return;
    if (drawerElement?.contains(target)) return;
    if (target instanceof Element && target.closest('[aria-controls="jobs-drawer"]')) return;
    onClose();
  }
</script>

<svelte:window
  onkeydown={(event) => { if (matchesShortcut(event, 'Escape')) onClose(); }}
  onclick={handleWindowClick}
/>

<div bind:this={drawerElement} class="jobs-drawer" role="dialog" aria-modal="false" aria-labelledby="jobs-drawer-title" tabindex="-1">
  <div class="jobs-drawer-head">
    <h3 id="jobs-drawer-title">Jobs</h3>
    <div class="jobs-drawer-actions">
      <CancelActiveJobsButton variant="icon" />
      <ClearCompletedJobsButton variant="icon" />
    </div>
  </div>
  <div class="jobs-list">
    {#each jobs as job (job.id)}
      <JobRow {job} onCancel={onCancel} />
    {:else}
      <div class="jobs-empty">No jobs have been recorded.</div>
    {/each}
    {#if totalCount > jobs.length}
      <button class="jobs-view-all" type="button" data-testid="jobs-view-all" onclick={onViewAll}>View all</button>
    {/if}
  </div>
</div>

<style>
  :global(#jobs-drawer) {
    display: contents;
  }

  .jobs-drawer {
    position: absolute;
    top: 56px;
    right: 14px;
    width: 360px;
    max-height: calc(100% - 80px);
    background: var(--bg-2);
    border: 1px solid var(--border);
    border-radius: var(--r-4);
    box-shadow: 0 24px 60px rgba(0, 0, 0, 0.5);
    display: flex;
    flex-direction: column;
    z-index: 50;
    overflow: hidden;
  }

  .jobs-drawer-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 14px;
    border-bottom: 1px solid var(--border);
  }

  .jobs-drawer-head h3 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 17px;
    font-weight: 400;
  }

  .jobs-drawer-actions {
    display: flex;
    gap: 4px;
  }

  .jobs-list {
    overflow-y: auto;
    flex: 1;
  }

  .jobs-empty {
    padding: 24px 14px;
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 11px;
    text-align: center;
  }

  .jobs-view-all {
    width: 100%;
    border: 0;
    border-top: 1px solid var(--border);
    padding: 11px 14px;
    background: transparent;
    color: var(--accent);
    font-family: var(--font-mono);
    font-size: 11px;
    cursor: pointer;
    text-align: center;
  }

  .jobs-view-all:hover,
  .jobs-view-all:focus-visible {
    background: var(--bg-3);
  }

  @media (max-width: 700px) {
    .jobs-drawer {
      left: 14px;
      width: auto;
    }
  }
</style>
