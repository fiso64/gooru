import { createMutation, createQuery } from '@tanstack/svelte-query';
import { ApiClient, ApiError } from '$lib/api/client';
import type { ApiErrorResponse, Job } from '$lib/api/types';
import type { QueryClient } from '@tanstack/query-core';

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
  const params = new URLSearchParams({ limit: String(limit) });
  if (pageToken) params.set('page_token', pageToken);
  const response = await fetch(`/api/v1/jobs?${params.toString()}`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' }
  });
  const payload = await response.json().catch(() => undefined) as (JobListPage & Partial<ApiErrorResponse>) | undefined;
  if (!response.ok) {
    throw new ApiError(
      response.status,
      payload?.error?.code ?? 'http_error',
      payload?.error?.message ?? `Request failed with HTTP ${response.status}`
    );
  }
  return payload ?? { items: [], active_count: 0 };
}

export function createJobQuery(_getCSRFToken: () => string, getJobID: () => string, getAuthScope: () => number) {
  return createQuery(() => {
    const jobID = getJobID();
    const jobIDs = jobID.split(',').map((id) => id.trim()).filter(Boolean);
    return {
      queryKey: jobKeys.detail(getAuthScope(), jobID),
      enabled: jobIDs.length > 0,
      queryFn: () => fetchJobBatch(jobIDs),
      refetchInterval: 700
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
