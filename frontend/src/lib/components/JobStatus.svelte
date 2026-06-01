<script lang="ts">
  let {
    jobID,
    status,
    cancelBusy,
    onCancel
  } = $props<{
    jobID: string;
    status: string;
    cancelBusy: boolean;
    onCancel: () => void;
  }>();
</script>

<div class="job-inline" role="status">
  <div>
    <strong>{status || 'Queued'}</strong>
    <span>{jobID}</span>
  </div>
  <button class="g-btn g-btn-sm" type="button" disabled={cancelBusy} onclick={onCancel}>
    {cancelBusy ? 'Canceling' : 'Cancel'}
  </button>
</div>
