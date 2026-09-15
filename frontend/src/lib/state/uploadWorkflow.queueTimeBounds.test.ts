import { describe, expect, it } from 'vitest';
import { uploadQueueTimeBounds } from './uploadWorkflow.svelte';

describe('upload queue-time bounds', () => {
  it('handles very large batches without spreading values into function arguments', () => {
    const queueTimes = Array.from({ length: 150_000 }, (_, index) => 1_000_000 + index);
    queueTimes[72_000] = 5;
    queueTimes[123_000] = 9_999_999;

    expect(uploadQueueTimeBounds(queueTimes)).toEqual({ first: 5, last: 9_999_999 });
  });

  it('returns zero bounds for an empty batch', () => {
    expect(uploadQueueTimeBounds([])).toEqual({ first: 0, last: 0 });
  });
});
