<script lang="ts">
  import { flip } from 'svelte/animate';
  import Icon from './Icon.svelte';
  import type { ShellCommonTag, ShellSavedSearch } from './shellModel';
  import { sidebarKindActive, sidebarKindFilters } from '$lib/utils/sidebarKinds';

  let {
    route,
    search,
    kindCounts,
    comicCount,
    comicAvailable,
    savedSearches,
    commonTags,
    commonTagsCollapsed,
    draggedSavedSearchID,
    savedSearchReorderBusy,
    savedSearchReorderError,
    onToggleKind,
    onCreateSavedSearch,
    onUpdateSavedSearch,
    onDeleteSavedSearch,
    onSavedSearch,
    onSavedSearchDragStart,
    onSavedSearchDragPreview,
    onSavedSearchDragEnd,
    onSavedSearchDrop,
    onToggleCommonTags,
    onCommonTag
  } = $props<{
    route: string;
    search: string;
    kindCounts: Array<{ value: string; count: number }>;
    comicCount: number;
    comicAvailable: boolean;
    savedSearches: ShellSavedSearch[];
    commonTags: ShellCommonTag[];
    commonTagsCollapsed: boolean;
    draggedSavedSearchID: string;
    savedSearchReorderBusy: boolean;
    savedSearchReorderError: string;
    onToggleKind: (filter: string) => void;
    onCreateSavedSearch: () => void;
    onUpdateSavedSearch: (id: string, name: string, query: string) => void;
    onDeleteSavedSearch: (id: string, name: string) => void;
    onSavedSearch: (query: string, name: string) => void;
    onSavedSearchDragStart: (event: DragEvent, id: string) => void;
    onSavedSearchDragPreview: (event: DragEvent, id: string) => void;
    onSavedSearchDragEnd: () => void;
    onSavedSearchDrop: (event: DragEvent, id: string) => void | Promise<void>;
    onToggleCommonTags: () => void;
    onCommonTag: (tag: string) => void;
  }>();

  const kinds = sidebarKindFilters;

  function kindCount(kind: string) {
    return kindCounts.find((item) => item.value === kind)?.count ?? 0;
  }

  function commonTagRankPercent(index: number) {
    if (commonTags.length <= 1) return 100;
    return Math.round((1 - index / (commonTags.length - 1)) * 100);
  }
</script>

<div class="sidebar-section">
  <div class="sidebar-section-head">Kinds</div>
  {#each kinds as kind}
    <button
      class:active={route === 'library' && sidebarKindActive(search, kind.query)}
      class="sidebar-item"
      type="button"
      onclick={() => onToggleKind(kind.query)}
    >
      <Icon name={kind.icon} size={16} active={route === 'library' && sidebarKindActive(search, kind.query)} />
      <span>{kind.label}</span>
      <span class="count">{kindCount(kind.key).toLocaleString()}</span>
    </button>
  {/each}
  {#if comicAvailable}
    <button
      class:active={route === 'library' && sidebarKindActive(search, 'ext:cbz')}
      class="sidebar-item"
      type="button"
      onclick={() => onToggleKind('ext:cbz')}
    >
      <Icon name="bookmark" size={16} active={route === 'library' && sidebarKindActive(search, 'ext:cbz')} />
      <span>Comics</span>
      <span class="count">{comicCount.toLocaleString()}</span>
    </button>
  {/if}
</div>

<div class="sidebar-section saved-searches-section">
  <div class="sidebar-section-head">
    <span>Saved searches</span>
    <button class="sidebar-head-action" type="button" title="Save current search" aria-label="Save current search" onclick={onCreateSavedSearch}>
      <Icon name="plus" size={11} />
    </button>
  </div>
  {#each savedSearches as saved (saved.id)}
    <div
      class="sidebar-saved-row"
      class:drag-target={Boolean(draggedSavedSearchID) && draggedSavedSearchID !== saved.id}
      animate:flip={{ duration: 160 }}
      ondragover={(event) => onSavedSearchDragPreview(event, saved.id)}
      ondrop={(event) => void onSavedSearchDrop(event, saved.id)}
    >
      <button
        class="sidebar-item saved-search-drag"
        type="button"
        draggable={!savedSearchReorderBusy}
        disabled={savedSearchReorderBusy}
        ondragstart={(event) => onSavedSearchDragStart(event, saved.id)}
        ondragend={onSavedSearchDragEnd}
        onclick={() => onSavedSearch(saved.query, saved.name)}
      >
        <Icon name="bookmark" size={14} />
        <span class="truncate">{saved.name}</span>
      </button>
      <div class="sidebar-saved-actions">
        <button class="sidebar-mini" type="button" title={`Update ${saved.name}`} aria-label={`Update ${saved.name}`} onclick={() => onUpdateSavedSearch(saved.id, saved.name, saved.query)}>
          <Icon name="check" size={11} />
        </button>
        <button class="sidebar-mini" type="button" title={`Delete ${saved.name}`} aria-label={`Delete ${saved.name}`} onclick={() => onDeleteSavedSearch(saved.id, saved.name)}>
          <Icon name="trash" size={11} />
        </button>
      </div>
    </div>
  {:else}
    <div class="sidebar-note">No saved searches yet</div>
  {/each}
  {#if savedSearchReorderError}
    <div class="sidebar-note saved-search-order-error" role="status">{savedSearchReorderError}</div>
  {/if}
</div>

{#if commonTags.length}
  <div class="sidebar-section common-tags-section">
    <button
      class="common-tags-toggle"
      type="button"
      title={commonTagsCollapsed ? 'Expand Common tags' : 'Collapse Common tags'}
      aria-expanded={!commonTagsCollapsed}
      aria-controls="common-tags-list"
      aria-label={commonTagsCollapsed ? 'Expand Common tags' : 'Collapse Common tags'}
      onclick={onToggleCommonTags}
    >
      <span class="sidebar-section-head common-tags-toggle-content">
        <span>Common tags</span>
        <span aria-hidden="true" class:expanded={!commonTagsCollapsed} class="common-tags-chevron"><Icon name="chev_right" size={12} /></span>
      </span>
    </button>
    <div id="common-tags-list" class:collapsed={commonTagsCollapsed} class="common-tags-list">
      {#each commonTags as item, index}
        <button
          class="sidebar-item common-tag-item"
          style={`--common-tag-rank: ${commonTagRankPercent(index)}%`}
          type="button"
          onclick={() => onCommonTag(item.tag)}
        >
          <Icon name="tags" size={14} />
          <span class="truncate">{item.tag}</span>
          <span class="count">{item.count.toLocaleString()}</span>
        </button>
      {/each}
    </div>
  </div>
{/if}

<style>
  .saved-search-drag { cursor: grab; }
  .saved-search-drag:active { cursor: grabbing; }
  .saved-search-order-error { color: var(--danger, var(--text-2)); }
  .common-tags-toggle {
    width: 100%;
    border: 0;
    padding: 0;
    background: transparent;
    color: inherit;
    cursor: pointer;
    text-align: left;
  }
  .common-tags-toggle:focus-visible {
    outline: 1px solid var(--accent-line);
    outline-offset: -1px;
    border-radius: var(--r-2);
  }
  .common-tags-toggle-content { width: 100%; }
  .common-tags-chevron {
    display: inline-flex;
    flex: 0 0 auto;
    color: var(--text-3);
    transition: color 0.1s ease, transform 0.1s ease;
  }
  .common-tags-toggle:hover .common-tags-chevron { color: var(--text-2); }
  .common-tags-chevron.expanded { transform: rotate(90deg); }
  .common-tags-list { display: flex; flex-direction: column; gap: 2px; }
  .common-tags-list.collapsed { display: none; }
  .common-tag-item { color: color-mix(in srgb, var(--text-2) var(--common-tag-rank), var(--text-4)); }
  .common-tag-item:hover { color: var(--text); }
</style>
