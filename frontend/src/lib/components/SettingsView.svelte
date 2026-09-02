<script lang="ts">
  import Icon from './Icon.svelte';
  import PasswordDialog from './PasswordDialog.svelte';

  let {
    username,
    onLogout,
    onChangePassword
  } = $props<{
    username: string;
    onLogout: () => void;
    onChangePassword: (currentPassword: string, newPassword: string) => Promise<void>;
  }>();

  const initial = $derived((username || 'g').slice(0, 1).toUpperCase());
  let passwordDialogOpen = $state(false);
  let passwordNotice = $state('');

  async function changePassword(currentPassword: string, newPassword: string) {
    await onChangePassword(currentPassword, newPassword);
    passwordNotice = 'Password updated.';
  }
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Settings</div>
      <h1>Server &amp; library settings</h1>
      <p>Server-managed values can be changed in <code>gooru.yaml</code>.</p>
    </div>

    <section class="settings-section">
      <div class="settings-section-head">
        <h2>Account</h2>
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
        <div class="field-row">
          <span>Password</span>
          <div class="field-control inline-control">
            <input class="g-input" aria-label="Password" type="password" value="••••••••••" readonly tabindex="-1" />
            <button class="g-btn" type="button" onclick={() => { passwordNotice = ''; passwordDialogOpen = true; }}>Change…</button>
          </div>
        </div>
        {#if passwordNotice}<p class="field-help settings-success" role="status">{passwordNotice}</p>{/if}
      </div>
    </section>

    <section class="settings-section is-disabled" aria-label="Appearance settings unavailable">
      <div class="settings-section-head">
        <h2>Appearance</h2>
      </div>
      <div class="settings-section-body">
        <div class="field-row">
          <span>Accent color</span>
          <div class="field-control settings-accent-control">
            <button class="settings-color-swatch" type="button" disabled aria-label="Accent color unavailable"></button>
            <input class="g-input settings-color-value" value="#F4D976" readonly disabled aria-label="Accent color value" />
            <div class="settings-color-presets" aria-hidden="true">
              {#each ['sodium', 'phosphor', 'coral', 'cyan', 'mono'] as preset}
                <button class={`settings-color-dot ${preset}`} type="button" disabled tabindex="-1" aria-label={`${preset} accent`}></button>
              {/each}
            </div>
          </div>
        </div>
        <div class="field-row">
          <span>Grid density</span>
          <div class="field-control settings-range-control">
            <input type="range" min="50" max="500" value="180" disabled aria-label="Grid density" />
            <span class="g-mono">180 px</span>
          </div>
        </div>
      </div>
    </section>

    <section class="settings-section is-disabled" aria-label="Server settings">
      <div class="settings-section-head">
        <h2>Server</h2>
      </div>
      <div class="settings-section-body">
        <div class="field-row">
          <span>Listen address</span>
          <input class="g-input settings-mono-input" value="Configured in gooru.yaml" readonly disabled aria-label="Listen address" />
        </div>
        <div class="field-row">
          <span>Public URL</span>
          <input class="g-input settings-mono-input" value="Configured in gooru.yaml" readonly disabled aria-label="Public URL" />
        </div>
        <div class="field-row">
          <span>CORS origins</span>
          <input class="g-input settings-mono-input" value="Configured in gooru.yaml" readonly disabled aria-label="CORS origins" />
        </div>
      </div>
    </section>

    <section class="settings-section is-disabled" aria-label="Library settings">
      <div class="settings-section-head">
        <h2>Library</h2>
      </div>
      <div class="settings-section-body">
        <div class="field-row">
          <span>Upload roots</span>
          <div class="field-control settings-server-row">
            <Icon name="folder" size={14} />
            <span>Configured in gooru.yaml</span>
          </div>
        </div>
        <div class="field-row">
          <span>Max upload</span>
          <div class="field-control settings-range-control">
            <input type="range" min="1" max="20" value="5" disabled aria-label="Maximum upload" />
            <span class="g-mono">Configured</span>
          </div>
        </div>
      </div>
    </section>

    <section class="settings-section is-disabled" aria-label="Media processing settings">
      <div class="settings-section-head">
        <h2>Media processing</h2>
      </div>
      <div class="settings-section-body">
        <div class="field-row">
          <span>libvips</span>
          <div class="field-control settings-runtime-status"><span class="settings-status-dot"></span><span>Unknown</span></div>
        </div>
        <div class="field-row">
          <span>ffmpeg</span>
          <div class="field-control settings-runtime-status"><span class="settings-status-dot"></span><span>Unknown</span></div>
        </div>
        <div class="field-row">
          <span>Thumbnail sizes</span>
          <input class="g-input settings-mono-input" value="Configured in gooru.yaml" readonly disabled aria-label="Thumbnail sizes" />
        </div>
        <div class="field-row">
          <span>Format</span>
          <div class="field-control">
            <div class="seg" aria-disabled="true">
              <button type="button" class="is-active" disabled>JPEG</button>
              <button type="button" disabled>WebP</button>
              <button type="button" disabled>AVIF</button>
            </div>
          </div>
        </div>
      </div>
    </section>
  </div>
</main>

{#if passwordDialogOpen}
  <PasswordDialog onCancel={() => (passwordDialogOpen = false)} onChange={changePassword} />
{/if}
