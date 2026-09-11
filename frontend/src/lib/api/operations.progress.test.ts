import { describe, expect, it } from 'vitest';
import { backgroundOperationAsJob, type BackgroundOperation } from './operations';

describe('backgroundOperationAsJob progress', () => {
  it('preserves aggregate counters and the upload row prefix', () => {
    const operation: BackgroundOperation = {
      id: 'upload-one',
      kind: 'upload_import',
      status: 'running',
      progress_total: 5,
      progress_completed: 3,
      progress_completed_prefix: 1,
      progress_failed: 1,
      progress: 0.71,
      created_at: '2026-09-11T00:00:00Z'
    };

    const job = backgroundOperationAsJob(operation);
    expect(job.progress).toBe(0.71);
    expect(job.progress_total).toBe(5);
    expect(job.progress_completed).toBe(3);
    expect(job.progress_completed_prefix).toBe(1);
    expect(job.progress_failed).toBe(1);
  });

  it('leaves receiving uploads indeterminate when the server has no overall fraction', () => {
    const operation: BackgroundOperation = {
      id: 'upload-receiving',
      kind: 'upload_import',
      status: 'running',
      stage: 'receiving',
      progress_total: 1,
      progress_completed: 0,
      progress_failed: 0,
      created_at: '2026-09-11T00:00:00Z'
    };
    const job = backgroundOperationAsJob(operation);
    expect(job.progress).toBeUndefined();
    expect(job.stage).toBe('receiving');
  });

  it('does not invent percentage progress for one-task operations', () => {
    const operation: BackgroundOperation = {
      id: 'remove-batch',
      kind: 'files_remove',
      status: 'running',
      progress_total: 1,
      progress_completed: 0,
      progress_failed: 0,
      created_at: '2026-09-11T00:00:00Z'
    };
    expect(backgroundOperationAsJob(operation).progress).toBeUndefined();
  });

  it('retains useful count-derived progress for multi-task operations', () => {
    const operation: BackgroundOperation = {
      id: 'multi-task',
      kind: 'thumbnail_backfill',
      status: 'running',
      progress_total: 4,
      progress_completed: 1,
      progress_failed: 1,
      created_at: '2026-09-11T00:00:00Z'
    };
    expect(backgroundOperationAsJob(operation).progress).toBe(0.5);
  });
});
