import { createQuery } from '@tanstack/svelte-query';
import { ApiClient } from '$lib/api/client';

export function createJobQuery(getCSRFToken: () => string, getJobID: () => string, getAuthScope: () => number) {
  return createQuery(() => ({
    queryKey: ['job', getAuthScope(), getJobID()],
    enabled: Boolean(getJobID()),
    queryFn: () => new ApiClient(getCSRFToken()).getJob(getJobID()),
    refetchInterval: 700
  }));
}
