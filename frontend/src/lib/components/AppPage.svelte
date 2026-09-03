<script lang="ts">
  import { onMount } from 'svelte';
  import AuthenticatedApp from './AuthenticatedApp.svelte';
  import AuthPanel from './AuthPanel.svelte';
  import SessionLoading from './SessionLoading.svelte';
  import { ApiClient } from '$lib/api/client';
  import { authState } from '$lib/stores/auth';
  import { defaultGridSize, runtimeConfig } from '$lib/stores/runtimeConfig';
  import { errorMessage } from '$lib/utils/format';
  import { accentTheme, type AccentTheme } from '$lib/utils/theme';

  type FontStyle = 'editorial' | 'modern' | 'comic';
  type UIConfig = { accent_color?: string; font_style?: FontStyle; load_full_media_by_default?: boolean; grid_size?: number };

  let loginUsername = $state('');
  let loginPassword = $state('');
  let loginBusy = $state(false);
  let loginError = $state('');
  let runtimeAccent = $state<AccentTheme | null>(null);
  let runtimeFontStyle = $state<FontStyle>('editorial');
  let runtimeGridSize = $state(defaultGridSize);
  let faviconHref = $state('/favicon.svg');

  async function applyRuntimeConfig(config: UIConfig) {
    runtimeAccent = accentTheme(config.accent_color ?? '');
    runtimeFontStyle = config.font_style ?? 'editorial';
    runtimeGridSize = config.grid_size ?? defaultGridSize;
    runtimeConfig.set({ loadFullMediaByDefault: config.load_full_media_by_default ?? false, gridSize: runtimeGridSize });
    faviconHref = '/favicon.svg';
    if (!runtimeAccent) return;

    try {
      const response = await fetch('/favicon.svg', { credentials: 'same-origin' });
      if (!response.ok) return;
      const svg = (await response.text()).replaceAll('#ffd060', runtimeAccent.accent);
      faviconHref = `data:image/svg+xml,${encodeURIComponent(svg)}`;
    } catch {
      faviconHref = '/favicon.svg';
    }
  }

  onMount(() => {
    const configPromise = fetch('/api/v1/ui-config', { credentials: 'same-origin' })
      .then(async (response) => {
        if (!response.ok) return { accent_color: '', font_style: 'editorial', load_full_media_by_default: false } satisfies UIConfig;
        return await response.json() as UIConfig;
      })
      .catch(() => ({ accent_color: '', font_style: 'editorial', load_full_media_by_default: false }) satisfies UIConfig);
    const sessionPromise = new ApiClient().me();

    void Promise.all([configPromise, sessionPromise])
      .then(async ([config, session]) => {
        await applyRuntimeConfig(config);
        authState.set({ user: session.user, csrfToken: session.csrf_token ?? '', checked: true });
      })
      .catch(async () => {
        const config = await configPromise;
        await applyRuntimeConfig(config);
        authState.set({ user: null, csrfToken: '', checked: true });
      });
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
  <link rel="icon" href={faviconHref} type="image/svg+xml" />
</svelte:head>

<div
  class={`gooru-root gooru-accent-sodium gooru-type-${runtimeFontStyle}`}
  style={`--grid-cell:${runtimeGridSize}px;${runtimeAccent ? `--accent:${runtimeAccent.accent};--accent-ink:${runtimeAccent.accentInk}` : ''}`}
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
