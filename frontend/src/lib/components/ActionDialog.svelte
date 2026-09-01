<script lang="ts">
  let {
    title,
    description,
    label,
    value,
    confirmText = 'Apply',
    destructive = false,
    busy = false,
    error = '',
    input = true,
    onInput,
    onCancel,
    onConfirm
  } = $props<{
    title: string;
    description: string;
    label?: string;
    value?: string;
    confirmText?: string;
    destructive?: boolean;
    busy?: boolean;
    error?: string;
    input?: boolean;
    onInput?: (value: string) => void;
    onCancel: () => void;
    onConfirm: () => void;
  }>();
</script>

<div class="modal-backdrop" role="presentation">
  <div class="action-dialog" role="dialog" aria-modal="true" aria-labelledby="action-dialog-title">
    <h2 id="action-dialog-title">{title}</h2>
    <p>{description}</p>
    {#if input}
      <label>
        <span>{label}</span>
        <input
          value={value ?? ''}
          disabled={busy}
          oninput={(event) => onInput?.(event.currentTarget.value)}
          onkeydown={(event) => { if (event.key === 'Enter') onConfirm(); }}
        />
      </label>
    {/if}
    {#if error}<div class="dialog-error">{error}</div>{/if}
    <div class="dialog-actions">
      <button class="g-btn g-btn-sm" type="button" disabled={busy} onclick={onCancel}>Cancel</button>
      <button class:danger={destructive} class="g-btn g-btn-sm" type="button" disabled={busy} onclick={onConfirm}>{busy ? 'Working' : confirmText}</button>
    </div>
  </div>
</div>
