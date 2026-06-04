import { loadConfig } from './config.js';
import { EventConsumer } from './broker/consumer.js';
import { buildServer } from './server.js';
import { Hub } from './ws/hub.js';
import { eventsReceived, trackedJobs } from './metrics/metrics.js';

async function main(): Promise<void> {
  const cfg = loadConfig();
  const hub = new Hub();

  const consumer = new EventConsumer(cfg.rabbitUrl, (event) => {
    eventsReceived.labels(event.status ?? 'unknown').inc();
    hub.broadcast(event);
    trackedJobs.set(hub.jobCount);
  });

  await consumer.start();
  console.log(JSON.stringify({ level: 'INFO', msg: 'connected to rabbitmq' }));

  const server = buildServer(hub, cfg.metricsPath);
  server.listen(cfg.httpPort, () => {
    console.log(JSON.stringify({ level: 'INFO', msg: `realtime gateway listening on :${cfg.httpPort}` }));
  });

  const shutdown = async (signal: string): Promise<void> => {
    console.log(JSON.stringify({ level: 'INFO', msg: `received ${signal}, shutting down` }));
    server.close();
    await consumer.close();
    process.exit(0);
  };
  process.on('SIGTERM', () => void shutdown('SIGTERM'));
  process.on('SIGINT', () => void shutdown('SIGINT'));
}

main().catch((err) => {
  console.error(JSON.stringify({ level: 'ERROR', msg: 'fatal', err: String(err) }));
  process.exit(1);
});
