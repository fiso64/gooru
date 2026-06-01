<script lang="ts">
  import Icon from './Icon.svelte';
  import { jobStatusText } from '$lib/utils/format';
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
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Jobs</div>
      <h1>Background work</h1>
      <p>Imports and bulk tag changes report progress here. Pause and resume are hidden until supported.</p>
    </div>
    <div class="list-head">
      <span class="g-eyebrow">{jobs.length} jobs</span>
      <button class="g-btn g-btn-sm" type="button" onclick={onClearCompleted}>Clear completed</button>
    </div>
    <div class="g-card jobs-list">
      {#each jobs as job}
        <div class="job-row">
          <div class="job-row-head">
            <span class="name"><Icon name={job.type === 'upload_import' ? 'upload' : 'tag'} size={14} /><b>{job.type}</b></span>
            <span class={`status ${job.status}`}>{job.status}</span>
          </div>
          <div class={`job-progress ${job.status}`}><div style={`width: ${Math.round((job.progress ?? 0) * 100)}%`}></div></div>
          <div class="job-meta">
            <span>{job.id}</span>
            <span>{job.error ?? jobStatusText(job)}</span>
          </div>
          {#if job.status === 'pending' || job.status === 'running'}
            <button class="g-btn g-btn-sm" type="button" onclick={() => onCancel(job)}>Cancel</button>
          {/if}
        </div>
      {:else}
        <div class="empty-row">No jobs have been recorded.</div>
      {/each}
    </div>
  </div>
</main>
