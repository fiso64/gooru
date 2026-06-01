import { errorMessage, parseTags } from '$lib/utils/format';
import type { FileItem, TagMutationResponse } from '$lib/api/types';
import type { TagMutationVariables } from '$lib/queries/files';

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

  async function mutateFile(file: FileItem, operation: 'add' | 'set' | 'remove', mutateTags: MutateTags) {
    const tags = parseTags(drafts[file.id] ?? '');
    if (!tags.length) return;
    busy = { ...busy, [file.id]: true };
    errors = { ...errors, [file.id]: '' };
    try {
      await mutateTags({ operation, body: { file_ids: [file.id], tags } });
      drafts = { ...drafts, [file.id]: '' };
    } catch (error) {
      errors = { ...errors, [file.id]: errorMessage(error) };
    } finally {
      busy = { ...busy, [file.id]: false };
    }
  }

  async function bulkSelected(ids: Set<string>, mutateTags: MutateTags) {
    const tags = parseTags(window.prompt('Tags to add to selected files') ?? '');
    if (!tags.length || !ids.size) return false;
    try {
      await mutateTags({ operation: 'add', body: { file_ids: Array.from(ids), tags } });
      return true;
    } catch (error) {
      window.alert(errorMessage(error));
      return false;
    }
  }

  async function bulkFiltered(query: string, mutateTags: MutateTags) {
    if (!query) return;
    const tags = parseTags(window.prompt('Tags to add to every file matching the current filter') ?? '');
    if (!tags.length) return;
    try {
      await mutateTags({ operation: 'add', body: { query, tags } });
    } catch (error) {
      window.alert(errorMessage(error));
    }
  }

  return {
    get drafts() { return drafts; },
    get busy() { return busy; },
    get errors() { return errors; },
    reset,
    updateDraft,
    mutateFile,
    bulkSelected,
    bulkFiltered
  };
}
