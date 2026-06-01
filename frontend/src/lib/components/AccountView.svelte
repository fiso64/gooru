<script lang="ts">
  import Icon from './Icon.svelte';

  let {
    username,
    onLogout
  } = $props<{
    username: string;
    onLogout: () => void;
  }>();
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Account</div>
      <h1>{username}</h1>
      <p>Session authentication uses same-origin cookies and CSRF-protected mutations.</p>
    </div>
    <button class="g-btn" type="button" onclick={onLogout}><Icon name="logout" size={14} /> Sign out</button>
  </div>
</main>
