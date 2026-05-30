import { createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';

export function createJobQuery(getToken: () => string, getJobID: () => string, getAuthScope: () => number) {
  return createQuery(() => ({
    queryKey: ['job', getAuthScope(), getJobID()],
    enabled: Boolean(getToken() && getJobID()),
    queryFn: () => new ApiClient(getToken()).getJob(getJobID()),
    refetchInterval: 700
  }));
}
