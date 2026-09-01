<script lang="ts">
  import { onMount } from 'svelte';
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

  onMount(() => {
    usernameInput?.focus();
  });
</script>

<div class="login-v2">
  <div class="login-v2-card">
    <div class="login-v2-id">
      <div class="login-v2-mark"><Logo size={16} /></div>
      <div class="login-v2-id-meta">
        <span>server</span>
        <span class="sep">&middot;</span>
        <span>ready</span>
        <span class="sep">&middot;</span>
        <span>gpl-3.0</span>
      </div>
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
      <div><span class="mono">first run?</span><span> on the server: </span><code>gooru user create-admin</code></div>
      <div class="login-v2-foot-links">
        <span class="login-v2-link-placeholder" aria-disabled="true">docs</span>
        <span class="sep">&middot;</span>
        <span class="login-v2-link-placeholder" aria-disabled="true">source</span>
        <span class="sep">&middot;</span>
        <span class="login-v2-link-placeholder" aria-disabled="true">changelog</span>
      </div>
    </div>
  </div>
  <div class="login-v2-bg" aria-hidden="true"></div>
</div>

<style>
  .login-v2-link-placeholder {
    color: var(--text-3);
  }
</style>
