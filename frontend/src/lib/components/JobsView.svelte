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
      <p>Completed jobs remain visible for 1 hour.</p>
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
</style>
