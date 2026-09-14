import { describe, expect, it } from 'vitest';
import type { Job } from '$lib/api/types';
import { jobAffectedCount, jobFailedCount } from '$lib/jobs';

function job(overrides: Partial<Job>): Job {
  return {
    id: 'operation-1',
    type: 'files.delete',
    status: 'completed',
    submitted_at: '2026-09-13T00:00:00Z',
    ...overrides
  };
}

describe('jobAffectedCount', () => {
  it('prefers the server-owned semantic count', () => {
    expect(jobAffectedCount(job({ affected_count: 5, result: { affected_count: 15 } }))).toBe(5);
  });

  it('uses the completed structured result as a backward-compatible fallback', () => {
    expect(jobAffectedCount(job({ progress_total: 1, result: { affected_count: 17 } }))).toBe(17);
  });

  it('derives completed upload counts from imported file results for legacy payloads', () => {
    expect(
      jobAffectedCount(
        job({
          type: 'upload_import',
          result: {
            affected_count: 0,
            files: [
              { name: 'one.jpg', status: 'imported' },
              { name: 'two.jpg', status: 'duplicate_existing' },
              { name: 'three.jpg', status: 'imported' }
            ]
          }
        })
      )
    ).toBe(2);
  });

  it('keeps running upload progress authoritative over partial results', () => {
    expect(
      jobAffectedCount(
        job({
          type: 'upload_import',
          status: 'running',
          stage: 'importing',
          progress_total: 42,
          result: { affected_count: 0, files: [] }
        })
      )
    ).toBe(42);
  });

  it('does not present generic durable task cardinality as files', () => {
    expect(jobAffectedCount(job({ progress_total: 1 }))).toBeUndefined();
  });

  it('retains server-projected upload file progress while importing', () => {
    expect(jobAffectedCount(job({ type: 'upload_import', status: 'running', stage: 'importing', progress_total: 42 }))).toBe(42);
  });

  it('does not treat receiving upload task progress as affected files', () => {
    expect(jobAffectedCount(job({ type: 'upload_import', status: 'running', stage: 'receiving', progress_total: 1 }))).toBeUndefined();
  });
});

describe('jobFailedCount', () => {
  it('returns the explicit API failure count', () => {
    expect(jobFailedCount(job({ failed_count: 3 }))).toBe(3);
  });

  it('does not infer semantic failures from durable task progress', () => {
    expect(jobFailedCount(job({ progress_failed: 3 }))).toBeUndefined();
  });
});
