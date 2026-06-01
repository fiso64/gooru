<script lang="ts">
  import { onMount } from 'svelte';
  import AuthenticatedApp from '$lib/components/AuthenticatedApp.svelte';
  import AuthPanel from '$lib/components/AuthPanel.svelte';
  import { ApiClient } from '$lib/api/client';
  import { authState } from '$lib/stores/auth';
  import { errorMessage } from '$lib/utils/format';

  let loginUsername = $state('');
  let loginPassword = $state('');
  let loginBusy = $state(false);
  let loginError = $state('');

  onMount(() => {
    new ApiClient()
      .me()
      .then((session) => authState.set({ user: session.user, csrfToken: session.csrf_token ?? '', checked: true }))
      .catch(() => authState.set({ user: null, csrfToken: '', checked: true }));
  });

  async function login() {
    if (loginBusy) return;
    loginBusy = true;
    loginError = '';
    try {
      const session = await new ApiClient().login(loginUsername.trim(), loginPassword);
      authState.set({ user: session.user, csrfToken: session.csrf_token ?? '', checked: true });
      loginPassword = '';
    } catch (error) {
      loginError = errorMessage(error);
    } finally {
      loginBusy = false;
    }
  }
</script>

<svelte:head>
  <title>Gooru Library</title>
</svelte:head>

<div class="gooru-root gooru-accent-sodium gooru-type-editorial">
  {#if !$authState.user}
    <AuthPanel
      checked={$authState.checked}
      username=""
      {loginUsername}
      {loginPassword}
      {loginBusy}
      {loginError}
      onUsernameInput={(value) => (loginUsername = value)}
      onPasswordInput={(value) => (loginPassword = value)}
      onLogin={login}
      onLogout={() => undefined}
    />
  {:else}
    <AuthenticatedApp />
  {/if}
</div>
