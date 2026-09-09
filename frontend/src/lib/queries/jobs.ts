import { createMutation, createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';
import type { Job } from '$lib/api/types';
import type { QueryClient } from '@tanstack/query-core';
import {
  backgroundOperationAsJob,
  cancelBackgroundOperation,
  listBackgroundOperations
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
  const params = new URLSearchParams();
  for (const id of ids) params.append('id', id);
  const response = await fetch(`/api/v1/jobs?${params.toString()}`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' }
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch upload job status (${response.status})`);
  }
  return await response.json() as { items: Job[] };
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

// Upload result polling still uses the legacy JobManager until uploads become
// durable-operation producers. Keep this mutation for callers outside the
// durable operation history UI while that migration is incomplete.
export function createClearJobsMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<{ removed: number }, Error, string>(() => ({
    mutationFn: (status) => new ApiClient(getCSRFToken()).clearJobs(status),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: jobKeys.all })
  }));
}
