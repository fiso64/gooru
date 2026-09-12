<script lang="ts">
  import ActionDialog from './ActionDialog.svelte';
  import Icon from './Icon.svelte';
  import { authState } from '$lib/stores/auth';
  import { createClearCompletedJobsMutation } from '$lib/queries/jobs';
  import { errorMessage } from '$lib/utils/format';
  import { useQueryClient } from '@tanstack/svelte-query';

  let {
    variant = 'button',
    onCleared
  } = $props<{
    variant?: 'button' | 'icon';
    onCleared?: (cleared: number) => void;
  }>();

  const queryClient = useQueryClient();
  const clearMutation = createClearCompletedJobsMutation(() => $authState.csrfToken, queryClient);
  let open = $state(false);
  let busy = $state(false);
  let error = $state('');

  function showDialog() {
    error = '';
    open = true;
  }

  function closeDialog() {
    if (busy) return;
    open = false;
    error = '';
  }

  async function confirmClear() {
    if (busy) return;
    busy = true;
    error = '';
    try {
      const result = await clearMutation.mutateAsync();
      open = false;
      onCleared?.(result.cleared);
    } catch (cause) {
      error = errorMessage(cause);
    } finally {
      busy = false;
    }
  }
</script>

{#if variant === 'icon'}
  <button
    class="g-btn g-btn-ghost g-btn-sm g-btn-icon"
    type="button"
    title="Clear completed jobs"
    aria-label="Clear completed jobs"
    onclick={showDialog}
  >
    <Icon name="trash" size={13} />
  </button>
{:else}
  <button class="g-btn g-btn-sm" type="button" onclick={showDialog}>
    <Icon name="trash" size={13} />
    Clear completed
  </button>
{/if}

{#if open}
  <ActionDialog
    title="Clear completed jobs"
    description="Remove succeeded, failed, and canceled jobs from history. Running and queued jobs will not be affected."
    confirmText="Clear jobs"
    destructive
    {busy}
    {error}
    input={false}
    onCancel={closeDialog}
    onConfirm={confirmClear}
  />
{/if}
