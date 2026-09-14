<script lang="ts">
  import { onMount } from 'svelte';
  import AuthenticatedApp from './AuthenticatedApp.svelte';
  import AuthPanel from './AuthPanel.svelte';
  import SessionLoading from './SessionLoading.svelte';
  import { ApiClient } from '$lib/api/client';
  import { setOpaqueURLState, setProtectedReadTransport } from '$lib/api/privacy';
  import { authState } from '$lib/stores/auth';
  import { defaultGridSize, effectiveGridSize, normalizeConfiguredUITheme, normalizeGridType, normalizeItemsPerPage, normalizePaginationMode, normalizeThumbnailSizes, normalizeUITheme, runtimeCapability, runtimeConfig, type ConfiguredUITheme, type GridType, type UITheme } from '$lib/stores/runtimeConfig';
  import { errorMessage } from '$lib/utils/format';
  import { accentTheme, type AccentTheme } from '$lib/utils/theme';
  import type { ViewerConfiguredFitMode } from '$lib/utils/viewer';
  import type { ViewerScaling } from '$lib/state/viewerSessionPreferences';

  type FontStyle = 'editorial' | 'modern' | 'comic';
  type UIConfig = {
    ui_theme?: string;
    accent_color?: string;
    font_style?: FontStyle;
    font_style_configured?: boolean;
    load_full_media_by_default?: boolean;
    fullscreen_media_by_default?: boolean;
    hover_play_videos?: boolean;
    hover_play_gifs?: boolean;
    viewer_fit_mode?: ViewerConfiguredFitMode;
    viewer_actual_size_fit_cap?: boolean;
    viewer_scaling?: ViewerScaling;
    grid_size?: number;
    grid_type?: string;
    thumbnail_sizes?: number[];
    pagination_mode?: string;
    items_per_page?: number;
    protected_mode?: boolean;
    opaque_url_state?: boolean;
    capabilities?: string[];
  };

  let loginUsername = $state('');
  let loginPassword = $state('');
  let loginBusy = $state(false);
  let loginError = $state('');
  let runtimeConfiguredTheme = $state<ConfiguredUITheme>('default');
  let runtimeTheme = $state<UITheme>('default');
  let runtimeAccent = $state<AccentTheme | null>(null);
  let runtimeFontStyle = $state<FontStyle | null>('comic');
  let runtimeGridSize = $state(defaultGridSize);
  let runtimeGridType = $state<GridType>('square');
  let faviconHref = $state('/favicon.svg');

  async function applyRuntimeConfig(config: UIConfig) {
    runtimeConfiguredTheme = normalizeConfiguredUITheme(config.ui_theme);
    runtimeTheme = normalizeUITheme(config.ui_theme);
    const booruStyle = runtimeTheme === 'booru-style';
    runtimeAccent = accentTheme(config.accent_color ?? '');
    runtimeFontStyle = booruStyle && !config.font_style_configured ? null : (config.font_style ?? 'comic');
    runtimeGridSize = config.grid_size ?? defaultGridSize;
    runtimeGridType = normalizeGridType(config.grid_type);
    setProtectedReadTransport(config.protected_mode ?? false);
    setOpaqueURLState(config.opaque_url_state ?? false);
    runtimeConfig.set({
      uiTheme: runtimeTheme,
      capabilities: Array.isArray(config.capabilities) ? config.capabilities : [runtimeCapability.previewImages],
      loadFullMediaByDefault: config.load_full_media_by_default ?? false,
      fullscreenMediaByDefault: config.fullscreen_media_by_default ?? false,
      hoverPlayVideos: config.hover_play_videos ?? false,
      hoverPlayGifs: config.hover_play_gifs ?? true,
      viewerFitMode: config.viewer_fit_mode ?? 'fit_window',
      viewerActualSizeFitCap: config.viewer_actual_size_fit_cap ?? true,
      viewerScaling: config.viewer_scaling ?? 'smooth',
      gridSize: runtimeGridSize,
      gridType: runtimeGridType,
      thumbnailSizes: normalizeThumbnailSizes(config.thumbnail_sizes ?? []),
      paginationMode: normalizePaginationMode(config.pagination_mode),
      itemsPerPage: normalizeItemsPerPage(config.items_per_page)
    });
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
        if (!response.ok) return { accent_color: '', font_style: 'comic', load_full_media_by_default: false, fullscreen_media_by_default: false } satisfies UIConfig;
        return await response.json() as UIConfig;
      })
      .catch(() => ({ accent_color: '', font_style: 'comic', load_full_media_by_default: false, fullscreen_media_by_default: false }) satisfies UIConfig);
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

{@const brandAccentStyle = runtimeTheme === 'booru-style'
  ? `--brand-accent:${runtimeAccent?.accent ?? '#ffd060'};`
  : runtimeAccent
    ? `--brand-accent:${runtimeAccent.accent};`
    : ''}
<div
  class={`gooru-root gooru-theme-${runtimeTheme}${runtimeConfiguredTheme === 'default' ? '' : ` gooru-theme-${runtimeConfiguredTheme}`} gooru-accent-sodium${runtimeFontStyle ? ` gooru-type-${runtimeFontStyle}` : ''}`}
  style={`--grid-cell:${effectiveGridSize(runtimeGridSize, runtimeGridType)}px;${brandAccentStyle}${runtimeTheme !== 'booru-style' && runtimeAccent ? `--accent:${runtimeAccent.accent};--accent-ink:${runtimeAccent.accentInk}` : ''}`}
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
