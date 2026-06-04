import { Counter, Gauge, Registry, collectDefaultMetrics } from 'prom-client';

export const registry = new Registry();
collectDefaultMetrics({ register: registry });

export const eventsReceived = new Counter({
  name: 'mediaforge_realtime_events_received_total',
  help: 'Job events consumed from RabbitMQ.',
  labelNames: ['status'] as const,
  registers: [registry],
});

export const eventsDelivered = new Counter({
  name: 'mediaforge_realtime_events_delivered_total',
  help: 'Event messages delivered to WebSocket clients.',
  registers: [registry],
});

export const connectedClients = new Gauge({
  name: 'mediaforge_realtime_connected_clients',
  help: 'Currently connected WebSocket clients.',
  registers: [registry],
});

export const trackedJobs = new Gauge({
  name: 'mediaforge_realtime_tracked_jobs',
  help: 'Distinct jobs with at least one subscriber.',
  registers: [registry],
});
