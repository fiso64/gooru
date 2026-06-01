import { createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';

export const jobKeys = {
  all: ['jobs'] as const,
  list: (scope: number) => ['jobs', 'list', scope] as const,
  detail: (scope: number, id: string) => ['job', scope, id] as const
};

export function createJobQuery(getCSRFToken: () => string, getJobID: () => string, getAuthScope: () => number) {
  return createQuery(() => {
    const jobID = getJobID();
    return {
      queryKey: jobKeys.detail(getAuthScope(), jobID),
      enabled: Boolean(jobID),
      queryFn: () => new ApiClient(getCSRFToken()).getJob(jobID),
      refetchInterval: 700
    };
  });
}

export function createJobsQuery(getAuthenticated: () => boolean, getAuthScope: () => number) {
  return createQuery(() => ({
    queryKey: jobKeys.list(getAuthScope()),
    enabled: getAuthenticated(),
    queryFn: () => new ApiClient().listJobs(),
    refetchInterval: 2000
  }));
}
