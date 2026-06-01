<script lang="ts">
  import { onMount } from 'svelte';
  import { writable } from 'svelte/store';
  import { useQueryClient } from '@tanstack/svelte-query';
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
  import { createFilesQuery, type FileSort, type SortOrder } from '$lib/queries/files';
  import { createJobQuery, createJobsQuery } from '$lib/queries/jobs';
  import { createSavedSearchesQuery, createSuggestionsQuery, createTagsQuery, createUploadTargetsQuery } from '$lib/queries/library';
  import { virtualGrid } from '$lib/state/ui';
  import { itemsFromJob, itemsFromResult, queuedItems, stagedUploadItems, uploadingItems, uploadSummary, type UploadItem } from '$lib/state/uploadItems';
  import { errorMessage, isTerminalJob, jobStatusText, parseTags } from '$lib/utils/format';
  import type { FileItem, Job } from '$lib/api/types';

  const searchDraft = writable('');
  const submittedSearch = writable('');
  const queryClient = useQueryClient();

  let route = $state('library');
  let activeKind = $state('');
  let activeSavedSearch = $state('');
  let sort: FileSort = $state('modified');
  let order: SortOrder = $state('desc');
  let authScope = $state(0);
  let observedCSRF = $state('');
  let loadMoreSentinel = $state<HTMLDivElement | undefined>();
  let viewportHeight = $state(900);
  let viewportWidth = $state(1200);
  let scrollY = $state(0);
  let tagDrafts = $state<Record<string, string>>({});
  let tagBusy = $state<Record<string, boolean>>({});
  let tagErrors = $state<Record<string, string>>({});
  let selectedIDs = $state(new Set<string>());
  let uploadFiles = $state<File[]>([]);
  let uploadItems = $state<UploadItem[]>([]);
  let uploadTags = $state('');
  let uploadTargetID = $state('');
  let uploadBusy = $state(false);
  let cancelBusy = $state(false);
  let uploadStatus = $state('');
  let activeUploadJobID = $state('');
  let handledUploadJobID = $state('');
  let activeFile = $state<FileItem | null>(null);
  let debounce: ReturnType<typeof setTimeout> | undefined;

  const filesQuery = createFilesQuery(
    () => Boolean($authState.user),
    () => $submittedSearch,
    () => activeKind,
    () => sort,
    () => order,
    () => authScope
  );
  const uploadJobQuery = createJobQuery(() => $authState.csrfToken, () => activeUploadJobID, () => authScope);
  const jobsQuery = createJobsQuery(() => Boolean($authState.user), () => authScope);
  const savedSearchesQuery = createSavedSearchesQuery(() => Boolean($authState.user), () => authScope);
  const tagsQuery = createTagsQuery(() => Boolean($authState.user), () => authScope);
  const uploadTargetsQuery = createUploadTargetsQuery(() => Boolean($authState.user), () => authScope);
  const suggestionsQuery = createSuggestionsQuery(() => Boolean($authState.user), () => $searchDraft, () => $submittedSearch, () => authScope);
  const loadedFiles = $derived(filesQuery.data?.pages.flatMap((page) => page.files) ?? []);
  const firstFilePage = $derived(filesQuery.data?.pages[0]);

  $effect(() => {
    const csrf = $authState.csrfToken;
    if (csrf === observedCSRF) return;
    observedCSRF = csrf;
    authScope += 1;
    queryClient.clear();
    resetAuthScopedState();
  });

  onMount(() => {
    let frame = 0;
    const updateViewport = () => {
      viewportHeight = window.innerHeight;
      viewportWidth = window.innerWidth;
      scrollY = window.scrollY;
    };
    const scheduleViewport = () => {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        updateViewport();
      });
    };
    updateViewport();
    window.addEventListener('resize', scheduleViewport);
    window.addEventListener('scroll', scheduleViewport, { passive: true });
    return () => {
      if (frame) cancelAnimationFrame(frame);
      window.removeEventListener('resize', scheduleViewport);
      window.removeEventListener('scroll', scheduleViewport);
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
    if (!job || job.id === handledUploadJobID) return;
    uploadStatus = jobStatusText(job);
    uploadItems = itemsFromJob(uploadItems, job);
    if (isTerminalJob(job)) {
      handledUploadJobID = job.id;
      activeUploadJobID = '';
      void jobsQuery.refetch();
      if (job.status === 'completed') {
        uploadFiles = [];
        uploadStatus = uploadSummary(uploadItems) || 'Import completed';
        void filesQuery.refetch();
      }
    }
  });

  $effect(() => {
    if (uploadJobQuery.isError && activeUploadJobID) {
      uploadStatus = errorMessage(uploadJobQuery.error);
      activeUploadJobID = '';
    }
  });

  function resetAuthScopedState() {
    tagDrafts = {};
    tagBusy = {};
    tagErrors = {};
    selectedIDs = new Set();
    uploadFiles = [];
    uploadItems = [];
    uploadTags = '';
    uploadBusy = false;
    cancelBusy = false;
    uploadStatus = '';
    activeUploadJobID = '';
    handledUploadJobID = '';
    activeFile = null;
  }

  function activeJobs() {
    return (jobsQuery.data?.items ?? []).filter((job) => job.status === 'pending' || job.status === 'running');
  }

  function submitSearch() {
    submittedSearch.set($searchDraft.trim());
    selectedIDs = new Set();
    route = 'library';
  }

  function filterQuery() {
    const parts = [$submittedSearch.trim()];
    if (activeKind) parts.push(`kind:${activeKind}`);
    return parts.filter(Boolean).join(' ');
  }

  function setSearch(value: string) {
    searchDraft.set(value);
    if (debounce) clearTimeout(debounce);
    debounce = setTimeout(submitSearch, 280);
  }

  function setKind(kind: string) {
    activeKind = kind;
    selectedIDs = new Set();
  }

  function runTagSearch(query: string) {
    activeKind = '';
    searchDraft.set(query);
    submittedSearch.set(query);
    selectedIDs = new Set();
    route = 'library';
  }

  function runSavedSearch(query: string, name: string) {
    activeSavedSearch = name;
    searchDraft.set(query);
    submittedSearch.set(query);
    route = 'library';
  }

  async function createSavedSearch() {
    await createSavedSearchAction(savedSearchContext(), activeSavedSearch);
  }

  async function updateSavedSearch(id: string, name: string, previousQuery: string) {
    await updateSavedSearchAction(savedSearchContext(), id, name, previousQuery);
  }

  async function deleteSavedSearch(id: string, name: string) {
    await deleteSavedSearchAction($authState.csrfToken, id, name, () => savedSearchesQuery.refetch());
  }

  function savedSearchContext() {
    return {
      csrfToken: $authState.csrfToken,
      query: filterQuery(),
      sort,
      order,
      refetch: () => savedSearchesQuery.refetch()
    };
  }

  function applySuggestion(value: string) {
    const next = [$searchDraft.trim(), value].filter(Boolean).join(' ');
    searchDraft.set(next);
    submittedSearch.set(next);
    route = 'library';
  }

  function openPreview(file: FileItem) {
    activeFile = file;
  }

  function closePreview() {
    activeFile = null;
  }

  function movePreview(delta: number) {
    const files = loadedFiles;
    if (!activeFile || !files.length) return;
    const index = files.findIndex((file) => file.id === activeFile?.id);
    activeFile = files[(index + delta + files.length) % files.length] ?? activeFile;
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && activeFile) closePreview();
    if (activeFile && event.key === 'ArrowLeft') movePreview(-1);
    if (activeFile && event.key === 'ArrowRight') movePreview(1);
  }

  function toggleSelect(file: FileItem) {
    const next = new Set(selectedIDs);
    if (next.has(file.id)) next.delete(file.id);
    else next.add(file.id);
    selectedIDs = next;
  }

  function selectLoaded() {
    selectedIDs = new Set(loadedFiles.map((file) => file.id));
  }

  function updateTagDraft(fileID: string, value: string) {
    tagDrafts = { ...tagDrafts, [fileID]: value };
  }

  async function mutateFileTags(file: FileItem, operation: 'add' | 'set' | 'remove') {
    const tags = parseTags(tagDrafts[file.id] ?? '');
    if (!tags.length) return;
    tagBusy = { ...tagBusy, [file.id]: true };
    tagErrors = { ...tagErrors, [file.id]: '' };
    try {
      await new ApiClient($authState.csrfToken).mutateTags(operation, { file_ids: [file.id], tags });
      tagDrafts = { ...tagDrafts, [file.id]: '' };
      await filesQuery.refetch();
    } catch (error) {
      tagErrors = { ...tagErrors, [file.id]: errorMessage(error) };
    } finally {
      tagBusy = { ...tagBusy, [file.id]: false };
    }
  }

  async function bulkTagSelected() {
    const tags = parseTags(window.prompt('Tags to add to selected files') ?? '');
    if (!tags.length || !selectedIDs.size) return;
    try {
      await new ApiClient($authState.csrfToken).mutateTags('add', { file_ids: Array.from(selectedIDs), tags });
      selectedIDs = new Set();
      await filesQuery.refetch();
    } catch (error) {
      window.alert(errorMessage(error));
    }
  }

  async function bulkTagFiltered() {
    const query = filterQuery();
    if (!query) return;
    const tags = parseTags(window.prompt('Tags to add to every file matching the current filter') ?? '');
    if (!tags.length) return;
    try {
      await new ApiClient($authState.csrfToken).mutateTags('add', { query, tags });
      await filesQuery.refetch();
    } catch (error) {
      window.alert(errorMessage(error));
    }
  }

  async function logout() {
    try {
      await new ApiClient($authState.csrfToken).logout();
    } finally {
      authState.set({ user: null, csrfToken: '', checked: true });
    }
  }

  function selectUploads(files: FileList | null) {
    uploadFiles = files ? Array.from(files) : [];
    uploadItems = stagedUploadItems(uploadFiles, uploadTargetID);
    uploadStatus = '';
  }

  function setUploadTarget(value: string) {
    uploadTargetID = value;
    if (uploadFiles.length) uploadItems = stagedUploadItems(uploadFiles, uploadTargetID);
  }

  async function submitUpload() {
    if (!uploadFiles.length || uploadBusy || activeUploadJobID || !$authState.user) return;
    uploadBusy = true;
    uploadStatus = 'Uploading';
    uploadItems = uploadingItems(uploadItems.length ? uploadItems : stagedUploadItems(uploadFiles, uploadTargetID));
    try {
      const client = new ApiClient($authState.csrfToken);
      const response = await client.uploadFiles(uploadFiles, parseTags(uploadTags), true, uploadTargetID);
      if ('id' in response) {
        handledUploadJobID = '';
        activeUploadJobID = response.id;
        uploadItems = queuedItems(uploadItems);
        uploadStatus = 'Queued';
        void jobsQuery.refetch();
      } else {
        uploadItems = itemsFromResult(response, uploadItems);
        uploadStatus = uploadSummary(uploadItems);
        uploadFiles = [];
        uploadTags = '';
        await filesQuery.refetch();
      }
    } catch (error) {
      uploadStatus = errorMessage(error);
      uploadItems = uploadItems.map((item) => ({ ...item, status: 'error', progress: 100, error: uploadStatus }));
    } finally {
      uploadBusy = false;
    }
  }

  async function cancelUploadJob(jobID = activeUploadJobID) {
    if (!jobID || cancelBusy || !$authState.user) return;
    cancelBusy = true;
    uploadStatus = 'Canceled';
    uploadItems = uploadItems.map((item) => ({ ...item, status: 'canceled', progress: item.progress || 100 }));
    handledUploadJobID = jobID;
    activeUploadJobID = '';
    try {
      const job = await new ApiClient($authState.csrfToken).cancelJob(jobID);
      uploadStatus = jobStatusText(job);
      uploadItems = itemsFromJob(uploadItems, job);
      if (isTerminalJob(job)) {
        handledUploadJobID = job.id;
      } else {
        activeUploadJobID = job.id;
        await uploadJobQuery.refetch();
      }
      void jobsQuery.refetch();
    } catch (error) {
      uploadStatus = errorMessage(error);
    } finally {
      cancelBusy = false;
    }
  }

  async function cancelJob(job: Job) {
    await new ApiClient($authState.csrfToken).cancelJob(job.id);
    await jobsQuery.refetch();
  }

  async function clearCompletedJobs() {
    await new ApiClient($authState.csrfToken).clearJobs('completed');
    await jobsQuery.refetch();
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if $authState.user}
  {@const files = loadedFiles}
  {@const page = firstFilePage}
  <AppShell
  username={$authState.user.username}
  {route}
  {activeKind}
  libraryCount={page?.library_count ?? files.length}
  tagCount={tagsQuery.data?.tags.length ?? 0}
  jobsActiveCount={activeJobs().length}
  kindCounts={page?.facets?.kind ?? []}
  savedSearches={savedSearchesQuery.data?.items ?? []}
  suggestions={suggestionsQuery.data?.items ?? []}
  search={$searchDraft}
  onRoute={(next) => (route = next)}
  onKind={setKind}
  onSavedSearch={runSavedSearch}
  onCreateSavedSearch={createSavedSearch}
  onUpdateSavedSearch={updateSavedSearch}
  onDeleteSavedSearch={deleteSavedSearch}
  onSuggestion={applySuggestion}
  onSearchInput={setSearch}
  onSearchSubmit={submitSearch}
  onJobs={() => (route = route === 'jobs' ? 'library' : 'jobs')}
>
  {#if route === 'upload'}
    <UploadPanel
      {uploadFiles}
      {uploadItems}
      {uploadTags}
      {uploadBusy}
      {cancelBusy}
      {uploadStatus}
      {activeUploadJobID}
      targets={uploadTargetsQuery.data?.items ?? []}
      targetID={uploadTargetID}
      onTargetInput={setUploadTarget}
      onFiles={selectUploads}
      onTagsInput={(value) => (uploadTags = value)}
      onSubmit={submitUpload}
      onCancel={cancelUploadJob}
      onClear={() => { uploadFiles = []; uploadItems = []; uploadStatus = ''; }}
    />
  {:else if route === 'jobs'}
    <JobsView jobs={jobsQuery.data?.items ?? []} onCancel={cancelJob} onClearCompleted={clearCompletedJobs} />
  {:else if route === 'tags'}
    <TagsView
      tags={tagsQuery.data?.tags ?? []}
      loading={tagsQuery.isLoading}
      error={tagsQuery.isError ? errorMessage(tagsQuery.error) : ''}
      onTag={runTagSearch}
      onNamespace={(namespace) => runTagSearch(`${namespace}:`)}
    />
  {:else if route === 'account'}
    <AccountView username={$authState.user.username} onLogout={logout} />
  {:else if route === 'shortcuts'}
    <ShortcutsView />
  {:else}
    <MediaGrid
      sessionActive={Boolean($authState.user)}
      isLoading={filesQuery.isLoading}
      isError={filesQuery.isError}
      error={filesQuery.error}
      {files}
      virtual={virtualGrid(files, viewportWidth, viewportHeight, scrollY)}
      totalCount={page?.total_count ?? files.length}
      libraryCount={page?.library_count ?? files.length}
      searchActive={Boolean($submittedSearch || activeKind)}
      {selectedIDs}
      hasNextPage={Boolean(filesQuery.hasNextPage)}
      isFetchingNextPage={Boolean(filesQuery.isFetchingNextPage)}
      bind:loadMoreSentinel
      onOpen={openPreview}
      onToggleSelect={toggleSelect}
      onSelectAll={selectLoaded}
      onClearSelection={() => (selectedIDs = new Set())}
      onBulkTag={bulkTagSelected}
    >
      {#snippet actions()}
        <div class="library-head-actions">
          <div class="seg" aria-label="Sort field">
            {#each [{ value: 'modified', label: 'Modified' }, { value: 'name', label: 'Name' }, { value: 'size', label: 'Size' }] as option}
              <button
                class:active={sort === option.value}
                type="button"
                onclick={() => (sort = option.value as FileSort)}
              >
                {option.label}
              </button>
            {/each}
          </div>
          <button class="g-btn g-btn-sm" type="button" title="Sort direction" onclick={() => (order = order === 'desc' ? 'asc' : 'desc')}>
            <Icon name="sort" size={14} /> {order === 'desc' ? 'Newest' : 'Oldest'}
          </button>
          {#if filterQuery()}
            <button class="g-btn g-btn-sm" type="button" title="Tag every matching file without materializing all results" onclick={bulkTagFiltered}>
              <Icon name="tag" size={14} /> Tag filtered
            </button>
          {/if}
        </div>
      {/snippet}
    </MediaGrid>
  {/if}
</AppShell>

{#if activeFile}
  <PreviewDialog
    file={activeFile}
    tagDraft={tagDrafts[activeFile.id] ?? ''}
    tagBusy={Boolean(tagBusy[activeFile.id])}
    tagError={tagErrors[activeFile.id] ?? ''}
    onClose={closePreview}
    onPrev={() => movePreview(-1)}
    onNext={() => movePreview(1)}
    onTagInput={updateTagDraft}
    onMutateTags={mutateFileTags}
  />
{/if}
{/if}
