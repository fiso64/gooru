import { describe, expect, it } from 'vitest';
import { jobsRefetchInterval } from './jobs';
import type { Job } from '$lib/api/types';

function job(status: Job['status']): Job {
  return { id: status, type: 'test', status, progress: 0 } as Job;
}

describe('jobsRefetchInterval', () => {
  it('stops polling when there is no active work', () => {
    expect(jobsRefetchInterval(undefined)).toBe(false);
    expect(jobsRefetchInterval([])).toBe(false);
    expect(jobsRefetchInterval([job('completed'), job('failed'), job('canceled')])).toBe(false);
  });

  it('keeps polling while pending or running jobs need progress updates', () => {
    expect(jobsRefetchInterval([job('pending')])).toBe(2000);
    expect(jobsRefetchInterval([job('running')])).toBe(2000);
  });
});
