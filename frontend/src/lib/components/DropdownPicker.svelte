<script lang="ts">
  import { onMount } from 'svelte';

  type DropdownOption = {
    value: string;
    label: string;
  };

  let {
    options,
    selectedValue,
    placeholder = 'Select',
    triggerAriaLabel,
    listAriaLabel,
    onSelect
  } = $props<{
    options: DropdownOption[];
    selectedValue: string;
    placeholder?: string;
    triggerAriaLabel: string;
    listAriaLabel: string;
    onSelect: (value: string) => void;
  }>();

  let picker: HTMLDetailsElement | undefined;
  const selected = $derived(options.find((option: DropdownOption) => option.value === selectedValue));

  function close() {
    picker?.removeAttribute('open');
  }

  function choose(value: string) {
    onSelect(value);
    close();
  }

  function handleDocumentPointerDown(event: PointerEvent) {
    if (!picker?.open) return;
    const target = event.target;
    if (target instanceof Node && !picker.contains(target)) close();
  }

  function handleKeydown(event: KeyboardEvent) {
    const target = event.target;
    if (event.key !== 'Escape' || !picker?.open || !(target instanceof Node) || !picker.contains(target)) return;
    event.preventDefault();
    close();
    picker.querySelector('summary')?.focus();
  }

  onMount(() => {
    document.addEventListener('pointerdown', handleDocumentPointerDown);
    return () => document.removeEventListener('pointerdown', handleDocumentPointerDown);
  });
</script>

<svelte:window onkeydown={handleKeydown} />

<details bind:this={picker} style="position: relative; width: 100%;">
  <summary class="g-input" aria-label={triggerAriaLabel} style="cursor: pointer; list-style: none;">
    {selected?.label ?? options[0]?.label ?? placeholder}
  </summary>
  <div class="g-card" role="listbox" aria-label={listAriaLabel} style="position: absolute; z-index: 20; left: 0; right: 0; top: calc(100% + 4px); padding: 6px; display: grid; gap: 2px;">
    {#each options as option}
      <button
        class="g-btn g-btn-ghost"
        type="button"
        role="option"
        aria-selected={option.value === selectedValue}
        style="justify-content: flex-start; width: 100%;"
        onclick={() => choose(option.value)}
      >
        {option.label}
      </button>
    {/each}
  </div>
</details>
