import { flushSync } from 'svelte';
import { describe, expect, it } from 'vitest';
import type { Job } from '$lib/api/types';
import { createUploadWorkflow } from './uploadWorkflow.svelte';

describe('createUploadWorkflow', () => {
  it('does not subscribe the caller effect to workflow state while applying a polled job', () => {
    const workflow = createUploadWorkflow();
    const job = {
      id: 'job-1',
      type: 'upload_import',
      status: 'running',
      submitted_at: '2026-09-01T00:00:00Z',
      progress: 0.5
    } as Job;

    let runs = 0;
    const dispose = $effect.root(() => {
      $effect(() => {
        runs += 1;
        workflow.applyJob(job);
      });
    });

    expect(() => flushSync()).not.toThrow();
    expect(runs).toBe(1);
    expect(workflow.status).toBe('Importing');

    dispose();
  });
});
