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

<div class="mt-2 rounded border border-emerald-400/20 bg-emerald-500/10 p-2 text-xs text-emerald-50" role="status">
  <div class="flex items-center justify-between gap-2">
    <span class="min-w-0 truncate">{status || 'Queued'}</span>
    <button
      class="rounded border border-white/10 px-2 py-1 text-[11px] text-zinc-100 transition hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-60"
      type="button"
      disabled={cancelBusy}
      onclick={onCancel}
    >
      {cancelBusy ? 'Canceling' : 'Cancel'}
    </button>
  </div>
  <div class="mt-1 truncate text-[11px] text-emerald-100/70">{jobID}</div>
</div>
