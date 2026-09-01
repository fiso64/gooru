<script lang="ts">
  import type { TagItem } from '$lib/api/types';

  let {
    tags,
    libraryCount,
    loading,
    error,
    onTag,
    onNamespace
  } = $props<{
    tags: TagItem[];
    libraryCount: number;
    loading: boolean;
    error: string;
    onTag: (tag: string) => void;
    onNamespace: (namespace: string) => void;
  }>();

  let filter = $state('');
  const preferredNamespaces = ['', 'rating', 'subject', 'location', 'people', 'color', 'year', 'collection', 'film', 'camera'];

  const grouped = $derived.by(() => {
    const needle = filter.trim().toLowerCase();
    const groups = new Map<string, TagItem[]>();
    for (const tag of tags) {
      if (needle && !tag.name.toLowerCase().includes(needle)) continue;
      const key = tag.namespace ?? '';
      const items = groups.get(key) ?? [];
      items.push(tag);
      groups.set(key, items);
    }

    const ordered = [
      ...preferredNamespaces.filter((namespace) => groups.has(namespace)),
      ...Array.from(groups.keys())
        .filter((namespace) => !preferredNamespaces.includes(namespace))
        .sort((a, b) => a.localeCompare(b))
    ];

    return ordered.map((namespace) => ({
      namespace,
      label: namespace || 'tags',
      tags: (groups.get(namespace) ?? []).sort((a, b) => (b.count ?? 0) - (a.count ?? 0) || a.name.localeCompare(b.name))
    }));
  });
</script>

<main class="main">
  <div class="page">
    <div class="page-header">
      <div class="g-eyebrow g-eyebrow-accent">Tags</div>
      <h1>{tags.length.toLocaleString()} tags across {libraryCount.toLocaleString()} files</h1>
      <p>Browse by namespace. Click any tag to filter the library.</p>
    </div>

    <div class="tag-filter-sticky">
      <label class="sr-only" for="tag-filter">Filter tags</label>
      <input id="tag-filter" class="g-input" placeholder="Filter tags…" bind:value={filter} />
    </div>

    {#if loading}
      <div class="tagscloud">
        {#each Array.from({ length: 10 }) as _}
          <div class="tagscloud-item skeleton"><span>Loading</span><span class="count">--</span></div>
        {/each}
      </div>
    {:else if error}
      <div class="empty-row error-state">{error}</div>
    {:else if grouped.length}
      {#each grouped as group}
        <section>
          <div class="tag-ns-header">
            <h3>{group.label}</h3>
            <span class="count">{group.tags.length.toLocaleString()} {group.tags.length === 1 ? 'tag' : 'tags'}</span>
          </div>
          <div class="tagscloud">
            {#each group.tags as tag}
              <button class="tagscloud-item" type="button" onclick={() => onTag(tag.name)}>
                <span class="tag-name">
                  {#if tag.namespace}<span class="ns">{tag.namespace}:</span>{tag.value}{:else}{tag.name}{/if}
                </span>
                <span class="count">{(tag.count ?? 0).toLocaleString()}</span>
              </button>
            {/each}
          </div>
        </section>
      {/each}
    {:else if tags.length}
      <div class="empty-row">No tags match “{filter}”.</div>
    {:else}
      <div class="empty-row">No tags have been indexed yet.</div>
    {/if}
  </div>
</main>
