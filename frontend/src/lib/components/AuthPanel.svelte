<script lang="ts">
  import { LogIn } from '@lucide/svelte';

  let {
    username,
    password,
    busy,
    error,
    onUsernameInput,
    onPasswordInput,
    onLogin
  } = $props<{
    username: string;
    password: string;
    busy: boolean;
    error: string;
    onUsernameInput: (value: string) => void;
    onPasswordInput: (value: string) => void;
    onLogin: () => void;
  }>();
</script>

<form class="grid gap-3 sm:grid-cols-[minmax(10rem,16rem)_minmax(12rem,18rem)_auto]" onsubmit={(event) => { event.preventDefault(); onLogin(); }}>
  <label class="min-w-0">
    <span class="mb-1 block text-xs font-medium uppercase text-zinc-400">Username</span>
    <input
      class="w-full rounded-md border border-white/10 bg-black/30 px-3 py-2 text-sm text-zinc-100 outline-none placeholder:text-zinc-500"
      value={username}
      oninput={(event) => onUsernameInput(event.currentTarget.value)}
      autocomplete="username"
    />
  </label>
  <label class="min-w-0">
    <span class="mb-1 block text-xs font-medium uppercase text-zinc-400">Password</span>
    <input
      class="w-full rounded-md border border-white/10 bg-black/30 px-3 py-2 text-sm text-zinc-100 outline-none placeholder:text-zinc-500"
      value={password}
      oninput={(event) => onPasswordInput(event.currentTarget.value)}
      type="password"
      autocomplete="current-password"
    />
  </label>
  <button class="self-end rounded-md border border-emerald-400/40 bg-emerald-500/15 px-4 py-2 text-sm font-semibold text-emerald-100 transition hover:bg-emerald-500/25 disabled:cursor-not-allowed disabled:opacity-50" type="submit" disabled={busy}>
    <span class="inline-flex items-center gap-2"><LogIn size={16} />{busy ? 'Signing in' : 'Sign in'}</span>
  </button>
</form>
{#if error}
  <div class="mt-2 text-sm text-red-200">{error}</div>
{/if}
