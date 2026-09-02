import { createMutation, createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import type { Job } from '$lib/api/types';
import type { QueryClient } from '@tanstack/query-core';

export const jobKeys = {
  all: ['jobs'] as const,
  list: (scope: number) => ['jobs', 'list', scope] as const,
  detail: (scope: number, id: string) => ['job', scope, id] as const
};

function jobIsActive(job: Job) {
  return job.status === 'pending' || job.status === 'running';
}

export function jobsRefetchInterval(jobs: Job[] | undefined) {
  return jobs?.some(jobIsActive) ? 2000 : false;
}

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
    refetchInterval: (query) => jobsRefetchInterval(query.state.data?.items)
  }));
}

export function createCancelJobMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<Job, Error, string>(() => ({
    mutationFn: (id) => new ApiClient(getCSRFToken()).cancelJob(id),
    onSuccess: async (job) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: jobKeys.all }),
        queryClient.invalidateQueries({ queryKey: ['job'] }),
        queryClient.invalidateQueries({ queryKey: ['files'] }),
        queryClient.invalidateQueries({ queryKey: ['library', 'tags'] })
      ]);
      return job;
    }
  }));
}

export function createClearJobsMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<{ removed: number }, Error, string>(() => ({
    mutationFn: (status) => new ApiClient(getCSRFToken()).clearJobs(status),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: jobKeys.all })
  }));
}
