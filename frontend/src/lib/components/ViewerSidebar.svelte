<script lang="ts">
  import Icon from './Icon.svelte';
  import TagEditor from './TagEditor.svelte';
  import { groupTags } from '$lib/utils/format';
  import type { TagCandidate } from '$lib/utils/tagSuggestions';

  let {
    titleID,
    eyebrow,
    fileID,
    fileName,
    metadata,
    tagValues,
    tagCandidates,
    tagDraft,
    tagBusy,
    tagError,
    tagMode,
    closeLabel = 'Close preview',
    removeTagFrom = '',
    tagStatus = '',
    onClose,
    onTagInput,
    onCommitTag,
    onRemoveTag,
    onModeToggle,
    onTagSearch
  } = $props<{
    titleID: string;
    eyebrow: string;
    fileID: string;
    fileName: string;
    metadata: Array<{ label: string; value: string; className?: string }>;
    tagValues: string[];
    tagCandidates: TagCandidate[];
    tagDraft: string;
    tagBusy: boolean;
    tagError: string;
    tagMode: 'add' | 'remove';
    closeLabel?: string;
    removeTagFrom?: string;
    tagStatus?: string;
    onClose: () => void;
    onTagInput: (value: string) => void;
    onCommitTag: (value: string) => void;
    onRemoveTag: (tag: string) => void;
    onModeToggle: () => void;
    onTagSearch?: (tag: string) => void;
  }>();

  const tagGroups = $derived(groupTags(tagValues));
  const hasTagNamespaces = $derived(tagGroups.some((group) => Boolean(group.namespace)));
</script>

<aside class="lightbox-aside">
  <div class="panel-row">
    <div class="g-eyebrow g-eyebrow-accent">{eyebrow}</div>
    <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" aria-label={closeLabel} onclick={onClose}>
      <Icon name="close" size={14} />
    </button>
  </div>

  <h2 id={titleID} class="lightbox-name">{fileName}</h2>

  <dl class="lightbox-meta">
    {#each metadata as row}
      <dt>{row.label}</dt><dd class={row.className ?? ''}>{row.value}</dd>
    {/each}
  </dl>

  <hr class="g-divider" />

  <div class="lightbox-tags">
    <div class="lightbox-tag-group-head lightbox-tags-head">
      <span>Tags · {tagValues.length}</span>
      <span class="lightbox-tag-tools">
        <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" disabled title="Tag history coming soon"><Icon name="info" size={13} /></button>
        <button class="g-btn g-btn-ghost g-btn-sm g-btn-icon" type="button" disabled title="Tag suggestions coming soon"><Icon name="sliders" size={13} /></button>
      </span>
    </div>

    {#each tagGroups as group (group.namespace)}
      <div class="lightbox-tag-group">
        {#if group.namespace || hasTagNamespaces}
          <div class="lightbox-tag-group-head"><span>{group.namespace || 'OTHER'}</span><span>{group.tags.length}</span></div>
        {/if}
        <div class="lightbox-tag-list">
          {#each group.tags as tag}
            <span class="g-tag">
              {#if onTagSearch}
                <button class="g-tag-search" type="button" aria-label={`Search for ${tag}`} onclick={() => onTagSearch?.(tag)}>
                  {#if tag.includes(':')}
                    <span class="ns">{tag.split(':')[0]}:</span><span>{tag.slice(tag.indexOf(':') + 1)}</span>
                  {:else}
                    <span>{tag}</span>
                  {/if}
                </button>
              {:else if tag.includes(':')}
                <span class="ns">{tag.split(':')[0]}:</span><span>{tag.slice(tag.indexOf(':') + 1)}</span>
              {:else}
                <span>{tag}</span>
              {/if}
              <button
                class="g-tag-x"
                type="button"
                aria-label={`Remove ${tag}${removeTagFrom ? ` from ${removeTagFrom}` : ''}`}
                disabled={tagBusy}
                onclick={() => onRemoveTag(tag)}
              >
                <Icon name="close" size={11} />
              </button>
            </span>
          {/each}
        </div>
      </div>
    {/each}

    <TagEditor
      {fileID}
      {fileName}
      draft={tagDraft}
      busy={tagBusy}
      error={tagError}
      tags={tagCandidates}
      existingTags={tagValues}
      mode={tagMode}
      onInput={onTagInput}
      onCommit={onCommitTag}
      {onModeToggle}
    />
    {#if tagStatus}<div class="viewer-sidebar-tag-status">{tagStatus}</div>{/if}
  </div>
</aside>

<style>
  :global(.lightbox-tag-list .g-tag-search) {
    display: inline-flex;
    align-items: center;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }

  :global(.lightbox-tag-list .g-tag-search:focus-visible) {
    outline: 2px solid var(--accent-line);
    outline-offset: 2px;
    border-radius: 2px;
  }

  .viewer-sidebar-tag-status {
    color: var(--text-3);
    font-family: var(--font-mono);
    font-size: 10px;
  }
</style>
