export type UploadTagReconciliationStep = () => Promise<boolean>;

export function createUploadTagReconciliationWave(onSettled: () => Promise<void> | void) {
  const runs = new Map<object, Promise<void>>();
  let generation = 0;
  let refreshedGeneration = 0;
  let refreshRun: Promise<void> | null = null;

  async function refreshWhenIdle() {
    if (refreshRun) return refreshRun;

    const run = (async () => {
      while (runs.size === 0 && refreshedGeneration < generation) {
        const targetGeneration = generation;
        try {
          await onSettled();
        } finally {
          refreshedGeneration = targetGeneration;
        }
      }
    })();
    refreshRun = run;
    try {
      await run;
    } finally {
      if (refreshRun === run) refreshRun = null;
      if (runs.size === 0 && refreshedGeneration < generation) void refreshWhenIdle();
    }
  }

  function run(key: object, step: UploadTagReconciliationStep) {
    const existing = runs.get(key);
    if (existing) return existing;

    generation += 1;
    const task = (async () => {
      try {
        while (await step()) {
          // Keep one row inside the same wave while edits arrive during an in-flight mutation.
        }
      } finally {
        runs.delete(key);
        if (runs.size === 0) await refreshWhenIdle();
      }
    })();
    runs.set(key, task);
    return task;
  }

  return {
    run,
    isRunning: (key: object) => runs.has(key)
  };
}
