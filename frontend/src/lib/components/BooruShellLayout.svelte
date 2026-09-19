<script lang="ts">
  import type { Snippet } from 'svelte';
  import SearchBar from './SearchBar.svelte';
  import ShellFilterSidebar from './ShellFilterSidebar.svelte';
  import { shellTopNavigationItems, type ShellLayoutActions, type ShellLayoutModel } from './shellModel';

  let { model, actions, children } = $props<{
    model: ShellLayoutModel;
    actions: ShellLayoutActions;
    children: Snippet;
  }>();
</script>

<div class="app-shell booru-app-shell">
  <header class="booru-header">
    <div class="booru-brand-row">
      <button class="booru-brand" type="button" onclick={() => actions.onNavigate('library')} aria-label="Gooru library">
        <span class="booru-brand-mark" aria-hidden="true"></span>
        <span>Gooru</span>
      </button>
      <div class="booru-account-actions">
        <button
          class="booru-link-button booru-jobs-button"
          type="button"
          title="Jobs drawer"
          aria-label="Jobs drawer"
          aria-expanded={model.jobsDrawerOpen}
          aria-controls="jobs-drawer"
          onclick={actions.onJobs}
        >
          Jobs{#if model.jobsActiveCount > 0} ({model.jobsActiveCount}){/if}
        </button>
        <button class="booru-link-button" type="button" title={model.username} onclick={() => actions.onNavigate('settings')}>{model.username}</button>
      </div>
    </div>

    <nav class="booru-main-nav" aria-label="Primary navigation">
      {#each shellTopNavigationItems as item}
        <button
          data-shell-shortcut
          class:current={model.route === item.route}
          class="booru-nav-tab"
          type="button"
          aria-current={model.route === item.route ? 'page' : undefined}
          onclick={() => actions.onNavigate(item.route)}
        >
          {item.label}
        </button>
      {/each}
    </nav>

    <nav class="booru-subnav" aria-label="Library utilities">
      <button class:current={model.route === 'library'} class="booru-subnav-item" type="button" onclick={() => actions.onNavigate('library')}>Listing</button>
      <button class="booru-subnav-item" type="button" onclick={actions.onCreateSavedSearch}>Save search</button>
      <button class="booru-subnav-item" type="button" onclick={actions.onOpenShortcuts}>Shortcuts</button>
    </nav>
  </header>

  <aside class="sidebar booru-sidebar">
    <section class="booru-search-section" aria-label="Search">
      <h2>Search</h2>
      <form onsubmit={(event) => event.preventDefault()}>
        <SearchBar
          value={model.search}
          suggestions={model.suggestions}
          metaTags={model.metaTags}
          tags={[]}
          onDraftInput={actions.onSearchDraft}
          onCommit={actions.onSearchCommit}
          presentation="text"
          placeholder=""
          showShortcutHint={false}
          enableSlashShortcut={false}
        />
      </form>
    </section>

    <ShellFilterSidebar model={model} actions={actions} />
  </aside>

  {@render children()}
</div>
