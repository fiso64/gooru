<script lang="ts">
  import Logo from './Logo.svelte';
</script>

<div class="session-loading" role="status" aria-live="polite" aria-label="Loading library session">
  <div class="session-loading-status">
    <span class="session-loading-mark"><Logo size={16} /></span>
    <span>Loading library…</span>
  </div>
</div>

<style>
  .session-loading {
    min-height: 100vh;
    display: grid;
    place-items: center;
    background: var(--bg);
    color: var(--text-3);
  }

  .session-loading-status {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    font-family: var(--font-mono);
    font-size: 11px;
    letter-spacing: 0.04em;
  }

  .session-loading-mark {
    display: grid;
    place-items: center;
    width: 30px;
    height: 30px;
    border: 1px solid var(--border);
    border-radius: var(--r-3);
    background: var(--surface);
    color: var(--accent);
  }
</style>
