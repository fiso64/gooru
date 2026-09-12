import { createMutation, createQuery } from '@tanstack/svelte-query';
import type { Job } from '$lib/api/types';
import type { QueryClient } from '@tanstack/query-core';
import {
  backgroundOperationAsJob,
  cancelActiveBackgroundOperations,
  cancelBackgroundOperation,
  clearCompletedBackgroundOperations,
  listBackgroundOperations,
  listBackgroundOperationsByIDs,
  type BackgroundOperationCancelAllResponse,
  type BackgroundOperationClearResponse
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

export function jobsPageRefetchInterval(_page: JobListPage | undefined) {
  // The visible operation list is also the discovery channel for work admitted
  // elsewhere in the UI. Stopping the list poll when active_count reaches zero
  // makes a later delete/upload operation invisible until another invalidation or
  // full page refresh. Keep the existing active cadence while idle so new durable
  // operations appear without relying on producer-specific cache coordination.
  return 2000;
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

function jobsPageOffset(pageToken: string) {
  const offset = Number.parseInt(pageToken, 10);
  return Number.isFinite(offset) && offset > 0 ? offset : 0;
}

export function jobsPageRequestLimit(limit: number, pageToken: string) {
  const start = jobsPageOffset(pageToken);
  // The operations endpoint is newest-first but currently exposes only a bounded
  // prefix, not a cursor. Fetch just enough prefix rows to cover this page plus
  // one lookahead row so local pagination can preserve the existing next-page
  // behavior without materializing and polling the full 1000-operation history.
  return Math.min(1000, start + limit + 1);
}

export function jobsPageActiveCount(serverActiveCount: number | undefined, jobs: Job[]) {
  return serverActiveCount ?? jobs.filter(jobIsActive).length;
}

async function fetchJobsPage(limit: number, pageToken: string): Promise<JobListPage> {
  const start = jobsPageOffset(pageToken);
  const response = await listBackgroundOperations(jobsPageRequestLimit(limit, pageToken));
  const jobs = response.items.map(backgroundOperationAsJob);
  const end = start + limit;
  return {
    items: jobs.slice(start, end),
    active_count: jobsPageActiveCount(response.active_count, jobs),
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

export function createClearCompletedJobsMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<BackgroundOperationClearResponse, Error, void>(() => ({
    mutationFn: () => clearCompletedBackgroundOperations(getCSRFToken()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: jobKeys.all });
    }
  }));
}

export function createCancelActiveJobsMutation(getCSRFToken: () => string, queryClient: QueryClient) {
  return createMutation<BackgroundOperationCancelAllResponse, Error, void>(() => ({
    mutationFn: () => cancelActiveBackgroundOperations(getCSRFToken()),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: jobKeys.all }),
        queryClient.invalidateQueries({ queryKey: ['files'] }),
        queryClient.invalidateQueries({ queryKey: ['library', 'tags'] })
      ]);
    }
  }));
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
