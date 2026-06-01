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
    onCancel: (jobID: string) => void;
  }>();

  let cancelRequested = $state(false);

  function requestCancel() {
    cancelRequested = true;
    onCancel(jobID);
  }
</script>

<div class="job-inline" role="status">
  <div>
    <strong>{cancelRequested ? 'Canceled' : status || 'Queued'}</strong>
    <span>{jobID}</span>
  </div>
  <button class="g-btn g-btn-sm" type="button" disabled={cancelBusy || cancelRequested} onclick={requestCancel}>
    {cancelBusy ? 'Canceling' : 'Cancel'}
  </button>
</div>
