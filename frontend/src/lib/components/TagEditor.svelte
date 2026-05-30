<script lang="ts">
  import { Check, Minus, Plus } from '@lucide/svelte';

  let {
    fileID,
    fileName,
    draft,
    busy,
    error,
    canSubmit,
    onInput,
    onMutate
  } = $props<{
    fileID: string;
    fileName: string;
    draft: string;
    busy: boolean;
    error: string;
    canSubmit: boolean;
    onInput: (value: string) => void;
    onMutate: (operation: 'add' | 'set' | 'remove') => void;
  }>();
</script>

<div class="border-t border-white/10 pt-2">
  <label class="sr-only" for={`tags-${fileID}`}>Tags for {fileName}</label>
  <input
    id={`tags-${fileID}`}
    class="w-full rounded border border-white/10 bg-black/20 px-2 py-1.5 text-xs text-zinc-100 outline-none placeholder:text-zinc-500 focus:border-emerald-300/60"
    value={draft}
    placeholder="tag:value"
    oninput={(event) => onInput(event.currentTarget.value)}
  />
  <div class="mt-2 grid grid-cols-3 gap-1">
    <button
      class="flex h-8 items-center justify-center rounded border border-white/10 bg-white/10 text-zinc-100 transition hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-50"
      type="button"
      title="Add tags"
      aria-label={`Add tags to ${fileName}`}
      disabled={!canSubmit || busy}
      onclick={() => onMutate('add')}
    >
      <Plus size={15} />
    </button>
    <button
      class="flex h-8 items-center justify-center rounded border border-white/10 bg-white/10 text-zinc-100 transition hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-50"
      type="button"
      title="Set tags"
      aria-label={`Set tags on ${fileName}`}
      disabled={!canSubmit || busy}
      onclick={() => onMutate('set')}
    >
      <Check size={15} />
    </button>
    <button
      class="flex h-8 items-center justify-center rounded border border-white/10 bg-white/10 text-zinc-100 transition hover:bg-white/15 disabled:cursor-not-allowed disabled:opacity-50"
      type="button"
      title="Remove tags"
      aria-label={`Remove tags from ${fileName}`}
      disabled={!canSubmit || busy}
      onclick={() => onMutate('remove')}
    >
      <Minus size={15} />
    </button>
  </div>
  {#if error}
    <p class="mt-2 text-xs text-red-200">{error}</p>
  {/if}
</div>
