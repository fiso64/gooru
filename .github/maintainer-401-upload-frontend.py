from pathlib import Path


def replace(path, old, new, count=1):
    p = Path(path)
    text = p.read_text()
    actual = text.count(old)
    if actual != count:
        raise SystemExit(f"{path}: expected {count} occurrences, found {actual}: {old[:100]!r}")
    p.write_text(text.replace(old, new))


def replace_between(path, start, end, replacement):
    p = Path(path)
    text = p.read_text()
    i = text.find(start)
    if i < 0:
        raise SystemExit(f"{path}: start marker missing: {start!r}")
    j = text.find(end, i)
    if j < 0:
        raise SystemExit(f"{path}: end marker missing: {end!r}")
    p.write_text(text[:i] + replacement + text[j:])


# Upload mutation metadata now describes the whole multipart batch.
replace("frontend/src/lib/queries/library.ts", "  queueTimeMs: number;", "  queueTimeMs: number[];")
replace("frontend/src/lib/queries/library.ts", "  queueIndex: number;", "  queueIndex: number[];")
replace("frontend/src/lib/queries/library.ts", "  queueTotal: number;", "  queueTotal: number[];")
replace("frontend/src/lib/queries/library.ts", "        queueTimeMs: [queueTimeMs],", "        queueTimeMs,")
replace("frontend/src/lib/queries/library.ts", "        queueIndex: [queueIndex],", "        queueIndex,")
replace("frontend/src/lib/queries/library.ts", "        queueTotal: [queueTotal]", "        queueTotal")

path = "frontend/src/lib/state/uploadWorkflow.svelte.ts"
p = Path(path)
text = p.read_text()
text = text.replace("  itemFromJob,\n  itemFromResult,", "  itemsFromJob,\n  itemsFromResult,", 1)
text = text.replace("export const browserUploadConcurrency = 4;\n\n", "", 1)
text = text.replace("let trackedJobs = $state<Record<string, number>>({});", "let trackedJobs = $state<Record<string, number[]>>({});", 1)
p.write_text(text)

replace_between(
    path,
    "  function applyJob(job: Job | JobBatch): JobApplyResult {",
    "\n  function applyJobError(error: unknown) {",
    '''  function applyJob(job: Job | JobBatch): JobApplyResult {
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
      wakeAdmissionWaiters();
      const changedFiles = job.status === 'completed';
      if (!busy && !hasActiveJobs()) finishBatch();
      else refreshStatus();
      return { completed: true, changedFiles };
    });
  }
''',
)
replace_between(
    path,
    "  function applyJobError(error: unknown) {",
    "\n  function applyJobs(jobs: Job[]): JobApplyResult {",
    '''  function applyJobError(error: unknown) {
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
''',
)
# A selected batch has one durable admission, so the old local accepted-job window is obsolete.
replace_between(
    path,
    "  async function waitForJobAdmissionSlot() {",
    "\n  async function submit(mutate: UploadMutate) {",
    "",
)
replace_between(
    path,
    "  async function submit(mutate: UploadMutate) {",
    "\n  async function cancel(mutate: CancelJob, _jobID = Object.keys(trackedJobs)[0] ?? '') {",
    '''  async function submit(mutate: UploadMutate) {
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
    const nextItems = [...items];
    for (const itemIndex of batchItemIndices) {
      const current = nextItems[itemIndex];
      if (current) nextItems[itemIndex] = { ...current, status: 'waiting', progress: 0, error: '' };
    }
    items = nextItems;
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
    const batchQueueIndices = batchFiles.map((_, index) => index);
    const batchQueueTotals = batchFiles.map(() => batchQueueTotal);
    let queued = false;
    let changedFiles = false;

    for (const itemIndex of batchItemIndices) {
      const current = items[itemIndex];
      replaceItem(itemIndex, current ? uploadingItem([current], 0)[0] : undefined);
    }
    refreshStatus();

    try {
      let response: BackgroundOperation | UploadImportResponse;
      for (;;) {
        admissionBackpressured = false;
        try {
          response = await mutate({
            files: batchFiles,
            tags: parsedTags,
            preferAsync: true,
            targetID: batchTargetID,
            conflictPolicy: batchConflictPolicy,
            addedAtStrategy: batchAddedAtStrategy,
            queueTimeMs: batchQueueTimes,
            queueFirstTimeMs: batchQueueFirstTimeMs,
            queueLastTimeMs: batchQueueLastTimeMs,
            queueIndex: batchQueueIndices,
            queueTotal: batchQueueTotals,
            onProgress: (progress) => {
              for (const itemIndex of batchItemIndices) {
                const current = items[itemIndex];
                replaceItem(itemIndex, current ? uploadProgressItem([current], 0, progress)[0] : undefined);
              }
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
        trackedJobs = { ...trackedJobs, [response.id]: [...batchItemIndices] };
        for (const itemIndex of batchItemIndices) {
          const current = items[itemIndex];
          replaceItem(itemIndex, current ? queuedItem([current], 0)[0] : undefined);
        }
        queued = true;
      } else {
        const previousItems = batchItemIndices.map((itemIndex) => items[itemIndex]).filter((item) => Boolean(item));
        const resultItems = itemsFromResult(response, previousItems);
        batchItemIndices.forEach((itemIndex, resultIndex) => replaceItem(itemIndex, resultItems[resultIndex]));
        changedFiles = true;
      }
    } catch (error) {
      const message = errorMessage(error);
      for (const itemIndex of batchItemIndices) {
        const current = items[itemIndex];
        if (current) replaceItem(itemIndex, { ...current, status: 'error', error: message });
      }
    }

    busy = false;
    admissionBackpressured = false;
    if (hasActiveJobs()) refreshStatus();
    else finishBatch();
    return { queued, changedFiles };
  }
''',
)
replace_between(
    path,
    "  async function cancel(mutate: CancelJob, _jobID = Object.keys(trackedJobs)[0] ?? '') {",
    "\n  return {",
    '''  async function cancel(mutate: CancelJob, _jobID = Object.keys(trackedJobs)[0] ?? '') {
    const jobIDs = Object.keys(trackedJobs);
    if (!jobIDs.length || cancelBusy) return { changed: false };
    cancelBusy = true;
    let changed = false;
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
      if (!busy && !hasActiveJobs()) finishBatch();
      else refreshStatus();
      return { changed };
    } finally {
      cancelBusy = false;
    }
  }
''',
)

# Replace per-file-worker tests with aggregate-operation behavior.
Path("frontend/src/lib/state/uploadWorkflow.svelte.test.ts").write_text('''import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';
import type { BackgroundOperation } from '$lib/api/operations';

const { untrackSpy } = vi.hoisted(() => ({
  untrackSpy: vi.fn((value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value)
}));

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: untrackSpy };
});

import { createUploadWorkflow } from './uploadWorkflow.svelte';

function uploadFile(name: string): File {
  return { name, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
}

function pendingJob(id: string, completed = 0, total = 2): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: completed > 0 ? 'running' : 'pending',
    progress: total > 0 ? completed / total : 0,
    progress_total: total,
    progress_completed: completed,
    progress_failed: 0,
    submitted_at: '2026-09-11T00:00:00Z',
    created_at: '2026-09-11T00:00:00Z'
  } as Job & BackgroundOperation;
}

describe('createUploadWorkflow aggregate uploads', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('defaults browser uploads to rename conflicts', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    let policy = '';
    await workflow.submit(async (variables) => {
      policy = variables.conflictPolicy;
      return pendingJob('job-batch', 0, 1);
    });
    expect(policy).toBe('rename');
  });

  it('appends later file selections to one staged batch', () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    workflow.select([uploadFile('second.jpg')]);
    expect(workflow.files.map((file) => file.name)).toEqual(['first.jpg', 'second.jpg']);
  });

  it('submits one durable operation with per-file queue metadata arrays', async () => {
    const now = vi.spyOn(Date, 'now');
    now.mockReturnValueOnce(1_700_000_000_000).mockReturnValueOnce(1_700_000_010_000).mockReturnValue(1_800_000_000_000);
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg')]);
    workflow.select([uploadFile('second.jpg')]);
    workflow.setTarget('archive', 'reverse_queue');
    let calls = 0;

    await workflow.submit(async (variables) => {
      calls += 1;
      expect(variables.files.map((file) => file.name)).toEqual(['first.jpg', 'second.jpg']);
      expect(variables.queueTimeMs).toEqual([1_700_000_000_000, 1_700_000_010_000]);
      expect(variables.queueFirstTimeMs).toBe(1_700_000_000_000);
      expect(variables.queueLastTimeMs).toBe(1_700_000_010_000);
      expect(variables.queueIndex).toEqual([0, 1]);
      expect(variables.queueTotal).toEqual([2, 2]);
      expect(variables.addedAtStrategy).toBe('reverse_queue');
      variables.onProgress?.(37);
      expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
        ['uploading', 37],
        ['uploading', 37]
      ]);
      return pendingJob('job-batch');
    });

    expect(calls).toBe(1);
    expect(workflow.activeJobIDs).toEqual(['job-batch']);
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['queued', 100],
      ['queued', 100]
    ]);
    now.mockRestore();
  });

  it('applies aggregate running progress to every file in the batch', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch'));

    expect(workflow.applyJob(pendingJob('job-batch', 1, 2))).toEqual({ completed: false, changedFiles: false });
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['importing', 50],
      ['importing', 50]
    ]);
    expect(workflow.activeJobID).toBe('job-batch');
  });

  it('maps one completed aggregate result back to all staged items', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch'));
    const completed = {
      ...pendingJob('job-batch', 2, 2),
      status: 'completed',
      progress: 1,
      result: {
        files: [
          { name: 'first.jpg', size: 10, target_id: 'default', status: 'imported' },
          { name: 'second.jpg', size: 10, target_id: 'default', status: 'duplicate_existing' }
        ]
      }
    } as Job;

    expect(workflow.applyJob(completed)).toEqual({ completed: true, changedFiles: true });
    expect(workflow.activeJobIDs).toEqual([]);
    expect(workflow.items.map((item) => item.status)).toEqual(['imported', 'duplicate_existing']);
  });

  it('marks the whole selection failed when the aggregate transport fails', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async (variables) => {
      variables.onProgress?.(43);
      throw new Error('Network error while uploading files');
    });
    expect(workflow.items.map((item) => [item.status, item.progress])).toEqual([
      ['error', 43],
      ['error', 43]
    ]);
    expect(workflow.items.every((item) => item.error?.includes('Network error'))).toBe(true);
  });

  it('cancels the single durable operation for the whole batch', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('first.jpg'), uploadFile('second.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch'));
    const canceled: string[] = [];
    const result = await workflow.cancel(async (id) => {
      canceled.push(id);
      return { ...pendingJob(id), status: 'canceled' } as Job;
    });
    expect(result).toEqual({ changed: true });
    expect(canceled).toEqual(['job-batch']);
    expect(workflow.items.map((item) => item.status)).toEqual(['canceled', 'canceled']);
  });

  it('does not let a new drop overwrite an active aggregate batch', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile('queued.jpg')]);
    await workflow.submit(async () => pendingJob('job-batch', 0, 1));
    workflow.select([uploadFile('later.jpg')]);
    expect(workflow.files.map((file) => file.name)).toEqual(['queued.jpg']);
    expect(workflow.status).toContain('Upload in progress');
  });

  it('preserves managed target defaults and user tags after direct completion', async () => {
    const workflow = createUploadWorkflow();
    workflow.setTarget('one', 'queue', ['project:inbox']);
    workflow.tags = 'project:inbox user:custom';
    workflow.select([uploadFile('first.jpg')]);
    await workflow.submit(async () => ({
      files: [{ name: 'first.jpg', size: 10, target_id: 'one', status: 'uploaded' }]
    } as never));
    expect(workflow.files).toEqual([]);
    expect(workflow.tags).toBe('project:inbox user:custom');
  });
});
''')

Path("frontend/src/lib/state/uploadWorkflow.admission.test.ts").write_text('''import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';
import type { BackgroundOperation } from '$lib/api/operations';
import { ApiError } from '$lib/api/client';
import { uploadAdmissionFallbackMs } from '$lib/uploadBackpressure';

const { untrackSpy } = vi.hoisted(() => ({
  untrackSpy: vi.fn((value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value)
}));

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: untrackSpy };
});

import { createUploadWorkflow } from './uploadWorkflow.svelte';

function uploadFile(index: number): File {
  return { name: `file-${index}.jpg`, size: 10, type: 'image/jpeg', lastModified: 0 } as File;
}

function pendingJob(id: string): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: 3,
    progress_completed: 0,
    progress_failed: 0,
    submitted_at: '2026-09-11T00:00:00Z',
    created_at: '2026-09-11T00:00:00Z'
  } as Job & BackgroundOperation;
}

describe('aggregate upload admission backpressure', () => {
  beforeEach(() => {
    untrackSpy.mockClear();
    vi.useFakeTimers();
  });

  it('retries the same aggregate batch after durable admission is full', async () => {
    const workflow = createUploadWorkflow();
    workflow.select([uploadFile(1), uploadFile(2), uploadFile(3)]);
    let calls = 0;
    const submission = workflow.submit(async (variables) => {
      calls += 1;
      expect(variables.files).toHaveLength(3);
      if (calls === 1) throw new ApiError(503, 'job_queue_full', 'job queue is full');
      return pendingJob('job-batch');
    });

    await vi.advanceTimersByTimeAsync(uploadAdmissionFallbackMs);
    await submission;
    expect(calls).toBe(2);
    expect(workflow.activeJobIDs).toEqual(['job-batch']);
    vi.useRealTimers();
  });
});
''')
