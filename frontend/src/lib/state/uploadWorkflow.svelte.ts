import { untrack } from 'svelte';
import {
  itemFromJob,
  itemFromResult,
  queuedItem,
  stagedUploadItems,
  uploadingItem,
  uploadProgressItem,
  uploadSummary,
  waitingUploadItems,
  type UploadItem
} from './uploadItems';
import { errorMessage, isTerminalJob, parseTags } from '$lib/utils/format';
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
  let trackedJobs = $state<Record<string, number>>({});

  function reset() {
    files = [];
    items = [];
    tags = '';
    conflictPolicy = 'rename';
    autoUpload = false;
    busy = false;
    cancelBusy = false;
    status = '';
    trackedJobs = {};
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

  function hasActiveJobs() {
    return Object.keys(trackedJobs).length > 0;
  }

  function select(nextFiles: FileList | File[] | null) {
    const additions = nextFiles ? Array.from(nextFiles) : [];
    if (!additions.length) return;
    if (busy || hasActiveJobs()) {
      status = 'Upload in progress; add more files after it finishes';
      return;
    }

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
      const index = trackedJobs[job?.id];
      if (!job || index === undefined) return { completed: false, changedFiles: false };
      items = itemFromJob(items, index, job);
      if (!isTerminalJob(job)) return { completed: false, changedFiles: false };

      const nextTrackedJobs = { ...trackedJobs };
      delete nextTrackedJobs[job.id];
      trackedJobs = nextTrackedJobs;
      const changedFiles = job.status === 'completed';
      if (!busy && !hasActiveJobs()) finishBatch();
      else status = uploadSummary(items);
      return { completed: true, changedFiles };
    });
  }

  function applyJobError(error: unknown) {
    untrack(() => {
      const jobID = Object.keys(trackedJobs)[0];
      if (!jobID) return;
      const index = trackedJobs[jobID];
      status = errorMessage(error);
      items = items.map((item, itemIndex) => itemIndex === index ? { ...item, error: status } : item);
    });
  }

  function applyJobs(jobs: Job[]) {
    let completed = false;
    let changedFiles = false;
    for (const job of jobs) {
      if (trackedJobs[job.id] === undefined) continue;
      const result = applyJob(job);
      completed ||= result.completed;
      changedFiles ||= result.changedFiles;
    }
    return { completed, changedFiles };
  }

  function finishBatch() {
    files = [];
    tags = '';
    status = uploadSummary(items) || 'Upload finished';
  }

  async function submit(mutate: UploadMutate) {
    if (!files.length || busy || hasActiveJobs()) return { queued: false, changedFiles: false };
    busy = true;
    trackedJobs = {};
    items = waitingUploadItems(items.length ? items : stagedUploadItems(files, targetID));
    const parsedTags = parseTags(tags);
    let queued = 0;
    let changedFiles = false;

    for (let index = 0; index < files.length; index += 1) {
      const file = files[index];
      items = uploadingItem(items, index);
      status = `Uploading ${index + 1}/${files.length} · 0%`;
      try {
        const response = await mutate({
          files: [file],
          tags: parsedTags,
          preferAsync: true,
          targetID,
          conflictPolicy,
          onProgress: (progress) => {
            items = uploadProgressItem(items, index, progress);
            status = `Uploading ${index + 1}/${files.length} · ${Math.round(progress)}%`;
          }
        });
        if ('id' in response) {
          trackedJobs = { ...trackedJobs, [response.id]: index };
          items = queuedItem(items, index);
          queued += 1;
        } else {
          items = itemFromResult(items, index, response);
          changedFiles = true;
        }
      } catch (error) {
        const message = errorMessage(error);
        items = items.map((item, itemIndex) => itemIndex === index ? { ...item, status: 'error', error: message } : item);
      }
    }

    busy = false;
    if (hasActiveJobs()) status = uploadSummary(items);
    else finishBatch();
    return { queued: queued > 0, changedFiles };
  }

  async function cancel(mutate: CancelJob, jobID = Object.keys(trackedJobs)[0] ?? '') {
    const index = trackedJobs[jobID];
    if (!jobID || index === undefined || cancelBusy) return { changed: false };
    cancelBusy = true;
    try {
      const job = await mutate(jobID);
      items = itemFromJob(items, index, job);
      if (isTerminalJob(job)) {
        const nextTrackedJobs = { ...trackedJobs };
        delete nextTrackedJobs[jobID];
        trackedJobs = nextTrackedJobs;
      }
      if (!busy && !hasActiveJobs()) finishBatch();
      else status = uploadSummary(items);
      return { changed: true };
    } catch (error) {
      const message = errorMessage(error);
      items = items.map((item, itemIndex) => itemIndex === index ? { ...item, error: message } : item);
      status = message;
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
    get activeJobID() { return Object.keys(trackedJobs)[0] ?? ''; },
    get activeJobIDs() { return Object.keys(trackedJobs); },
    reset,
    clear,
    removeAt,
    select,
    setTarget,
    applyJob,
    applyJobError,
    applyJobs,
    submit,
    cancel
  };
}
