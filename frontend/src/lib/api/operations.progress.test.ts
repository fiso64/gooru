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
      created_at: '2026-09-11T00:00:00Z'
    };

    const job = backgroundOperationAsJob(operation);
    expect(job.progress).toBe(0.8);
    expect(job.progress_total).toBe(5);
    expect(job.progress_completed).toBe(3);
    expect(job.progress_completed_prefix).toBe(1);
    expect(job.progress_failed).toBe(1);
  });
});
