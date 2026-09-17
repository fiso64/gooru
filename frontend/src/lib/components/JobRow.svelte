<script lang="ts">
  import Icon from './Icon.svelte';
  import type { Job } from '$lib/api/types';
  import { jobAffectedCount, jobFailedCount } from '$lib/jobs';

  let {
    job,
    onCancel
  } = $props<{
    job: Job;
    onCancel?: (job: Job) => void;
  }>();

  const percent = $derived(job.progress === undefined ? undefined : Math.round(Math.max(0, Math.min(1, job.progress)) * 100));
  const cancellable = $derived(job.status === 'pending' || job.status === 'running');
  const visualStatus = $derived(
    job.status === 'canceled'
      ? 'canceled'
      : job.status === 'failed' || job.outcome === 'error'
        ? 'error'
        : job.status === 'completed' && job.outcome === 'partial_success'
          ? 'partial'
          : job.status === 'completed'
            ? 'done'
            : job.status === 'pending'
              ? 'queued'
              : job.status
  );
  const affectedCount = $derived(jobAffectedCount(job));
  const failedCount = $derived(jobFailedCount(job));

  function titleFor(job: Job) {
    const type = job.type;
    if (type === 'upload_import') return job.stage === 'receiving' ? 'Upload media' : 'Import media';
    if (type.includes('tag')) return 'Tag edit';
    if (type.includes('thumbnail')) return 'Generate thumbnails';
    return type
      .split('_')
      .filter(Boolean)
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(' ');
  }

  function iconFor(type: string) {
    if (type.includes('upload') || type.includes('import')) return 'upload';
    if (type.includes('tag')) return 'tag';
    return 'photo';
  }

  function timeLabel(value?: string) {
    if (!value) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString('en-GB', {
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit'
    });
  }

  const detail = $derived(job.error || timeLabel(job.started_at || job.submitted_at));
</script>

<div class="job-row">
  <div class="job-row-head">
    <span class="name">
      <Icon name={iconFor(job.type)} size={14} />
      <b>{titleFor(job)}</b>
    </span>
    <span class="job-row-actions">
      {#if cancellable && onCancel}
        <button class="job-cancel" type="button" aria-label={`Cancel ${titleFor(job)}`} onclick={() => onCancel?.(job)}>Cancel</button>
      {/if}
      <span class={`status ${visualStatus}`}>{visualStatus}</span>
    </span>
  </div>
  {#if percent !== undefined}
    <div class={`job-progress ${visualStatus}`} aria-label={`${percent}% complete`}>
      <div style={`width: ${percent}%`}></div>
    </div>
  {/if}
  <div class="job-meta">
    {#if percent !== undefined}<span>{percent}%</span>{/if}
    {#if affectedCount !== undefined}<span>{affectedCount.toLocaleString()} file{affectedCount === 1 ? '' : 's'}</span>{/if}
    {#if failedCount !== undefined && failedCount > 0}<span>{failedCount.toLocaleString()} failed</span>{/if}
    {#if detail}<span class="job-detail">{detail}</span>{/if}
  </div>
</div>

<style>
  .job-row {
    position: relative;
    width: 100%;
    box-sizing: border-box;
    padding: 14px 16px;
    border-top: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 7px;
    text-align: left;
    transition: background 0.12s ease;
  }

  .job-row:first-child {
    border-top: 0;
  }

  .job-row:hover,
  .job-row:focus-within {
    background: color-mix(in srgb, var(--surface-2) 58%, transparent);
  }

  .job-row-head {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 10px;
    box-sizing: border-box;
    font-size: 12.5px;
  }

  .name {
    min-width: 0;
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 8px;
  }

  .name b {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
  }

  .job-row-actions {
    margin-left: auto;
    flex: 0 0 auto;
    display: inline-flex;
    align-items: center;
    gap: 8px;
  }

  .status {
    flex: 0 0 auto;
    font-family: var(--font-mono);
    font-size: 10.5px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--text-3);
  }

  .status.running { color: var(--info, oklch(0.72 0.15 250)); }
  .status.done { color: var(--ok); }
  .status.partial { color: var(--accent); }
  .status.error { color: var(--danger); }

  .job-progress {
    height: 4px;
  }

  .job-progress.running > div { background: var(--info, oklch(0.72 0.15 250)); }
  .job-progress.done > div { background: var(--ok); }
  .job-progress.partial > div { background: var(--accent); }
  .job-progress.error > div { background: var(--danger); }
  .job-progress.canceled > div { background: var(--text-3); }

  .job-meta {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 10px;
    min-width: 0;
    font-family: var(--font-mono);
    font-size: 10.5px;
    color: var(--text-4);
  }

  .job-detail {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .job-cancel {
    min-height: 24px;
    padding: 2px 7px;
    border: 1px solid color-mix(in srgb, var(--danger) 35%, var(--border));
    border-radius: var(--r-2);
    background: var(--bg-2);
    color: var(--danger);
    font-family: var(--font-mono);
    font-size: 10.5px;
    cursor: pointer;
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.12s, background 0.12s;
  }

  .job-row:hover .job-cancel,
  .job-row:focus-within .job-cancel {
    opacity: 1;
    pointer-events: auto;
  }

  .job-cancel:hover {
    background: var(--danger-soft);
  }
</style>
