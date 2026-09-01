<script lang="ts">
  const groups = [
    {
      name: 'Navigation',
      items: [
        { keys: ['/'], description: 'Focus search', supported: false },
        { keys: ['g', 'l'], description: 'Go to library', supported: false },
        { keys: ['g', 't'], description: 'Go to tags', supported: false },
        { keys: ['g', 's'], description: 'Go to settings', supported: false },
        { keys: ['?'], description: 'Show this cheatsheet', supported: true }
      ]
    },
    {
      name: 'Browsing',
      items: [
        { keys: ['j'], description: 'Next file', supported: true },
        { keys: ['k'], description: 'Previous file', supported: true },
        { keys: ['↵'], description: 'Open lightbox', supported: false },
        { keys: ['Esc'], description: 'Close lightbox / clear selection', supported: true },
        { keys: ['Space'], description: 'Quick preview', supported: false }
      ]
    },
    {
      name: 'Selection',
      items: [
        { keys: ['x'], description: 'Toggle select', supported: false },
        { keys: ['⇧', 'click'], description: 'Range select', supported: false },
        { keys: ['⌘', 'a'], description: 'Select all', supported: false },
        { keys: ['Esc'], description: 'Clear selection', supported: false }
      ]
    },
    {
      name: 'Tagging',
      items: [
        { keys: ['t'], description: 'Edit tags on focused file', supported: false },
        { keys: ['T'], description: 'Bulk tag selection', supported: false },
        { keys: ['f'], description: 'Toggle @favorite', supported: false },
        { keys: ['r'], description: 'Toggle @review', supported: false }
      ]
    }
  ] as const;
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Keyboard</div>
      <h1>Shortcuts</h1>
      <p>Press <span class="g-kbd">?</span> from anywhere to open this cheatsheet.</p>
    </div>

    <div class="shortcut-grid">
      {#each groups as group}
        <section class="shortcut-group">
          <h3>{group.name}</h3>
          {#each group.items as item}
            <div class:shortcut-future={!item.supported} class="shortcut-row" title={item.supported ? undefined : 'Coming soon'}>
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
  </div>
</main>

<style>
  .shortcut-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 24px 40px;
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
    gap: 12px;
    padding: 6px 0;
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
    gap: 4px;
  }

  .shortcut-future {
    opacity: 0.42;
  }
</style>
