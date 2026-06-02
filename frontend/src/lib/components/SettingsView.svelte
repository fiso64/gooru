<script lang="ts">
  import Icon from './Icon.svelte';

  let {
    username,
    onLogout
  } = $props<{
    username: string;
    onLogout: () => void;
  }>();

  const initial = $derived((username || 'g').slice(0, 1).toUpperCase());
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Settings</div>
      <h1>Server &amp; library settings</h1>
      <p>Configuration is read from the running Gooru server. Settings without backend support are shown disabled until the API exists.</p>
    </div>

    <section class="settings-section">
      <div class="settings-section-head">
        <h2>Account</h2>
        <p>Your sign-in identity for this server.</p>
      </div>
      <div class="settings-section-body">
        <div class="field-row">
          <span>Signed in as</span>
          <div class="field-control account-control">
            <span class="settings-avatar">{initial}</span>
            <span class="settings-user">{username || 'guest'}</span>
            <button class="g-btn g-btn-sm" type="button" onclick={onLogout}><Icon name="logout" size={13} /> Sign out</button>
          </div>
        </div>
        <div class="field-row is-disabled">
          <span>Password</span>
          <div class="field-control inline-control">
            <input class="g-input" type="password" value="••••••••••" readonly disabled />
            <button class="g-btn" type="button" disabled>Change…</button>
          </div>
        </div>
        <p class="field-help">Password changes are coming soon.</p>
      </div>
    </section>

    <section class="settings-section is-disabled">
      <div class="settings-section-head">
        <h2>Library</h2>
        <p>Server-managed import and scan behavior.</p>
      </div>
      <div class="settings-section-body">
        <div class="field-row">
          <span>Scan roots</span>
          <div class="field-control"><input class="g-input" value="configured in gooru.yaml" readonly disabled /></div>
        </div>
        <div class="field-row">
          <span>Auto rescan</span>
          <div class="field-control">
            <div class="seg" aria-disabled="true">
              <button type="button" class="is-active" disabled>Manual</button>
              <button type="button" disabled>Watch</button>
            </div>
          </div>
        </div>
        <p class="field-help">Editable library settings require a future server settings API.</p>
      </div>
    </section>

    <section class="settings-section is-disabled">
      <div class="settings-section-head">
        <h2>Appearance</h2>
        <p>Concept controls reserved for persisted preferences.</p>
      </div>
      <div class="settings-section-body">
        <div class="field-row">
          <span>Accent</span>
          <div class="field-control">
            <div class="seg" aria-disabled="true">
              <button type="button" class="is-active" disabled>Sodium</button>
              <button type="button" disabled>Phosphor</button>
              <button type="button" disabled>Cyan</button>
            </div>
          </div>
        </div>
        <div class="field-row">
          <span>Grid density</span>
          <div class="field-control"><input class="g-input" value="180 px" readonly disabled /></div>
        </div>
        <p class="field-help">Appearance persistence is coming soon.</p>
      </div>
    </section>
  </div>
</main>
