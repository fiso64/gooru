<script lang="ts">
  import { onMount } from 'svelte';
  import Icon from './Icon.svelte';
  import { claimFocus } from '$lib/utils/focus';

  let { onClose = () => undefined } = $props<{ onClose?: () => void }>();
  let dialogElement = $state<HTMLDivElement | undefined>();

  const navigationGroup = {
    name: 'Navigation',
    items: [
      { keys: ['1–9'], description: 'Open the matching visible sidebar item' },
      { keys: ['/'], description: 'Focus search' },
      { keys: ['f'], description: 'Search filenames' },
      { keys: ['b'], description: 'Save current search' },
      { keys: ['⇧', 'PageUp'], description: 'Previous page' },
      { keys: ['⇧', 'PageDown'], description: 'Next page' },
      { keys: ['?'], description: 'Show shortcuts' }
    ]
  } as const;

  const viewerGroup = {
    name: 'Viewer',
    items: [
      { keys: ['j', '→'], description: 'Next file or comic page' },
      { keys: ['k', '←'], description: 'Previous file or comic page' },
      { keys: ['t'], description: 'Focus tag input in tag mode' },
      { keys: ['u'], description: 'Focus tag input in untag mode' },
      { keys: ['Space'], description: 'Play/pause media or enter/exit comic (also from empty tag input)' },
      { keys: ['q'], description: 'Toggle original / preview media' },
      { keys: ['v'], description: 'Cycle viewer fit mode' },
      { keys: ['s'], description: 'Toggle smooth / nearest-neighbor scaling' },
      { keys: ['1'], description: 'Fit media to window' },
      { keys: ['2'], description: 'Show media at actual size' },
      { keys: ['o'], description: 'Open original in new tab' },
      { keys: ['d'], description: 'Download original' }, 
      { keys: ['Del'], description: 'Remove from library' },
      { keys: ['⇧', 'Del'], description: 'Delete file from disk' },
    ]
  } as const;

  const selectionGroup = {
    name: 'Library actions',
    items: [
      { keys: ['(Ctrl)', 'a'], description: 'Select all files in the current view' },
      { keys: ['d'], description: 'Download selection' },
      { keys: ['t'], description: 'Tag selection' },
      { keys: ['Alt', 'Enter'], description: 'Tag selection' },
      { keys: ['u'], description: 'Untag selection' },
      { keys: ['Del'], description: 'Remove selection from library' },
      { keys: ['⇧', 'Del'], description: 'Delete selection from disk' }
    ]
  } as const;

  const shortcutColumns = [
    [navigationGroup, selectionGroup],
    [viewerGroup]
  ] as const;

  onMount(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : undefined;
    return claimFocus(dialogElement, previous);
  });
</script>

<div class="shortcut-backdrop" role="presentation" onclick={(event) => { if (event.target === event.currentTarget) onClose(); }}>
  <div bind:this={dialogElement} class="shortcut-modal" role="dialog" aria-modal="true" aria-labelledby="shortcut-title" tabindex="-1">
    <div class="shortcut-head">
      <div>
        <div class="g-eyebrow g-eyebrow-accent">Keyboard</div>
        <h2 id="shortcut-title">Shortcuts</h2>
      </div>
      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" aria-label="Close shortcuts" onclick={onClose}>
        <Icon name="close" size={15} />
      </button>
    </div>

    <div class="shortcut-grid">
      {#each shortcutColumns as column}
        <div class="shortcut-column">
          {#each column as group}
            <section class="shortcut-group">
              <h3>{group.name}</h3>
              {#each group.items as item}
                <div class="shortcut-row">
                  <span class="shortcut-description">{item.description}</span>
                  <span class="shortcut-keys">
                    {#each item.keys as key}
                      <span class="g-kbd">{key}</span>
                    {/each}
                  </span>
                </div>
              {/each}
            </section>
          {/each}
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .shortcut-backdrop {
    position: fixed;
    z-index: 1200;
    inset: 0;
    display: grid;
    place-items: center;
    padding: 32px;
    background: rgba(8, 8, 7, 0.72);
    backdrop-filter: blur(10px);
  }

  .shortcut-modal {
    width: min(780px, calc(100vw - 64px));
    max-height: min(760px, calc(100vh - 64px));
    overflow: auto;
    padding: 28px 30px 32px;
    border: 1px solid var(--border-strong);
    border-radius: var(--r-4);
    background: var(--bg-2);
    box-shadow: 0 28px 90px rgba(0, 0, 0, 0.5);
    outline: none;
  }

  .shortcut-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 28px;
  }

  .shortcut-head h2 {
    margin: 5px 0 5px;
    font-family: var(--font-display);
    font-size: 22px;
    font-weight: 400;
  }

  .shortcut-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(250px, 1fr));
    gap: 42px;
  }

  .shortcut-column {
    display: grid;
    align-content: start;
    gap: 28px;
    min-width: 0;
  }

  .shortcut-group h3 {
    margin: 0 0 12px;
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 10.5px;
    letter-spacing: 0.16em;
    text-transform: uppercase;
  }

  .shortcut-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    min-height: 38px;
    padding: 7px 0;
    border-top: 1px solid var(--border);
    font-size: 13px;
  }

  .shortcut-row:first-of-type {
    border-top: 0;
  }

  .shortcut-description {
    color: var(--text-2);
  }

  .shortcut-keys {
    display: flex;
    align-items: center;
    gap: 4px;
    flex: 0 0 auto;
  }

  @media (max-width: 720px) {
    .shortcut-backdrop { padding: 12px; }
    .shortcut-modal {
      width: calc(100vw - 24px);
      max-height: calc(100vh - 24px);
      padding: 22px 18px;
    }
    .shortcut-grid {
      grid-template-columns: 1fr;
      gap: 28px;
    }
  }
</style>
