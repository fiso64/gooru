<script lang="ts">
  import { matchesShortcut, matchesShortcutModifiers } from '$lib/utils/keyboard';
  import { errorMessage } from '$lib/utils/format';

  let {
    onCancel,
    onChange
  } = $props<{
    onCancel: () => void;
    onChange: (currentPassword: string, newPassword: string) => Promise<void>;
  }>();

  let currentPassword = $state('');
  let newPassword = $state('');
  let confirmPassword = $state('');
  let busy = $state(false);
  let error = $state('');

  function close() {
    if (!busy) onCancel();
  }

  async function submit() {
    if (busy) return;
    error = '';
    if (!currentPassword || !newPassword) {
      error = 'Enter your current and new password.';
      return;
    }
    if (newPassword !== confirmPassword) {
      error = 'New passwords do not match.';
      return;
    }
    if (currentPassword === newPassword) {
      error = 'Choose a different new password.';
      return;
    }

    busy = true;
    try {
      await onChange(currentPassword, newPassword);
      onCancel();
    } catch (cause) {
      error = errorMessage(cause);
      busy = false;
    }
  }

  function isEditableTarget(target: EventTarget | null) {
    return target instanceof Element && Boolean(target.closest('input, textarea, [contenteditable=""], [contenteditable="true"]'));
  }

  function isNativeEnterControl(target: EventTarget | null) {
    return target instanceof Element && Boolean(target.closest('button, a[href], select, summary, [role="button"], [role="link"]'));
  }

  function handleBackdropKeydown(event: KeyboardEvent) {
    if (event.target !== event.currentTarget || !matchesShortcut(event, 'Escape')) return;
    event.preventDefault();
    event.stopPropagation();
    close();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (matchesShortcut(event, 'Escape')) {
      event.preventDefault();
      close();
      return;
    }
    if (!(matchesShortcut(event, 'Enter') || matchesShortcut(event, 'Enter', { ctrl: true }) || matchesShortcut(event, 'Enter', { meta: true }))) return;

    const modifiedSubmit = !matchesShortcutModifiers(event);
    if (modifiedSubmit || (!isEditableTarget(event.target) && !isNativeEnterControl(event.target))) {
      event.preventDefault();
      event.stopImmediatePropagation();
      void submit();
      return;
    }

    if (isEditableTarget(event.target)) event.preventDefault();
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<div
  class="modal-backdrop"
  role="dialog"
  aria-modal="true"
  aria-labelledby="password-dialog-title"
  tabindex="-1"
  onclick={(event) => { if (event.target === event.currentTarget) close(); }}
  onkeydown={handleBackdropKeydown}
>
  <form class="action-dialog password-dialog" onsubmit={(event) => { event.preventDefault(); void submit(); }}>
    <div>
      <h2 id="password-dialog-title">Change password</h2>
      <p>Update the password for the current Gooru account. Your active browser session stays signed in.</p>
    </div>

    <label>
      Current password
      <input name="current-password" type="password" autocomplete="current-password" bind:value={currentPassword} disabled={busy} />
    </label>
    <label>
      New password
      <input name="new-password" type="password" autocomplete="new-password" bind:value={newPassword} disabled={busy} />
    </label>
    <label>
      Confirm new password
      <input name="confirm-password" type="password" autocomplete="new-password" bind:value={confirmPassword} disabled={busy} />
    </label>

    {#if error}<div class="dialog-error" role="alert">{error}</div>{/if}

    <div class="dialog-actions">
      <button class="g-btn" type="button" onclick={close} disabled={busy}>Cancel</button>
      <button class="g-btn g-btn-primary" type="submit" disabled={busy}>{busy ? 'Changing…' : 'Change password'}</button>
    </div>
  </form>
</div>
