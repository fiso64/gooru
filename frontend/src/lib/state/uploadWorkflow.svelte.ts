import { untrack } from 'svelte';
import { itemsFromJob, itemsFromResult, queuedItems, stagedUploadItems, uploadingItems, uploadSummary, type UploadItem } from './uploadItems';
import { errorMessage, isTerminalJob, jobStatusText, parseTags } from '$lib/utils/format';
import type { Job, UploadImportResponse } from '$lib/api/types';
import type { UploadVariables } from '$lib/queries/library';

type UploadMutate = (variables: UploadVariables) => Promise<Job | UploadImportResponse>;
type CancelJob = (jobID: string) => Promise<Job>;

export function createUploadWorkflow() {
  let files = $state<File[]>([]);
  let items = $state<UploadItem[]>([]);
  let tags = $state('');
  let targetID = $state('');
  let conflictPolicy = $state('rename');
  let autoUpload = $state(false);
  let busy = $state(false);
  let cancelBusy = $state(false);
  let status = $state('');
  let activeJobID = $state('');
  let handledJobID = $state('');

  function reset() {
    files = [];
    items = [];
    tags = '';
    conflictPolicy = 'rename';
    autoUpload = false;
    busy = false;
    cancelBusy = false;
    status = '';
    activeJobID = '';
    handledJobID = '';
  }

  function clear() {
    files = [];
    items = [];
    status = '';
  }

  function removeAt(index: number) {
    files = files.filter((_, fileIndex) => fileIndex !== index);
    items = stagedUploadItems(files, targetID);
    status = '';
  }

  function select(nextFiles: FileList | File[] | null) {
    const additions = nextFiles ? Array.from(nextFiles) : [];
    if (!additions.length) return;
    if (busy || activeJobID) {
      status = 'Upload in progress; add more files after it finishes';
      return;
    }

    // File picking and drop gestures are additive while a batch is staged. The
    // explicit Clear/remove controls own destructive staging changes instead of
    // a later drop silently replacing earlier work.
    files = [...files, ...additions];
    items = stagedUploadItems(files, targetID);
    status = '';
    if (autoUpload) {
      queueMicrotask(() => {
        status = 'Ready to auto-upload';
      });
    }
  }

  function setTarget(value: string) {
    targetID = value;
    if (files.length) items = stagedUploadItems(files, targetID);
  }

  function applyJob(job: Job) {
    return untrack(() => {
      if (!job || job.id === handledJobID) return { completed: false, changedFiles: false };
      status = jobStatusText(job);
      items = itemsFromJob(items, job);
      if (!isTerminalJob(job)) return { completed: false, changedFiles: false };
      handledJobID = job.id;
      activeJobID = '';
      if (job.status !== 'completed') return { completed: true, changedFiles: false };
      files = [];
      status = uploadSummary(items) || 'Import completed';
      return { completed: true, changedFiles: true };
    });
  }

  function applyJobError(error: unknown) {
    untrack(() => {
      if (!activeJobID) return;
      status = errorMessage(error);
      activeJobID = '';
    });
  }

  async function submit(mutate: UploadMutate) {
    if (!files.length || busy || activeJobID) return { queued: false, changedFiles: false };
    busy = true;
    status = 'Uploading';
    items = uploadingItems(items.length ? items : stagedUploadItems(files, targetID));
    try {
      const response = await mutate({ files, tags: parseTags(tags), preferAsync: true, targetID, conflictPolicy });
      if ('id' in response) {
        handledJobID = '';
        activeJobID = response.id;
        items = queuedItems(items);
        status = 'Queued';
        return { queued: true, changedFiles: false };
      }
      items = itemsFromResult(response, items);
      status = uploadSummary(items);
      files = [];
      tags = '';
      return { queued: false, changedFiles: true };
    } catch (error) {
      status = errorMessage(error);
      items = items.map((item) => ({ ...item, status: 'error', progress: 100, error: status }));
      return { queued: false, changedFiles: false };
    } finally {
      busy = false;
    }
  }

  async function cancel(mutate: CancelJob, jobID = activeJobID) {
    if (!jobID || cancelBusy) return { changed: false };
    cancelBusy = true;
    status = 'Canceled';
    items = items.map((item) => ({ ...item, status: 'canceled', progress: item.progress || 100 }));
    handledJobID = jobID;
    activeJobID = '';
    try {
      const job = await mutate(jobID);
      status = jobStatusText(job);
      items = itemsFromJob(items, job);
      if (isTerminalJob(job)) handledJobID = job.id;
      else activeJobID = job.id;
      return { changed: true };
    } catch (error) {
      status = errorMessage(error);
      return { changed: false };
    } finally {
      cancelBusy = false;
    }
  }

  return {
    get files() { return files; },
    get items() { return items; },
    get tags() { return tags; },
    set tags(value: string) { tags = value; },
    get targetID() { return targetID; },
    get conflictPolicy() { return conflictPolicy; },
    set conflictPolicy(value: string) { conflictPolicy = value; },
    get autoUpload() { return autoUpload; },
    set autoUpload(value: boolean) { autoUpload = value; },
    get busy() { return busy; },
    get cancelBusy() { return cancelBusy; },
    get status() { return status; },
    get activeJobID() { return activeJobID; },
    reset,
    clear,
    removeAt,
    select,
    setTarget,
    applyJob,
    applyJobError,
    submit,
    cancel
  };
}
