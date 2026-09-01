<script lang="ts">
  import Icon from './Icon.svelte';
  import TagAutocompleteInput from './TagAutocompleteInput.svelte';
  import type { TagCandidate } from '$lib/utils/tagSuggestions';

  let {
    fileID,
    fileName,
    draft,
    busy,
    error,
    tags,
    existingTags,
    onInput,
    onCommit
  } = $props<{
    fileID: string;
    fileName: string;
    draft: string;
    busy: boolean;
    error: string;
    tags: TagCandidate[];
    existingTags: string[];
    onInput: (value: string) => void;
    onCommit: (value: string) => void;
  }>();
</script>

<div class="tag-editor">
  <div class="lightbox-tag-input">
    <Icon name="plus" size={12} />
    <TagAutocompleteInput
      value={draft}
      {tags}
      existing={existingTags}
      placeholder="add tag — e.g. subject:portrait"
      disabled={busy}
      ariaLabel={`Tags for ${fileName}`}
      {onInput}
      {onCommit}
    />
  </div>
  {#if error}<p class="form-error">{error}</p>{/if}
</div>
