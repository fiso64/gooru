import { describe, expect, it } from 'vitest';
import { createUploadTagReconciliationWave } from './uploadTagReconciliation';

function deferred() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => { release = resolve; });
  return { promise, release };
}

describe('upload tag reconciliation wave', () => {
  it('coalesces chained and concurrent row work into one settled refresh', async () => {
    const gate = deferred();
    let firstSteps = 0;
    let secondSteps = 0;
    let refreshes = 0;
    const wave = createUploadTagReconciliationWave(async () => { refreshes += 1; });
    const first = {};
    const second = {};

    const firstRun = wave.run(first, async () => {
      firstSteps += 1;
      if (firstSteps === 1) {
        await gate.promise;
        return true;
      }
      return false;
    });
    const duplicateRun = wave.run(first, async () => {
      throw new Error('duplicate row run should not start');
    });
    const secondRun = wave.run(second, async () => {
      secondSteps += 1;
      await gate.promise;
      return false;
    });

    expect(duplicateRun).toBe(firstRun);
    expect(wave.isRunning(first)).toBe(true);
    gate.release();
    await Promise.all([firstRun, secondRun]);

    expect(firstSteps).toBe(2);
    expect(secondSteps).toBe(1);
    expect(refreshes).toBe(1);
    expect(wave.isRunning(first)).toBe(false);
    expect(wave.isRunning(second)).toBe(false);
  });

  it('runs a follow-up refresh when another wave settles during an in-flight refresh', async () => {
    const refreshGate = deferred();
    let refreshes = 0;
    const wave = createUploadTagReconciliationWave(async () => {
      refreshes += 1;
      if (refreshes === 1) await refreshGate.promise;
    });

    const firstRun = wave.run({}, async () => false);
    await Promise.resolve();
    await Promise.resolve();
    expect(refreshes).toBe(1);

    const secondRun = wave.run({}, async () => false);
    await Promise.resolve();
    refreshGate.release();
    await Promise.all([firstRun, secondRun]);

    expect(refreshes).toBe(2);
  });
});
