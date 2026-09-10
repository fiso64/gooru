<script lang="ts">
  import { onMount } from 'svelte';

  let { onClose } = $props<{ onClose: () => void }>();
  let dialogElement: HTMLDivElement | undefined;

  const shortcutColumns = [
    [
      {
        title: 'Navigation',
        items: [
          ['1–9', 'Switch sidebar section'],
          ['↑ / ↓ / ← / →', 'Move through media'],
          ['Enter', 'Open focused media'],
          ['Space', 'Play media or enter / exit comic'],
          ['Esc', 'Close active overlay'],
          ['/', 'Focus search']
        ]
      },
      {
        title: 'Selection',
        items: [
          ['Shift+click', 'Select a range'],
          ['Ctrl / ⌘ + click', 'Toggle selection'],
          ['Esc', 'Clear selection']
        ]
      }
    ],
    [
      {
        title: 'Viewer',
        items: [
          ['← / →', 'Previous / next media'],
          ['Space', 'Play / pause media or enter / exit comic'],
          ['F', 'Toggle fullscreen'],
          ['Q', 'Toggle original / preview media'],
          ['O', 'Open original in new tab'],
          ['1', 'Fit to window'],
          ['V', 'Cycle fit mode'],
          ['2', 'Actual size'],
          ['L / R', 'Rotate left / right'],
          ['T', 'Add tag'],
          ['U', 'Remove tag'],
          ['D', 'Download original'],
          ['Delete', 'Remove from library'],
          ['Shift+Delete', 'Delete file from disk']
        ]
      }
    ]
  ];

  onMount(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : undefined;
    dialogElement?.focus();
    return () => previous?.focus();
  });
</script>

<svelte:window onkeydown={(event) => { if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); onClose(); } }} />

<div class="shortcuts-overlay" role="presentation" onclick={(event) => { if (event.target === event.currentTarget) onClose(); }}>
  <div bind:this={dialogElement} class="shortcuts-modal" role="dialog" aria-modal="true" aria-labelledby="shortcuts-title" tabindex="-1">
    <header class="shortcuts-head">
      <div>
        <div class="g-eyebrow g-eyebrow-accent">Reference</div>
        <h2 id="shortcuts-title">Shortcuts</h2>
      </div>
      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" aria-label="Close shortcuts" onclick={onClose}>×</button>
    </header>

    <div class="shortcuts-grid">
      {#each shortcutColumns as column}
        <div class="shortcut-column">
          {#each column as group}
            <section class="shortcut-group">
              <h3>{group.title}</h3>
              <dl>
                {#each group.items as item}
                  <div class="shortcut-row">
                    <dt><span class="g-kbd">{item[0]}</span></dt>
                    <dd>{item[1]}</dd>
                  </div>
                {/each}
              </dl>
            </section>
          {/each}
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .shortcuts-overlay {
    position: fixed;
    inset: 0;
    z-index: 130;
    display: grid;
    place-items: center;
    padding: 28px;
    background: color-mix(in srgb, #080b10 34%, transparent);
    backdrop-filter: blur(3px);
  }

  .shortcuts-modal {
    width: min(980px, calc(100vw - 40px));
    max-height: min(760px, calc(100vh - 40px));
    overflow: auto;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius);
    background: var(--panel-bg);
    box-shadow: var(--shadow);
    outline: none;
  }

  .shortcuts-head {
    min-height: 38px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 9px 12px 8px;
    border-bottom: 1px solid var(--border);
  }

  .shortcuts-head h2 {
    margin: 2px 0 0;
    font-family: var(--font-display);
    font-size: 22px;
    font-weight: 500;
    line-height: 1;
  }

  .shortcuts-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0;
  }

  .shortcut-column {
    min-width: 0;
  }

  .shortcut-column + .shortcut-column {
    border-left: 1px solid var(--border);
  }

  .shortcut-group {
    min-width: 0;
    padding: 12px 14px 14px;
  }

  .shortcut-group + .shortcut-group {
    border-top: 1px solid var(--border);
  }

  .shortcut-group h3 {
    margin: 0 0 8px;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.09em;
    text-transform: uppercase;
    color: var(--text-3);
  }

  .shortcut-group dl {
    margin: 0;
    display: grid;
    gap: 5px;
  }

  .shortcut-row {
    display: grid;
    grid-template-columns: minmax(86px, max-content) minmax(0, 1fr);
    align-items: center;
    gap: 9px;
  }

  .shortcut-row dt,
  .shortcut-row dd {
    margin: 0;
  }

  .shortcut-row dd {
    min-width: 0;
    font-size: 12px;
    line-height: 1.2;
    color: var(--text-2);
  }

  :global(.shortcut-row .g-kbd) {
    min-width: 21px;
    min-height: 19px;
    justify-content: center;
    font-size: 9px;
    line-height: 1.05;
    white-space: nowrap;
  }

  @media (max-width: 780px) {
    .shortcuts-overlay {
      padding: 14px;
    }

    .shortcuts-modal {
      width: min(100%, 620px);
      max-height: calc(100vh - 28px);
    }

    .shortcuts-grid {
      grid-template-columns: 1fr;
    }

    .shortcut-column + .shortcut-column {
      border-left: 0;
      border-top: 1px solid var(--border);
    }
  }
</style>
