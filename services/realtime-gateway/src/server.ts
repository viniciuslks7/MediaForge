import { createServer, type Server } from 'node:http';
import { randomUUID } from 'node:crypto';
import { WebSocketServer, type WebSocket } from 'ws';
import { Hub, type Client } from './ws/hub.js';
import {
  connectedClients,
  eventsDelivered,
  registry,
  trackedJobs,
} from './metrics/metrics.js';

interface ClientMessage {
  type: 'subscribe' | 'unsubscribe' | 'ping';
  job_id?: string;
}

/** WSClient adapts a ws socket to the Hub's Client interface. */
class WSClient implements Client {
  readonly id = randomUUID();
  constructor(private readonly socket: WebSocket) {}
  send(data: string): void {
    this.socket.send(data);
  }
}

/**
 * buildServer creates the HTTP server (health + metrics) and attaches a
 * WebSocket server at /ws driven by the supplied Hub.
 */
export function buildServer(hub: Hub, metricsPath: string): Server {
  // Live, per-replica counts surfaced as JSON for the control room's pulse
  // widget (Prometheus stays the source of truth for dashboards/alerts).
  let connected = 0;

  const http = createServer((req, res) => {
    if (req.url === '/healthz' || req.url === '/readyz') {
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end('{"status":"ok"}');
      return;
    }
    if (req.url === '/stats') {
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ connected_clients: connected, tracked_jobs: hub.jobCount }));
      return;
    }
    if (req.url === metricsPath) {
      registry
        .metrics()
        .then((body) => {
          res.writeHead(200, { 'content-type': registry.contentType });
          res.end(body);
        })
        .catch(() => {
          res.writeHead(500);
          res.end();
        });
      return;
    }
    res.writeHead(404);
    res.end();
  });

  const wss = new WebSocketServer({ server: http, path: '/ws' });

  wss.on('connection', (socket) => {
    const client = new WSClient(socket);
    connected++;
    connectedClients.inc();

    socket.send(JSON.stringify({ type: 'welcome', client_id: client.id }));

    socket.on('message', (raw) => {
      let msg: ClientMessage;
      try {
        msg = JSON.parse(raw.toString()) as ClientMessage;
      } catch {
        socket.send(JSON.stringify({ type: 'error', message: 'invalid JSON' }));
        return;
      }
      switch (msg.type) {
        case 'subscribe':
          if (msg.job_id) {
            hub.subscribe(client, msg.job_id);
            socket.send(JSON.stringify({ type: 'subscribed', job_id: msg.job_id }));
          }
          break;
        case 'unsubscribe':
          if (msg.job_id) hub.unsubscribe(client, msg.job_id);
          break;
        case 'ping':
          socket.send(JSON.stringify({ type: 'pong' }));
          break;
      }
      trackedJobs.set(hub.jobCount);
    });

    socket.on('close', () => {
      hub.remove(client);
      connected--;
      connectedClients.dec();
      trackedJobs.set(hub.jobCount);
    });
  });

  // Surface delivery counts as events flow through the hub.
  hub.onDeliver = (n: number) => eventsDelivered.inc(n);

  return http;
}
