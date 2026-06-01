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
      <div class="login-v2-mark"><Logo size={76} /></div>
      <div class="login-v2-id-meta">
        <span>{checked ? 'session required' : 'checking session'}</span>
        <span class="sep">.</span>
        <span>same-origin cookies</span>
      </div>
    </div>

    <form class="login-v2-form" onsubmit={(event) => { event.preventDefault(); onLogin(); }}>
      <div class="login-v2-field">
        <label for="lg-user">Username</label>
        <input
          id="lg-user"
          class="g-input"
          value={loginUsername}
          autocomplete="username"
          spellcheck="false"
          autocapitalize="off"
          oninput={(event) => onUsernameInput(event.currentTarget.value)}
        />
      </div>
      <div class="login-v2-field">
        <label for="lg-pass">Password</label>
        <input
          id="lg-pass"
          class="g-input"
          type="password"
          value={loginPassword}
          autocomplete="current-password"
          oninput={(event) => onPasswordInput(event.currentTarget.value)}
        />
      </div>
      {#if loginError}
        <div class="login-v2-error"><span class="prompt">!</span><span>{loginError}</span></div>
      {/if}
      <button class="g-btn g-btn-primary auth-submit" type="submit" disabled={loginBusy}>
        {loginBusy ? 'Authenticating' : 'Sign in'}
      </button>
    </form>

    <div class="login-v2-foot">
      <div><span class="mono">first run?</span> <code>gooru auth init</code></div>
    </div>
  </div>
  <div class="login-v2-bg" aria-hidden="true"></div>
</div>
