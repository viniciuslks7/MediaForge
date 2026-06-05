import amqp, { type Channel, type ChannelModel } from 'amqplib';
import { context, propagation, trace, SpanKind } from '@opentelemetry/api';
import type { JobEvent } from '../ws/hub.js';
import { log } from '../log.js';

const EXCHANGE_EVENTS = 'media.events';
const tracer = trace.getTracer('mediaforge.realtime.consumer');

export type EventHandler = (event: JobEvent) => void;

/**
 * EventConsumer binds an exclusive, auto-delete queue to the media.events topic
 * exchange and invokes the handler for every job event. Because the queue is
 * per-instance, every realtime-gateway replica receives every event and can
 * serve whichever clients happen to be connected to it.
 */
export class EventConsumer {
  private connection?: ChannelModel;
  private channel?: Channel;

  constructor(
    private readonly url: string,
    private readonly onEvent: EventHandler,
  ) {}

  async start(): Promise<void> {
    this.connection = await amqp.connect(this.url);
    this.channel = await this.connection.createChannel();

    await this.channel.assertExchange(EXCHANGE_EVENTS, 'topic', { durable: true });
    const { queue } = await this.channel.assertQueue('', { exclusive: true, autoDelete: true });
    await this.channel.bindQueue(queue, EXCHANGE_EVENTS, 'event.#');

    await this.channel.consume(
      queue,
      (msg) => {
        if (!msg) return;
        // Continue the distributed trace: extract the W3C context the worker
        // injected into the event headers, then open a consumer span for the
        // WebSocket fan-out — the terminal hop of the trace.
        const headers = (msg.properties.headers ?? {}) as Record<string, unknown>;
        const parentCtx = propagation.extract(context.active(), headers);
        const span = tracer.startSpan(
          'consume media.events',
          {
            kind: SpanKind.CONSUMER,
            attributes: {
              'messaging.system': 'rabbitmq',
              'messaging.rabbitmq.destination.routing_key': msg.fields.routingKey,
            },
          },
          parentCtx,
        );
        context.with(trace.setSpan(parentCtx, span), () => {
          try {
            const event = JSON.parse(msg.content.toString()) as JobEvent;
            span.setAttribute('mediaforge.job.id', event.job_id ?? '');
            span.setAttribute('mediaforge.event.status', event.status ?? '');
            this.onEvent(event);
          } catch (err) {
            span.recordException(err as Error);
            log.error('failed to parse event', { err: String(err) });
          } finally {
            span.end();
          }
        });
      },
      { noAck: true },
    );

    this.connection.on('close', () => log.warn('rabbitmq connection closed'));
  }

  async close(): Promise<void> {
    await this.channel?.close().catch(() => undefined);
    await this.connection?.close().catch(() => undefined);
  }
}
