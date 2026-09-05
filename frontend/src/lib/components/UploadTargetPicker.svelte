<script lang="ts">
  import DropdownPicker from './DropdownPicker.svelte';
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

  const options = $derived(
    targets.length
      ? targets.map((target: UploadTargetOption) => ({ value: target.id, label: target.name }))
      : [{ value: '', label: 'Default target' }]
  );
</script>

<DropdownPicker
  {options}
  selectedValue={selectedID}
  placeholder="Default target"
  triggerAriaLabel="Upload target"
  listAriaLabel="Upload targets"
  {onSelect}
/>
