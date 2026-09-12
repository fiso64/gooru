<script lang="ts">
  import ClearCompletedJobsButton from './ClearCompletedJobsButton.svelte';
  import Icon from './Icon.svelte';
  import JobRow from './JobRow.svelte';
  import type { Job } from '$lib/api/types';

  let {
    jobs,
    onClose,
    onCancel
  } = $props<{
    jobs: Job[];
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
  onkeydown={(event) => { if (event.key === 'Escape') onClose(); }}
  onclick={handleWindowClick}
/>

<div bind:this={drawerElement} class="jobs-drawer" role="dialog" aria-modal="false" aria-labelledby="jobs-drawer-title" tabindex="-1">
  <div class="jobs-drawer-head">
    <h3 id="jobs-drawer-title">Jobs</h3>
    <div class="jobs-drawer-actions">
      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" disabled title="Pause all coming soon" aria-label="Pause all coming soon">
        <Icon name="pause" size={13} />
      </button>
      <ClearCompletedJobsButton variant="icon" />
    </div>
  </div>
  <div class="jobs-list">
    {#each jobs as job (job.id)}
      <JobRow {job} onCancel={onCancel} />
    {:else}
      <div class="jobs-empty">No jobs have been recorded.</div>
    {/each}
  </div>
</div>

<style>
  /* AppShell keeps this host for aria-controls; it must not become an in-flow grid item. */
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

  @media (max-width: 700px) {
    .jobs-drawer {
      left: 14px;
      width: auto;
    }
  }
</style>
