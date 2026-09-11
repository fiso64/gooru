import { describe, expect, it } from 'vitest';
import { jobsPageRefetchInterval, jobsRefetchInterval } from './jobs';
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
