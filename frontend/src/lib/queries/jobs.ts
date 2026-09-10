import { createMutation, createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import type { Job } from '$lib/api/types';
import type { QueryClient } from '@tanstack/query-core';
import {
  backgroundOperationAsJob,
  cancelBackgroundOperation,
  listBackgroundOperations,
  listBackgroundOperationsByIDs
} from '$lib/api/operations';
import {
  uploadBackpressuredJobStatusRefetchMs,
  uploadJobStatusBatchSize,
  uploadJobStatusRefetchMs
} from '$lib/uploadBackpressure';

export interface JobListPage {
  items: Job[];
  active_count: number;
  next_page_token?: string;
}

export const jobKeys = {
  all: ['jobs'] as const,
  list: (scope: number, limit: number, pageToken: string) => ['jobs', 'list', scope, limit, pageToken] as const,
  detail: (scope: number, id: string) => ['job', scope, id] as const
};

function jobIsActive(job: Job) {
  return job.status === 'pending' || job.status === 'running';
}

export function jobsRefetchInterval(jobs: Job[] | undefined) {
  return jobs?.some(jobIsActive) ? 2000 : false;
}

export function jobsPageRefetchInterval(page: JobListPage | undefined) {
  if (typeof page?.active_count === 'number') return page.active_count > 0 ? 2000 : false;
  return jobsRefetchInterval(page?.items);
}

export function uploadJobRefetchInterval(jobIDs: string[]) {
  return jobIDs.length >= uploadJobStatusBatchSize
    ? uploadBackpressuredJobStatusRefetchMs
    : uploadJobStatusRefetchMs;
}

async function fetchJobBatch(ids: string[]) {
  const response = await listBackgroundOperationsByIDs(ids);
  return { items: response.items.map(backgroundOperationAsJob) };
}

async function fetchJobsPage(limit: number, pageToken: string): Promise<JobListPage> {
  const response = await listBackgroundOperations(1000);
  const jobs = response.items.map(backgroundOperationAsJob);
  const offset = Number.parseInt(pageToken, 10);
  const start = Number.isFinite(offset) && offset > 0 ? offset : 0;
  const end = start + limit;
  return {
    items: jobs.slice(start, end),
    active_count: jobs.filter(jobIsActive).length,
    next_page_token: end < jobs.length ? String(end) : undefined
  };
}

export function createJobQuery(_getCSRFToken: () => string, getJobID: () => string, getAuthScope: () => number) {
  return createQuery(() => {
    const jobID = getJobID();
    const jobIDs = jobID.split(',').map((id) => id.trim()).filter(Boolean);
    return {
      queryKey: jobKeys.detail(getAuthScope(), jobID),
      enabled: jobIDs.length > 0,
      queryFn: () => fetchJobBatch(jobIDs),
      refetchInterval: uploadJobRefetchInterval(jobIDs)
    };
  });
}

export function createJobsQuery(
  getAuthenticated: () => boolean,
  getAuthScope: () => number,
  getLimit: () => number = () => 20,
  getPageToken: () => string = () => '',
  getEnabled: () => boolean = getAuthenticated
) {
  return createQuery(() => {
    const limit = getLimit();
    const pageToken = getPageToken();
    return {
      queryKey: jobKeys.list(getAuthScope(), limit, pageToken),
      enabled: getAuthenticated() && getEnabled(),
      queryFn: () => fetchJobsPage(limit, pageToken),
      refetchInterval: (query) => jobsPageRefetchInterval(query.state.data)
    };
  });
}

export function createCancelJobMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<Job, Error, string>(() => ({
    mutationFn: async (id) => backgroundOperationAsJob(await cancelBackgroundOperation(id, getCSRFToken())),
    onSuccess: async (job) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: jobKeys.all }),
        queryClient.invalidateQueries({ queryKey: ['files'] }),
        queryClient.invalidateQueries({ queryKey: ['library', 'tags'] })
      ]);
      return job;
    }
  }));
}

// Tag mutation is the final legacy JobManager producer. Keep finished-job
// clearing only until tag mutation moves to durable operations in the next slice.
export function createClearJobsMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<{ removed: number }, Error, string>(() => ({
    mutationFn: (status) => new ApiClient(getCSRFToken()).clearJobs(status),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: jobKeys.all })
  }));
}
