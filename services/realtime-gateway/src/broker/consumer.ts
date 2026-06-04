import amqp, { type Channel, type ChannelModel } from 'amqplib';
import type { JobEvent } from '../ws/hub.js';

const EXCHANGE_EVENTS = 'media.events';

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
        try {
          const event = JSON.parse(msg.content.toString()) as JobEvent;
          this.onEvent(event);
        } catch (err) {
          console.error('failed to parse event', err);
        }
      },
      { noAck: true },
    );

    this.connection.on('close', () => console.warn('rabbitmq connection closed'));
  }

  async close(): Promise<void> {
    await this.channel?.close().catch(() => undefined);
    await this.connection?.close().catch(() => undefined);
  }
}
