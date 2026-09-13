import { describe, expect, it } from 'vitest';
import { jobsPageActiveCount, jobsPageOffset, jobsPageRefetchInterval, jobsPageTotalCount, jobsRefetchInterval } from './jobs';
import type { Job } from '$lib/api/types';

function job(status: Job['status']): Job {
  return { id: status, type: 'test', status, progress: 0 } as Job;
}

describe('jobsRefetchInterval', () => {
  it('stops detail polling when there is no active work', () => {
    expect(jobsRefetchInterval(undefined)).toBe(false);
    expect(jobsRefetchInterval([])).toBe(false);
    expect(jobsRefetchInterval([job('completed'), job('failed'), job('canceled')])).toBe(false);
  });

  it('keeps detail polling while pending or running jobs need progress updates', () => {
    expect(jobsRefetchInterval([job('pending')])).toBe(2000);
    expect(jobsRefetchInterval([job('running')])).toBe(2000);
  });

  it('keeps the jobs list polling while idle so newly admitted operations are discovered', () => {
    expect(jobsPageRefetchInterval(undefined)).toBe(2000);
    expect(jobsPageRefetchInterval({ items: [], active_count: 0, total_count: 0 })).toBe(2000);
    expect(jobsPageRefetchInterval({ items: [job('completed')], active_count: 0, total_count: 1 })).toBe(2000);
    expect(jobsPageRefetchInterval({ items: [job('completed')], active_count: 1, total_count: 1 })).toBe(2000);
  });
});

describe('jobsPageOffset', () => {
  it('uses page tokens as direct server offsets', () => {
    expect(jobsPageOffset('')).toBe(0);
    expect(jobsPageOffset('50')).toBe(50);
    expect(jobsPageOffset('150')).toBe(150);
  });

  it('treats invalid page tokens as the first page', () => {
    expect(jobsPageOffset('not-a-number')).toBe(0);
    expect(jobsPageOffset('-20')).toBe(0);
  });
});

describe('jobsPageActiveCount', () => {
  it('uses the exact server aggregate even when the fetched page is terminal', () => {
    expect(jobsPageActiveCount(7, [job('completed')])).toBe(7);
  });

  it('falls back to the fetched jobs for older responses without the aggregate', () => {
    expect(jobsPageActiveCount(undefined, [job('pending'), job('running'), job('completed')])).toBe(2);
  });
});

describe('jobsPageTotalCount', () => {
  it('uses the exact server aggregate rather than the current page size', () => {
    expect(jobsPageTotalCount(137, 50, [job('completed')])).toBe(137);
  });

  it('falls back to the end of the fetched page for older responses', () => {
    expect(jobsPageTotalCount(undefined, 50, [job('completed'), job('failed')])).toBe(52);
  });
});
