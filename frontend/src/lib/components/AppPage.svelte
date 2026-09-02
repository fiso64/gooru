<script lang="ts">
  import { onMount } from 'svelte';
  import AuthenticatedApp from './AuthenticatedApp.svelte';
  import AuthPanel from './AuthPanel.svelte';
  import SessionLoading from './SessionLoading.svelte';
  import { ApiClient } from '$lib/api/client';
  import { authState } from '$lib/stores/auth';
  import { errorMessage } from '$lib/utils/format';
  import { accentTheme, type AccentTheme } from '$lib/utils/theme';

  let loginUsername = $state('');
  let loginPassword = $state('');
  let loginBusy = $state(false);
  let loginError = $state('');
  let runtimeAccent = $state<AccentTheme | null>(null);

  onMount(() => {
    fetch('/api/v1/ui-config', { credentials: 'same-origin' })
      .then(async (response) => {
        if (!response.ok) return null;
        return await response.json() as { accent_color?: string };
      })
      .then((config) => { runtimeAccent = accentTheme(config?.accent_color ?? ''); })
      .catch(() => { runtimeAccent = null; });

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

<div
  class="gooru-root gooru-accent-sodium gooru-type-editorial"
  style={runtimeAccent ? `--accent:${runtimeAccent.accent};--accent-ink:${runtimeAccent.accentInk}` : undefined}
>
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
