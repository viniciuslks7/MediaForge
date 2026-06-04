import { describe, expect, it, vi } from 'vitest';
import { Hub, type Client, type JobEvent } from './hub.js';

function fakeClient(id: string): Client & { received: string[] } {
  const received: string[] = [];
  return { id, received, send: (d: string) => received.push(d) };
}

function event(jobId: string, status = 'processing'): JobEvent {
  return { job_id: jobId, kind: 'image', status, progress: 50, ts: Date.now() };
}

describe('Hub', () => {
  it('delivers an event only to clients subscribed to that job', () => {
    const hub = new Hub();
    const a = fakeClient('a');
    const b = fakeClient('b');
    hub.subscribe(a, 'job-1');
    hub.subscribe(b, 'job-2');

    const delivered = hub.broadcast(event('job-1'));

    expect(delivered).toBe(1);
    expect(a.received).toHaveLength(1);
    expect(b.received).toHaveLength(0);
    expect(JSON.parse(a.received[0])).toMatchObject({ type: 'event', job_id: 'job-1' });
  });

  it('fans out to multiple subscribers of the same job', () => {
    const hub = new Hub();
    const a = fakeClient('a');
    const b = fakeClient('b');
    hub.subscribe(a, 'job-1');
    hub.subscribe(b, 'job-1');

    expect(hub.broadcast(event('job-1'))).toBe(2);
    expect(hub.jobCount).toBe(1);
    expect(hub.clientCount).toBe(2);
  });

  it('removing a client cleans up its subscriptions', () => {
    const hub = new Hub();
    const a = fakeClient('a');
    hub.subscribe(a, 'job-1');
    hub.subscribe(a, 'job-2');

    hub.remove(a);

    expect(hub.jobCount).toBe(0);
    expect(hub.clientCount).toBe(0);
    expect(hub.broadcast(event('job-1'))).toBe(0);
  });

  it('invokes the onDeliver hook with the delivery count', () => {
    const hub = new Hub();
    const spy = vi.fn();
    hub.onDeliver = spy;
    hub.subscribe(fakeClient('a'), 'job-1');

    hub.broadcast(event('job-1'));

    expect(spy).toHaveBeenCalledWith(1);
  });

  it('broadcast to an unknown job delivers to nobody', () => {
    const hub = new Hub();
    expect(hub.broadcast(event('ghost'))).toBe(0);
  });
});
