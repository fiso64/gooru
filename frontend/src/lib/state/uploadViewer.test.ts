import { describe, expect, it } from 'vitest';
import type { UploadItem } from './uploadItems';
import { uploadItemCanOpenViewer, uploadViewerNeighborIndex } from './uploadViewer';

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
  it('navigates across queue batches while skipping staged rows and wrapping around', () => {
    const items = [
      item('staged.jpg', 'staged', { previewFile: localImage }),
      item('batch-1.jpg', 'uploading', { batchID: 1, previewFile: localImage }),
      item('staged-b.jpg', 'staged', { previewFile: localImage }),
      item('batch-2.jpg', 'uploaded', { batchID: 2, remoteFileID: 'remote-2' }),
      item('batch-3.jpg', 'duplicate_existing', { batchID: 3, remoteFileID: 'remote-3' })
    ];

    expect(uploadViewerNeighborIndex(items, 1, 'queue', 1)).toBe(3);
    expect(uploadViewerNeighborIndex(items, 3, 'queue', 1)).toBe(4);
    expect(uploadViewerNeighborIndex(items, 4, 'queue', 1)).toBe(1);
    expect(uploadViewerNeighborIndex(items, 1, 'queue', -1)).toBe(4);
    expect(uploadViewerNeighborIndex(items, 0, 'staged', 1)).toBe(2);
  });

  it('skips unsupported local-only rows until the server provides a remote identity', () => {
    const items = [
      item('local.jpg', 'staged', { previewFile: localImage }),
      item('local.cbz', 'staged', { type: 'application/zip', previewFile: localArchive }),
      item('other.jpg', 'staged', { previewFile: localImage }),
      item('imported.cbz', 'uploaded', { type: 'application/zip', previewFile: localArchive, remoteFileID: 'remote-cbz' })
    ];

    expect(uploadItemCanOpenViewer(items[0]!)).toBe(true);
    expect(uploadItemCanOpenViewer(items[1]!)).toBe(false);
    expect(uploadItemCanOpenViewer(items[3]!)).toBe(true);
    expect(uploadViewerNeighborIndex(items, 0, 'staged', 1)).toBe(2);
    expect(uploadViewerNeighborIndex(items, 2, 'staged', 1)).toBe(0);
    expect(uploadViewerNeighborIndex(items, 3, 'queue', 1)).toBe(3);
  });

  it('rejects an active row outside the requested viewer scope', () => {
    const items = [
      item('staged.jpg', 'staged', { previewFile: localImage }),
      item('queued.jpg', 'uploaded', { remoteFileID: 'remote' })
    ];

    expect(uploadViewerNeighborIndex(items, 0, 'queue', 1)).toBeNull();
    expect(uploadViewerNeighborIndex(items, 1, 'staged', -1)).toBeNull();
  });
});
