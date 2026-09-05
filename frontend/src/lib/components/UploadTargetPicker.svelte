<script lang="ts">
  import type { UploadTargetOption } from '$lib/state/uploadItems';

  let {
    targets,
    selectedID,
    onSelect
  } = $props<{
    targets: UploadTargetOption[];
    selectedID: string;
    onSelect: (id: string) => void;
  }>();

  let picker: HTMLDetailsElement | undefined;
  const selected = $derived(targets.find((target: UploadTargetOption) => target.id === selectedID));

  function choose(id: string) {
    onSelect(id);
    picker?.removeAttribute('open');
  }
</script>

<details bind:this={picker} style="position: relative; width: 100%;">
  <summary class="g-input" aria-label="Upload target" style="cursor: pointer; list-style: none;">
    {selected?.name ?? targets[0]?.name ?? 'Default target'}
  </summary>
  <div class="g-card" role="listbox" aria-label="Upload targets" style="position: absolute; z-index: 20; left: 0; right: 0; top: calc(100% + 4px); padding: 6px; display: grid; gap: 2px;">
    {#each targets as target}
      <button
        class="g-btn g-btn-ghost"
        type="button"
        role="option"
        aria-selected={target.id === selectedID}
        style="justify-content: flex-start; width: 100%;"
        onclick={() => choose(target.id)}
      >
        {target.name}
      </button>
    {:else}
      <button class="g-btn g-btn-ghost" type="button" role="option" aria-selected="true" onclick={() => choose('')}>Default target</button>
    {/each}
  </div>
</details>
