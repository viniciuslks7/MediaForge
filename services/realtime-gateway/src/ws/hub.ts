/**
 * Hub maintains the mapping between job IDs and the clients subscribed to them,
 * and fans job events out to the right subscribers.
 *
 * It is deliberately decoupled from the WebSocket library: a Client is anything
 * with an id and a `send` function, which makes the fan-out logic trivial to
 * unit-test without opening real sockets.
 */
export interface JobEvent {
  job_id: string;
  kind: string;
  status: string;
  message?: string;
  progress: number;
  artifact?: string;
  ts: number;
}

export interface Client {
  readonly id: string;
  send(data: string): void;
}

export class Hub {
  /** jobId -> set of subscribed clients */
  private readonly byJob = new Map<string, Set<Client>>();
  /** client -> set of jobIds it is subscribed to (for O(1) cleanup) */
  private readonly byClient = new Map<Client, Set<string>>();

  /** optional hook invoked with the number of clients an event was delivered to */
  onDeliver?: (delivered: number) => void;

  subscribe(client: Client, jobId: string): void {
    let clients = this.byJob.get(jobId);
    if (!clients) {
      clients = new Set();
      this.byJob.set(jobId, clients);
    }
    clients.add(client);

    let jobs = this.byClient.get(client);
    if (!jobs) {
      jobs = new Set();
      this.byClient.set(client, jobs);
    }
    jobs.add(jobId);
  }

  unsubscribe(client: Client, jobId: string): void {
    this.byJob.get(jobId)?.delete(client);
    if (this.byJob.get(jobId)?.size === 0) this.byJob.delete(jobId);
    this.byClient.get(client)?.delete(jobId);
  }

  /** Drop a client entirely (call on disconnect). */
  remove(client: Client): void {
    const jobs = this.byClient.get(client);
    if (!jobs) return;
    for (const jobId of jobs) {
      const set = this.byJob.get(jobId);
      set?.delete(client);
      if (set && set.size === 0) this.byJob.delete(jobId);
    }
    this.byClient.delete(client);
  }

  /** Fan an event out to every client subscribed to its job. Returns the count. */
  broadcast(event: JobEvent): number {
    const clients = this.byJob.get(event.job_id);
    if (!clients || clients.size === 0) return 0;
    const payload = JSON.stringify({ type: 'event', ...event });
    let delivered = 0;
    for (const client of clients) {
      try {
        client.send(payload);
        delivered++;
      } catch {
        // a failing client will be cleaned up on its 'close' event
      }
    }
    this.onDeliver?.(delivered);
    return delivered;
  }

  get jobCount(): number {
    return this.byJob.size;
  }

  get clientCount(): number {
    return this.byClient.size;
  }
}
