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
  waitingUploadItems,
  type UploadAddedAtStrategy,
  type UploadItem,
  type UploadStatusCounts
} from './uploadItems';
import { errorMessage, isTerminalJob, parseTags } from '$lib/utils/format';
import type { Job, UploadImportResponse } from '$lib/api/types';
import type { UploadVariables } from '$lib/queries/library';

type UploadMutate = (variables: UploadVariables) => Promise<Job | UploadImportResponse>;
type CancelJob = (jobID: string) => Promise<Job>;
type JobBatch = { items: Job[] };
type JobApplyResult = { completed: boolean; changedFiles: boolean };

export const browserUploadConcurrency = 4;
export const uploadJobStatusBatchSize = 64;

export function createUploadWorkflow() {
  let files = $state<File[]>([]);
  let items = $state<UploadItem[]>([]);
  let tags = $state('');
  let targetID = $state('');
  let managedDefaultTags = $state<string[]>([]);
  let conflictPolicy = $state('skip');
  let addedAtStrategy = $state<UploadAddedAtStrategy>('queue');
  let autoUpload = $state(false);
  let busy = $state(false);
  let cancelBusy = $state(false);
  let status = $state('');
  let trackedJobs = $state<Record<string, number>>({});
  let statusCounts: UploadStatusCounts = {};

  $effect(() => {
    if (!busy) return;
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  });

  function reset() {
    files = [];
    items = [];
    tags = '';
    managedDefaultTags = [];
    conflictPolicy = 'skip';
    addedAtStrategy = 'queue';
    autoUpload = false;
    busy = false;
    cancelBusy = false;
    status = '';
    trackedJobs = {};
    statusCounts = {};
  }

  function clear() {
    files = [];
    items = [];
    status = '';
    statusCounts = {};
  }

  function removeAt(index: number) {
    files = files.filter((_, fileIndex) => fileIndex !== index);
    items = items.filter((_, itemIndex) => itemIndex !== index);
    status = '';
    statusCounts = countUploadStatuses(items);
  }

  function hasActiveJobs() {
    return Object.keys(trackedJobs).length > 0;
  }

  function pollJobID() {
    const ids = Object.keys(trackedJobs);
    if (busy && ids.length < uploadJobStatusBatchSize) return '';
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
    status = uploadSummaryFromCounts(statusCounts) || 'Upload finished';
  }

  async function submit(mutate: UploadMutate) {
    if (!files.length || busy || hasActiveJobs()) return { queued: false, changedFiles: false };
    busy = true;
    trackedJobs = {};
    items = waitingUploadItems(items.length ? items : stagedUploadItems(files, targetID));
    statusCounts = countUploadStatuses(items);
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
        const current = items[index];
        replaceItem(index, current ? uploadingItem([current], 0)[0] : undefined);
        refreshStatus();
        try {
          const response = await mutate({
            files: [file],
            tags: parsedTags,
            // Each WebUI request already represents one file and is bounded by
            // browserUploadConcurrency. Keep the request open through the
            // typically short import so this row receives its final state as
            // soon as the request finishes instead of creating a second,
            // timer-driven job-status protocol for every file.
            preferAsync: false,
            targetID: batchTargetID,
            conflictPolicy: batchConflictPolicy,
            addedAtStrategy: batchAddedAtStrategy,
            queueTimeMs: batchQueueTimes[index],
            queueFirstTimeMs: batchQueueFirstTimeMs,
            queueLastTimeMs: batchQueueLastTimeMs,
            queueIndex: index,
            queueTotal: batchQueueTotal,
            onProgress: (progress) => {
              const progressItem = items[index];
              replaceItem(index, progressItem ? uploadProgressItem([progressItem], 0, progress)[0] : undefined);
            }
          });
          if ('id' in response) {
            // Retain async-response compatibility for callers/servers that
            // explicitly return jobs despite the WebUI's inline preference.
            trackedJobs = { ...trackedJobs, [response.id]: index };
            const queuedItemState = items[index];
            replaceItem(index, queuedItemState ? queuedItem([queuedItemState], 0)[0] : undefined);
            queued += 1;
          } else {
            const resultItem = items[index];
            replaceItem(index, resultItem ? itemFromResult([resultItem], 0, response)[0] : undefined);
            changedFiles = true;
          }
        } catch (error) {
          const message = errorMessage(error);
          const failed = items[index];
          if (failed) replaceItem(index, { ...failed, status: 'error', error: message });
        }
        refreshStatus();
      }
    }

    const workerCount = Math.min(browserUploadConcurrency, batchFiles.length);
    await Promise.all(Array.from({ length: workerCount }, () => uploadNext()));

    busy = false;
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
