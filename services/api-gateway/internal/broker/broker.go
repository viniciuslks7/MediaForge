// Package broker owns the RabbitMQ topology and the publisher used by the
// gateway. The topology is declared idempotently on connect so any service can
// be the first to come up.
//
// Topology:
//
//	media.jobs        (topic)   ─ image.process ─▶ q.image  (quorum)
//	                            └ ocr.extract   ─▶ q.ocr    (quorum)
//	media.events      (topic)   ─ #             ─▶ (transient, bound by consumers)
//	media.jobs.dlx    (topic)   ─ #             ─▶ q.retry  (TTL) ──┐
//	                                                                 └─dead-letter back to media.jobs
//	media.jobs.parking(topic)   ─ #             ─▶ q.parking (terminal DLQ)
package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/viniciusoliveira/mediaforge/api-gateway/broker")

const (
	ExchangeJobs    = "media.jobs"
	ExchangeEvents  = "media.events"
	ExchangeRetry   = "media.jobs.dlx"
	ExchangeParking = "media.jobs.parking"

	QueueImage   = "q.image"
	QueueOCR     = "q.ocr"
	QueueRetry   = "q.retry"
	QueueParking = "q.parking"
)

// Broker holds a connection and a channel and exposes a confirm-mode publisher.
type Broker struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

// Connect dials RabbitMQ, opens a channel in confirm mode and declares the
// full topology.
func Connect(url string, retryTTL time.Duration) (*Broker, error) {
	conn, err := dialWithRetry(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("enable confirms: %w", err)
	}
	b := &Broker{conn: conn, ch: ch}
	if err := b.declareTopology(retryTTL); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return b, nil
}

// dialWithRetry dials RabbitMQ with bounded exponential backoff. RabbitMQ can
// report healthy (rabbitmq-diagnostics ping) a moment before its AMQP listener
// accepts connections, so a single dial races the broker on a cold boot. We
// retry for ~30s before giving up.
func dialWithRetry(url string) (*amqp.Connection, error) {
	const maxAttempts = 12
	backoff := 250 * time.Millisecond
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		conn, err := amqp.Dial(url)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if attempt == maxAttempts {
			break
		}
		time.Sleep(backoff)
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
	return nil, lastErr
}

func (b *Broker) declareTopology(retryTTL time.Duration) error {
	for _, ex := range []string{ExchangeJobs, ExchangeEvents, ExchangeRetry, ExchangeParking} {
		if err := b.ch.ExchangeDeclare(ex, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", ex, err)
		}
	}

	// Work queues are quorum queues that dead-letter to the retry exchange.
	workArgs := amqp.Table{
		"x-queue-type":           "quorum",
		"x-dead-letter-exchange": ExchangeRetry,
	}
	for queue, key := range map[string]string{QueueImage: "image.process", QueueOCR: "ocr.extract"} {
		if _, err := b.ch.QueueDeclare(queue, true, false, false, false, workArgs); err != nil {
			return fmt.Errorf("declare queue %s: %w", queue, err)
		}
		if err := b.ch.QueueBind(queue, key, ExchangeJobs, false, nil); err != nil {
			return fmt.Errorf("bind queue %s: %w", queue, err)
		}
	}

	// Retry queue: messages sit here for retryTTL, then dead-letter back onto
	// media.jobs to be re-attempted. Bounded attempts are enforced in workers.
	retryArgs := amqp.Table{
		"x-dead-letter-exchange": ExchangeJobs,
		"x-message-ttl":          int32(retryTTL.Milliseconds()),
	}
	if _, err := b.ch.QueueDeclare(QueueRetry, true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("declare retry queue: %w", err)
	}
	if err := b.ch.QueueBind(QueueRetry, "#", ExchangeRetry, false, nil); err != nil {
		return fmt.Errorf("bind retry queue: %w", err)
	}

	// Parking lot: terminal DLQ for messages that exhausted their retries.
	if _, err := b.ch.QueueDeclare(QueueParking, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare parking queue: %w", err)
	}
	if err := b.ch.QueueBind(QueueParking, "#", ExchangeParking, false, nil); err != nil {
		return fmt.Errorf("bind parking queue: %w", err)
	}
	return nil
}

// PublishJSON publishes v to exchange/key as a persistent JSON message and waits
// for the broker's publisher confirmation. It opens a producer span and injects
// the W3C trace context into the message headers, so a worker consuming the
// message continues the same distributed trace.
func (b *Broker) PublishJSON(ctx context.Context, exchange, key string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	ctx, span := tracer.Start(ctx, "publish "+exchange,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination.name", exchange),
			attribute.String("messaging.rabbitmq.destination.routing_key", key),
		),
	)
	defer span.End()

	headers := amqp.Table{}
	otel.GetTextMapPropagator().Inject(ctx, amqpHeaderCarrier(headers))

	conf, err := b.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, key, true, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Headers:      headers,
		Body:         body,
	})
	if err != nil {
		span.RecordError(err)
		return fmt.Errorf("publish to %s/%s: %w", exchange, key, err)
	}
	if ok := conf.Wait(); !ok {
		err := fmt.Errorf("publish to %s/%s not confirmed (nacked)", exchange, key)
		span.RecordError(err)
		return err
	}
	return nil
}

// amqpHeaderCarrier adapts an amqp.Table to the OTel TextMapCarrier interface so
// trace context can be injected into / extracted from message headers.
type amqpHeaderCarrier amqp.Table

func (c amqpHeaderCarrier) Get(key string) string {
	if v, ok := c[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (c amqpHeaderCarrier) Set(key, value string) { c[key] = value }

func (c amqpHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func (b *Broker) Close() error {
	if b.conn != nil {
		return b.conn.Close()
	}
	return nil
}
