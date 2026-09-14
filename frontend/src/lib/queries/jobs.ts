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

export interface JobListPage {
  items: Job[];
  active_count: number;
  total_count: number;
  next_page_token?: string;
}

export const jobKeys = {
  all: ['jobs'] as const,
  list: (scope: number, limit: number, pageToken: string) => ['jobs', 'list', scope, limit, pageToken] as const,
  detail: (scope: number, id: string) => ['jobs', 'detail', scope, id] as const
};

function jobIsActive(job: Job) {
  return job.status === 'pending' || job.status === 'running';
}

async function fetchJobBatch(ids: string[]) {
  const response = await listBackgroundOperationsByIDs(ids);
  return { items: response.items.map(backgroundOperationAsJob) };
}

export function jobsPageOffset(pageToken: string) {
  const offset = Number.parseInt(pageToken, 10);
  return Number.isFinite(offset) && offset > 0 ? offset : 0;
}

export function jobsPageActiveCount(serverActiveCount: number | undefined, jobs: Job[]) {
  return serverActiveCount ?? jobs.filter(jobIsActive).length;
}

export function jobsPageTotalCount(serverTotalCount: number | undefined, offset: number, jobs: Job[]) {
  return serverTotalCount ?? offset + jobs.length;
}

async function fetchJobsPage(limit: number, pageToken: string): Promise<JobListPage> {
  const start = jobsPageOffset(pageToken);
  const response = await listBackgroundOperations(limit, start);
  const jobs = response.items.map(backgroundOperationAsJob);
  const totalCount = jobsPageTotalCount(response.total_count, start, jobs);
  const nextOffset = start + jobs.length;
  return {
    items: jobs,
    active_count: jobsPageActiveCount(response.active_count, jobs),
    total_count: totalCount,
    next_page_token: nextOffset < totalCount ? String(nextOffset) : undefined
  };
}

export function createJobQuery(_getCSRFToken: () => string, getJobID: () => string, getAuthScope: () => number) {
  return createQuery(() => {
    const jobID = getJobID();
    const jobIDs = jobID.split(',').map((id) => id.trim()).filter(Boolean);
    return {
      queryKey: jobKeys.detail(getAuthScope(), jobID),
      enabled: jobIDs.length > 0,
      queryFn: () => fetchJobBatch(jobIDs)
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
      queryFn: () => fetchJobsPage(limit, pageToken)
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
