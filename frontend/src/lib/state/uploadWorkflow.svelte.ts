import { untrack } from 'svelte';
import {
  countUploadStatuses,
  itemsFromJob,
  itemsFromResult,
  markUploadItemTagSyncAppliedInPlace,
  markUploadItemTagSyncErrorInPlace,
  normalizeUploadItemTags,
  queuedItem,
  rebaseUploadItemTagsFromRemoteInPlace,
  replaceUploadItemInPlace,
  retargetStagedUploadItems,
  setUploadItemTagsInPlace,
  stagedUploadItems,
  uploadItemTagSyncDelta,
  uploadProgressItem,
  uploadSummaryFromCounts,
  type UploadAddedAtStrategy,
  type UploadItem,
  type UploadItemTagSyncOperation,
  type UploadItemStatus,
  type UploadStatusCounts
} from './uploadItems';
import { errorMessage, isTerminalJob, parseTags } from '$lib/utils/format';
import type { Job, UploadImportResponse } from '$lib/api/types';
import type { BackgroundOperation } from '$lib/api/operations';
import { ApiError } from '$lib/api/client';
import type { UploadVariables } from '$lib/queries/library';
import { uploadJobStatusBatchSize } from '$lib/uploadBackpressure';
import { createUploadStagedTagCounts } from './uploadStagedTagCounts';

export { uploadJobStatusBatchSize } from '$lib/uploadBackpressure';
export const maxFilesPerMultipartUpload = 1000;
export const maxFilesPerGeckoMultipartUpload = 16;

export function multipartUploadChunkSize(userAgent = typeof navigator === 'undefined' ? '' : navigator.userAgent): number {
  return /\bGecko\/\d/i.test(userAgent) ? maxFilesPerGeckoMultipartUpload : maxFilesPerMultipartUpload;
}

type UploadMutate = (variables: UploadVariables) => Promise<BackgroundOperation | UploadImportResponse>;
type CancelJob = (jobID: string) => Promise<Job>;
type JobBatch = { items: Job[] };
type JobApplyResult = { completed: boolean; changedFiles: boolean };
type UploadClearScope = 'all' | 'staged' | 'done';
export type UploadStagedTagOperation = 'add' | 'remove' | 'set';

const doneUploadStatuses = new Set<UploadItemStatus>([
  'imported',
  'uploaded',
  'duplicate_existing',
  'duplicate_in_batch',
  'skipped',
  'error',
  'canceled'
]);

export interface UploadSubmissionSegment {
  start: number;
  end: number;
}

export function uploadSubmissionSegments(itemCount: number, maxFiles: number): UploadSubmissionSegment[] {
  const count = Math.max(0, Math.trunc(itemCount));
  if (!count) return [];
  const limit = Math.max(1, Math.trunc(maxFiles));
  const segments: UploadSubmissionSegment[] = [];
  for (let start = 0; start < count; start += limit) {
    segments.push({ start, end: Math.min(count, start + limit) });
  }
  return segments;
}

export function uploadQueueTimeBounds(queueTimes: number[]): { first: number; last: number } {
  if (!queueTimes.length) return { first: 0, last: 0 };
  let first = queueTimes[0]!;
  let last = queueTimes[0]!;
  for (let index = 1; index < queueTimes.length; index += 1) {
    const value = queueTimes[index]!;
    if (value < first) first = value;
    if (value > last) last = value;
  }
  return { first, last };
}

export function perFileUploadProgress(files: File[], aggregateProgress: number): number[] {
  if (!files.length) return [];
  const safeAggregate = Math.max(0, Math.min(100, aggregateProgress));
  const weights = files.map((file) => Math.max(1, file.size));
  const totalWeight = weights.reduce((sum, weight) => sum + weight, 0);
  const estimatedLoaded = totalWeight * safeAggregate / 100;
  let offset = 0;
  return weights.map((weight) => {
    const loaded = Math.max(0, Math.min(weight, estimatedLoaded - offset));
    offset += weight;
    return Math.round((loaded / weight) * 100);
  });
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
  let activeSubmissions = $state(0);
  const activeSubmissionControllers = new Set<AbortController>();
  let cancelBusy = $state(false);
  let cancelPending = false;
  let cancelPendingMutate: CancelJob | undefined;
  let status = $state('');
  let trackedJobs = $state<Record<string, number[]>>({});
  let statusCounts: UploadStatusCounts = {};
  let nextBatchID = 0;
  const stagedTagCounts = createUploadStagedTagCounts();
  let stagedTagCandidates = $state(stagedTagCounts.candidates());

  function refreshStagedTagCandidates() {
    stagedTagCandidates = stagedTagCounts.candidates();
  }

  function clearStagedTagCandidates() {
    stagedTagCounts.clear();
    stagedTagCandidates = [];
  }

  $effect(() => {
    if (activeSubmissions <= 0) return;
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  });

  function reset() {
    for (const controller of activeSubmissionControllers) controller.abort();
    activeSubmissionControllers.clear();
    files = [];
    items = [];
    tags = '';
    managedDefaultTags = [];
    conflictPolicy = 'rename';
    addedAtStrategy = 'queue';
    autoUpload = false;
    activeSubmissions = 0;
    cancelBusy = false;
    cancelPending = false;
    cancelPendingMutate = undefined;
    status = '';
    trackedJobs = {};
    statusCounts = {};
    nextBatchID = 0;
    clearStagedTagCandidates();
  }

  function clear(scope: UploadClearScope = 'all') {
    if (scope === 'all') {
      files = [];
      items = [];
      status = '';
      statusCounts = {};
      clearStagedTagCandidates();
      return;
    }
    if (scope === 'done' && (activeSubmissions > 0 || hasActiveJobs())) return;

    if (scope === 'staged') {
      files = [];
      items = items.filter((item) => item.status !== 'staged');
      clearStagedTagCandidates();
    } else {
      items = items.filter((item) => !doneUploadStatuses.has(item.status) || item.tagSyncPending);
    }
    statusCounts = countUploadStatuses(items);
    status = items.some((item) => item.status !== 'staged') ? uploadSummaryFromCounts(statusCounts) : '';
  }

  function removeAt(index: number) {
    const removedItem = items[index];
    if (removedItem?.status === 'staged') {
      stagedTagCounts.remove(removedItem.tags ?? []);
      refreshStagedTagCandidates();
    }
    const stagedIndices = items.flatMap((item, itemIndex) => item.status === 'staged' ? [itemIndex] : []);
    const stagedFileIndex = stagedIndices.indexOf(index);
    if (stagedFileIndex >= 0) {
      files = files.filter((_, fileIndex) => fileIndex !== stagedFileIndex);
    }
    items = items.filter((_, itemIndex) => itemIndex !== index);
    statusCounts = countUploadStatuses(items);
    status = items.some((item) => item.status !== 'staged') ? uploadSummaryFromCounts(statusCounts) : '';
  }

  function setItemTags(index: number, nextTags: string[]) {
    const current = items[index];
    const previousStagedTags = current?.status === 'staged' ? [...(current.tags ?? [])] : undefined;
    setUploadItemTagsInPlace(items, index, nextTags);
    if (previousStagedTags && current) {
      stagedTagCounts.replace(previousStagedTags, current.tags ?? []);
      refreshStagedTagCandidates();
    }
  }

  function applyStagedTags(operation: UploadStagedTagOperation, rawTags: string[]) {
    const operand = normalizeUploadItemTags(rawTags);
    if (operation !== 'set' && operand.length === 0) return false;

    const removed = operation === 'remove' ? new Set(operand) : null;
    let changed = false;
    for (let index = 0; index < items.length; index += 1) {
      const current = items[index];
      if (!current || current.status !== 'staged') continue;

      const previous = normalizeUploadItemTags(current.tags ?? []);
      const next = operation === 'set'
        ? operand
        : operation === 'add'
          ? normalizeUploadItemTags([...previous, ...operand])
          : previous.filter((tag) => !removed!.has(tag));
      if (previous.length === next.length && previous.every((tag, tagIndex) => tag === next[tagIndex])) continue;

      setUploadItemTagsInPlace(items, index, next);
      stagedTagCounts.replace(previous, current.tags ?? []);
      changed = true;
    }
    if (changed) refreshStagedTagCandidates();
    return changed;
  }

  function itemTagSyncDelta(index: number) {
    const current = items[index];
    return current ? uploadItemTagSyncDelta(current) : { add: [], remove: [] };
  }

  function markItemTagSyncApplied(index: number, operation: UploadItemTagSyncOperation, tags: string[]) {
    markUploadItemTagSyncAppliedInPlace(items, index, operation, tags);
  }

  function rebaseItemTagsFromRemote(index: number, remoteTags: string[], expectedBaseTags: string[] | undefined) {
    return rebaseUploadItemTagsFromRemoteInPlace(items, index, remoteTags, expectedBaseTags);
  }

  function markItemTagSyncError(index: number, message: string) {
    markUploadItemTagSyncErrorInPlace(items, index, message);
  }

  function hasActiveJobs() {
    return Object.keys(trackedJobs).length > 0;
  }

  function pollJobID() {
    return Object.keys(trackedJobs).slice(0, uploadJobStatusBatchSize).join(',');
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

    const queueTimeMs = Date.now();
    const stagedAdditions = stagedUploadItems(additions, targetID, queueTimeMs, parseTags(tags));
    files = [...files, ...additions];
    items = [...items, ...stagedAdditions];
    for (const item of stagedAdditions) stagedTagCounts.add(item.tags ?? []);
    refreshStagedTagCandidates();
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
      const itemIndices = trackedJobs[job?.id];
      if (!job || itemIndices === undefined) return { completed: false, changedFiles: false };
      const currentItems = itemIndices.map((itemIndex) => items[itemIndex]).filter((item) => Boolean(item));
      const nextItems = itemsFromJob(currentItems, job);
      itemIndices.forEach((itemIndex, resultIndex) => replaceItem(itemIndex, nextItems[resultIndex]));
      if (!isTerminalJob(job)) return { completed: false, changedFiles: false };

      const nextTrackedJobs = { ...trackedJobs };
      delete nextTrackedJobs[job.id];
      trackedJobs = nextTrackedJobs;
      const changedFiles = job.status === 'completed';
      if (activeSubmissions === 0 && !hasActiveJobs()) finishBatch();
      else refreshStatus();
      return { completed: true, changedFiles };
    });
  }

  function applyJobError(error: unknown) {
    untrack(() => {
      const jobID = Object.keys(trackedJobs)[0];
      if (!jobID) return;
      const itemIndices = trackedJobs[jobID] ?? [];
      status = errorMessage(error);
      for (const itemIndex of itemIndices) {
        const current = items[itemIndex];
        if (current) items[itemIndex] = { ...current, error: status };
      }
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
    status = uploadSummaryFromCounts(statusCounts) || 'Upload finished';
  }

  function trackQueuedJob(jobID: string, itemIndices: number[], itemError = '') {
    const existingIndices = trackedJobs[jobID] ?? [];
    trackedJobs = { ...trackedJobs, [jobID]: [...existingIndices, ...itemIndices] };
    for (const itemIndex of itemIndices) {
      const current = items[itemIndex];
      const queuedItemState = current ? queuedItem([current], 0)[0] : undefined;
      replaceItem(itemIndex, queuedItemState && itemError ? { ...queuedItemState, error: itemError } : queuedItemState);
    }
  }

  async function submit(mutate: UploadMutate) {
    if (!files.length) return { queued: false, changedFiles: false };

    const batchFiles = [...files];
    if (!items.length) items = stagedUploadItems(batchFiles, targetID, Date.now(), parseTags(tags));
    const batchItemIndices = items.flatMap((item, itemIndex) => item.status === 'staged' ? [itemIndex] : []);
    if (batchItemIndices.length !== batchFiles.length) {
      status = 'Upload queue changed unexpectedly; please restage the pending files';
      return { queued: false, changedFiles: false };
    }

    const parsedTags = parseTags(tags);
    const segments = uploadSubmissionSegments(batchItemIndices.length, multipartUploadChunkSize());
    const submittedItemTags = batchItemIndices.map((itemIndex) => [
      ...(items[itemIndex]?.tags ?? parsedTags)
    ]);
    const batchID = ++nextBatchID;
    const nextItems = [...items];
    for (const [submissionIndex, itemIndex] of batchItemIndices.entries()) {
      const current = nextItems[itemIndex];
      if (current) {
        const submittedTags = submittedItemTags[submissionIndex] ?? parsedTags;
        const syncBaseItem: UploadItem = {
          ...current,
          tags: [...(current.tags ?? parsedTags)],
          tagSyncBaseTags: [...submittedTags]
        };
        const syncDelta = uploadItemTagSyncDelta(syncBaseItem);
        nextItems[itemIndex] = {
          ...syncBaseItem,
          batchID,
          tagSyncPending: syncDelta.add.length > 0 || syncDelta.remove.length > 0,
          tagSyncError: '',
          status: 'uploading',
          progress: 0,
          error: ''
        };
      }
    }
    items = nextItems;
    files = [];
    clearStagedTagCandidates();
    const controller = new AbortController();
    activeSubmissionControllers.add(controller);
    activeSubmissions += 1;
    statusCounts = countUploadStatuses(items);

    const batchTargetID = targetID;
    const batchConflictPolicy = conflictPolicy;
    const batchAddedAtStrategy = addedAtStrategy;
    const fallbackQueueTimeMs = Date.now();
    const batchQueueTimes = batchItemIndices.map((itemIndex) => items[itemIndex]?.queueTimeMs ?? fallbackQueueTimeMs);
    const batchQueueTimeBounds = uploadQueueTimeBounds(batchQueueTimes);
    const batchQueueFirstTimeMs = batchQueueTimeBounds.first;
    const batchQueueLastTimeMs = batchQueueTimeBounds.last;
    const batchQueueTotal = batchFiles.length;
    let queued = false;
    let changedFiles = false;
    let currentChunkStart = 0;
    const segmentCount = segments.length;
    let logicalOperationID = '';

    refreshStatus();

    try {
      for (let segmentIndex = 0; segmentIndex < segments.length; segmentIndex += 1) {
        const segment = segments[segmentIndex];
        currentChunkStart = segment.start;
        if (controller.signal.aborted || cancelPending) {
          throw new ApiError(0, 'request_aborted', 'Upload was canceled');
        }
        const chunkEnd = segment.end;
        const chunkFiles = batchFiles.slice(currentChunkStart, chunkEnd);
        const chunkItemIndices = batchItemIndices.slice(currentChunkStart, chunkEnd);
        const chunkQueueTimes = batchQueueTimes.slice(currentChunkStart, chunkEnd);
        const chunkQueueIndices = chunkFiles.map((_, index) => currentChunkStart + index);
        const chunkQueueTotals = chunkFiles.map(() => batchQueueTotal);
        let lastTransportProgress = -1;

        const response = await mutate({
          files: chunkFiles,
          tags: parsedTags,
          itemTags: submittedItemTags.slice(currentChunkStart, chunkEnd).map((itemTags) => [...itemTags]),
          preferAsync: true,
          targetID: batchTargetID,
          conflictPolicy: batchConflictPolicy,
          addedAtStrategy: batchAddedAtStrategy,
          queueTimeMs: chunkQueueTimes,
          queueFirstTimeMs: batchQueueFirstTimeMs,
          queueLastTimeMs: batchQueueLastTimeMs,
          queueIndex: chunkQueueIndices,
          queueTotal: chunkQueueTotals,
          operationID: segmentCount > 1 && logicalOperationID ? logicalOperationID : undefined,
          segmentIndex: segmentCount > 1 ? segmentIndex : undefined,
          segmentCount,
          onProgress: (progress) => {
            if (progress === lastTransportProgress) return;
            lastTransportProgress = progress;
            const fileProgress = perFileUploadProgress(chunkFiles, progress);
            chunkItemIndices.forEach((itemIndex, fileIndex) => {
              const current = items[itemIndex];
              const nextProgress = fileProgress[fileIndex] ?? progress;
              if (!current || (current.status === 'uploading' && current.progress === nextProgress)) return;
              replaceItem(itemIndex, uploadProgressItem([current], 0, nextProgress)[0]);
            });
          },
          signal: controller.signal
        });

        if ('id' in response) {
          const jobID = logicalOperationID || response.id;
          if (segmentCount > 1 && !logicalOperationID) logicalOperationID = response.id;
          if (cancelPending && cancelPendingMutate) {
            try {
              const canceledJob = await cancelPendingMutate(jobID);
              const previousItems = chunkItemIndices.map((itemIndex) => items[itemIndex]).filter((item) => Boolean(item));
              const canceledItems = itemsFromJob(previousItems, canceledJob);
              chunkItemIndices.forEach((itemIndex, resultIndex) => replaceItem(itemIndex, canceledItems[resultIndex]));
            } catch (error) {
              const message = errorMessage(error);
              trackQueuedJob(jobID, chunkItemIndices, message);
              status = message;
              queued = true;
            }
            throw new ApiError(0, 'request_aborted', 'Upload was canceled');
          }
          trackQueuedJob(jobID, chunkItemIndices);
          queued = true;
        } else {
          const previousItems = chunkItemIndices.map((itemIndex) => items[itemIndex]).filter((item) => Boolean(item));
          const resultItems = itemsFromResult(response, previousItems);
          chunkItemIndices.forEach((itemIndex, resultIndex) => replaceItem(itemIndex, resultItems[resultIndex]));
          changedFiles = true;
        }
      }
    } catch (error) {
      const affectedItemIndices = batchItemIndices.slice(currentChunkStart);
      if (controller.signal.aborted || (error instanceof ApiError && error.code === 'request_aborted')) {
        for (const itemIndex of affectedItemIndices) {
          const current = items[itemIndex];
          if (current && current.status === 'uploading') replaceItem(itemIndex, { ...current, status: 'canceled', error: '' });
        }
      } else {
        const message = errorMessage(error);
        for (const itemIndex of affectedItemIndices) {
          const current = items[itemIndex];
          if (current && current.status === 'uploading') replaceItem(itemIndex, { ...current, status: 'error', error: message });
        }
      }
    } finally {
      activeSubmissionControllers.delete(controller);
      activeSubmissions = Math.max(0, activeSubmissions - 1);
      if (activeSubmissions === 0 && cancelPending) {
        cancelPending = false;
        cancelPendingMutate = undefined;
        cancelBusy = false;
      }
      if (activeSubmissions === 0 && !hasActiveJobs()) finishBatch();
      else refreshStatus();
    }
    return { queued, changedFiles };
  }

  async function cancel(mutate: CancelJob, _jobID = Object.keys(trackedJobs)[0] ?? '') {
    const jobIDs = Object.keys(trackedJobs);
    if ((!jobIDs.length && activeSubmissions === 0) || cancelBusy) return { changed: false };
    cancelBusy = true;
    let changed = false;
    if (activeSubmissions > 0) {
      cancelPending = true;
      cancelPendingMutate = mutate;
      for (const controller of activeSubmissionControllers) controller.abort();
      changed = true;
    }
    try {
      for (const jobID of jobIDs) {
        const itemIndices = trackedJobs[jobID];
        if (!itemIndices) continue;
        try {
          const job = await mutate(jobID);
          applyJob(job);
          changed = true;
        } catch (error) {
          const message = errorMessage(error);
          for (const itemIndex of itemIndices) {
            const current = items[itemIndex];
            if (current) items[itemIndex] = { ...current, error: message };
          }
          status = message;
        }
      }
      if (activeSubmissions === 0 && !hasActiveJobs()) finishBatch();
      else refreshStatus();
      return { changed };
    } finally {
      if (!cancelPending) cancelBusy = false;
    }
  }

  return {
    get files() { return files; },
    get items() { return items; },
    get tags() { return tags; },
    set tags(value: string) { tags = value; },
    get stagedTagCandidates() { return stagedTagCandidates; },
    get targetID() { return targetID; },
    get conflictPolicy() { return conflictPolicy; },
    set conflictPolicy(value: string) { conflictPolicy = value; },
    get addedAtStrategy() { return addedAtStrategy; },
    set addedAtStrategy(value: UploadAddedAtStrategy) { addedAtStrategy = value; },
    get autoUpload() { return autoUpload; },
    set autoUpload(value: boolean) { autoUpload = value; },
    get busy() { return activeSubmissions > 0; },
    get cancelBusy() { return cancelBusy; },
    get status() { return status; },
    get activeJobID() { return pollJobID(); },
    get activeJobIDs() { return Object.keys(trackedJobs); },
    reset,
    clear,
    removeAt,
    setItemTags,
    applyStagedTags,
    itemTagSyncDelta,
    markItemTagSyncApplied,
    rebaseItemTagsFromRemote,
    markItemTagSyncError,
    select,
    setTarget,
    applyJob,
    applyJobError,
    applyJobs,
    submit,
    cancel
  };
}
