<script lang="ts">
  import AppShell from '$lib/components/AppShell.svelte';
  import AccountView from '$lib/components/AccountView.svelte';
  import ActionDialog from '$lib/components/ActionDialog.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import JobsView from '$lib/components/JobsView.svelte';
  import MediaGrid from '$lib/components/MediaGrid.svelte';
  import PreviewDialog from '$lib/components/PreviewDialog.svelte';
  import SettingsView from '$lib/components/SettingsView.svelte';
  import ShortcutsView from '$lib/components/ShortcutsView.svelte';
  import TagsView from '$lib/components/TagsView.svelte';
  import UploadPanel from '$lib/components/UploadPanel.svelte';
  import { ApiClient } from '$lib/api/client';
  import { authState } from '$lib/stores/auth';
  import { createFilesQuery, createTagMutation, createUntrackFileMutation, pageTokenOffset, type FileSort } from '$lib/queries/files';
  import { createCancelJobMutation, createClearJobsMutation, createJobQuery, createJobsQuery } from '$lib/queries/jobs';
  import {
    createSavedSearchCreateMutation,
    createSavedSearchDeleteMutation,
    createSavedSearchesQuery,
    createSavedSearchUpdateMutation,
    createSuggestionsQuery,
    createTagsQuery,
    createUploadMutation,
    createUploadTargetsQuery
  } from '$lib/queries/library';
  import { createLibraryWorkflow } from '$lib/state/libraryWorkflow.svelte';
  import { createTagWorkflow } from '$lib/state/tagWorkflow.svelte';
  import { createUploadWorkflow } from '$lib/state/uploadWorkflow.svelte';
  import { errorMessage } from '$lib/utils/format';
  import { useQueryClient } from '@tanstack/svelte-query';
  import type { Job, SavedSearchRequest } from '$lib/api/types';

  const queryClient = useQueryClient();
  const library = createLibraryWorkflow();
  const searchDraft = library.searchDraft;
  const submittedSearch = library.submittedSearch;
  const suggestionSearch = library.suggestionSearch;
  const tagWorkflow = createTagWorkflow();
  const upload = createUploadWorkflow();

  let authScope = $state(0);
  let observedCSRF = $state('');
  let loadMoreSentinel = $state<HTMLDivElement | undefined>();
  let cancelRequestedJobID = $state('');
  let jobsDrawerOpen = $state(false);
  let fileMetadata = $state<{
    total_count: number;
    library_count: number;
    facets?: { kind?: Array<{ value: string; count: number }> };
  } | null>(null);
  let actionDialog = $state<{
    kind: 'none' | 'save-create' | 'save-update' | 'save-delete' | 'bulk-selected' | 'bulk-remove-selected' | 'bulk-filtered' | 'untrack-file';
    value: string;
    error: string;
    busy: boolean;
    id: string;
    name: string;
    previousQuery: string;
  }>({ kind: 'none', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' });

  const filesQuery = createFilesQuery(
    () => Boolean($authState.user),
    () => $submittedSearch,
    () => library.activeKind,
    () => library.sort,
    () => library.order,
    () => authScope
  );
  const uploadJobQuery = createJobQuery(() => $authState.csrfToken, () => upload.activeJobID, () => authScope);
  const jobsQuery = createJobsQuery(() => Boolean($authState.user), () => authScope);
  const savedSearchesQuery = createSavedSearchesQuery(() => Boolean($authState.user), () => authScope);
  const tagsQuery = createTagsQuery(() => Boolean($authState.user), () => authScope);
  const uploadTargetsQuery = createUploadTargetsQuery(() => Boolean($authState.user), () => authScope);
  const suggestionsQuery = createSuggestionsQuery(() => Boolean($authState.user), () => $suggestionSearch, () => $submittedSearch, () => authScope);

  const tagMutation = createTagMutation(() => $authState.csrfToken, queryClient);
  const untrackFileMutation = createUntrackFileMutation(() => $authState.csrfToken, queryClient);
  const uploadMutation = createUploadMutation(() => $authState.csrfToken, queryClient);
  const cancelJobMutation = createCancelJobMutation(() => $authState.csrfToken, queryClient);
  const clearJobsMutation = createClearJobsMutation(() => $authState.csrfToken, queryClient);
  const createSavedSearchMutation = createSavedSearchCreateMutation(() => $authState.csrfToken, queryClient);
  const updateSavedSearchMutation = createSavedSearchUpdateMutation(() => $authState.csrfToken, queryClient);
  const deleteSavedSearchMutation = createSavedSearchDeleteMutation(() => $authState.csrfToken, queryClient);

  const loadedFiles = $derived(filesQuery.data?.pages.flatMap((page) => page.files) ?? []);
  const retainedStartIndex = $derived(pageTokenOffset(String(filesQuery.data?.pageParams[0] ?? '')));
  const activeJobs = $derived((jobsQuery.data?.items ?? []).filter((job) => job.status === 'pending' || job.status === 'running'));
  const fileMetadataKey = $derived(`${authScope}|${$submittedSearch}|${library.activeKind}|${library.sort}|${library.order}`);

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
    cancelRequestedJobID = '';
    jobsDrawerOpen = false;
    closeActionDialog();
  });

  $effect(() => {
    fileMetadataKey;
    fileMetadata = null;
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
    if (result.completed) void jobsQuery.refetch();
  });

  $effect(() => {
    if (uploadJobQuery.isError) upload.applyJobError(uploadJobQuery.error);
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

  function isTypingTarget(target: EventTarget | null) {
    if (!(target instanceof HTMLElement)) return false;
    return target.isContentEditable || target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT';
  }

  function handleKeydown(event: KeyboardEvent) {
    const shortcutsKey = event.key === '?' || (event.code === 'Slash' && event.shiftKey);
    if (shortcutsKey && !event.altKey && !event.ctrlKey && !event.metaKey && !isTypingTarget(event.target)) {
      event.preventDefault();
      setRoute('shortcuts');
      return;
    }
    library.handleKeydown(event, loadedFiles);
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

  function bulkTagSelected() {
    actionDialog = { kind: 'bulk-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function bulkUntagSelected() {
    actionDialog = { kind: 'bulk-remove-selected', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  function bulkTagFiltered() {
    actionDialog = { kind: 'bulk-filtered', value: '', error: '', busy: false, id: '', name: '', previousQuery: '' };
  }

  async function submitActionDialog() {
    if (actionDialog.busy || actionDialog.kind === 'none') return;
    const ctx = savedSearchContext();
    const value = actionDialog.value.trim();
    if ((actionDialog.kind === 'save-create' || actionDialog.kind === 'save-update' || actionDialog.kind.startsWith('bulk-')) && !value) {
      actionDialog = { ...actionDialog, error: actionDialog.kind.startsWith('bulk-') ? 'Enter at least one tag.' : 'Enter a name.' };
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
        const changed = await tagWorkflow.bulkSelected(library.selectedIDs, value, 'add', (variables) => tagMutation.mutateAsync(variables));
        if (changed) library.clearSelection();
      } else if (actionDialog.kind === 'bulk-remove-selected') {
        const changed = await tagWorkflow.bulkSelected(library.selectedIDs, value, 'remove', (variables) => tagMutation.mutateAsync(variables));
        if (changed) library.clearSelection();
      } else if (actionDialog.kind === 'bulk-filtered') {
        await tagWorkflow.bulkFiltered(library.filterQuery(), value, (variables) => tagMutation.mutateAsync(variables));
      } else if (actionDialog.kind === 'untrack-file') {
        await untrackFileMutation.mutateAsync(actionDialog.id);
        if (library.activeFile?.id === actionDialog.id) library.closePreview();
      }
      closeActionDialog();
    } catch (error) {
      actionDialog = { ...actionDialog, busy: false, error: errorMessage(error) };
    }
  }

  async function submitUpload() {
    cancelRequestedJobID = '';
    try {
      const result = await upload.submit((variables) => uploadMutation.mutateAsync(variables));
      if (result.queued) void jobsQuery.refetch();
    } finally {
      uploadMutation.reset();
    }
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
    if (result.changed) void jobsQuery.refetch();
  }

  async function cancelJob(job: Job) {
    await cancelJobMutation.mutateAsync(job.id);
  }

  async function clearCompletedJobs() {
    await clearJobsMutation.mutateAsync('completed');
  }

  function actionDialogTitle() {
    switch (actionDialog.kind) {
      case 'save-create': return 'Save search';
      case 'save-update': return 'Update saved search';
      case 'save-delete': return 'Delete saved search';
      case 'bulk-selected': return 'Tag selected files';
      case 'bulk-remove-selected': return 'Untag selected files';
      case 'bulk-filtered': return 'Tag filtered results';
      case 'untrack-file': return 'Remove from library';
      default: return '';
    }
  }

  function actionDialogDescription() {
    switch (actionDialog.kind) {
      case 'save-create': return 'Name the current search so it stays available in the sidebar.';
      case 'save-update': return `Update "${actionDialog.name}" with the current search and sort.`;
      case 'save-delete': return `Delete "${actionDialog.name}" from saved searches.`;
      case 'bulk-selected': return `Add tags to ${library.selectedIDs.size} selected file${library.selectedIDs.size === 1 ? '' : 's'}.`;
      case 'bulk-remove-selected': return `Remove tags from ${library.selectedIDs.size} selected file${library.selectedIDs.size === 1 ? '' : 's'}.`;
      case 'bulk-filtered': return 'Add tags to every file matching the current filter without materializing all results.';
      case 'untrack-file': return `Untrack \"${actionDialog.name}\" from the library. The file remains on disk.`;
      default: return '';
    }
  }

  function actionDialogLabel() {
    return actionDialog.kind.startsWith('bulk-') ? 'Tags' : 'Name';
  }

  function actionDialogConfirmText() {
    switch (actionDialog.kind) {
      case 'save-delete': return 'Delete';
      case 'bulk-selected':
      case 'bulk-filtered': return 'Add tags';
      case 'bulk-remove-selected': return 'Remove tags';
      case 'untrack-file': return 'Remove';
      default: return 'Save';
    }
  }

  async function changePassword(currentPassword: string, newPassword: string) {
    await new ApiClient($authState.csrfToken).changePassword(currentPassword, newPassword);
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
  <AppShell
    username={$authState.user.username}
    route={library.route}
    activeKind={library.activeKind}
    libraryCount={page?.library_count ?? files.length}
    tagCount={tagsQuery.data?.tags.length ?? 0}
    jobsActiveCount={activeJobs.length}
    jobs={jobsQuery.data?.items ?? []}
    jobsDrawerOpen={jobsDrawerOpen}
    kindCounts={page?.facets?.kind ?? []}
    savedSearches={savedSearchesQuery.data?.items ?? []}
    suggestions={suggestionsQuery.data?.items ?? []}
    tags={tagsQuery.data?.tags ?? []}
    search={$searchDraft}
    onRoute={setRoute}
    onKind={library.setKind}
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
        conflictPolicy={upload.conflictPolicy}
        autoUpload={upload.autoUpload}
        onTargetInput={upload.setTarget}
        onFiles={selectUploadFiles}
        onTagsInput={(value) => (upload.tags = value)}
        onConflictInput={(value) => (upload.conflictPolicy = value)}
        onAutoUploadInput={(value) => (upload.autoUpload = value)}
        onSubmit={submitUpload}
        onCancel={cancelUploadJob}
        onClear={upload.clear}
        onRemove={upload.removeAt}
      />
    {:else if library.route === 'jobs'}
      <JobsView jobs={jobsQuery.data?.items ?? []} onCancel={cancelJob} onClearCompleted={clearCompletedJobs} />
    {:else if library.route === 'settings'}
      <SettingsView username={$authState.user.username} onLogout={logout} onChangePassword={changePassword} />
    {:else if library.route === 'tags'}
      <TagsView
        tags={tagsQuery.data?.tags ?? []}
        libraryCount={page?.library_count ?? files.length}
        loading={tagsQuery.isLoading}
        error={tagsQuery.isError ? errorMessage(tagsQuery.error) : ''}
        onTag={library.runTagSearch}
        onNamespace={(namespace) => library.runTagSearch(`${namespace}:`)}
      />
    {:else if library.route === 'account'}
      <AccountView username={$authState.user.username} onLogout={logout} />
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
        totalCount={page?.total_count ?? files.length}
        libraryCount={page?.library_count ?? files.length}
        searchActive={Boolean($submittedSearch || library.activeKind)}
        selectedIDs={library.selectedIDs}
        hasNextPage={Boolean(filesQuery.hasNextPage)}
        isFetchingNextPage={Boolean(filesQuery.isFetchingNextPage)}
        hasPreviousPage={Boolean(filesQuery.hasPreviousPage)}
        isFetchingPreviousPage={Boolean(filesQuery.isFetchingPreviousPage)}
        bind:loadMoreSentinel
        onOpen={library.openPreview}
        onToggleSelect={library.toggleSelect}
        onSelectAll={() => library.selectFiles(files)}
        onClearSelection={library.clearSelection}
        onBulkTag={bulkTagSelected}
        onBulkUntag={bulkUntagSelected}
        onLoadMore={() => { if (filesQuery.hasNextPage && !filesQuery.isFetchingNextPage) void filesQuery.fetchNextPage(); }}
        onLoadPrevious={() => { if (filesQuery.hasPreviousPage && !filesQuery.isFetchingPreviousPage) void filesQuery.fetchPreviousPage(); }}
      >
        {#snippet actions()}
          <div class="library-head-actions">
            <div class="seg" aria-label="Sort field">
              {#each [{ value: 'modified', label: 'Modified' }, { value: 'name', label: 'Name' }, { value: 'size', label: 'Size' }] as option}
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
            {#if library.filterQuery()}
              <button class="g-btn g-btn-sm" type="button" title="Tag every matching file without materializing all results" onclick={bulkTagFiltered}>
                <Icon name="tag" size={14} /> Tag filtered
              </button>
            {/if}
          </div>
        {/snippet}
      </MediaGrid>
    {/if}
  </AppShell>

  {#if library.activeFile}
    <PreviewDialog
      file={library.activeFile}
      tagDraft={tagWorkflow.drafts[library.activeFile.id] ?? ''}
      tagBusy={Boolean(tagWorkflow.busy[library.activeFile.id])}
      tagError={tagWorkflow.errors[library.activeFile.id] ?? ''}
      onClose={library.closePreview}
      onPrev={() => library.movePreview(-1, files)}
      onNext={() => library.movePreview(1, files)}
      onTagInput={tagWorkflow.updateDraft}
      onMutateTags={(file, operation) => tagWorkflow.mutateFile(file, operation, (variables) => tagMutation.mutateAsync(variables))}
      onRemoveTag={(file, tag) => tagWorkflow.removeTag(file, tag, (variables) => tagMutation.mutateAsync(variables))}
      onUntrack={untrackPreview}
    />
  {/if}

  {#if actionDialog.kind !== 'none'}
    <ActionDialog
      title={actionDialogTitle()}
      description={actionDialogDescription()}
      label={actionDialogLabel()}
      value={actionDialog.value}
      confirmText={actionDialogConfirmText()}
      destructive={actionDialog.kind === 'save-delete' || actionDialog.kind === 'untrack-file'}
      busy={actionDialog.busy}
      error={actionDialog.error}
      input={actionDialog.kind !== 'save-delete' && actionDialog.kind !== 'untrack-file'}
      onInput={(value) => (actionDialog = { ...actionDialog, value, error: '' })}
      onCancel={closeActionDialog}
      onConfirm={submitActionDialog}
    />
  {/if}
{/if}
