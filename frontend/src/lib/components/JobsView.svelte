<script lang="ts">
  import JobRow from './JobRow.svelte';
  import type { Job } from '$lib/api/types';

  let {
    jobs,
    onCancel,
    onClearCompleted
  } = $props<{
    jobs: Job[];
    onCancel: (job: Job) => void;
    onClearCompleted: () => void;
  }>();

  const hasCompleted = $derived(jobs.some((job: Job) => job.status === 'completed' || job.status === 'failed' || job.status === 'canceled'));
</script>

<main class="main">
  <div class="page jobs-page">
    <div class="page-header jobs-page-header">
      <div class="g-eyebrow g-eyebrow-accent">Jobs</div>
      <h1>Background work</h1>
      <p>Thumbnailing, imports, bulk tag edits. Cancel anything that's still running. Completed jobs are kept for 1 hour.</p>
      {#if hasCompleted}
        <button class="g-btn g-btn-ghost g-btn-sm jobs-clear" type="button" onclick={onClearCompleted}>Clear completed</button>
      {/if}
    </div>

    <div class="g-card jobs-card">
      {#each jobs as job (job.id)}
        <JobRow {job} onCancel={onCancel} />
      {:else}
        <div class="jobs-empty">No jobs have been recorded.</div>
      {/each}
    </div>
  </div>
</main>

<style>
  .jobs-page {
    max-width: none;
    margin: 0;
  }

  .jobs-page-header {
    position: relative;
    max-width: 72ch;
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
  }

  .jobs-empty {
    padding: 36px 18px;
    color: var(--text-3);
    text-align: center;
    font-family: var(--font-mono);
    font-size: 11px;
  }
</style>
