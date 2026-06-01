<script lang="ts">
  let {
    jobID,
    status,
    cancelBusy,
    cancelRequested = false,
    onCancel
  } = $props<{
    jobID: string;
    status: string;
    cancelBusy: boolean;
    cancelRequested?: boolean;
    onCancel: (jobID: string) => void;
  }>();

  let localCancelRequested = $state(false);
  const canceled = $derived(cancelRequested || localCancelRequested);

  function requestCancel() {
    localCancelRequested = true;
    onCancel(jobID);
  }
</script>

<div class="job-inline" role="status">
  <div>
    <strong>{canceled ? 'Canceled' : status || 'Queued'}</strong>
    <span>{jobID}</span>
  </div>
  <button class="g-btn g-btn-sm" type="button" disabled={cancelBusy || canceled} onclick={requestCancel}>
    {cancelBusy ? 'Canceling' : 'Cancel'}
  </button>
</div>
