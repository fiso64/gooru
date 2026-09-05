export type ViewerBenchmarkSnapshot = {
  presentations: number;
  pending: number;
  presentationsPerSecond: number;
  requestToPresentMs: {
    min: number;
    p50: number;
    p95: number;
    max: number;
  } | null;
};

type ViewerBenchmarkControl = {
  enable: () => void;
  disable: () => void;
  reset: () => void;
  snapshot: () => ViewerBenchmarkSnapshot;
};

let enabled = false;
let pending = new Map<number, number>();
let latencies: number[] = [];
let presentationTimes: number[] = [];

function now() {
  return typeof performance === 'undefined' ? Date.now() : performance.now();
}

function percentile(sorted: number[], ratio: number) {
  if (!sorted.length) return 0;
  return sorted[Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * ratio) - 1))];
}

function reset() {
  pending.clear();
  latencies = [];
  presentationTimes = [];
}

export function viewerBenchmarkSnapshot(): ViewerBenchmarkSnapshot {
  const sorted = [...latencies].sort((a, b) => a - b);
  const first = presentationTimes[0];
  const last = presentationTimes[presentationTimes.length - 1];
  const elapsedSeconds = presentationTimes.length > 1 && last > first ? (last - first) / 1000 : 0;
  return {
    presentations: presentationTimes.length,
    pending: pending.size,
    presentationsPerSecond: elapsedSeconds > 0 ? (presentationTimes.length - 1) / elapsedSeconds : 0,
    requestToPresentMs: sorted.length ? {
      min: sorted[0],
      p50: percentile(sorted, 0.5),
      p95: percentile(sorted, 0.95),
      max: sorted[sorted.length - 1]
    } : null
  };
}

export function recordViewerRequest(generation: number) {
  if (!enabled) return;
  // Navigation is latest-wins: superseded requests are intentionally dropped from the latency
  // sample rather than being allowed to look like an ever-growing decode backlog.
  pending.clear();
  pending.set(generation, now());
}

export function recordViewerPresentation(generation: number) {
  if (!enabled) return;
  const requestedAt = pending.get(generation);
  if (requestedAt === undefined) return;
  const presentedAt = now();
  pending.delete(generation);
  latencies.push(Math.max(0, presentedAt - requestedAt));
  presentationTimes.push(presentedAt);
}

if (typeof window !== 'undefined') {
  const benchmarkWindow = window as Window & { __gooruViewerBenchmark?: ViewerBenchmarkControl };
  benchmarkWindow.__gooruViewerBenchmark = {
    enable: () => { reset(); enabled = true; },
    disable: () => { enabled = false; reset(); },
    reset,
    snapshot: viewerBenchmarkSnapshot
  };
}
