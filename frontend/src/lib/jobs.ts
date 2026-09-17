import type { Job } from '$lib/api/types';

export function jobAffectedCount(job: Job): number | undefined {
  if (typeof job.affected_count === 'number' && Number.isFinite(job.affected_count) && job.affected_count >= 0) {
    return Math.trunc(job.affected_count);
  }

  // Backward-compatible fallback for operation payloads produced before the
  // server-owned job summary was introduced.
  if (job.result && typeof job.result === 'object' && !Array.isArray(job.result)) {
    const result = job.result as Record<string, unknown>;
    if (job.type === 'upload_import' && job.status === 'completed' && Array.isArray(result.files)) {
      return result.files.reduce((count, file) => {
        if (file && typeof file === 'object' && !Array.isArray(file)) {
          return (file as Record<string, unknown>).status === 'imported' ? count + 1 : count;
        }
        return count;
      }, 0);
    }

    if (job.type !== 'upload_import' || job.status === 'completed') {
      const value = result.affected_count;
      if (typeof value === 'number' && Number.isFinite(value) && value >= 0) {
        return Math.trunc(value);
      }
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

export function jobFailedCount(job: Job): number | undefined {
  if (typeof job.failed_count !== 'number' || !Number.isFinite(job.failed_count) || job.failed_count < 0) {
    return undefined;
  }
  return Math.trunc(job.failed_count);
}


export function jobProgressPercent(job: Job): number | undefined {
  const rawProgress =
    typeof job.progress === 'number' && Number.isFinite(job.progress)
      ? Math.max(0, Math.min(1, job.progress))
      : undefined;

  if (job.type === 'upload_import') {
    if (job.status === 'completed' || job.stage === 'importing') return 100;
    if (job.stage === 'receiving' && rawProgress !== undefined) {
      // Upload operation progress reserves the first half for request-body
      // transport. Present that phase as its honest 0–100% byte progress
      // instead of exposing the server's cross-phase 0–50% weighting.
      return Math.round(Math.min(1, rawProgress * 2) * 100);
    }
  }

  return rawProgress === undefined ? undefined : Math.round(rawProgress * 100);
}
