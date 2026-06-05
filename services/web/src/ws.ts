import type { JobEvent } from './types';

type Handler = (ev: JobEvent) => void;
type StateHandler = (connected: boolean) => void;

/**
 * RealtimeClient holds a single WebSocket to the realtime-gateway and lets the
 * UI subscribe to job ids. It auto-reconnects and re-subscribes, and keeps a
 * heartbeat so idle proxies don't drop the socket.
 */
export class RealtimeClient {
  private socket: WebSocket | null = null;
  private readonly subscriptions = new Set<string>();
  private readonly handlers = new Set<Handler>();
  private readonly stateHandlers = new Set<StateHandler>();
  private heartbeat: ReturnType<typeof setInterval> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private closedByUser = false;

  constructor(private readonly url = wsUrl()) {}

  connect(): void {
    this.closedByUser = false;
    const socket = new WebSocket(this.url);
    this.socket = socket;

    socket.onopen = () => {
      this.emitState(true);
      this.subscriptions.forEach((id) => this.send({ type: 'subscribe', job_id: id }));
      this.heartbeat = setInterval(() => this.send({ type: 'ping' }), 25_000);
    };

    socket.onmessage = (raw) => {
      let msg: unknown;
      try {
        msg = JSON.parse(raw.data as string);
      } catch {
        return;
      }
      if (isJobEvent(msg)) this.handlers.forEach((h) => h(msg));
    };

    socket.onclose = () => {
      this.cleanup();
      this.emitState(false);
      if (!this.closedByUser) {
        this.reconnectTimer = setTimeout(() => this.connect(), 1500);
      }
    };

    socket.onerror = () => socket.close();
  }

  subscribe(jobId: string): void {
    this.subscriptions.add(jobId);
    this.send({ type: 'subscribe', job_id: jobId });
  }

  onEvent(h: Handler): () => void {
    this.handlers.add(h);
    return () => this.handlers.delete(h);
  }

  onState(h: StateHandler): () => void {
    this.stateHandlers.add(h);
    return () => this.stateHandlers.delete(h);
  }

  close(): void {
    this.closedByUser = true;
    this.cleanup();
    this.socket?.close();
  }

  private send(payload: Record<string, unknown>): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(payload));
    }
  }

  private cleanup(): void {
    if (this.heartbeat) clearInterval(this.heartbeat);
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.heartbeat = null;
    this.reconnectTimer = null;
  }

  private emitState(connected: boolean): void {
    this.stateHandlers.forEach((h) => h(connected));
  }
}

function wsUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  return `${proto}://${location.host}/ws`;
}

function isJobEvent(v: unknown): v is JobEvent {
  return (
    typeof v === 'object' &&
    v !== null &&
    'job_id' in v &&
    'status' in v &&
    'progress' in v
  );
}
