<script lang="ts">
  import AppShell from '$lib/components/AppShell.svelte';
  import AccountView from '$lib/components/AccountView.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import JobsView from '$lib/components/JobsView.svelte';
  import MediaGrid from '$lib/components/MediaGrid.svelte';
  import PreviewDialog from '$lib/components/PreviewDialog.svelte';
  import ShortcutsView from '$lib/components/ShortcutsView.svelte';
  import TagsView from '$lib/components/TagsView.svelte';
  import UploadPanel from '$lib/components/UploadPanel.svelte';
  import { createSavedSearch as createSavedSearchAction, deleteSavedSearch as deleteSavedSearchAction, updateSavedSearch as updateSavedSearchAction } from '$lib/actions/savedSearches';
  import { ApiClient } from '$lib/api/client';
  import { authState } from '$lib/stores/auth';
  import { createFilesQuery, createTagMutation, type FileSort } from '$lib/queries/files';
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
  import { createViewportState } from '$lib/state/viewport.svelte';
  import { virtualGrid } from '$lib/state/ui';
  import { errorMessage } from '$lib/utils/format';
  import { useQueryClient } from '@tanstack/svelte-query';
  import type { Job, SavedSearchRequest } from '$lib/api/types';

  const queryClient = useQueryClient();
  const library = createLibraryWorkflow();
  const searchDraft = library.searchDraft;
  const submittedSearch = library.submittedSearch;
  const tagWorkflow = createTagWorkflow();
  const upload = createUploadWorkflow();
  const viewport = createViewportState();

  let authScope = $state(0);
  let observedCSRF = $state('');
  let loadMoreSentinel = $state<HTMLDivElement | undefined>();
  let cancelRequestedJobID = $state('');
  let fileMetadata = $state<{
    total_count: number;
    library_count: number;
    facets?: { kind?: Array<{ value: string; count: number }> };
  } | null>(null);

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
  const suggestionsQuery = createSuggestionsQuery(() => Boolean($authState.user), () => $searchDraft, () => $submittedSearch, () => authScope);

  const tagMutation = createTagMutation(() => $authState.csrfToken, queryClient);
  const uploadMutation = createUploadMutation(() => $authState.csrfToken, queryClient);
  const cancelJobMutation = createCancelJobMutation(() => $authState.csrfToken, queryClient);
  const clearJobsMutation = createClearJobsMutation(() => $authState.csrfToken, queryClient);
  const createSavedSearchMutation = createSavedSearchCreateMutation(() => $authState.csrfToken, queryClient);
  const updateSavedSearchMutation = createSavedSearchUpdateMutation(() => $authState.csrfToken, queryClient);
  const deleteSavedSearchMutation = createSavedSearchDeleteMutation(() => $authState.csrfToken, queryClient);

  const loadedFiles = $derived(filesQuery.data?.pages.flatMap((page) => page.files) ?? []);
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
    const node = loadMoreSentinel;
    if (!node || !$authState.user) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting) && filesQuery.hasNextPage && !filesQuery.isFetchingNextPage) {
          void filesQuery.fetchNextPage();
        }
      },
      { rootMargin: '900px 0px' }
    );
    observer.observe(node);
    return () => observer.disconnect();
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

  function handleKeydown(event: KeyboardEvent) {
    library.handleKeydown(event, loadedFiles);
  }

  async function createSavedSearch() {
    await createSavedSearchAction(savedSearchContext(), library.activeSavedSearch);
  }

  async function updateSavedSearch(id: string, name: string, previousQuery: string) {
    await updateSavedSearchAction(savedSearchContext(), id, name, previousQuery);
  }

  async function deleteSavedSearch(id: string, name: string) {
    await deleteSavedSearchAction(savedSearchContext(), id, name);
  }

  async function bulkTagSelected() {
    const changed = await tagWorkflow.bulkSelected(library.selectedIDs, (variables) => tagMutation.mutateAsync(variables));
    if (changed) library.clearSelection();
  }

  async function bulkTagFiltered() {
    await tagWorkflow.bulkFiltered(library.filterQuery(), (variables) => tagMutation.mutateAsync(variables));
  }

  async function submitUpload() {
    cancelRequestedJobID = '';
    const result = await upload.submit((variables) => uploadMutation.mutateAsync(variables));
    if (result.queued) void jobsQuery.refetch();
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
    kindCounts={page?.facets?.kind ?? []}
    savedSearches={savedSearchesQuery.data?.items ?? []}
    suggestions={suggestionsQuery.data?.items ?? []}
    search={$searchDraft}
    onRoute={library.setRoute}
    onKind={library.setKind}
    onSavedSearch={library.runSavedSearch}
    onCreateSavedSearch={createSavedSearch}
    onUpdateSavedSearch={updateSavedSearch}
    onDeleteSavedSearch={deleteSavedSearch}
    onSuggestion={library.applySuggestion}
    onSearchInput={library.setSearch}
    onSearchSubmit={library.submitSearch}
    onJobs={library.toggleJobsRoute}
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
        onTargetInput={upload.setTarget}
        onFiles={upload.select}
        onTagsInput={(value) => (upload.tags = value)}
        onSubmit={submitUpload}
        onCancel={cancelUploadJob}
        onClear={upload.clear}
      />
    {:else if library.route === 'jobs'}
      <JobsView jobs={jobsQuery.data?.items ?? []} onCancel={cancelJob} onClearCompleted={clearCompletedJobs} />
    {:else if library.route === 'tags'}
      <TagsView
        tags={tagsQuery.data?.tags ?? []}
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
        virtual={virtualGrid(files, viewport.width, viewport.height, viewport.scrollY)}
        totalCount={page?.total_count ?? files.length}
        libraryCount={page?.library_count ?? files.length}
        searchActive={Boolean($submittedSearch || library.activeKind)}
        selectedIDs={library.selectedIDs}
        hasNextPage={Boolean(filesQuery.hasNextPage)}
        isFetchingNextPage={Boolean(filesQuery.isFetchingNextPage)}
        bind:loadMoreSentinel
        onOpen={library.openPreview}
        onToggleSelect={library.toggleSelect}
        onSelectAll={() => library.selectFiles(files)}
        onClearSelection={library.clearSelection}
        onBulkTag={bulkTagSelected}
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
    />
  {/if}
{/if}
