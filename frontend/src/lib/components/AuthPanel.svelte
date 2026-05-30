<script lang="ts">
  import { KeyRound } from '@lucide/svelte';

  let {
    tokenDraft,
    onTokenInput,
    onSave
  } = $props<{
    tokenDraft: string;
    onTokenInput: (value: string) => void;
    onSave: () => void;
  }>();
</script>

<form class="grid gap-3 sm:grid-cols-[minmax(14rem,24rem)_auto]" onsubmit={(event) => { event.preventDefault(); onSave(); }}>
  <label class="min-w-0">
    <span class="mb-1 block text-xs font-medium uppercase text-zinc-400">Bearer token</span>
    <span class="flex items-center gap-2 rounded-md border border-white/10 bg-black/30 px-3 py-2 shadow-inner shadow-black/20">
      <KeyRound size={16} class="shrink-0 text-zinc-400" />
      <input
        class="min-w-0 flex-1 bg-transparent text-sm text-zinc-100 outline-none placeholder:text-zinc-500"
        value={tokenDraft}
        oninput={(event) => onTokenInput(event.currentTarget.value)}
        type="password"
        autocomplete="current-password"
        placeholder="Paste server token"
      />
    </span>
  </label>
  <button class="self-end rounded-md border border-emerald-400/40 bg-emerald-500/15 px-4 py-2 text-sm font-semibold text-emerald-100 transition hover:bg-emerald-500/25" type="submit">
    Save
  </button>
</form>
