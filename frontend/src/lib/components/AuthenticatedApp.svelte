<script lang="ts">
  import { untrack } from 'svelte';
  import AppShell from '$lib/components/AppShell.svelte';
  import ActionDialog from '$lib/components/ActionDialog.svelte';
  import FileTagDialog from '$lib/components/FileTagDialog.svelte';
  import GlobalFileDrop from '$lib/components/GlobalFileDrop.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import JobsView from '$lib/components/JobsView.svelte';
  import MediaGrid from '$lib/components/MediaGrid.svelte';
  import PreviewDialog from '$lib/components/PreviewDialog.svelte';
  import SettingsView from '$lib/components/SettingsView.svelte';
  import ShortcutsView from '$lib/components/ShortcutsView.svelte';
  import TagsView from '$lib/components/TagsView.svelte';
  import UploadPanel from '$lib/components/UploadPanel.svelte';
  import { ApiClient, ApiError } from '$lib/api/client';
  import { createFileDownload, type FileDownloadSelector } from '$lib/api/fileDownloads';
  import { createFileSelection, deleteFileSelection, fileSelectionMembers } from '$lib/api/fileSelections';
  import { authState } from '$lib/stores/auth';
  import { runtimeConfig, type PaginationMode } from '$lib/stores/runtimeConfig';
  import { createFileCountQuery, createFileFacetsQuery, createFilesQuery, createFileRemovalMutation, createFilesRemovalMutation, createTagMutation, pageTokenOffset, type FileSort } from '$lib/queries/files';
  import { createCancelJobMutation, createJobQuery, createJobsQuery } from '$lib/queries/jobs';
  import {
    createSavedSearchCreateMutation,
    createSavedSearchDeleteMutation,
    createSavedSearchesQuery,
    createSavedSearchUpdateMutation,
    createSuggestionsQuery,
    createTagsQuery,
    createUploadMutation,
    createUploadTargetsQuery,
    refreshUploadQueries
  } from '$lib/queries/library';
  import { createLibraryWorkflow } from '$lib/state/libraryWorkflow.svelte';
  import { selectionRequest } from '$lib/state/selection';
  import { createTagWorkflow } from '$lib/state/tagWorkflow.svelte';
  import { createUploadWorkflow } from '$lib/state/uploadWorkflow.svelte';
  import { createUploadTagReconciliationWave } from '$lib/state/uploadTagReconciliation';
  import { browserPersistenceRegistry, readBrowserPreference, writeBrowserPreference } from '$lib/utils/browserStorage';
  import { errorMessage } from '$lib/utils/format';
  import { hasCommandModifier, isEditableShortcutTarget, libraryShortcutAction } from '$lib/utils/keyboard';
  import { queryWithoutSidebarKind } from '$lib/utils/sidebarKinds';
  import { previewNeighbor } from '$lib/utils/viewerNavigation';
  import { useQueryClient } from '@tanstack/svelte-query';
  import type { FileItem, Job, SavedSearchRequest } from '$lib/api/types';

  const paginationPreferenceKey = browserPersistenceRegistry.libraryPaginationMode.key;
  const uploadMetadataRefreshIntervalMs = 700;
  const isPaginationMode = (value: unknown): value is PaginationMode => value === 'infinite' || value === 'paged';
  let paginationModeOverride = $state<PaginationMode | undefined>(
    readBrowserPreference<PaginationMode | undefined>(paginationPreferenceKey, undefined, isPaginationMode)
  );
  const effectivePaginationMode = $derived(paginationModeOverride ?? $runtimeConfig.paginationMode);
  const pagedMode = $derived(effectivePaginationMode === 'paged');

  const queryClient = useQueryClient();
  const library = createLibraryWorkflow(undefined, untrack(() => pagedMode));
  const searchDraft = library.searchDraft;
  const submittedSearch = library.submittedSearch;
  const suggestionSearch = library.suggestionSearch;
  const tagWorkflow = createTagWorkflow();
  const upload = createUploadWorkflow();

  let authScope = $state(0);
  let observedCSRF = $state($authState.csrfToken);
  let loadMoreSentinel = $state<HTMLDivElement | undefined>();
  let cancelRequestedJobID = $state('');
  let jobsDrawerOpen = $state(false);
  let bulkDownloadBusy = $state(false);
  let bulkDownloadError = $state('');
  let bulkDownloadURL = $state('');
  let nestedPreviewNavigation = $state(false);
  let observedItemsPerPage = $state($runtimeConfig.itemsPerPage);
  let trackUploadResults = $state(false);
  let uploadResultsFloor = $state(0);
  let observedUploadResultQueryKey = $state('');
  let observedSelectionSnapshotID = $state('');
  let uploadMetadataRefreshTimer: number | undefined;
  let uploadMetadataRefreshPromise: Promise<number | undefined> | null = null;
  let selectionSnapshotPromise: Promise<void> | null = null;
  const selectionMembershipPending = new Set<string>();
  const selectionMembershipRuns = new Set<Promise<void>>();
  const uploadTagReconciliation = createUploadTagReconciliationWave(() => refreshUploadQueries(queryClient).catch(() => undefined));
  let fileMetadata = $state<{
    total_count: number;
    library_count: number;
    facets?: { kind?: Array<{ value: string; count: number }> };
  } | null>(null);
  let actionDialog = $state<{
    kind: 'none' | 'save-create' | 'save-update' | 'save-delete' | 'bulk-selected' | 'bulk-remove-selected' | 'bulk-untrack-selected' | 'bulk-delete-selected' | 'bulk-delete-or-untrack-selected' | 'untrack-file' | 'delete-file';
    value: string;
    error: string;
    busy: boolean;
    id: string;
    name: string;
    previousQuery: string;
  }>({ kind: 'none', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' });
  let fileTagDialog = $state<{ file: FileItem | null; mode: 'add' | 'remove'; fromSelection: boolean; busy: boolean; error: string }>({ file: null, mode: 'add', fromSelection: false, busy: false, error: '' });

  const filesQuery = createFilesQuery(
    () => Boolean($authState.user),
    () => $submittedSearch,
    () => '',
    () => library.sort,
    () => library.order,
    () => authScope,
    () => library.route === 'library',
    () => $runtimeConfig.itemsPerPage,
    () => pagedMode,
    () => library.page - 1
  );
  const sidebarBaseQuery = $derived(queryWithoutSidebarKind($submittedSearch));
  const kindFacetsQuery = createFileFacetsQuery(() => Boolean($authState.user), () => sidebarBaseQuery, () => authScope, () => library.route === 'library' && sidebarBaseQuery !== $submittedSearch);
  const uploadResultsCountQuery = createFileCountQuery(() => Boolean($authState.user), () => $submittedSearch, () => authScope, () => library.route === 'library' && trackUploadResults);
  const uploadJobQuery = createJobQuery(() => $authState.csrfToken, () => upload.activeJobID, () => authScope);
  const jobsQuery = createJobsQuery(() => Boolean($authState.user), () => authScope, () => 50);
  const savedSearchesQuery = createSavedSearchesQuery(() => Boolean($authState.user), () => authScope);
  const tagsQuery = createTagsQuery(() => Boolean($authState.user), () => authScope);
  const uploadTargetsQuery = createUploadTargetsQuery(() => Boolean($authState.user), () => authScope);
  const suggestionsQuery = createSuggestionsQuery(() => Boolean($authState.user), () => $suggestionSearch, () => $submittedSearch, () => authScope);

  const tagMutation = createTagMutation(() => $authState.csrfToken, queryClient);
  const fileRemovalMutation = createFileRemovalMutation(() => $authState.csrfToken, queryClient);
  const filesRemovalMutation = createFilesRemovalMutation(() => $authState.csrfToken, queryClient);
  const uploadMutation = createUploadMutation(() => $authState.csrfToken);
  const cancelJobMutation = createCancelJobMutation(() => $authState.csrfToken, queryClient);
  const createSavedSearchMutation = createSavedSearchCreateMutation(() => $authState.csrfToken, queryClient);
  const updateSavedSearchMutation = createSavedSearchUpdateMutation(() => $authState.csrfToken, queryClient);
  const deleteSavedSearchMutation = createSavedSearchDeleteMutation(() => $authState.csrfToken, queryClient);

  const loadedFiles = $derived(filesQuery.data?.pages.flatMap((page) => page.files) ?? []);
  const retainedStartIndex = $derived(pagedMode ? 0 : pageTokenOffset(String(filesQuery.data?.pageParams[0] ?? '')));
  const activeJobs = $derived((jobsQuery.data?.items ?? []).filter((job) => job.status === 'pending' || job.status === 'running'));
  const fileMetadataKey = $derived(`${authScope}|${$submittedSearch}|${library.sort}|${library.order}|${effectivePaginationMode}`);
  const uploadResultQueryKey = $derived(`${authScope}|${$submittedSearch}`);
  // Keep the grid snapshot separate from server-refreshed counts. Upload-time metadata can
  // update visible counts without changing the rows/virtual geometry until Refresh results.
  const gridSnapshotTotalCount = $derived(fileMetadata?.total_count ?? loadedFiles.length);
  const liveCurrentQueryCount = $derived(trackUploadResults
    ? (uploadResultsCountQuery.data?.total_count ?? gridSnapshotTotalCount)
    : gridSnapshotTotalCount);
  const liveLibraryCount = $derived(tagsQuery.data?.library_count ?? fileMetadata?.library_count ?? loadedFiles.length);
  const uploadResultsBaseline = $derived(Math.max(uploadResultsFloor, gridSnapshotTotalCount));
  const newUploadResultCount = $derived(trackUploadResults ? Math.max(0, liveCurrentQueryCount - uploadResultsBaseline) : 0);
  const pagedPageCount = $derived(Math.max(1, Math.ceil(gridSnapshotTotalCount / $runtimeConfig.itemsPerPage)));
  const selectedCount = $derived(library.selectedCount());
  const sidebarKindCounts = $derived(
    sidebarBaseQuery !== $submittedSearch
      ? (kindFacetsQuery.data?.facets?.kind ?? [])
      : (fileMetadata?.facets?.kind ?? [])
  );
  const comicCount = $derived(sidebarKindCounts.find((item) => item.value === 'comic')?.count ?? 0);
  const comicAvailable = $derived((tagsQuery.data?.facets?.kind ?? []).some((item) => item.value === 'comic' && item.count > 0));

  $effect(() => {
    const targets = uploadTargetsQuery.data?.items ?? [];
    if (!targets.length || upload.targetID) return;
    const target = targets[0];
    upload.setTarget(target.id, target.added_at_strategy ?? 'queue', target.default_tags ?? []);
  });

  $effect(() => {
    const csrf = $authState.csrfToken;
    if (csrf === observedCSRF) return;
    observedCSRF = csrf;
    authScope += 1;
    queryClient.clear();
    library.reset();
    tagWorkflow.reset();
    upload.reset();
    fileMetadata = null;
    trackUploadResults = false;
    uploadResultsFloor = 0;
    cancelRequestedJobID = '';
    jobsDrawerOpen = false;
    bulkDownloadBusy = false;
    bulkDownloadError = '';
    bulkDownloadURL = '';
    stopUploadMetadataRefresh();
    closeActionDialog();
    closeFileTagDialog();
  });

  $effect(() => {
    fileMetadataKey;
    fileMetadata = null;
  });

  $effect(() => {
    const key = uploadResultQueryKey;
    if (key === observedUploadResultQueryKey) return;
    observedUploadResultQueryKey = key;
    trackUploadResults = upload.busy;
    uploadResultsFloor = gridSnapshotTotalCount;
  });

  $effect(() => {
    library.setPaginationEnabled(pagedMode);
    const itemsPerPage = $runtimeConfig.itemsPerPage;
    if (itemsPerPage === observedItemsPerPage) return;
    observedItemsPerPage = itemsPerPage;
    library.page = 1;
  });

  $effect(() => {
    fileMetadataKey;
    const metadataPage = filesQuery.data?.pages.find((page) => page.facets);
    if (!metadataPage) return;
    fileMetadata = {
      total_count: metadataPage.total_count,
      library_count: metadataPage.library_count,
      facets: metadataPage.facets
    };
  });

  $effect(() => {
    const job = uploadJobQuery.data;
    if (!job) return;
    const result = upload.applyJob(job);
    if (result.completed) reconcilePendingUploadItemTags();
    if (result.changedFiles && !trackUploadResults) {
      trackUploadResults = true;
      uploadResultsFloor = gridSnapshotTotalCount;
    }
    if (result.completed && !upload.busy && !upload.activeJobIDs.length) {
      void finishUploadMetadataRefresh();
    }
  });

  $effect(() => {
    if (uploadJobQuery.isError) upload.applyJobError(uploadJobQuery.error);
  });

  $effect(() => {
    if (!library.activeFile) nestedPreviewNavigation = false;
  });

  $effect(() => {
    if (selectedCount === 0) bulkDownloadError = '';
  });

  $effect(() => {
    const nextSnapshotID = library.selectionSnapshotID;
    const csrf = $authState.csrfToken;
    if (nextSnapshotID === observedSelectionSnapshotID) return;
    const previousSnapshotID = observedSelectionSnapshotID;
    observedSelectionSnapshotID = nextSnapshotID;
    if (previousSnapshotID) void deleteFileSelection(csrf, previousSnapshotID).catch(() => undefined);
  });

  $effect(() => {
    library.selection;
    loadedFiles;
    syncSelectionMembership();
  });

  $effect(() => {
    const files = loadedFiles;
    if (!files.length) return;
    untrack(() => {
      const tagsByID = new Map(files.map((file) => [file.id, file.tags ?? []]));
      upload.items.forEach((item, index) => {
        if (!item.remoteFileID || !tagsByID.has(item.remoteFileID)) return;
        const expectedBaseTags = item.tagSyncBaseTags ? [...item.tagSyncBaseTags] : undefined;
        rebaseUploadItemTagsFromRemote(index, tagsByID.get(item.remoteFileID) ?? [], expectedBaseTags);
      });
    });
  });

  function savedSearchContext() {
    return {
      query: library.filterQuery(),
      sort: library.sort,
      order: library.order,
      create: (body: SavedSearchRequest) => createSavedSearchMutation.mutateAsync(body),
      update: (id: string, body: SavedSearchRequest) => updateSavedSearchMutation.mutateAsync({ id, body }),
      remove: (id: string) => deleteSavedSearchMutation.mutateAsync(id)
    };
  }

  function libraryCursorFile(target: EventTarget | null) {
    if (!(target instanceof Element)) return null;
    const fileID = target.closest<HTMLElement>('.thumb-open[data-file-id]')?.dataset.fileId;
    return fileID ? (loadedFiles.find((file) => file.id === fileID) ?? null) : null;
  }

  function singleSelectedFileID() {
    if (selectedCount !== 1) return '';
    const loaded = loadedFiles.find((file) => library.isSelected(file.id));
    if (loaded) return loaded.id;
    const selection = library.selection;
    if (selection.mode === 'explicit') return [...selection.ids][0] ?? '';
    const candidates = new Set([...selection.optimisticIDs, ...selection.knownMembers, ...selection.includedIDs]);
    for (const excluded of selection.excludedIDs) candidates.delete(excluded);
    return candidates.size === 1 ? ([...candidates][0] ?? '') : '';
  }

  function openFileTagDialog(file: FileItem, mode: 'add' | 'remove', fromSelection: boolean) {
    fileTagDialog = { file, mode, fromSelection, busy: false, error: '' };
  }

  function closeFileTagDialog() {
    fileTagDialog = { file: null, mode: 'add', fromSelection: false, busy: false, error: '' };
  }

  async function openTagShortcut(mode: 'add' | 'remove', cursorFile: FileItem | null) {
    if (selectedCount === 0) {
      if (cursorFile) openFileTagDialog(cursorFile, mode, false);
      return;
    }
    if (selectedCount > 1) {
      if (mode === 'add') bulkTagSelected();
      else bulkUntagSelected();
      return;
    }

    const loaded = loadedFiles.find((file) => library.isSelected(file.id));
    if (loaded) {
      openFileTagDialog(loaded, mode, true);
      return;
    }

    const fileID = singleSelectedFileID();
    if (fileID) {
      try {
        openFileTagDialog(await new ApiClient().getFile(fileID), mode, true);
        return;
      } catch {
        // Fall through to the existing selection modal when the sole row is not materializable.
      }
    }
    if (mode === 'add') bulkTagSelected();
    else bulkUntagSelected();
  }

  async function submitFileTagDialog(tags: string[]) {
    const { file, fromSelection } = fileTagDialog;
    if (!file || fileTagDialog.busy) return;
    fileTagDialog = { ...fileTagDialog, busy: true, error: '' };
    try {
      await tagMutation.mutateAsync({ operation: 'set', body: { file_ids: [file.id], tags } });
      file.tags = [...tags];
      if (fromSelection) library.clearSelection();
      closeFileTagDialog();
    } catch (error) {
      fileTagDialog = { ...fileTagDialog, busy: false, error: errorMessage(error) };
    }
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented) return;
    const editable = isEditableShortcutTarget(event.target);
    const modified = hasCommandModifier(event);
    const altTagShortcut = event.altKey && !event.ctrlKey && !event.metaKey && !event.shiftKey && event.key === 'Enter';
    const cursorFile = selectedCount === 0 ? libraryCursorFile(event.target) : null;
    const shortcutsKey = event.key === '?' || (event.code === 'Slash' && event.shiftKey);
    if (shortcutsKey && !modified && !editable) {
      event.preventDefault();
      setRoute('shortcuts');
      return;
    }

    if ((!modified || altTagShortcut) && !editable && library.route === 'library' && !library.activeFile && actionDialog.kind === 'none' && !fileTagDialog.file) {
      const action = libraryShortcutAction(event.key, {
        selectedCount,
        cursorAvailable: Boolean(cursorFile),
        shiftKey: event.shiftKey,
        altKey: event.altKey
      });
      if (action) {
        event.preventDefault();
        if (action === 'select-all') selectAllFiles();
        else if (action === 'download-selected') {
          if (selectedCount > 0) void bulkDownloadSelected();
          else if (cursorFile) void downloadCursorFile(cursorFile);
        } else if (action === 'tag-selected') void openTagShortcut('add', cursorFile);
        else if (action === 'untag-selected') void openTagShortcut('remove', cursorFile);
        else if (action === 'untrack-selected') {
          if (selectedCount > 0) bulkUntrackSelected();
          else if (cursorFile) untrackPreview(cursorFile);
        } else {
          if (selectedCount > 0) bulkDeleteSelected();
          else if (cursorFile) deletePreview(cursorFile);
        }
        return;
      }
    }

    if (nestedPreviewNavigation && library.activeFile && (event.key === 'ArrowLeft' || event.key === 'ArrowRight' || event.key === 'j' || event.key === 'k')) return;
    library.handleKeydown(event, loadedFiles);
  }

  function selectAllFiles() {
    const requestID = library.selectAll(gridSnapshotTotalCount, loadedFiles);
    const query = library.filterQuery();
    const csrf = $authState.csrfToken;
    const run = createFileSelection(csrf, query)
      .then(async (snapshot) => {
        library.applySnapshot(requestID, snapshot.id, snapshot.count);
        if (library.selectionSnapshotID !== snapshot.id) {
          await deleteFileSelection(csrf, snapshot.id).catch(() => undefined);
          return;
        }
        syncSelectionMembership();
      })
      .catch((error) => library.failSnapshot(requestID, errorMessage(error)));
    selectionSnapshotPromise = run;
    void run.finally(() => {
      if (selectionSnapshotPromise === run) selectionSnapshotPromise = null;
    });
  }

  function syncSelectionMembership() {
    const snapshotID = library.selectionSnapshotID;
    if (!snapshotID) return;
    const candidates = library.unknownSnapshotIDs(loadedFiles.map((file) => file.id)).filter((fileID) => {
      const key = `${snapshotID}:${fileID}`;
      if (selectionMembershipPending.has(key)) return false;
      selectionMembershipPending.add(key);
      return true;
    });
    if (!candidates.length) return;

    const csrf = $authState.csrfToken;
    const run = fileSelectionMembers(csrf, snapshotID, candidates)
      .then((members) => library.applySnapshotMembership(snapshotID, candidates, members))
      .catch((error) => {
        if (error instanceof ApiError && error.code === 'selection_expired' && library.selectionSnapshotID === snapshotID) {
          library.failSnapshot(library.selectionRequestID, errorMessage(error));
        }
      })
      .finally(() => {
        for (const fileID of candidates) selectionMembershipPending.delete(`${snapshotID}:${fileID}`);
        selectionMembershipRuns.delete(run);
      });
    selectionMembershipRuns.add(run);
  }

  async function ensureSelectionReady() {
    if (library.selectionPending && selectionSnapshotPromise) await selectionSnapshotPromise;
    if (library.selectionError) throw new Error(library.selectionError);
    if (library.selectionPending) throw new Error('Selection snapshot is still loading.');
    syncSelectionMembership();
    if (selectionMembershipRuns.size) await Promise.allSettled([...selectionMembershipRuns]);
    if (library.selectionError) throw new Error(library.selectionError);
    return selectionRequest(library.selection);
  }

  function setRoute(route: string) {
    jobsDrawerOpen = false;
    library.setRoute(route);
  }

  function toggleJobsDrawer() {
    jobsDrawerOpen = !jobsDrawerOpen;
  }

  function closeJobsDrawer() {
    jobsDrawerOpen = false;
  }

  function closeActionDialog() {
    actionDialog = { kind: 'none', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function createSavedSearch() {
    actionDialog = {
      kind: 'save-create',
      value: library.activeSavedSearch || library.filterQuery(),
      error: library.filterQuery() ? '' : 'Search or choose a kind before saving.',
      busy: false,
      id: '',
      name: '',
      previousQuery: ''
    };
  }

  function updateSavedSearch(id: string, name: string, previousQuery: string) {
    actionDialog = { kind: 'save-update', value: name, error: '', busy: false, id, name, previousQuery };
  }

  function deleteSavedSearch(id: string, name: string) {
    actionDialog = { kind: 'save-delete', value: '', error: '', busy: false, id, name, previousQuery: '' };
  }

  function untrackPreview(file: { id: string; name: string }) {
    actionDialog = { kind: 'untrack-file', value: '', error: '', busy: false, id: file.id, name: file.name, previousQuery: '' };
  }

  function deletePreview(file: { id: string; name: string }) {
    actionDialog = { kind: 'delete-file', value: '', error: '', busy: false, id: file.id, name: file.name, previousQuery: '' };
  }

  function bulkTagSelected() {
    actionDialog = { kind: 'bulk-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function bulkUntagSelected() {
    actionDialog = { kind: 'bulk-remove-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function bulkUntrackSelected() {
    actionDialog = { kind: 'bulk-untrack-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function bulkDeleteSelected() {
    actionDialog = { kind: 'bulk-delete-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  async function requestFileDownload(selector: FileDownloadSelector) {
    const download = await createFileDownload($authState.csrfToken, selector);
    const url = new URL(download.url, window.location.origin);
    if (url.origin !== window.location.origin || !url.pathname.startsWith('/api/v1/file-downloads/')) {
      throw new Error('Server returned an invalid file download URL.');
    }
    bulkDownloadURL = url.href;
  }

  async function bulkDownloadSelected() {
    if (bulkDownloadBusy || selectedCount <= 0) return;
    bulkDownloadBusy = true;
    bulkDownloadError = '';
    try {
      await requestFileDownload(await ensureSelectionReady());
    } catch (error) {
      bulkDownloadError = errorMessage(error);
    } finally {
      bulkDownloadBusy = false;
    }
  }

  async function downloadCursorFile(file: FileItem) {
    if (bulkDownloadBusy) return;
    bulkDownloadBusy = true;
    bulkDownloadError = '';
    try {
      await requestFileDownload({ file_ids: [file.id] });
    } catch (error) {
      bulkDownloadError = errorMessage(error);
    } finally {
      bulkDownloadBusy = false;
    }
  }

  function isBulkTagDialog(kind = actionDialog.kind) {
    return kind === 'bulk-selected' || kind === 'bulk-remove-selected';
  }

  async function submitActionDialog() {
    if (actionDialog.busy || actionDialog.kind === 'none') return;
    const ctx = savedSearchContext();
    const value = actionDialog.value.trim();
    if ((actionDialog.kind === 'save-create' || actionDialog.kind === 'save-update' || isBulkTagDialog()) && !value) {
      actionDialog = { ...actionDialog, error: isBulkTagDialog() ? 'Enter at least one tag.' : 'Enter a name.' };
      return;
    }
    actionDialog = { ...actionDialog, busy: true, error: '' };
    try {
      if (actionDialog.kind === 'save-create') {
        if (!ctx.query) throw new Error('Search or choose a kind before saving.');
        await ctx.create({ name: value, query: ctx.query, sort: ctx.sort, order: ctx.order });
      } else if (actionDialog.kind === 'save-update') {
        await ctx.update(actionDialog.id, { name: value, query: ctx.query || actionDialog.previousQuery, sort: ctx.sort, order: ctx.order });
      } else if (actionDialog.kind === 'save-delete') {
        await ctx.remove(actionDialog.id);
      } else if (actionDialog.kind === 'bulk-selected') {
        const selector = await ensureSelectionReady();
        const changed = await tagWorkflow.bulkSelected(selector, value, 'add', (variables) => tagMutation.mutateAsync(variables));
        if (changed) library.clearSelection();
      } else if (actionDialog.kind === 'bulk-remove-selected') {
        const selector = await ensureSelectionReady();
        const changed = await tagWorkflow.bulkSelected(selector, value, 'remove', (variables) => tagMutation.mutateAsync(variables));
        if (changed) library.clearSelection();
      } else if (actionDialog.kind === 'bulk-untrack-selected' || actionDialog.kind === 'bulk-delete-selected' || actionDialog.kind === 'bulk-delete-or-untrack-selected') {
        const selector = await ensureSelectionReady();
        await filesRemovalMutation.mutateAsync({
          ...selector,
          mode: actionDialog.kind === 'bulk-delete-selected'
            ? 'delete'
            : actionDialog.kind === 'bulk-delete-or-untrack-selected'
              ? 'delete_or_untrack'
              : 'untrack'
        });
        library.clearSelection();
      } else if (actionDialog.kind === 'untrack-file' || actionDialog.kind === 'delete-file') {
        await fileRemovalMutation.mutateAsync({ id: actionDialog.id, mode: actionDialog.kind === 'delete-file' ? 'delete' : 'untrack' });
        if (library.activeFile?.id === actionDialog.id) library.closePreview();
        for (let index = upload.items.length - 1; index >= 0; index -= 1) {
          if (upload.items[index]?.remoteFileID === actionDialog.id) upload.removeAt(index);
        }
      }
      closeActionDialog();
    } catch (error) {
      if (error instanceof ApiError && error.code === 'file_not_managed' && actionDialog.kind === 'bulk-delete-selected') {
        actionDialog = { ...actionDialog, kind: 'bulk-delete-or-untrack-selected', busy: false, error: '' };
        return;
      }
      actionDialog = { ...actionDialog, busy: false, error: errorMessage(error) };
    }
  }

  function beginTrackingUploadResults() {
    if (trackUploadResults) return;
    trackUploadResults = true;
    uploadResultsFloor = gridSnapshotTotalCount;
  }

  async function refreshUploadMetadata(forceAfterCurrent = false) {
    if (uploadMetadataRefreshPromise) {
      const currentResult = await uploadMetadataRefreshPromise;
      if (!forceAfterCurrent) return currentResult;
    }

    const run = (async () => {
      let currentViewTotal: number | undefined;
      const requests: Promise<unknown>[] = [
        tagsQuery.refetch(),
        kindFacetsQuery.refetch(),
        jobsQuery.refetch()
      ];
      if (trackUploadResults) {
        requests.push(uploadResultsCountQuery.refetch().then((result) => {
          currentViewTotal = result.data?.total_count;
        }));
      }
      await Promise.allSettled(requests);
      return currentViewTotal;
    })();

    uploadMetadataRefreshPromise = run;
    try {
      return await run;
    } finally {
      if (uploadMetadataRefreshPromise === run) uploadMetadataRefreshPromise = null;
    }
  }

  async function finishUploadMetadataRefresh() {
    stopUploadMetadataRefresh();
    const finalTotal = await refreshUploadMetadata(true);
    if (!upload.busy && !upload.activeJobIDs.length && finalTotal != null && finalTotal <= uploadResultsBaseline) {
      trackUploadResults = false;
    }
    return finalTotal;
  }

  function startUploadMetadataRefresh() {
    beginTrackingUploadResults();
    if (uploadMetadataRefreshTimer) return;
    uploadMetadataRefreshTimer = window.setInterval(() => void refreshUploadMetadata(), uploadMetadataRefreshIntervalMs);
  }

  function stopUploadMetadataRefresh() {
    if (!uploadMetadataRefreshTimer) return;
    window.clearInterval(uploadMetadataRefreshTimer);
    uploadMetadataRefreshTimer = undefined;
  }

  function reconcilePendingUploadItemTags() {
    upload.items.forEach((item, index) => {
      if (item.tagSyncPending && item.remoteFileID) void reconcileUploadItemTags(index);
    });
  }

  function reconcileUploadItemTags(index: number) {
    const item = upload.items[index];
    if (!item?.tagSyncPending || !item.remoteFileID) return;

    void uploadTagReconciliation.run(item, async () => {
      const currentIndex = upload.items[index] === item ? index : upload.items.indexOf(item);
      if (currentIndex < 0 || !item.tagSyncPending || !item.remoteFileID || item.tagSyncError) return false;

      const delta = upload.itemTagSyncDelta(currentIndex);
      const operation = delta.add.length ? 'add' : delta.remove.length ? 'remove' : null;
      if (!operation) {
        upload.markItemTagSyncApplied(currentIndex, 'add', []);
        return false;
      }

      const changedTags = operation === 'add' ? delta.add : delta.remove;
      const remoteFileID = item.remoteFileID;
      try {
        await new ApiClient($authState.csrfToken).mutateTags(operation, { file_ids: [remoteFileID], tags: changedTags });
        const appliedIndex = upload.items[index] === item ? index : upload.items.indexOf(item);
        if (appliedIndex < 0) return false;
        upload.markItemTagSyncApplied(appliedIndex, operation, changedTags);
        return item.tagSyncPending && Boolean(item.remoteFileID) && !item.tagSyncError;
      } catch (error) {
        const failedIndex = upload.items[index] === item ? index : upload.items.indexOf(item);
        if (failedIndex >= 0) upload.markItemTagSyncError(failedIndex, errorMessage(error));
        return false;
      }
    });
  }

  function setUploadItemTags(index: number, nextTags: string[]) {
    upload.setItemTags(index, nextTags);
    void reconcileUploadItemTags(index);
  }

  function rebaseUploadItemTagsFromRemote(index: number, remoteTags: string[], expectedBaseTags: string[] | undefined) {
    if (!upload.rebaseItemTagsFromRemote(index, remoteTags, expectedBaseTags)) return;
    void reconcileUploadItemTags(index);
  }

  async function submitUpload() {
    if (!upload.files.length) return;
    cancelRequestedJobID = '';
    startUploadMetadataRefresh();
    try {
      const result = await upload.submit((variables) => uploadMutation.mutateAsync(variables));
      reconcilePendingUploadItemTags();
      if (result.changedFiles) beginTrackingUploadResults();
      if (result.queued) void jobsQuery.refetch();
    } finally {
      if (!upload.busy && !upload.activeJobIDs.length) await finishUploadMetadataRefresh();
      uploadMutation.reset();
    }
  }

  async function refreshUploadResults() {
    uploadResultsFloor = Math.max(uploadResultsFloor, liveCurrentQueryCount);
    if (!upload.busy) trackUploadResults = false;
    await refreshUploadQueries(queryClient);
  }

  function selectUploadTarget(value: string) {
    const target = (uploadTargetsQuery.data?.items ?? []).find((candidate) => candidate.id === value);
    upload.setTarget(value, target?.added_at_strategy ?? 'queue', target?.default_tags ?? []);
  }

  function selectUploadFiles(files: FileList | File[] | null) {
    upload.select(files);
    if (upload.autoUpload && files && Array.from(files).length) {
      queueMicrotask(() => void submitUpload());
    }
  }

  async function cancelUploadJob(jobID = upload.activeJobID) {
    cancelRequestedJobID = jobID;
    const result = await upload.cancel((id) => cancelJobMutation.mutateAsync(id), jobID);
    if (result.changed) {
      void jobsQuery.refetch();
      if (!upload.busy && !upload.activeJobIDs.length) void finishUploadMetadataRefresh();
    }
  }

  async function cancelJob(job: Job) {
    await cancelJobMutation.mutateAsync(job.id);
  }


  function actionDialogTitle() {
    switch (actionDialog.kind) {
      case 'save-create': return 'Save search';
      case 'save-update': return 'Update saved search';
      case 'save-delete': return 'Delete saved search';
      case 'bulk-selected': return 'Tag selected files';
      case 'bulk-remove-selected': return 'Untag selected files';
      case 'bulk-untrack-selected': return 'Untrack selected files';
      case 'bulk-delete-selected': return 'Delete selected files';
      case 'bulk-delete-or-untrack-selected': return 'Delete managed files and untrack external files?';
      case 'untrack-file': return 'Remove from library';
      case 'delete-file': return 'Delete file';
      default: return '';
    }
  }

  function actionDialogDescription() {
    switch (actionDialog.kind) {
      case 'save-create': return 'Name the current search so it stays available in the sidebar.';
      case 'save-update': return `Update "${actionDialog.name}" with the current search and sort.`;
      case 'save-delete': return `Delete "${actionDialog.name}" from saved searches.`;
      case 'bulk-selected': return `Add tags to ${selectedCount} selected file${selectedCount === 1 ? '' : 's'}.`;
      case 'bulk-remove-selected': return `Remove tags from ${selectedCount} selected file${selectedCount === 1 ? '' : 's'}.`;
      case 'bulk-untrack-selected': return `Untrack ${selectedCount} selected file${selectedCount === 1 ? '' : 's'} from the library. Files remain on disk.`;
      case 'bulk-delete-selected': return `Permanently delete ${selectedCount} selected file${selectedCount === 1 ? '' : 's'} from disk and remove them from the library. Only files in managed upload targets can be deleted.`;
      case 'bulk-delete-or-untrack-selected': return 'Some selected files are outside configured upload targets. Delete managed files and untrack the external files? External files will remain on disk.';
      case 'untrack-file': return `Untrack \"${actionDialog.name}\" from the library. The file remains on disk.`;
      case 'delete-file': return `Permanently delete \"${actionDialog.name}\" from disk and remove it from the library.`;
      default: return '';
    }
  }

  function actionDialogLabel() {
    return isBulkTagDialog() ? 'Tags' : 'Name';
  }

  function actionDialogConfirmText() {
    switch (actionDialog.kind) {
      case 'save-delete': return 'Delete';
      case 'bulk-selected': return 'Add tags';
      case 'bulk-remove-selected': return 'Remove tags';
      case 'bulk-untrack-selected': return 'Untrack';
      case 'bulk-delete-selected': return 'Delete files';
      case 'bulk-delete-or-untrack-selected': return 'Delete and untrack';
      case 'untrack-file': return 'Remove';
      case 'delete-file': return 'Delete file';
      default: return 'Save';
    }
  }

  async function changePassword(currentPassword: string, newPassword: string) {
    await new ApiClient($authState.csrfToken).changePassword(currentPassword, newPassword);
  }

  async function loadNextFilesPage() {
    if (pagedMode || !filesQuery.hasNextPage || filesQuery.isFetchingNextPage) return;
    await filesQuery.fetchNextPage();
  }

  async function loadPreviousFilesPage() {
    if (pagedMode || !filesQuery.hasPreviousPage || filesQuery.isFetchingPreviousPage) return;
    await filesQuery.fetchPreviousPage();
  }

  function selectFilesPage(pageIndex: number) {
    const currentPageIndex = library.page - 1;
    if (!pagedMode || filesQuery.isFetching || pageIndex === currentPageIndex) return;
    library.page = Math.min(Math.max(0, pageIndex), Math.max(0, pagedPageCount - 1)) + 1;
  }

  function setPaginationMode(mode: PaginationMode) {
    if (mode === effectivePaginationMode) return;
    paginationModeOverride = mode;
    writeBrowserPreference(paginationPreferenceKey, mode);
    library.page = 1;
  }

  async function logout() {
    try {
      await new ApiClient($authState.csrfToken).logout();
    } finally {
      authState.set({ user: null, csrfToken: '', checked: true });
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if $authState.user}
  {@const files = loadedFiles}
  {@const page = fileMetadata}
  <GlobalFileDrop
    onFiles={(files) => {
      library.closePreview();
      setRoute('upload');
      selectUploadFiles(files);
    }}
  />
  <AppShell
    username={$authState.user.username}
    route={library.route}
    libraryCount={liveLibraryCount}
    tagCount={tagsQuery.data?.tags.length ?? 0}
    jobsActiveCount={jobsQuery.data?.active_count ?? activeJobs.length}
    jobs={(jobsQuery.data?.items ?? []).slice(0, 20)}
    jobsTotalCount={jobsQuery.data?.total_count ?? (jobsQuery.data?.items.length ?? 0)}
    jobsDrawerOpen={jobsDrawerOpen}
    kindCounts={kindFacetsQuery.data?.facets?.kind ?? tagsQuery.data?.facets?.kind ?? page?.facets?.kind ?? []}
    comicCount={comicCount}
    comicAvailable={comicAvailable}
    savedSearches={savedSearchesQuery.data?.items ?? []}
    suggestions={suggestionsQuery.data?.items ?? []}
    metaTags={suggestionsQuery.data?.meta_tags ?? []}
    tags={tagsQuery.data?.tags ?? []}
    search={$searchDraft}
    onRoute={setRoute}
    onSavedSearch={library.runSavedSearch}
    onCreateSavedSearch={createSavedSearch}
    onUpdateSavedSearch={updateSavedSearch}
    onDeleteSavedSearch={deleteSavedSearch}
    onSearchDraft={library.setSearchDraft}
    onSearchCommit={library.commitSearch}
    onJobs={toggleJobsDrawer}
    onCloseJobs={closeJobsDrawer}
    onCancelJob={cancelJob}
  >
    {#if library.route === 'upload'}
      <UploadPanel
        uploadFiles={upload.files}
        uploadItems={upload.items}
        uploadTags={upload.tags}
        uploadBusy={upload.busy}
        cancelBusy={upload.cancelBusy}
        cancelRequested={Boolean(cancelRequestedJobID)}
        uploadStatus={upload.status}
        activeUploadJobID={upload.activeJobID}
        targets={uploadTargetsQuery.data?.items ?? []}
        targetID={upload.targetID}
        addedAtStrategy={upload.addedAtStrategy}
        autoUpload={upload.autoUpload}
        tags={tagsQuery.data?.tags ?? []}
        stagedTagCandidates={upload.stagedTagCandidates}
        onTargetInput={selectUploadTarget}
        onFiles={selectUploadFiles}
        onTagsInput={(value) => (upload.tags = value)}
        onItemTagsInput={setUploadItemTags}
        onItemRemoteTagsLoaded={rebaseUploadItemTagsFromRemote}
        onViewerUntrack={untrackPreview}
        onViewerDelete={deletePreview}
        onViewerTagSearch={library.runTagSearch}
        onAddedAtStrategyInput={(value) => (upload.addedAtStrategy = value)}
        onAutoUploadInput={(value) => (upload.autoUpload = value)}
        onSubmit={submitUpload}
        onCancel={cancelUploadJob}
        onClear={upload.clear}
        onRemove={upload.removeAt}
      />
    {:else if library.route === 'jobs'}
      <JobsView jobs={jobsQuery.data?.items ?? []} {authScope} onCancel={cancelJob} />
    {:else if library.route === 'settings'}
      <SettingsView username={$authState.user.username} onLogout={logout} onChangePassword={changePassword} />
    {:else if library.route === 'tags'}
      <TagsView
        libraryCount={liveLibraryCount}
        onTag={library.runTagSearch}
        onNamespace={(namespace) => library.runTagSearch(`${namespace}:`)}
      />
    {:else if library.route === 'shortcuts'}
      <ShortcutsView />
    {:else}
      <MediaGrid
        sessionActive={Boolean($authState.user)}
        isLoading={filesQuery.isLoading}
        isError={filesQuery.isError}
        error={filesQuery.error}
        {files}
        retainedStartIndex={retainedStartIndex}
        totalCount={gridSnapshotTotalCount}
        displayTotalCount={liveCurrentQueryCount}
        libraryCount={liveLibraryCount}
        searchActive={Boolean($submittedSearch)}
        selectedCount={selectedCount}
        {bulkDownloadBusy}
        {bulkDownloadError}
        isSelected={library.isSelected}
        hasNextPage={Boolean(filesQuery.hasNextPage)}
        isFetchingNextPage={Boolean(filesQuery.isFetchingNextPage)}
        hasPreviousPage={Boolean(filesQuery.hasPreviousPage)}
        isFetchingPreviousPage={Boolean(filesQuery.isFetchingPreviousPage)}
        {pagedMode}
        pageNumber={library.page}
        pageCount={pagedPageCount}
        bind:loadMoreSentinel
        onOpen={library.openPreview}
        onToggleSelect={library.toggleSelect}
        onSelectAll={selectAllFiles}
        onClearSelection={library.clearSelection}
        onBulkDownload={bulkDownloadSelected}
        onBulkTag={bulkTagSelected}
        onBulkUntag={bulkUntagSelected}
        onBulkUntrack={bulkUntrackSelected}
        onBulkDelete={bulkDeleteSelected}
        onLoadMore={loadNextFilesPage}
        onLoadPrevious={loadPreviousFilesPage}
        onPage={selectFilesPage}
      >
        {#snippet actions()}
          <div class="library-head-actions">
            {#if newUploadResultCount > 0}
              <button class="g-btn g-btn-sm" type="button" data-testid="refresh-upload-results" onclick={() => void refreshUploadResults()}>
                {newUploadResultCount.toLocaleString()} new item{newUploadResultCount === 1 ? '' : 's'} · Refresh results
              </button>
            {/if}
            <div class="seg" aria-label="Library display mode">
              {#each [{ value: 'infinite', label: 'Infinite' }, { value: 'paged', label: 'Paged' }] as option}
                <button
                  class:active={effectivePaginationMode === option.value}
                  type="button"
                  aria-pressed={effectivePaginationMode === option.value}
                  onclick={() => setPaginationMode(option.value as PaginationMode)}
                >
                  {option.label}
                </button>
              {/each}
            </div>
            <div class="seg" aria-label="Sort field">
              {#each [{ value: 'added', label: 'Added' }, { value: 'name', label: 'Name' }, { value: 'size', label: 'Size' }] as option}
                <button
                  class:active={library.sort === option.value}
                  type="button"
                  onclick={() => (library.sort = option.value as FileSort)}
                >
                  {option.label}
                </button>
              {/each}
            </div>
            <button class="g-btn g-btn-sm" type="button" title="Sort direction" onclick={() => (library.order = library.order === 'desc' ? 'asc' : 'desc')}>
              <Icon name="sort" size={14} /> {library.order === 'desc' ? 'Newest' : 'Oldest'}
            </button>
          </div>
        {/snippet}
      </MediaGrid>
    {/if}
  </AppShell>

  {#if bulkDownloadURL}
    <iframe title="Bulk download" src={bulkDownloadURL} aria-hidden="true" tabindex="-1" style="display: none"></iframe>
  {/if}

  {#if library.activeFile}
    <PreviewDialog
      file={library.activeFile}
      preloadPrev={previewNeighbor(library.activeFile, files, -1) ?? undefined}
      preloadNext={previewNeighbor(library.activeFile, files, 1) ?? undefined}
      tagDraft={tagWorkflow.drafts[library.activeFile.id] ?? ''}
      tagBusy={Boolean(tagWorkflow.busy[library.activeFile.id])}
      tagError={tagWorkflow.errors[library.activeFile.id] ?? ''}
      tags={tagsQuery.data?.tags ?? []}
      onClose={library.closePreview}
      onPrev={() => library.movePreview(-1, files)}
      onNext={() => library.movePreview(1, files)}
      onNestedNavigationChange={(active) => (nestedPreviewNavigation = active)}
      onTagInput={tagWorkflow.updateDraft}
      onMutateTags={(file, operation, value) => value == null
        ? tagWorkflow.mutateFile(file, operation, (variables) => tagMutation.mutateAsync(variables))
        : tagWorkflow.mutateFileTags(file, operation, value, (variables) => tagMutation.mutateAsync(variables))}
      onRemoveTag={(file, tag) => tagWorkflow.removeTag(file, tag, (variables) => tagMutation.mutateAsync(variables))}
      onTagSearch={library.runTagSearch}
      onUntrack={untrackPreview}
      onDelete={deletePreview}
    />
  {/if}

  {#if fileTagDialog.file}
    <FileTagDialog
      file={fileTagDialog.file}
      tagCandidates={tagsQuery.data?.tags ?? []}
      initialMode={fileTagDialog.mode}
      busy={fileTagDialog.busy}
      error={fileTagDialog.error}
      onCancel={closeFileTagDialog}
      onConfirm={submitFileTagDialog}
    />
  {/if}

  {#if actionDialog.kind !== 'none'}
    <ActionDialog
      title={actionDialogTitle()}
      description={actionDialogDescription()}
      label={actionDialogLabel()}
      value={actionDialog.value}
      confirmText={actionDialogConfirmText()}
      destructive={actionDialog.kind === 'save-delete' || actionDialog.kind === 'bulk-untrack-selected' || actionDialog.kind === 'bulk-delete-selected' || actionDialog.kind === 'bulk-delete-or-untrack-selected' || actionDialog.kind === 'untrack-file' || actionDialog.kind === 'delete-file'}
      busy={actionDialog.busy}
      error={actionDialog.error}
      input={actionDialog.kind !== 'save-delete' && actionDialog.kind !== 'bulk-untrack-selected' && actionDialog.kind !== 'bulk-delete-selected' && actionDialog.kind !== 'bulk-delete-or-untrack-selected' && actionDialog.kind !== 'untrack-file' && actionDialog.kind !== 'delete-file'}
      tagInput={isBulkTagDialog()}
      tagCandidates={tagsQuery.data?.tags ?? []}
      onInput={(value) => (actionDialog = { ...actionDialog, value, error: '' })}
      onCancel={closeActionDialog}
      onConfirm={submitActionDialog}
    />
  {/if}
{/if}