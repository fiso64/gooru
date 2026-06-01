<script lang="ts">
  import { onMount } from 'svelte';
  import { writable } from 'svelte/store';
  import { useQueryClient } from '@tanstack/svelte-query';
  import AppShell from '$lib/components/AppShell.svelte';
  import AuthPanel from '$lib/components/AuthPanel.svelte';
  import Icon from '$lib/components/Icon.svelte';
  import MediaGrid from '$lib/components/MediaGrid.svelte';
  import PreviewDialog from '$lib/components/PreviewDialog.svelte';
  import UploadPanel from '$lib/components/UploadPanel.svelte';
  import { ApiClient } from '$lib/api/client';
  import { authState } from '$lib/stores/auth';
  import { createFilesQuery, type FileSort, type SortOrder } from '$lib/queries/files';
  import { createJobQuery, createJobsQuery } from '$lib/queries/jobs';
  import { createSavedSearchesQuery, createSuggestionsQuery, createUploadTargetsQuery } from '$lib/queries/library';
  import { virtualGrid } from '$lib/state/ui';
  import { errorMessage, formatBytes, isTerminalJob, jobStatusText, parseTags } from '$lib/utils/format';
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
  let loginUsername = $state('');
  let loginPassword = $state('');
  let loginBusy = $state(false);
  let loginError = $state('');
  let tagDrafts = $state<Record<string, string>>({});
  let tagBusy = $state<Record<string, boolean>>({});
  let tagErrors = $state<Record<string, string>>({});
  let selectedIDs = $state(new Set<string>());
  let uploadFiles = $state<File[]>([]);
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
  const uploadTargetsQuery = createUploadTargetsQuery(() => Boolean($authState.user), () => authScope);
  const suggestionsQuery = createSuggestionsQuery(() => Boolean($authState.user), () => $searchDraft, () => $submittedSearch, () => authScope);

  $effect(() => {
    const csrf = $authState.csrfToken;
    if (csrf === observedCSRF) return;
    observedCSRF = csrf;
    authScope += 1;
    queryClient.clear();
    resetAuthScopedState();
  });

  onMount(() => {
    const updateViewport = () => {
      viewportHeight = window.innerHeight;
      viewportWidth = window.innerWidth;
      scrollY = window.scrollY;
    };
    updateViewport();
    window.addEventListener('resize', updateViewport);
    window.addEventListener('scroll', updateViewport, { passive: true });
    return () => {
      window.removeEventListener('resize', updateViewport);
      window.removeEventListener('scroll', updateViewport);
    };
  });

  onMount(() => {
    new ApiClient()
      .me()
      .then((session) => authState.set({ user: session.user, csrfToken: session.csrf_token ?? '', checked: true }))
      .catch(() => authState.set({ user: null, csrfToken: '', checked: true }));
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
    if (isTerminalJob(job)) {
      handledUploadJobID = job.id;
      activeUploadJobID = '';
      void jobsQuery.refetch();
      if (job.status === 'completed') {
        const result = job.result as { files?: unknown[] } | undefined;
        uploadStatus = `Imported ${result?.files?.length ?? 0} file(s)`;
        uploadFiles = [];
        uploadTags = '';
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
    uploadTags = '';
    uploadBusy = false;
    cancelBusy = false;
    uploadStatus = '';
    activeUploadJobID = '';
    handledUploadJobID = '';
    activeFile = null;
  }

  function visibleFiles() {
    return filesQuery.data?.pages.flatMap((page) => page.files) ?? [];
  }

  function firstPage() {
    return filesQuery.data?.pages[0];
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

  function runSavedSearch(query: string, name: string) {
    activeSavedSearch = name;
    searchDraft.set(query);
    submittedSearch.set(query);
    route = 'library';
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
    const files = visibleFiles();
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
    selectedIDs = new Set(visibleFiles().map((file) => file.id));
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

  async function login() {
    if (loginBusy) return;
    loginBusy = true;
    loginError = '';
    try {
      const session = await new ApiClient().login(loginUsername.trim(), loginPassword);
      authState.set({ user: session.user, csrfToken: session.csrf_token ?? '', checked: true });
      loginPassword = '';
    } catch (error) {
      loginError = errorMessage(error);
    } finally {
      loginBusy = false;
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
    uploadStatus = '';
  }

  async function submitUpload() {
    if (!uploadFiles.length || uploadBusy || activeUploadJobID || !$authState.user) return;
    uploadBusy = true;
    uploadStatus = 'Uploading';
    try {
      const client = new ApiClient($authState.csrfToken);
      const response = await client.uploadFiles(uploadFiles, parseTags(uploadTags), true, uploadTargetID);
      if ('id' in response) {
        handledUploadJobID = '';
        activeUploadJobID = response.id;
        uploadStatus = 'Queued';
        void jobsQuery.refetch();
      } else {
        uploadStatus = `Imported ${response.files.length} file(s)`;
        uploadFiles = [];
        uploadTags = '';
        await filesQuery.refetch();
      }
    } catch (error) {
      uploadStatus = errorMessage(error);
    } finally {
      uploadBusy = false;
    }
  }

  async function cancelUploadJob() {
    if (!activeUploadJobID || cancelBusy || !$authState.user) return;
    cancelBusy = true;
    try {
      const job = await new ApiClient($authState.csrfToken).cancelJob(activeUploadJobID);
      uploadStatus = jobStatusText(job);
      if (isTerminalJob(job)) {
        handledUploadJobID = job.id;
        activeUploadJobID = '';
      } else {
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

<svelte:head>
  <title>Gooru Library</title>
</svelte:head>

<div class="gooru-root gooru-accent-sodium gooru-type-editorial">
  {#if !$authState.user}
    <AuthPanel
      checked={$authState.checked}
      username=""
      {loginUsername}
      {loginPassword}
      {loginBusy}
      {loginError}
      onUsernameInput={(value) => (loginUsername = value)}
      onPasswordInput={(value) => (loginPassword = value)}
      onLogin={login}
      onLogout={logout}
    />
  {:else}
    {@const files = visibleFiles()}
    {@const page = firstPage()}
    <AppShell
      username={$authState.user.username}
      {route}
      {activeKind}
      libraryCount={page?.library_count ?? files.length}
      tagCount={page?.facets?.kind?.reduce((sum, item) => sum + item.count, 0) ?? 0}
      jobsActiveCount={activeJobs().length}
      kindCounts={page?.facets?.kind ?? []}
      savedSearches={savedSearchesQuery.data?.items ?? []}
      suggestions={suggestionsQuery.data?.items ?? []}
      search={$searchDraft}
      onRoute={(next) => (route = next)}
      onKind={setKind}
      onSavedSearch={runSavedSearch}
      onSuggestion={applySuggestion}
      onSearchInput={setSearch}
      onSearchSubmit={submitSearch}
      onJobs={() => (route = route === 'jobs' ? 'library' : 'jobs')}
    >
      {#if route === 'upload'}
        <UploadPanel
          {uploadFiles}
          {uploadTags}
          {uploadBusy}
          {cancelBusy}
          {uploadStatus}
          {activeUploadJobID}
          targets={uploadTargetsQuery.data?.items ?? []}
          targetID={uploadTargetID}
          onTargetInput={(value) => (uploadTargetID = value)}
          onFiles={selectUploads}
          onTagsInput={(value) => (uploadTags = value)}
          onSubmit={submitUpload}
          onCancel={cancelUploadJob}
          onClear={() => (uploadFiles = [])}
        />
      {:else if route === 'jobs'}
        <main class="main">
          <div class="page">
            <div class="page-header">
              <div class="g-eyebrow g-eyebrow-accent">Jobs</div>
              <h1>Background work</h1>
              <p>Imports and bulk tag changes report progress here. Pause and resume are hidden until supported.</p>
            </div>
            <div class="list-head">
              <span class="g-eyebrow">{jobsQuery.data?.items.length ?? 0} jobs</span>
              <button class="g-btn g-btn-sm" type="button" onclick={clearCompletedJobs}>Clear completed</button>
            </div>
            <div class="g-card jobs-list">
              {#each jobsQuery.data?.items ?? [] as job}
                <div class="job-row">
                  <div class="job-row-head">
                    <span class="name"><Icon name={job.type === 'upload_import' ? 'upload' : 'tag'} size={14} /><b>{job.type}</b></span>
                    <span class={`status ${job.status}`}>{job.status}</span>
                  </div>
                  <div class={`job-progress ${job.status}`}><div style={`width: ${Math.round((job.progress ?? 0) * 100)}%`}></div></div>
                  <div class="job-meta">
                    <span>{job.id}</span>
                    <span>{job.error ?? jobStatusText(job)}</span>
                  </div>
                  {#if job.status === 'pending' || job.status === 'running'}
                    <button class="g-btn g-btn-sm" type="button" onclick={() => cancelJob(job)}>Cancel</button>
                  {/if}
                </div>
              {:else}
                <div class="empty-row">No jobs have been recorded.</div>
              {/each}
            </div>
          </div>
        </main>
      {:else if route === 'tags'}
        <main class="main">
          <div class="page">
            <div class="page-header">
              <div class="g-eyebrow g-eyebrow-accent">Tags</div>
              <h1>Tag index</h1>
              <p>Use search for tag suggestions and counts. Full tag-index browsing will expand as the API exposes richer namespace data.</p>
            </div>
            <div class="tagscloud">
              {#each page?.facets?.kind ?? [] as item}
                <button class="tagscloud-item" type="button" onclick={() => setKind(item.value)}>
                  <span>{item.value}</span>
                  <span class="count">{item.count}</span>
                </button>
              {/each}
            </div>
          </div>
        </main>
      {:else if route === 'account'}
        <main class="main">
          <div class="page">
            <div class="page-header">
              <div class="g-eyebrow g-eyebrow-accent">Account</div>
              <h1>{$authState.user.username}</h1>
              <p>Session authentication uses same-origin cookies and CSRF-protected mutations.</p>
            </div>
            <button class="g-btn" type="button" onclick={logout}><Icon name="logout" size={14} /> Sign out</button>
          </div>
        </main>
      {:else if route === 'shortcuts'}
        <main class="main">
          <div class="page">
            <div class="page-header">
              <div class="g-eyebrow g-eyebrow-accent">Keyboard</div>
              <h1>Shortcuts</h1>
              <p>Escape closes preview. Arrow keys move through open preview items.</p>
            </div>
          </div>
        </main>
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
</div>
