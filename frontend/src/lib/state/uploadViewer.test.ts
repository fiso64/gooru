import { describe, expect, it } from 'vitest';
import type { UploadItem } from './uploadItems';
import {
  uploadItemCanOpenViewer,
  uploadViewerIndices,
  uploadViewerNavigableIndices,
  uploadViewerNeighborIndex
} from './uploadViewer';

function item(name: string, status: UploadItem['status'], overrides: Partial<UploadItem> = {}): UploadItem {
  return {
    name,
    size: 10,
    type: 'image/jpeg',
    status,
    progress: status === 'staged' ? 0 : 100,
    ...overrides
  };
}

const localImage = { type: 'image/jpeg' } as File;
const localArchive = { type: 'application/zip' } as File;

describe('upload viewer navigation', () => {
  it('keeps staged rows separate from one queue set spanning all batches', () => {
    const items = [
      item('staged-a.jpg', 'staged', { previewFile: localImage }),
      item('batch-1.jpg', 'importing', { batchID: 1, previewFile: localImage }),
      item('staged-b.jpg', 'staged', { previewFile: localImage }),
      item('batch-2.jpg', 'uploaded', { batchID: 2, remoteFileID: 'remote-2' }),
      item('batch-1-done.jpg', 'duplicate_existing', { batchID: 1, remoteFileID: 'remote-1' })
    ];

    expect(uploadViewerIndices(items, 'staged')).toEqual([0, 2]);
    expect(uploadViewerIndices(items, 'queue')).toEqual([1, 3, 4]);
  });

  it('navigates across queue batches and wraps around', () => {
    const items = [
      item('staged.jpg', 'staged', { previewFile: localImage }),
      item('batch-1.jpg', 'uploading', { batchID: 1, previewFile: localImage }),
      item('batch-2.jpg', 'uploaded', { batchID: 2, remoteFileID: 'remote-2' }),
      item('batch-3.jpg', 'duplicate_existing', { batchID: 3, remoteFileID: 'remote-3' })
    ];

    expect(uploadViewerNeighborIndex(items, 1, 'queue', 1)).toBe(2);
    expect(uploadViewerNeighborIndex(items, 2, 'queue', 1)).toBe(3);
    expect(uploadViewerNeighborIndex(items, 3, 'queue', 1)).toBe(1);
    expect(uploadViewerNeighborIndex(items, 1, 'queue', -1)).toBe(3);
  });

  it('skips unsupported local-only rows until the server provides a remote identity', () => {
    const items = [
      item('local.jpg', 'staged', { previewFile: localImage }),
      item('local.cbz', 'staged', { type: 'application/zip', previewFile: localArchive }),
      item('imported.cbz', 'uploaded', { type: 'application/zip', previewFile: localArchive, remoteFileID: 'remote-cbz' })
    ];

    expect(uploadItemCanOpenViewer(items[0]!)).toBe(true);
    expect(uploadItemCanOpenViewer(items[1]!)).toBe(false);
    expect(uploadItemCanOpenViewer(items[2]!)).toBe(true);
    expect(uploadViewerNavigableIndices(items, 'staged')).toEqual([0]);
    expect(uploadViewerNavigableIndices(items, 'queue')).toEqual([2]);
  });
});
