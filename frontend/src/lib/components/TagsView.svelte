<script lang="ts">
  import Icon from './Icon.svelte';
  import type { TagItem } from '$lib/api/types';

  let {
    tags,
    loading,
    error,
    onTag,
    onNamespace
  } = $props<{
    tags: TagItem[];
    loading: boolean;
    error: string;
    onTag: (tag: string) => void;
    onNamespace: (namespace: string) => void;
  }>();

  const namespaces = $derived.by(() => {
    const counts = new Map<string, number>();
    for (const tag of tags) {
      if (!tag.namespace) continue;
      counts.set(tag.namespace, (counts.get(tag.namespace) ?? 0) + (tag.count ?? 0));
    }
    return Array.from(counts.entries())
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
  });

  const unnamespaced = $derived.by(() => tags.filter((tag: TagItem) => !tag.namespace));
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Tags</div>
      <h1>Tag index</h1>
      <p>Browse real tag counts from the library and jump directly into filtered searches.</p>
    </div>

    {#if loading}
      <div class="tagscloud">
        {#each Array.from({ length: 10 }) as _}
          <div class="tagscloud-item skeleton"><span>Loading</span><span class="count">--</span></div>
        {/each}
      </div>
    {:else if error}
      <div class="empty-row error-state">{error}</div>
    {:else if tags.length}
      {#if namespaces.length}
        <section class="tag-index-section">
          <div class="list-head"><span class="g-eyebrow">Namespaces</span><span class="g-eyebrow">{namespaces.length}</span></div>
          <div class="tagscloud">
            {#each namespaces as item}
              <button class="tagscloud-item" type="button" onclick={() => onNamespace(item.name)}>
                <span><Icon name="tags" size={13} /> {item.name}:</span>
                <span class="count">{item.count.toLocaleString()}</span>
              </button>
            {/each}
          </div>
        </section>
      {/if}

      <section class="tag-index-section">
        <div class="list-head"><span class="g-eyebrow">Tags</span><span class="g-eyebrow">{tags.length.toLocaleString()}</span></div>
        <div class="tagscloud tagscloud-dense">
          {#each tags as tag}
            <button class="tagscloud-item" type="button" onclick={() => onTag(tag.name)}>
              <span>
                {#if tag.namespace}<span class="ns">{tag.namespace}:</span>{tag.value}{:else}{tag.name}{/if}
              </span>
              <span class="count">{(tag.count ?? 0).toLocaleString()}</span>
            </button>
          {/each}
        </div>
      </section>

      {#if unnamespaced.length}
        <section class="tag-index-section">
          <div class="list-head"><span class="g-eyebrow">Plain tags</span><span class="g-eyebrow">{unnamespaced.length}</span></div>
          <div class="tag-strip">
            {#each unnamespaced as tag}
              <button class="g-tag" type="button" onclick={() => onTag(tag.name)}>{tag.name}<span class="tag-count">{tag.count ?? 0}</span></button>
            {/each}
          </div>
        </section>
      {/if}
    {:else}
      <div class="empty-row">No tags have been indexed yet.</div>
    {/if}
  </div>
</main>
