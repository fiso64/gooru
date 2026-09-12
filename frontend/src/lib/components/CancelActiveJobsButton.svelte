<script lang="ts">
  import ActionDialog from './ActionDialog.svelte';
  import Icon from './Icon.svelte';
  import { authState } from '$lib/stores/auth';
  import { createCancelActiveJobsMutation } from '$lib/queries/jobs';
  import { errorMessage } from '$lib/utils/format';
  import { useQueryClient } from '@tanstack/svelte-query';

  let { variant = 'button' } = $props<{ variant?: 'button' | 'icon' }>();
  const queryClient = useQueryClient();
  const cancelMutation = createCancelActiveJobsMutation(() => $authState.csrfToken, queryClient);
  let open = $state(false);
  let busy = $state(false);
  let error = $state('');

  function showDialog() { error = ''; open = true; }
  function closeDialog() { if (!busy) { open = false; error = ''; } }
  async function confirmCancel() {
    if (busy) return;
    busy = true;
    error = '';
    try {
      await cancelMutation.mutateAsync();
      open = false;
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      busy = false;
    }
  }
</script>

{#if variant === 'icon'}
  <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" title="Cancel all active jobs" aria-label="Cancel all active jobs" onclick={showDialog}>
    <Icon name="close" size={13} />
  </button>
{:else}
  <button class="g-btn g-btn-sm" type="button" onclick={showDialog}>
    <Icon name="close" size={13} /> Cancel all
  </button>
{/if}

{#if open}
  <ActionDialog
    title="Cancel all active jobs"
    description="Cancel every running and queued background job. Completed job history will not be affected."
    confirmText="Cancel jobs"
    destructive
    {busy}
    {error}
    input={false}
    onCancel={closeDialog}
    onConfirm={confirmCancel}
  />
{/if}
