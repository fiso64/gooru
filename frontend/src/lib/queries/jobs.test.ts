import { describe, expect, it } from 'vitest';
import { jobsPageActiveCount, jobsPageRefetchInterval, jobsPageRequestLimit, jobsRefetchInterval } from './jobs';
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
    expect(jobsPageRefetchInterval({ items: [], active_count: 0 })).toBe(2000);
    expect(jobsPageRefetchInterval({ items: [job('completed')], active_count: 0 })).toBe(2000);
    expect(jobsPageRefetchInterval({ items: [job('completed')], active_count: 1 })).toBe(2000);
  });
});

describe('jobsPageRequestLimit', () => {
  it('fetches only the visible prefix plus one lookahead row', () => {
    expect(jobsPageRequestLimit(20, '')).toBe(21);
    expect(jobsPageRequestLimit(50, '')).toBe(51);
    expect(jobsPageRequestLimit(50, '50')).toBe(101);
  });

  it('preserves the existing 1000-operation history bound', () => {
    expect(jobsPageRequestLimit(50, '950')).toBe(1000);
    expect(jobsPageRequestLimit(50, '1000')).toBe(1000);
  });

  it('treats invalid page tokens as the first page', () => {
    expect(jobsPageRequestLimit(20, 'not-a-number')).toBe(21);
    expect(jobsPageRequestLimit(20, '-20')).toBe(21);
  });
});

describe('jobsPageActiveCount', () => {
  it('uses the exact server aggregate even when the fetched prefix is terminal', () => {
    expect(jobsPageActiveCount(7, [job('completed')])).toBe(7);
  });

  it('falls back to the fetched jobs for older responses without the aggregate', () => {
    expect(jobsPageActiveCount(undefined, [job('pending'), job('running'), job('completed')])).toBe(2);
  });
});
