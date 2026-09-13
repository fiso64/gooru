import { afterEach, describe, expect, it, vi } from 'vitest';
import { jobsEventsURL, jobsRefreshMinIntervalMs, subscribeJobsEvents } from './jobsEvents';

class FakeEventSource {
  readonly url: string;
  closed = false;
  private operationsListener: EventListener | undefined;

  constructor(url: string) {
    this.url = url;
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    if (type !== 'operations') return;
    this.operationsListener = typeof listener === 'function' ? listener : (event) => listener.handleEvent(event);
  }

  close() {
    this.closed = true;
  }

  emitOperation() {
    this.operationsListener?.(new Event('operations'));
  }
}

afterEach(() => vi.useRealTimers());

describe('subscribeJobsEvents', () => {
  it('uses one operations stream and coalesces refreshes to the 500 ms minimum interval', () => {
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
    expect(vi.getTimerCount()).toBe(1);

    clock = jobsRefreshMinIntervalMs;
    vi.advanceTimersByTime(400);
    expect(refresh).toHaveBeenCalledTimes(2);

    stop();
    expect(source?.closed).toBe(true);
  });

  it('cancels a queued refresh when the authenticated subscription closes', () => {
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
    vi.runAllTimers();

    expect(refresh).toHaveBeenCalledTimes(1);
    expect(source?.closed).toBe(true);
  });
});
