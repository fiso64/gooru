export const jobsEventsURL = '/api/v1/operations/events';
export const jobsRefreshMinIntervalMs = 200;

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
// starts remain at least 200 ms apart.
export function subscribeJobsEvents(refresh: () => void | Promise<unknown>, options: JobsEventsOptions = {}) {
  const createEventSource = options.createEventSource ?? createBrowserEventSource;
  const now = options.now ?? (() => Date.now());
  const setTimer = options.setTimer ?? ((callback, delay) => setTimeout(callback, delay));
  const clearTimer = options.clearTimer ?? ((timer) => clearTimeout(timer));
  const source = createEventSource(jobsEventsURL);
  let lastRefreshAt = Number.NEGATIVE_INFINITY;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let closed = false;

  const refreshNow = () => {
    timer = undefined;
    if (closed) return;
    lastRefreshAt = now();
    void refresh();
  };

  const scheduleRefresh = () => {
    if (closed || timer !== undefined) return;
    const delay = Math.max(0, jobsRefreshMinIntervalMs - (now() - lastRefreshAt));
    if (delay === 0) {
      refreshNow();
      return;
    }
    timer = setTimer(refreshNow, delay);
  };

  source.addOperationListener(scheduleRefresh);
  return () => {
    closed = true;
    if (timer !== undefined) clearTimer(timer);
    timer = undefined;
    source.close();
  };
}