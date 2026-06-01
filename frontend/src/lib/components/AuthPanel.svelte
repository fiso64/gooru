<script lang="ts">
  import Logo from './Logo.svelte';

  let {
    checked,
    loginUsername,
    loginPassword,
    loginBusy,
    loginError,
    onUsernameInput,
    onPasswordInput,
    onLogin
  } = $props<{
    checked: boolean;
    username: string;
    loginUsername: string;
    loginPassword: string;
    loginBusy: boolean;
    loginError: string;
    onUsernameInput: (value: string) => void;
    onPasswordInput: (value: string) => void;
    onLogin: () => void;
    onLogout: () => void;
  }>();
</script>

<div class="login-v2">
  <div class="login-v2-card">
    <div class="login-v2-id">
      <div class="login-v2-mark"><Logo size={16} /></div>
      <div class="login-v2-id-meta">
        <span>v0.4.2</span>
        <span class="sep">.</span>
        <span>{checked ? 'session' : 'checking'}</span>
        <span class="sep">.</span>
        <span>gpl-3.0</span>
      </div>
    </div>

    <div class="login-v2-status">
      <div class="login-v2-status-head">
        <span class="dot {checked ? 'warn' : ''}"></span>
        <b>{checked ? 'authentication required' : 'checking session'}</b>
        <span class="mono">same-origin cookies</span>
      </div>
      <dl class="login-v2-status-grid">
        <dt>csrf</dt><dd>{checked ? 'ready' : 'pending'}</dd>
        <dt>mode</dt><dd>cookie</dd>
        <dt>upload</dt><dd>enabled after sign in</dd>
        <dt>tags</dt><dd>protected mutation</dd>
      </dl>
    </div>

    <form class="login-v2-form" onsubmit={(event) => { event.preventDefault(); onLogin(); }}>
      <div class="login-v2-field">
        <label for="lg-user">user</label>
        <input
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
        <div class="login-v2-error"><span class="prompt">!</span><span>{loginError}</span></div>
      {/if}
      <button class="g-btn g-btn-primary auth-submit" type="submit" disabled={loginBusy} aria-label="Sign in">
        {loginBusy ? 'authenticating...' : 'sign in'}
      </button>
    </form>

    <div class="login-v2-foot">
      <div><span class="mono">first run?</span><span> on the server: </span><code>gooru auth init</code></div>
      <div class="login-v2-foot-links">
        <a href="/docs" tabindex="-1">docs</a>
        <span class="sep">.</span>
        <a href="/source" tabindex="-1">source</a>
        <span class="sep">.</span>
        <a href="/changelog" tabindex="-1">changelog</a>
      </div>
    </div>
  </div>
  <div class="login-v2-bg" aria-hidden="true"></div>
</div>
