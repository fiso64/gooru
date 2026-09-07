import { describe, expect, it } from 'vitest';
import { uploadJobRefetchInterval } from './jobs';
import {
  uploadBackpressuredJobStatusRefetchMs,
  uploadJobStatusBatchSize,
  uploadJobStatusRefetchMs
} from '$lib/uploadBackpressure';

describe('uploadJobRefetchInterval', () => {
  it('uses the normal progress cadence below the admission window', () => {
    expect(uploadJobRefetchInterval(['one'])).toBe(uploadJobStatusRefetchMs);
  });

  it('polls promptly only while the full admission window needs capacity', () => {
    const ids = Array.from({ length: uploadJobStatusBatchSize }, (_, index) => `job-${index}`);
    expect(uploadJobRefetchInterval(ids)).toBe(uploadBackpressuredJobStatusRefetchMs);
  });
});
