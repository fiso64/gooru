<script lang="ts">
  import { onMount } from 'svelte';
  import { getBuildInfo, type BuildInfo } from '$lib/api/buildInfo';
  import Logo from './Logo.svelte';

  let {
    loginUsername,
    loginPassword,
    loginBusy,
    loginError,
    onUsernameInput,
    onPasswordInput,
    onLogin
  } = $props<{
    loginUsername: string;
    loginPassword: string;
    loginBusy: boolean;
    loginError: string;
    onUsernameInput: (value: string) => void;
    onPasswordInput: (value: string) => void;
    onLogin: () => void;
  }>();

  let usernameInput: HTMLInputElement;
  let buildInfo: BuildInfo | null = $state(null);

  function versionLabel(): string { return buildInfo ? `v${buildInfo.version}` : 'v…'; }
  function buildRef(): string {
    if (!buildInfo) return 'develop';
    if (!buildInfo.development) return `v${buildInfo.version}`;
    return buildInfo.revision && buildInfo.revision !== 'unknown' ? buildInfo.revision : 'develop';
  }
  function sourceHref(): string {
    if (!buildInfo || !buildInfo.revision || buildInfo.revision === 'unknown') return 'https://github.com/fiso64/gooru';
    return `https://github.com/fiso64/gooru/tree/${buildInfo.revision}`;
  }
  function docsHref(): string { return `https://github.com/fiso64/gooru/tree/${buildRef()}/docs`; }
  function setupHref(): string { return `https://github.com/fiso64/gooru/blob/${buildRef()}/docs/SERVE.md`; }

  onMount(() => {
    usernameInput?.focus();
    void getBuildInfo().then((info) => { buildInfo = info; }).catch(() => {});
  });
</script>

<div class="login-v2">
  <div class="login-v2-card">
    <div class="login-v2-id">
      <div class="login-v2-mark"><Logo size={16} /></div>
      <div class="login-v2-id-meta"><span>{versionLabel()}</span><span>agpl-3.0</span></div>
    </div>

    <form class="login-v2-form" onsubmit={(event) => { event.preventDefault(); onLogin(); }}>
      <div class="login-v2-field">
        <label for="lg-user">user</label>
        <input
          bind:this={usernameInput}
          id="lg-user"
          class="g-input"
          aria-label="Username"
          value={loginUsername}
          autocomplete="username"
          spellcheck="false"
          autocapitalize="off"
          oninput={(event) => onUsernameInput(event.currentTarget.value)}
        />
      </div>
      <div class="login-v2-field">
        <label for="lg-pass">password</label>
        <input
          id="lg-pass"
          class="g-input"
          type="password"
          aria-label="Password"
          value={loginPassword}
          autocomplete="current-password"
          oninput={(event) => onPasswordInput(event.currentTarget.value)}
        />
      </div>
      {#if loginError}
        <div class="login-v2-error" role="alert"><span class="prompt">!</span><span>{loginError}</span></div>
      {/if}
      <button class="g-btn g-btn-primary auth-submit" type="submit" disabled={loginBusy} aria-label="Sign in">
        {loginBusy ? 'authenticating…' : 'sign in'}
      </button>
    </form>

    <div class="login-v2-foot">
      <div class="login-v2-first-run"><span class="mono">first run?</span><span>see the first-time setup</span><a href={setupHref()} target="_blank" rel="noreferrer">docs</a></div>
      <div class="login-v2-foot-links">
        <a href={docsHref()} target="_blank" rel="noreferrer">docs</a>
        <span class="sep">&middot;</span>
        <a href={sourceHref()} target="_blank" rel="noreferrer">source</a>
      </div>
    </div>
  </div>

  <div class="login-v2-bg" aria-hidden="true">
    <svg class="login-v2-spirals" width="100%" height="100%">
      <defs>
        <pattern id="login-spiral-pattern" width="58" height="58" patternUnits="userSpaceOnUse">
          <g transform="translate(29 29) scale(2.64233)">
            <path
              d="M 0.084 1.197 L 0.001 1.232 L -0.087 1.262 L -0.179 1.285 L -0.274 1.301 L -0.373 1.311 L -0.474 1.312 L -0.577 1.306 L -0.681 1.291 L -0.786 1.269 L -0.891 1.238 L -0.995 1.198 L -1.099 1.150 L -1.200 1.092 L -1.298 1.027 L -1.393 0.953 L -1.484 0.870 L -1.569 0.780 L -1.650 0.682 L -1.724 0.576 L -1.791 0.463 L -1.851 0.344 L -1.903 0.218 L -1.946 0.087 L -1.979 -0.048 L -2.004 -0.189 L -2.018 -0.332 L -2.022 -0.479 L -2.015 -0.627 L -1.996 -0.777 L -1.967 -0.928 L -1.927 -1.078 L -1.874 -1.227 L -1.811 -1.373 L -1.736 -1.517 L -1.649 -1.656 L -1.552 -1.791 L -1.443 -1.921 L -1.324 -2.043 L -1.195 -2.159 L -1.057 -2.266 L -0.909 -2.364 L -0.752 -2.452 L -0.588 -2.530 L -0.416 -2.597 L -0.238 -2.652 L -0.054 -2.694 L 0.134 -2.724 L 0.327 -2.741 L 0.522 -2.743 L 0.719 -2.732 L 0.917 -2.706 L 1.115 -2.666 L 1.312 -2.611 L 1.506 -2.542 L 1.698 -2.458 L 1.884 -2.360 L 2.065 -2.248 L 2.240 -2.122 L 2.406 -1.982 L 2.564 -1.829 L 2.713 -1.664 L 2.850 -1.487 L 2.976 -1.299 L 3.090 -1.100 L 3.190 -0.892 L 3.276 -0.675 L 3.347 -0.451 L 3.403 -0.220 L 3.442 0.017 L 3.465 0.258 L 3.471 0.502 L 3.460 0.748 L 3.431 0.995 L 3.384 1.242 L 3.320 1.487 L 3.237 1.729 L 3.137 1.966 L 3.020 2.198 L 2.885 2.423 L 2.733 2.640 L 2.566 2.847 L 2.382 3.044 L 2.184 3.228 L 1.971 3.400 L 1.745 3.558 L 1.506 3.700 L 1.256 3.827 L 0.996 3.936 L 0.726 4.028 L 0.449 4.100 L 0.165 4.154 L -0.124 4.188 L -0.418 4.202 L -0.713 4.195 L -1.010 4.167 L -1.306 4.118 L -1.600 4.048 L -1.891 3.956 L -2.177 3.844 L -2.456 3.711 L -2.727 3.557 L -2.989 3.384 L -3.239 3.192 L -3.477 2.981 L -3.702 2.752 L -3.911 2.506 L -4.104 2.244 L -4.279 1.968 L -4.436 1.678 L -4.572 1.376 L -4.688 1.063 L -4.783 0.740 L -4.855 0.410 L -4.904 0.073 L -4.930 -0.269 L -4.932 -0.614 L -4.909 -0.961 L -4.862 -1.307 L -4.791 -1.652 L -4.695 -1.993 L -4.574 -2.328 L -4.430 -2.656 L -4.261 -2.976 L -4.070 -3.284 L -3.857 -3.581 L -3.621 -3.863 L -3.365 -4.130 L -3.090 -4.380 L -2.795 -4.611 L -2.484 -4.823 L -2.157 -5.013 L -1.815 -5.181 L -1.460 -5.326 L -1.093 -5.446 L -0.717 -5.541 L -0.333 -5.610 L 0.057 -5.652 L 0.451 -5.667 L 0.848 -5.654 L 1.245 -5.614 L 1.640 -5.545 L 2.032 -5.449 L 2.418 -5.324 L 2.797 -5.172 L 3.166 -4.993 L 3.524 -4.788 L 3.868 -4.557 L 4.197 -4.301 L 4.510 -4.022 L 4.803 -3.719 L 5.077 -3.395 L 5.328 -3.051 L 5.556 -2.689 L 5.759 -2.309 L 5.936 -1.915 L 6.086 -1.506 L 6.208 -1.086 L 6.301 -0.657 L 6.364 -0.219 L 6.396 0.223 L 6.398 0.670 L 6.368 1.117 L 6.307 1.564 L 6.214 2.007 L 6.090 2.445 L 5.935 2.876 L 5.750 3.296 L 5.534 3.705 L 5.290 4.100 L 5.017 4.478 L 4.717 4.838 L 4.392 5.179 L 4.041 5.497 L 3.668 5.791 L 3.273 6.060 L 2.859 6.302 L 2.426 6.515 L 1.978 6.699 L 1.516 6.852 L 1.042 6.973 L 0.559 7.060 L 0.068 7.115 L -0.427 7.135 L -0.925 7.120 L -1.423 7.071 L -1.918 6.987 L -2.408 6.867 L -2.891 6.714 L -3.365 6.526 L -3.826 6.305 L -4.273 6.051 L -4.702 5.766 L -5.113 5.449 L -5.502 5.104 L -5.868 4.731 L -6.209 4.331 L -6.522 3.907 L -6.806 3.460 L -7.059 2.993 L -7.280 2.507 L -7.468 2.005 L -7.621 1.488 L -7.738 0.960 L -7.819 0.423 L -7.862 -0.120 L -7.867 -0.668 L -7.834 -1.216 L -7.762 -1.764 L -7.652 -2.307 L -7.504 -2.843 L -7.319 -3.370 L -7.096 -3.885 L -6.837 -4.385 L -6.543 -4.868 L -6.214 -5.331 L -5.853 -5.772 L -5.460 -6.188 L -5.038 -6.577 L -4.588 -6.938 L -4.112 -7.267 L -3.612 -7.564 L -3.091 -7.827 L -2.551 -8.053 L -1.994 -8.242 L -1.423 -8.393 L -0.841 -8.503 L -0.251 -8.574 L 0.346 -8.603 L 0.945 -8.591 L 1.544 -8.537 L 2.140 -8.441 L 2.730 -8.303 L 3.311 -8.124 L 3.881 -7.904 L 4.435 -7.644 L 4.973 -7.345 L 5.490 -7.008 L 5.984 -6.635 L 6.453 -6.227 L 6.894 -5.785"
              fill="none"
              stroke="#ffd060"
              stroke-width="5.2"
              stroke-linecap="round"
              stroke-linejoin="round"
              vector-effect="non-scaling-stroke"
            />
          </g>
        </pattern>
      </defs>
      <rect width="100%" height="100%" fill="url(#login-spiral-pattern)" />
    </svg>
  </div>
</div>

<style>
  .login-v2-first-run a { text-decoration: underline; }

  .login-v2-foot-links a {
    color: var(--text-3);
  }

  .login-v2-bg {
    background: radial-gradient(60% 80% at 50% 40%, oklch(0.20 0.012 80), transparent 70%);
    mask-image: none;
    opacity: 1;
  }

  .login-v2-spirals {
    position: absolute;
    inset: 0;
    display: block;
    width: 100%;
    height: 100%;
    opacity: 0.11;
    -webkit-mask-image: radial-gradient(80% 80% at 50% 45%, black 30%, transparent 85%);
    mask-image: radial-gradient(80% 80% at 50% 45%, black 30%, transparent 85%);
  }
</style>
