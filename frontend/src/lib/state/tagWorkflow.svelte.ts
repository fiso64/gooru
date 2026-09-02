import { errorMessage, parseTags } from '$lib/utils/format';
import { applyTagOperation } from '$lib/utils/tags';
import type { FileItem, TagMutationResponse } from '$lib/api/types';
import type { TagMutationVariables } from '$lib/queries/files';
import { selectionRequest, type LibrarySelection } from '$lib/state/selection';

type MutateTags = (variables: TagMutationVariables) => Promise<TagMutationResponse>;

export function createTagWorkflow() {
  let drafts = $state<Record<string, string>>({});
  let busy = $state<Record<string, boolean>>({});
  let errors = $state<Record<string, string>>({});

  function reset() {
    drafts = {};
    busy = {};
    errors = {};
  }

  function updateDraft(fileID: string, value: string) {
    drafts = { ...drafts, [fileID]: value };
  }

  async function mutateFileTags(file: FileItem, operation: 'add' | 'set' | 'remove', tagInput: string, mutateTags: MutateTags) {
    const tags = parseTags(tagInput);
    if (!tags.length) return;
    busy = { ...busy, [file.id]: true };
    errors = { ...errors, [file.id]: '' };
    try {
      await mutateTags({ operation, body: { file_ids: [file.id], tags } });
      file.tags = applyTagOperation(file.tags, operation, tags);
      drafts = { ...drafts, [file.id]: '' };
    } catch (error) {
      errors = { ...errors, [file.id]: errorMessage(error) };
    } finally {
      busy = { ...busy, [file.id]: false };
    }
  }

  async function mutateFile(file: FileItem, operation: 'add' | 'set' | 'remove', mutateTags: MutateTags) {
    return mutateFileTags(file, operation, drafts[file.id] ?? '', mutateTags);
  }

  async function removeTag(file: FileItem, tag: string, mutateTags: MutateTags) {
    busy = { ...busy, [file.id]: true };
    errors = { ...errors, [file.id]: '' };
    try {
      await mutateTags({ operation: 'remove', body: { file_ids: [file.id], tags: [tag] } });
      file.tags = applyTagOperation(file.tags, 'remove', [tag]);
    } catch (error) {
      errors = { ...errors, [file.id]: errorMessage(error) };
    } finally {
      busy = { ...busy, [file.id]: false };
    }
  }

  async function bulkSelected(selection: LibrarySelection, tagInput: string, operation: 'add' | 'remove', mutateTags: MutateTags) {
    const tags = parseTags(tagInput);
    const selector = selectionRequest(selection);
    if (!tags.length || ('file_ids' in selector && !selector.file_ids.length)) return false;
    await mutateTags({ operation, body: { ...selector, tags } });
    return true;
  }

  return {
    get drafts() { return drafts; },
    get busy() { return busy; },
    get errors() { return errors; },
    reset,
    updateDraft,
    mutateFile,
    mutateFileTags,
    removeTag,
    bulkSelected
  };
}
