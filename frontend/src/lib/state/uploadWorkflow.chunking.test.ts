import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';
import type { BackgroundOperation } from '$lib/api/operations';
import { ApiError } from '$lib/api/client';

const { untrackSpy } = vi.hoisted(() => ({
  untrackSpy: vi.fn((value: unknown) => typeof value === 'function' ? (value as () => unknown)() : value)
}));

vi.mock('svelte', async () => {
  const actual = await vi.importActual<typeof import('svelte')>('svelte');
  return { ...actual, untrack: untrackSpy };
});

import {
  createUploadWorkflow,
  maxFilesPerGeckoMultipartUpload,
  maxFilesPerMultipartUpload,
  multipartUploadChunkSize
} from './uploadWorkflow.svelte';

function uploadFile(index: number): File {
  return { name: `file-${index}.jpg`, size: 10, type: 'image/jpeg', lastModified: index } as File;
}

function pendingJob(id: string, total: number): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: total,
    progress_completed: 0,
    progress_completed_prefix: 0,
    progress_failed: 0,
    submitted_at: '2026-09-12T00:00:00Z',
    created_at: '2026-09-12T00:00:00Z'
  } as Job & BackgroundOperation;
}

describe('createUploadWorkflow bounded multipart submissions', () => {
  beforeEach(() => untrackSpy.mockClear());

  it('uses a conservative multipart bound only for Gecko browsers', () => {
    expect(multipartUploadChunkSize('Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0')).toBe(maxFilesPerGeckoMultipartUpload);
    expect(multipartUploadChunkSize('Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36')).toBe(maxFilesPerMultipartUpload);
    expect(multipartUploadChunkSize('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15')).toBe(maxFilesPerMultipartUpload);
  });

  it('splits a large selection into bounded requests while preserving one logical job and global queue metadata', async () => {
    const workflow = createUploadWorkflow();
    const files = Array.from({ length: maxFilesPerMultipartUpload * 2 + 1 }, (_, index) => uploadFile(index));
    workflow.select(files);
    const calls: Array<{
      names: string[];
      queueIndex: number[];
      queueTotal: number[];
      queueTimeMs: number[];
      queueFirstTimeMs?: number;
      queueLastTimeMs?: number;
      operationID?: string;
      segmentIndex?: number;
      segmentCount?: number;
    }> = [];

    await workflow.submit(async (variables) => {
      calls.push({
        names: variables.files.map((file) => file.name),
        queueIndex: [...(variables.queueIndex ?? [])],
        queueTotal: [...(variables.queueTotal ?? [])],
        queueTimeMs: [...(variables.queueTimeMs ?? [])],
        queueFirstTimeMs: variables.queueFirstTimeMs,
        queueLastTimeMs: variables.queueLastTimeMs,
        operationID: variables.operationID,
        segmentIndex: variables.segmentIndex,
        segmentCount: variables.segmentCount
      });
      return pendingJob('job-logical', 3);
    });

    expect(calls.map((call) => call.names.length)).toEqual([1000, 1000, 1]);
    expect(calls[0]?.queueIndex).toEqual(Array.from({ length: 1000 }, (_, index) => index));
    expect(calls[1]?.queueIndex).toEqual(Array.from({ length: 1000 }, (_, index) => index + 1000));
    expect(calls[2]?.queueIndex).toEqual([2000]);
    expect(calls.map((call) => call.segmentIndex)).toEqual([0, 1, 2]);
    expect(calls.map((call) => call.segmentCount)).toEqual([3, 3, 3]);
    expect(calls.map((call) => call.operationID)).toEqual([undefined, 'job-logical', 'job-logical']);
    for (const call of calls) {
      expect(call.queueTotal).toEqual(Array.from({ length: call.names.length }, () => 2001));
      expect(call.queueTimeMs).toHaveLength(call.names.length);
      expect(call.queueFirstTimeMs).toBe(call.queueLastTimeMs);
    }
    expect(workflow.activeJobIDs).toEqual(['job-logical']);
    expect(workflow.items.map((item) => item.status)).toEqual(Array.from({ length: 2001 }, () => 'queued'));
  });

  it('keeps per-file tags aligned across a multipart segmentation boundary', async () => {
    const workflow = createUploadWorkflow();
    const files = Array.from({ length: maxFilesPerMultipartUpload + 1 }, (_, index) => uploadFile(index));
    workflow.tags = 'fallback';
    workflow.select(files);
    workflow.setItemTags(maxFilesPerMultipartUpload - 1, ['edge:left']);
    workflow.setItemTags(maxFilesPerMultipartUpload, ['edge:right']);
    const calls: Array<{ names: string[]; itemTags: string[][] }> = [];

    await workflow.submit(async (variables) => {
      calls.push({
        names: variables.files.map((file) => file.name),
        itemTags: variables.itemTags?.map((itemTags) => [...itemTags]) ?? []
      });
      return pendingJob('job-boundary-tags', 2);
    });

    expect(calls.map((call) => call.names.length)).toEqual([maxFilesPerMultipartUpload, 1]);
    expect(calls[0]?.names[maxFilesPerMultipartUpload - 1]).toBe(`file-${maxFilesPerMultipartUpload - 1}.jpg`);
    expect(calls[0]?.itemTags[maxFilesPerMultipartUpload - 1]).toEqual(['edge:left']);
    expect(calls[1]?.names[0]).toBe(`file-${maxFilesPerMultipartUpload}.jpg`);
    expect(calls[1]?.itemTags[0]).toEqual(['edge:right']);
  });

  it('keeps differing item tags in one request with aligned native per-file tags', async () => {
    const workflow = createUploadWorkflow();
    workflow.tags = 'project:inbox common';
    workflow.select([uploadFile(0), uploadFile(1), uploadFile(2)]);
    workflow.setItemTags(1, ['common', 'special']);
    workflow.setItemTags(2, ['common', 'special']);

    const calls: Array<{
      names: string[];
      tags: string[];
      itemTags?: string[][];
      operationID?: string;
      segmentIndex?: number;
      segmentCount?: number;
    }> = [];

    await workflow.submit(async (variables) => {
      calls.push({
        names: variables.files.map((file) => file.name),
        tags: [...variables.tags],
        itemTags: variables.itemTags?.map((itemTags) => [...itemTags]),
        operationID: variables.operationID,
        segmentIndex: variables.segmentIndex,
        segmentCount: variables.segmentCount
      });
      return pendingJob('job-item-tags', 1);
    });

    expect(calls).toEqual([
      {
        names: ['file-0.jpg', 'file-1.jpg', 'file-2.jpg'],
        tags: ['project:inbox', 'common'],
        itemTags: [
          ['project:inbox', 'common'],
          ['common', 'special'],
          ['common', 'special']
        ],
        operationID: undefined,
        segmentIndex: undefined,
        segmentCount: 1
      }
    ]);
    expect(workflow.activeJobIDs).toEqual(['job-item-tags']);
    expect(workflow.items.map((item) => item.status)).toEqual(['queued', 'queued', 'queued']);
    expect(workflow.items.map((item) => item.tagSyncBaseTags)).toEqual([
      ['project:inbox', 'common'],
      ['common', 'special'],
      ['common', 'special']
    ]);
    expect(workflow.items.map((_, index) => workflow.itemTagSyncDelta(index))).toEqual([
      { add: [], remove: [] },
      { add: [], remove: [] },
      { add: [], remove: [] }
    ]);
    expect(workflow.items.map((item) => item.tagSyncPending)).toEqual([false, false, false]);
  });

  it('does not fragment a transport-sized batch when every item has a different tag set', async () => {
    const workflow = createUploadWorkflow();
    const files = Array.from({ length: 100 }, (_, index) => uploadFile(index));
    workflow.select(files);
    files.forEach((_, index) => workflow.setItemTags(index, [`item:${index}`]));
    let requests = 0;

    await workflow.submit(async (variables) => {
      requests += 1;
      expect(variables.files).toHaveLength(100);
      expect(variables.tags).toEqual([]);
      expect(variables.itemTags).toEqual(files.map((_, index) => [`item:${index}`]));
      expect(variables.segmentCount).toBe(1);
      return pendingJob('job-unique-tags', 1);
    });

    expect(requests).toBe(1);
    expect(workflow.items.every((item) => !item.tagSyncPending)).toBe(true);
  });

  it('captures submitted tags at synchronous admission and preserves later edits as a delta', async () => {
    const workflow = createUploadWorkflow();
    workflow.tags = 'submitted common';
    workflow.select([uploadFile(0)]);

    let release!: () => void;
    const response = new Promise<BackgroundOperation>((resolve) => {
      release = () => resolve(pendingJob('job-baseline', 1));
    });
    const submission = workflow.submit(() => response);

    expect(workflow.items[0]).toMatchObject({
      status: 'uploading',
      tags: ['submitted', 'common'],
      tagSyncBaseTags: ['submitted', 'common'],
      tagSyncPending: false
    });

    workflow.setItemTags(0, ['submitted', 'later']);
    expect(workflow.itemTagSyncDelta(0)).toEqual({ add: ['later'], remove: ['common'] });
    expect(workflow.items[0].tagSyncPending).toBe(true);

    release();
    await submission;
    expect(workflow.items[0].tagSyncBaseTags).toEqual(['submitted', 'common']);
    expect(workflow.itemTagSyncDelta(0)).toEqual({ add: ['later'], remove: ['common'] });
  });

  it('keeps one visible job whether a selection uses one request or several', async () => {
    const small = createUploadWorkflow();
    small.select(Array.from({ length: 10 }, (_, index) => uploadFile(index)));
    await small.submit(async (variables) => {
      expect(variables.segmentCount).toBe(1);
      expect(variables.segmentIndex).toBeUndefined();
      expect(variables.operationID).toBeUndefined();
      return pendingJob('small-job', 1);
    });

    const large = createUploadWorkflow();
    large.select(Array.from({ length: maxFilesPerMultipartUpload + 1 }, (_, index) => uploadFile(index)));
    let requests = 0;
    await large.submit(async () => {
      requests += 1;
      return pendingJob('large-job', 2);
    });

    expect(requests).toBe(2);
    expect(small.activeJobIDs).toEqual(['small-job']);
    expect(large.activeJobIDs).toEqual(['large-job']);
  });

  it('cancels the admitted logical job and stops submitting later segments', async () => {
    const workflow = createUploadWorkflow();
    workflow.select(Array.from({ length: maxFilesPerMultipartUpload * 2 + 1 }, (_, index) => uploadFile(index)));
    let callCount = 0;
    let resolveSecondStarted!: () => void;
    const secondStarted = new Promise<void>((resolve) => { resolveSecondStarted = resolve; });

    const submission = workflow.submit(async (variables) => {
      callCount += 1;
      if (callCount === 1) {
        expect(variables.operationID).toBeUndefined();
        expect(variables.segmentIndex).toBe(0);
        expect(variables.segmentCount).toBe(3);
        return pendingJob('job-first', 3);
      }
      expect(variables.operationID).toBe('job-first');
      expect(variables.segmentIndex).toBe(1);
      resolveSecondStarted();
      return new Promise<BackgroundOperation>((_resolve, reject) => {
        const rejectCanceled = () => reject(new ApiError(0, 'request_aborted', 'Upload was canceled'));
        if (variables.signal?.aborted) rejectCanceled();
        else variables.signal?.addEventListener('abort', rejectCanceled, { once: true });
      });
    });

    await secondStarted;
    const canceled: string[] = [];
    const cancellation = workflow.cancel(async (id) => {
      canceled.push(id);
      return { ...pendingJob(id, 3), status: 'canceled' } as Job;
    });

    await Promise.all([submission, cancellation]);

    expect(callCount).toBe(2);
    expect(canceled).toEqual(['job-first']);
    expect(workflow.activeJobIDs).toEqual([]);
    expect(workflow.busy).toBe(false);
    expect(workflow.items.every((item) => item.status === 'canceled')).toBe(true);
  });
});
