export const jobsEventsURL = '/api/v1/operations/events';
export const jobsRefreshMinIntervalMs = 200;
export const jobsRefreshFallbackIntervalMs = 15_000;

type JobsEventSource = {
  addOperationListener: (listener: () => void) => void;
  close: () => void;
};

type JobsEventsOptions = {
  createEventSource?: (url: string) => JobsEventSource;
  now?: () => number;
  setTimer?: (callback: () => void, delay: number) => ReturnType<typeof setTimeout>;
  clearTimer?: (timer: ReturnType<typeof setTimeout>) => void;
};

function createBrowserEventSource(url: string): JobsEventSource {
  const source = new EventSource(url);
  return {
    addOperationListener: (listener) => source.addEventListener('operations', listener),
    close: () => source.close()
  };
}

// The SSE stream is a payload-free invalidation hint, not an event log. Every
// signal re-reads the shared jobs cache, while bursts are coalesced so request
// starts remain at least 200 ms apart. The server-side change bus is
// process-local, so a low-frequency fallback bounds staleness when another
// process commits durable operation state without producing an SSE signal.
export function subscribeJobsEvents(refresh: () => void | Promise<unknown>, options: JobsEventsOptions = {}) {
  const createEventSource = options.createEventSource ?? createBrowserEventSource;
  const now = options.now ?? (() => Date.now());
  const setTimer = options.setTimer ?? ((callback, delay) => setTimeout(callback, delay));
  const clearTimer = options.clearTimer ?? ((timer) => clearTimeout(timer));
  const source = createEventSource(jobsEventsURL);
  let lastRefreshAt = Number.NEGATIVE_INFINITY;
  let refreshTimer: ReturnType<typeof setTimeout> | undefined;
  let fallbackTimer: ReturnType<typeof setTimeout> | undefined;
  let closed = false;

  const refreshNow = () => {
    refreshTimer = undefined;
    if (closed) return;
    lastRefreshAt = now();
    void refresh();
  };

  const scheduleRefresh = () => {
    if (closed || refreshTimer !== undefined) return;
    const delay = Math.max(0, jobsRefreshMinIntervalMs - (now() - lastRefreshAt));
    if (delay === 0) {
      refreshNow();
      return;
    }
    refreshTimer = setTimer(refreshNow, delay);
  };

  const scheduleFallbackRefresh = () => {
    if (closed || fallbackTimer !== undefined) return;
    fallbackTimer = setTimer(() => {
      fallbackTimer = undefined;
      if (closed) return;
      scheduleRefresh();
      scheduleFallbackRefresh();
    }, jobsRefreshFallbackIntervalMs);
  };

  source.addOperationListener(scheduleRefresh);
  scheduleFallbackRefresh();
  return () => {
    closed = true;
    if (refreshTimer !== undefined) clearTimer(refreshTimer);
    if (fallbackTimer !== undefined) clearTimer(fallbackTimer);
    refreshTimer = undefined;
    fallbackTimer = undefined;
    source.close();
  };
}
