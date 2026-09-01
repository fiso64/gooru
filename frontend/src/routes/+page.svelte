<script lang="ts">
  import { onMount } from 'svelte';
  import AuthenticatedApp from '$lib/components/AuthenticatedApp.svelte';
  import AuthPanel from '$lib/components/AuthPanel.svelte';
  import SessionLoading from '$lib/components/SessionLoading.svelte';
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
    const username = loginUsername.trim();
    if (!username || !loginPassword) {
      loginError = 'username and password required.';
      return;
    }

    loginBusy = true;
    loginError = '';
    try {
      const session = await new ApiClient().login(username, loginPassword);
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
  {#if !$authState.checked}
    <SessionLoading />
  {:else if !$authState.user}
    <AuthPanel
      {loginUsername}
      {loginPassword}
      {loginBusy}
      {loginError}
      onUsernameInput={(value) => (loginUsername = value)}
      onPasswordInput={(value) => (loginPassword = value)}
      onLogin={login}
    />
  {:else}
    <AuthenticatedApp />
  {/if}
</div>
