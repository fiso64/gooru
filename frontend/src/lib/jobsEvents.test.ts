import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  jobsEventsURL,
  jobsRefreshFallbackIntervalMs,
  jobsRefreshMinIntervalMs,
  subscribeJobsEvents
} from './jobsEvents';

class FakeEventSource {
  readonly url: string;
  closed = false;
  private operationsListener: (() => void) | undefined;

  constructor(url: string) {
    this.url = url;
  }

  addOperationListener(listener: () => void) {
    this.operationsListener = listener;
  }

  close() {
    this.closed = true;
  }

  emitOperation() {
    this.operationsListener?.();
  }
}

afterEach(() => vi.useRealTimers());

describe('subscribeJobsEvents', () => {
  it('uses one operations stream and coalesces refreshes to the 200 ms minimum interval', () => {
    vi.useFakeTimers();
    let clock = 0;
    let source: FakeEventSource | undefined;
    const refresh = vi.fn();
    const stop = subscribeJobsEvents(refresh, {
      createEventSource: (url) => (source = new FakeEventSource(url)),
      now: () => clock
    });

    expect(source?.url).toBe(jobsEventsURL);
    source?.emitOperation();
    expect(refresh).toHaveBeenCalledTimes(1);

    clock = 100;
    source?.emitOperation();
    source?.emitOperation();
    expect(refresh).toHaveBeenCalledTimes(1);

    clock = jobsRefreshMinIntervalMs;
    vi.advanceTimersByTime(100);
    expect(refresh).toHaveBeenCalledTimes(2);

    stop();
    expect(vi.getTimerCount()).toBe(0);
    expect(source?.closed).toBe(true);
  });

  it('refreshes periodically when the operations stream stays silent', () => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    let source: FakeEventSource | undefined;
    const refresh = vi.fn();
    const stop = subscribeJobsEvents(refresh, {
      createEventSource: (url) => (source = new FakeEventSource(url))
    });

    vi.advanceTimersByTime(jobsRefreshFallbackIntervalMs - 1);
    expect(refresh).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(refresh).toHaveBeenCalledTimes(1);
    expect(source?.closed).toBe(false);

    vi.advanceTimersByTime(jobsRefreshFallbackIntervalMs);
    expect(refresh).toHaveBeenCalledTimes(2);

    stop();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('cancels queued and fallback refreshes when the authenticated subscription closes', () => {
    vi.useFakeTimers();
    let clock = 0;
    let source: FakeEventSource | undefined;
    const refresh = vi.fn();
    const stop = subscribeJobsEvents(refresh, {
      createEventSource: (url) => (source = new FakeEventSource(url)),
      now: () => clock
    });

    source?.emitOperation();
    clock = 10;
    source?.emitOperation();
    stop();
    expect(vi.getTimerCount()).toBe(0);
    vi.runAllTimers();

    expect(refresh).toHaveBeenCalledTimes(1);
    expect(source?.closed).toBe(true);
  });
});
