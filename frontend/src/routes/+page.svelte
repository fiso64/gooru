<script lang="ts">
  import { onMount } from 'svelte';
  import { writable } from 'svelte/store';
  import { useQueryClient } from '@tanstack/svelte-query';
  import AppShell from '$lib/components/AppShell.svelte';
  import AuthPanel from '$lib/components/AuthPanel.svelte';
  import MediaGrid from '$lib/components/MediaGrid.svelte';
  import PreviewDialog from '$lib/components/PreviewDialog.svelte';
  import SearchSidebar from '$lib/components/SearchSidebar.svelte';
  import { ApiClient } from '$lib/api/client';
  import { authState } from '$lib/stores/auth';
  import { createFilesQuery } from '$lib/queries/files';
  import { createJobQuery } from '$lib/queries/jobs';
  import { virtualGrid } from '$lib/state/ui';
  import { errorMessage, isTerminalJob, jobStatusText, parseTags, selectedKind } from '$lib/utils/format';
  import type { FileItem } from '$lib/api/types';

  const searchDraft = writable('');
  const submittedSearch = writable('');
  const queryClient = useQueryClient();

  let loginUsername = $state('');
  let loginPassword = $state('');
  let loginBusy = $state(false);
  let loginError = $state('');
  let observedCSRF = $state('');
  let authScope = $state(0);
  let loadMoreSentinel = $state<HTMLDivElement | undefined>();
  let viewportHeight = $state(900);
  let viewportWidth = $state(1200);
  let scrollY = $state(0);
  let tagDrafts = $state<Record<string, string>>({});
  let tagBusy = $state<Record<string, boolean>>({});
  let tagErrors = $state<Record<string, string>>({});
  let uploadFiles = $state<File[]>([]);
  let uploadTags = $state('');
  let uploadBusy = $state(false);
  let cancelBusy = $state(false);
  let uploadStatus = $state('');
  let activeUploadJobID = $state('');
  let handledUploadJobID = $state('');
  let activeFile = $state<FileItem | null>(null);
  let previewURL = $state('');
  let previewLoading = $state(false);
  let previewError = $state('');

  const filesQuery = createFilesQuery(() => Boolean($authState.user), () => $submittedSearch, () => authScope);
  const uploadJobQuery = createJobQuery(() => $authState.csrfToken, () => activeUploadJobID, () => authScope);

  $effect(() => {
    const csrf = $authState.csrfToken;
    if (csrf === observedCSRF) return;
    observedCSRF = csrf;
    authScope += 1;
    queryClient.removeQueries({ queryKey: ['files'] });
    queryClient.removeQueries({ queryKey: ['job'] });
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

  $effect(() => {
    const file = activeFile;
    previewURL = '';
    previewError = '';
    if (!file || !$authState.user) {
      previewLoading = false;
      return;
    }

    let objectURL = '';
    let canceled = false;
    const controller = new AbortController();
    previewLoading = true;
    fetch(file.media_urls.preview, { credentials: 'same-origin', signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error(`preview request failed: ${response.status}`);
        return response.blob();
      })
      .then((blob) => {
        if (canceled) return;
        objectURL = URL.createObjectURL(blob);
        previewURL = objectURL;
      })
      .catch((error) => {
        if (!controller.signal.aborted) previewError = errorMessage(error);
      })
      .finally(() => {
        if (!controller.signal.aborted) previewLoading = false;
      });

    return () => {
      canceled = true;
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  });

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

  function resetAuthScopedState() {
    tagDrafts = {};
    tagBusy = {};
    tagErrors = {};
    uploadFiles = [];
    uploadTags = '';
    uploadBusy = false;
    cancelBusy = false;
    uploadStatus = '';
    activeUploadJobID = '';
    handledUploadJobID = '';
    activeFile = null;
  }

  function submitSearch() {
    submittedSearch.set($searchDraft.trim());
  }

  function visibleFiles() {
    return filesQuery.data?.pages.flatMap((page) => page.files) ?? [];
  }

  function openPreview(file: FileItem) {
    activeFile = file;
  }

  function closePreview() {
    activeFile = null;
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && activeFile) closePreview();
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
      const response = await client.uploadFiles(uploadFiles, parseTags(uploadTags), true);
      if ('id' in response) {
        handledUploadJobID = '';
        activeUploadJobID = response.id;
        uploadStatus = 'Queued';
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
    } catch (error) {
      uploadStatus = errorMessage(error);
    } finally {
      cancelBusy = false;
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<svelte:head>
  <title>Gooru Library</title>
</svelte:head>

<AppShell>
  {#snippet auth()}
    <AuthPanel
      username={loginUsername}
      password={loginPassword}
      busy={loginBusy}
      error={loginError}
      onUsernameInput={(value) => (loginUsername = value)}
      onPasswordInput={(value) => (loginPassword = value)}
      onLogin={login}
    />
  {/snippet}

  {#snippet sidebar()}
    {@const files = visibleFiles()}
    <SearchSidebar
      authSaved={Boolean($authState.user)}
      searchDraft={$searchDraft}
      loadedKinds={files.length ? selectedKind(files) : ''}
      {uploadFiles}
      {uploadTags}
      {uploadBusy}
      {cancelBusy}
      {uploadStatus}
      {activeUploadJobID}
      onSearchInput={(value) => searchDraft.set(value)}
      onSearchSubmit={submitSearch}
      onRefresh={() => filesQuery.refetch()}
      onUploadFiles={selectUploads}
      onUploadTagsInput={(value) => (uploadTags = value)}
      onUploadSubmit={submitUpload}
      onUploadCancel={cancelUploadJob}
    />
  {/snippet}

  {@const files = visibleFiles()}
  <MediaGrid
    sessionActive={Boolean($authState.user)}
    isLoading={filesQuery.isLoading}
    isError={filesQuery.isError}
    error={filesQuery.error}
    {files}
    virtual={virtualGrid(files, viewportWidth, viewportHeight, scrollY)}
    hasNextPage={Boolean(filesQuery.hasNextPage)}
    isFetchingNextPage={Boolean(filesQuery.isFetchingNextPage)}
    bind:loadMoreSentinel
    {tagDrafts}
    {tagBusy}
    {tagErrors}
    onOpen={openPreview}
    onTagInput={updateTagDraft}
    onMutateTags={mutateFileTags}
  />
</AppShell>

{#if activeFile}
  <PreviewDialog file={activeFile} {previewURL} {previewLoading} {previewError} onClose={closePreview} />
{/if}
