import { untrack } from 'svelte';
import {
  itemFromJob,
  itemFromResult,
  queuedItem,
  retargetStagedUploadItems,
  stagedUploadItems,
  uploadingItem,
  uploadProgressItem,
  uploadSummary,
  waitingUploadItems,
  type UploadAddedAtStrategy,
  type UploadItem
} from './uploadItems';
import { errorMessage, isTerminalJob, parseTags } from '$lib/utils/format';
import type { Job, UploadImportResponse } from '$lib/api/types';
import type { UploadVariables } from '$lib/queries/library';

type UploadMutate = (variables: UploadVariables) => Promise<Job | UploadImportResponse>;
type CancelJob = (jobID: string) => Promise<Job>;

export const browserUploadConcurrency = 4;

export function createUploadWorkflow() {
  let files = $state<File[]>([]);
  let items = $state<UploadItem[]>([]);
  let tags = $state('');
  let targetID = $state('');
  let conflictPolicy = $state('rename');
  let addedAtStrategy = $state<UploadAddedAtStrategy>('queue');
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
    addedAtStrategy = 'queue';
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
    items = items.filter((_, itemIndex) => itemIndex !== index);
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

    const queueTimeMs = Date.now();
    files = [...files, ...additions];
    items = [...items, ...stagedUploadItems(additions, targetID, queueTimeMs)];
    status = '';
    if (autoUpload) {
      queueMicrotask(() => {
        status = 'Ready to auto-upload';
      });
    }
  }

  function setTarget(value: string, defaultStrategy?: UploadAddedAtStrategy) {
    targetID = value;
    if (defaultStrategy) addedAtStrategy = defaultStrategy;
    if (files.length) items = retargetStagedUploadItems(items, targetID);
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
    const batchFiles = files;
    const parsedTags = parseTags(tags);
    const batchTargetID = targetID;
    const batchConflictPolicy = conflictPolicy;
    const batchAddedAtStrategy = addedAtStrategy;
    const fallbackQueueTimeMs = Date.now();
    const batchQueueTimes = items.map((item) => item.queueTimeMs ?? fallbackQueueTimeMs);
    const batchQueueFirstTimeMs = Math.min(...batchQueueTimes);
    const batchQueueLastTimeMs = Math.max(...batchQueueTimes);
    const batchQueueTotal = batchFiles.length;
    let queued = 0;
    let changedFiles = false;
    let nextIndex = 0;

    async function uploadNext() {
      while (nextIndex < batchFiles.length) {
        const index = nextIndex;
        nextIndex += 1;
        const file = batchFiles[index];
        items = uploadingItem(items, index);
        status = uploadSummary(items);
        try {
          const response = await mutate({
            files: [file],
            tags: parsedTags,
            preferAsync: true,
            targetID: batchTargetID,
            conflictPolicy: batchConflictPolicy,
            addedAtStrategy: batchAddedAtStrategy,
            queueTimeMs: batchQueueTimes[index],
            queueFirstTimeMs: batchQueueFirstTimeMs,
            queueLastTimeMs: batchQueueLastTimeMs,
            queueIndex: index,
            queueTotal: batchQueueTotal,
            onProgress: (progress) => {
              items = uploadProgressItem(items, index, progress);
              status = uploadSummary(items);
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
        status = uploadSummary(items);
      }
    }

    const workerCount = Math.min(browserUploadConcurrency, batchFiles.length);
    await Promise.all(Array.from({ length: workerCount }, () => uploadNext()));

    busy = false;
    if (hasActiveJobs()) status = uploadSummary(items);
    else finishBatch();
    return { queued: queued > 0, changedFiles };
  }

  async function cancel(mutate: CancelJob, _jobID = Object.keys(trackedJobs)[0] ?? '') {
    const jobIDs = Object.keys(trackedJobs);
    if (!jobIDs.length || cancelBusy) return { changed: false };
    cancelBusy = true;
    let changed = false;
    try {
      for (const jobID of jobIDs) {
        const index = trackedJobs[jobID];
        if (index === undefined) continue;
        try {
          const job = await mutate(jobID);
          items = itemFromJob(items, index, job);
          if (isTerminalJob(job)) {
            const nextTrackedJobs = { ...trackedJobs };
            delete nextTrackedJobs[jobID];
            trackedJobs = nextTrackedJobs;
          }
          changed = true;
        } catch (error) {
          const message = errorMessage(error);
          items = items.map((item, itemIndex) => itemIndex === index ? { ...item, error: message } : item);
          status = message;
        }
      }
      if (!busy && !hasActiveJobs()) finishBatch();
      else status = uploadSummary(items);
      return { changed };
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
    get addedAtStrategy() { return addedAtStrategy; },
    set addedAtStrategy(value: UploadAddedAtStrategy) { addedAtStrategy = value; },
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
