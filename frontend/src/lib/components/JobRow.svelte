<script lang="ts">
  import Icon from './Icon.svelte';
  import type { Job } from '$lib/api/types';

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
    job.status === 'completed' ? 'done' : job.status === 'failed' ? 'error' : job.status === 'pending' ? 'queued' : job.status
  );

  function titleFor(type: string) {
    if (type === 'upload_import') return 'Import media';
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
      <b>{titleFor(job.type)}</b>
    </span>
    <span class={`status ${visualStatus}`}>{visualStatus}</span>
    {#if cancellable && onCancel}
      <button class="job-cancel" type="button" aria-label={`Cancel ${titleFor(job.type)}`} onclick={() => onCancel?.(job)}>Cancel</button>
    {/if}
  </div>
  {#if percent !== undefined}
    <div class={`job-progress ${visualStatus}`} aria-label={`${percent}% complete`}>
      <div style={`width: ${percent}%`}></div>
    </div>
  {/if}
  <div class="job-meta">
    {#if percent !== undefined}<span>{percent}%</span>{/if}
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
  }

  .job-row:first-child {
    border-top: 0;
  }

  .job-row-head {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 10px;
    padding-right: 58px;
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

  .status {
    flex: 0 0 auto;
    font-family: var(--font-mono);
    font-size: 10.5px;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--text-3);
    transition: opacity 0.12s;
  }

  .status.running { color: var(--accent); }
  .status.done { color: var(--ok); }
  .status.error { color: var(--danger); }

  .job-progress {
    width: 100%;
    height: 4px;
    background: var(--surface-2);
    border-radius: 2px;
    overflow: hidden;
  }

  .job-progress > div {
    height: 100%;
    background: var(--accent);
    transition: width 0.3s ease;
  }

  .job-progress.done > div { background: var(--ok); }
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
    position: absolute;
    top: 9px;
    right: 10px;
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

  .job-row:hover .status,
  .job-row:focus-within .status {
    opacity: 0;
  }

  .job-cancel:hover {
    background: var(--danger-soft);
  }
</style>
