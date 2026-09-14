<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from './Icon.svelte';
  import Logo from './Logo.svelte';
  import SearchBar from './SearchBar.svelte';
  import ShellFilterSidebar from './ShellFilterSidebar.svelte';
  import {
    shellPrimaryNavigationItems,
    shellSettingsNavigationItem,
    type ShellLayoutActions,
    type ShellLayoutModel
  } from './shellModel';

  let { model, actions, children } = $props<{
    model: ShellLayoutModel;
    actions: ShellLayoutActions;
    children: Snippet;
  }>();
</script>

<div class="app-shell default-app-shell">
  <header class="topbar">
    <button class="topbar-brand" type="button" onclick={() => actions.onNavigate('library')} aria-label="Gooru library">
      <span class="topbar-brand-mark"><Logo size={17} /></span>
    </button>
    <form class="topbar-search" onsubmit={(event) => event.preventDefault()}>
      <div class="topbar-search-inner">
        <SearchBar
          value={model.search}
          suggestions={model.suggestions}
          metaTags={model.metaTags}
          tags={model.tags}
          onDraftInput={actions.onSearchDraft}
          onCommit={actions.onSearchCommit}
        />
      </div>
    </form>
    <div class="topbar-right">
      <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" title="Jobs" aria-label="Jobs" aria-expanded={model.jobsDrawerOpen} aria-controls="jobs-drawer" onclick={actions.onJobs}>
        <Icon name="jobs" size={16} />
        {#if model.jobsActiveCount > 0}<span class="topbar-badge">{model.jobsActiveCount}</span>{/if}
      </button>
      <button class="g-btn g-btn-ghost g-btn-sm" type="button" title={model.username} onclick={() => actions.onNavigate('settings')}>
        <Icon name="user" size={14} />
        <span>{model.username}</span>
      </button>
    </div>
  </header>

  <aside class="sidebar">
    <div class="sidebar-section">
      {#each shellPrimaryNavigationItems as item}
        <button
          data-shell-shortcut
          class:active={model.route === item.route}
          class="sidebar-item"
          type="button"
          onclick={() => actions.onNavigate(item.route)}
        >
          <Icon name={item.icon} size={16} active={model.route === item.route} />
          <span>{item.label}</span>
          {#if item.route === 'library'}
            <span class="count">{model.libraryCount.toLocaleString()}</span>
          {:else if item.route === 'tags'}
            <span class="count">{model.tagCount.toLocaleString()}</span>
          {:else if item.route === 'jobs' && model.jobsActiveCount}
            <span class="count">{model.jobsActiveCount}</span>
          {/if}
        </button>
      {/each}
    </div>

    <ShellFilterSidebar model={model} actions={actions} />

    <div class="sidebar-section bottom">
      <button
        data-shell-shortcut
        class:active={model.route === shellSettingsNavigationItem.route}
        class="sidebar-item"
        type="button"
        onclick={() => actions.onNavigate(shellSettingsNavigationItem.route)}
      >
        <Icon name={shellSettingsNavigationItem.icon} size={16} />
        <span>{shellSettingsNavigationItem.label}</span>
      </button>
      <button class="sidebar-item" type="button" onclick={actions.onOpenShortcuts}>
        <Icon name="keyboard" size={16} />
        <span>Shortcuts</span>
      </button>
    </div>
  </aside>

  {@render children()}
</div>
