import type { Job } from '$lib/api/types';

export function jobAffectedCount(job: Job): number | undefined {
  if (job.result && typeof job.result === 'object' && !Array.isArray(job.result)) {
    const value = (job.result as Record<string, unknown>).affected_count;
    if (typeof value === 'number' && Number.isFinite(value) && value >= 0) {
      return Math.trunc(value);
    }
  }

  // Upload progress is deliberately projected from transport/task state into file
  // cardinality by the server while an import is active. Generic durable operation
  // progress is task cardinality and must never be presented as affected files.
  if (
    job.type === 'upload_import' &&
    job.stage !== 'receiving' &&
    typeof job.progress_total === 'number' &&
    Number.isFinite(job.progress_total) &&
    job.progress_total >= 1
  ) {
    return Math.trunc(job.progress_total);
  }
  return undefined;
}
