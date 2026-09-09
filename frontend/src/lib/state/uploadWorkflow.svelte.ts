import { untrack } from 'svelte';
import {
  countUploadStatuses,
  itemFromJob,
  itemFromResult,
  queuedItem,
  replaceUploadItemInPlace,
  retargetStagedUploadItems,
  stagedUploadItems,
  uploadingItem,
  uploadProgressItem,
  uploadSummaryFromCounts,
  type UploadAddedAtStrategy,
  type UploadItem,
  type UploadStatusCounts
} from './uploadItems';
import { ApiError } from '$lib/api/client';
import { errorMessage, isTerminalJob, parseTags } from '$lib/utils/format';
import type { Job, UploadImportResponse } from '$lib/api/types';
import type { UploadVariables } from '$lib/queries/library';
import { uploadAdmissionFallbackMs, uploadJobStatusBatchSize } from '$lib/uploadBackpressure';

export { uploadJobStatusBatchSize } from '$lib/uploadBackpressure';

type UploadMutate = (variables: UploadVariables) => Promise<Job | UploadImportResponse>;
type CancelJob = (jobID: string) => Promise<Job>;
type JobBatch = { items: Job[] };
type JobApplyResult = { completed: boolean; changedFiles: boolean };

export const browserUploadConcurrency = 4;

function isJobQueueFull(error: unknown) {
  return error instanceof ApiError && error.code === 'job_queue_full';
}

export function createUploadWorkflow() {
  let files = $state<File[]>([]);
  let items = $state<UploadItem[]>([]);
  let tags = $state('');
  let targetID = $state('');
  let managedDefaultTags = $state<string[]>([]);
  let conflictPolicy = $state('rename');
  let addedAtStrategy = $state<UploadAddedAtStrategy>('queue');
  let autoUpload = $state(false);
  let busy = $state(false);
  let cancelBusy = $state(false);
  let status = $state('');
  let trackedJobs = $state<Record<string, number>>({});
  let admissionBackpressured = $state(false);
  let statusCounts: UploadStatusCounts = {};
  let admissionWaiters = new Set<() => void>();

  $effect(() => {
    if (!busy) return;
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  });

  function wakeAdmissionWaiters() {
    const waiters = admissionWaiters;
    admissionWaiters = new Set();
    for (const wake of waiters) wake();
  }

  function reset() {
    files = [];
    items = [];
    tags = '';
    managedDefaultTags = [];
    conflictPolicy = 'rename';
    addedAtStrategy = 'queue';
    autoUpload = false;
    busy = false;
    cancelBusy = false;
    status = '';
    trackedJobs = {};
    admissionBackpressured = false;
    statusCounts = {};
    wakeAdmissionWaiters();
  }

  function clear() {
    files = [];
    items = [];
    status = '';
    statusCounts = {};
  }

  function removeAt(index: number) {
    const stagedIndices = items.flatMap((item, itemIndex) => item.status === 'staged' ? [itemIndex] : []);
    const stagedFileIndex = stagedIndices.indexOf(index);
    if (stagedFileIndex >= 0) {
      files = files.filter((_, fileIndex) => fileIndex !== stagedFileIndex);
    }
    items = items.filter((_, itemIndex) => itemIndex !== index);
    status = '';
    statusCounts = countUploadStatuses(items);
  }

  function hasActiveJobs() {
    return Object.keys(trackedJobs).length > 0;
  }

  function pollJobID() {
    const ids = Object.keys(trackedJobs);
    if (busy && ids.length < uploadJobStatusBatchSize && !admissionBackpressured) return '';
    return ids.slice(0, uploadJobStatusBatchSize).join(',');
  }

  function replaceItem(index: number, next: UploadItem | undefined) {
    replaceUploadItemInPlace(items, index, next, statusCounts);
  }

  function refreshStatus() {
    status = uploadSummaryFromCounts(statusCounts);
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
    statusCounts = countUploadStatuses(items);
    status = '';
    if (autoUpload) {
      queueMicrotask(() => {
        status = 'Ready to auto-upload';
      });
    }
  }

  function setTarget(value: string, defaultStrategy?: UploadAddedAtStrategy, defaultTags: string[] = [], explicitSelection = false) {
    const previousDefaults = new Set(managedDefaultTags);
    const nextDefaults = Array.from(new Set(defaultTags.map((tag) => tag.trim()).filter(Boolean)));
    if ((explicitSelection || value === targetID) && nextDefaults.length > 0) {
      tags = nextDefaults.join(' ');
    } else {
      const retained = parseTags(tags).filter((tag) => !previousDefaults.has(tag));
      const merged = [...retained];
      const seen = new Set(merged);
      for (const tag of nextDefaults) {
        if (!seen.has(tag)) {
          merged.push(tag);
          seen.add(tag);
        }
      }
      tags = merged.join(' ');
    }
    managedDefaultTags = nextDefaults;
    targetID = value;
    if (defaultStrategy) addedAtStrategy = defaultStrategy;
    if (files.length) items = retargetStagedUploadItems(items, targetID);
  }

  function applyJob(job: Job | JobBatch): JobApplyResult {
    if ('items' in job) return applyJobs(job.items);
    return untrack(() => {
      const index = trackedJobs[job?.id];
      if (!job || index === undefined) return { completed: false, changedFiles: false };
      const current = items[index];
      replaceItem(index, current ? itemFromJob([current], 0, job)[0] : undefined);
      if (!isTerminalJob(job)) return { completed: false, changedFiles: false };

      const nextTrackedJobs = { ...trackedJobs };
      delete nextTrackedJobs[job.id];
      trackedJobs = nextTrackedJobs;
      wakeAdmissionWaiters();
      const changedFiles = job.status === 'completed';
      if (!busy && !hasActiveJobs()) finishBatch();
      else refreshStatus();
      return { completed: true, changedFiles };
    });
  }

  function applyJobError(error: unknown) {
    untrack(() => {
      const jobID = Object.keys(trackedJobs)[0];
      if (!jobID) return;
      const index = trackedJobs[jobID];
      status = errorMessage(error);
      const current = items[index];
      if (current) items[index] = { ...current, error: status };
    });
  }

  function applyJobs(jobs: Job[]): JobApplyResult {
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
    admissionBackpressured = false;
    wakeAdmissionWaiters();
    status = uploadSummaryFromCounts(statusCounts) || 'Upload finished';
  }

  function waitForAdmissionChange(skipIfCapacityAvailable = true) {
    return new Promise<void>((resolve) => {
      let timeout: ReturnType<typeof setTimeout> | undefined;
      const wake = () => {
        if (timeout !== undefined) clearTimeout(timeout);
        admissionWaiters.delete(wake);
        resolve();
      };
      admissionWaiters.add(wake);
      // Avoid sleeping through a completion that raced between the caller's
      // local-window capacity check and registering this waiter. A server-side
      // queue-full response cannot use this shortcut because its saturation may
      // come from work outside this browser's tracked window.
      if (skipIfCapacityAvailable && Object.keys(trackedJobs).length < uploadJobStatusBatchSize) {
        wake();
        return;
      }
      timeout = setTimeout(wake, uploadAdmissionFallbackMs);
    });
  }

  async function waitForJobAdmissionSlot() {
    while (Object.keys(trackedJobs).length >= uploadJobStatusBatchSize) {
      admissionBackpressured = true;
      await waitForAdmissionChange();
    }
  }

  async function submit(mutate: UploadMutate) {
    if (!files.length || busy || hasActiveJobs()) return { queued: false, changedFiles: false };
    busy = true;
    trackedJobs = {};
    admissionBackpressured = false;

    if (!items.length) items = stagedUploadItems(files, targetID);
    const batchItemIndices = items.flatMap((item, itemIndex) => item.status === 'staged' ? [itemIndex] : []);
    if (batchItemIndices.length !== files.length) {
      busy = false;
      status = 'Upload queue changed unexpectedly; please restage the pending files';
      return { queued: false, changedFiles: false };
    }
    for (const itemIndex of batchItemIndices) {
      const current = items[itemIndex];
      replaceItem(itemIndex, current ? { ...current, status: 'waiting', progress: 0, error: '' } : undefined);
    }
    statusCounts = countUploadStatuses(items);

    const batchFiles = [...files];
    const parsedTags = parseTags(tags);
    const batchTargetID = targetID;
    const batchConflictPolicy = conflictPolicy;
    const batchAddedAtStrategy = addedAtStrategy;
    const fallbackQueueTimeMs = Date.now();
    const batchQueueTimes = batchItemIndices.map((itemIndex) => items[itemIndex]?.queueTimeMs ?? fallbackQueueTimeMs);
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
        const itemIndex = batchItemIndices[index];
        const current = items[itemIndex];
        replaceItem(itemIndex, current ? uploadingItem([current], 0)[0] : undefined);
        refreshStatus();
        try {
          let response: Job | UploadImportResponse;
          for (;;) {
            await waitForJobAdmissionSlot();
            admissionBackpressured = false;
            try {
              response = await mutate({
                files: [file],
                tags: parsedTags,
                // Release browser transfer capacity once the server has staged
                // the file and accepted its import job, but bound the number of
                // accepted unfinished imports to one batched status window.
                // This keeps large batches from outrunning the server job queue.
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
                  const progressItem = items[itemIndex];
                  replaceItem(itemIndex, progressItem ? uploadProgressItem([progressItem], 0, progress)[0] : undefined);
                }
              });
              break;
            } catch (error) {
              if (!isJobQueueFull(error)) throw error;
              admissionBackpressured = true;
              await waitForAdmissionChange(false);
            }
          }
          if ('id' in response) {
            trackedJobs = { ...trackedJobs, [response.id]: itemIndex };
            const queuedItemState = items[itemIndex];
            replaceItem(itemIndex, queuedItemState ? queuedItem([queuedItemState], 0)[0] : undefined);
            queued += 1;
          } else {
            const resultItem = items[itemIndex];
            replaceItem(itemIndex, resultItem ? itemFromResult([resultItem], 0, response)[0] : undefined);
            changedFiles = true;
          }
        } catch (error) {
          const message = errorMessage(error);
          const failed = items[itemIndex];
          if (failed) replaceItem(itemIndex, { ...failed, status: 'error', error: message });
        }
        refreshStatus();
      }
    }

    const workerCount = Math.min(browserUploadConcurrency, batchFiles.length);
    await Promise.all(Array.from({ length: workerCount }, () => uploadNext()));

    busy = false;
    admissionBackpressured = false;
    if (hasActiveJobs()) refreshStatus();
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
          const current = items[index];
          replaceItem(index, current ? itemFromJob([current], 0, job)[0] : undefined);
          if (isTerminalJob(job)) {
            const nextTrackedJobs = { ...trackedJobs };
            delete nextTrackedJobs[jobID];
            trackedJobs = nextTrackedJobs;
            wakeAdmissionWaiters();
          }
          changed = true;
        } catch (error) {
          const message = errorMessage(error);
          const current = items[index];
          if (current) items[index] = { ...current, error: message };
          status = message;
        }
      }
      if (!busy && !hasActiveJobs()) finishBatch();
      else refreshStatus();
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
    get activeJobID() { return pollJobID(); },
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
