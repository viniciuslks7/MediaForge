// Package broker is the worker's RabbitMQ adapter: it declares the (shared,
// idempotent) topology, consumes jobs with bounded retries via a dead-letter +
// TTL retry loop, and publishes lifecycle events.
package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

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

// Job is the wire contract published by the gateway on media.jobs. It mirrors
// api-gateway/internal/media.Job — keep the JSON tags in sync.
type Job struct {
	ID         string   `json:"job_id"`
	Kind       string   `json:"kind"`
	Status     string   `json:"status"`
	SourceKey  string   `json:"source_key"`
	SourceMIME string   `json:"source_mime"`
	SizeBytes  int64    `json:"size_bytes"`
	Operations []string `json:"operations"`
}

// Event is emitted on media.events as a job changes state.
type Event struct {
	JobID     string `json:"job_id"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	Progress  int    `json:"progress"`
	Artifact  string `json:"artifact,omitempty"`
	Timestamp int64  `json:"ts"`
}

type Broker struct {
	conn       *amqp.Connection
	ch         *amqp.Channel
	maxRetries int
}

func Connect(url string, prefetch, maxRetries int, retryTTL time.Duration) (*Broker, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("set qos: %w", err)
	}
	b := &Broker{conn: conn, ch: ch, maxRetries: maxRetries}
	if err := b.declareTopology(retryTTL); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return b, nil
}

func (b *Broker) declareTopology(retryTTL time.Duration) error {
	for _, ex := range []string{ExchangeJobs, ExchangeEvents, ExchangeRetry, ExchangeParking} {
		if err := b.ch.ExchangeDeclare(ex, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare exchange %s: %w", ex, err)
		}
	}
	workArgs := amqp.Table{"x-queue-type": "quorum", "x-dead-letter-exchange": ExchangeRetry}
	for queue, key := range map[string]string{QueueImage: "image.process", QueueOCR: "ocr.extract"} {
		if _, err := b.ch.QueueDeclare(queue, true, false, false, false, workArgs); err != nil {
			return fmt.Errorf("declare queue %s: %w", queue, err)
		}
		if err := b.ch.QueueBind(queue, key, ExchangeJobs, false, nil); err != nil {
			return fmt.Errorf("bind queue %s: %w", queue, err)
		}
	}
	retryArgs := amqp.Table{"x-dead-letter-exchange": ExchangeJobs, "x-message-ttl": int32(retryTTL.Milliseconds())}
	if _, err := b.ch.QueueDeclare(QueueRetry, true, false, false, false, retryArgs); err != nil {
		return fmt.Errorf("declare retry queue: %w", err)
	}
	if err := b.ch.QueueBind(QueueRetry, "#", ExchangeRetry, false, nil); err != nil {
		return fmt.Errorf("bind retry queue: %w", err)
	}
	if _, err := b.ch.QueueDeclare(QueueParking, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare parking queue: %w", err)
	}
	if err := b.ch.QueueBind(QueueParking, "#", ExchangeParking, false, nil); err != nil {
		return fmt.Errorf("bind parking queue: %w", err)
	}
	return nil
}

// Handler processes a single job. Returning an error triggers the retry/DLQ path.
type Handler func(ctx context.Context, job Job, attempt int) error

// Consume runs the delivery loop on the given queue until ctx is cancelled.
// Retry policy:
//   - success            -> ack
//   - failure, attempt<N  -> nack(requeue=false) => dead-letters to retry queue,
//     which TTLs the message back onto media.jobs for another attempt
//   - failure, attempt>=N -> republish to the parking exchange, then ack
func (b *Broker) Consume(ctx context.Context, queue string, h Handler) error {
	deliveries, err := b.ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", queue, err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}
			b.handle(ctx, d, h)
		}
	}
}

func (b *Broker) handle(ctx context.Context, d amqp.Delivery, h Handler) {
	attempt := deathCount(d.Headers) + 1

	var job Job
	if err := json.Unmarshal(d.Body, &job); err != nil {
		// Unparseable: send straight to the parking lot, it will never succeed.
		_ = b.park(ctx, d, "unmarshal: "+err.Error())
		_ = d.Ack(false)
		return
	}

	if err := h(ctx, job, attempt); err != nil {
		if attempt >= b.maxRetries {
			_ = b.park(ctx, d, err.Error())
			_ = d.Ack(false) // terminal — remove from work queue
			return
		}
		_ = d.Nack(false, false) // dead-letter into the retry loop
		return
	}
	_ = d.Ack(false)
}

func (b *Broker) park(ctx context.Context, d amqp.Delivery, reason string) error {
	headers := amqp.Table{"x-parking-reason": reason}
	return b.ch.PublishWithContext(ctx, ExchangeParking, d.RoutingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
		Body:         d.Body,
	})
}

// PublishEvent emits a lifecycle event on media.events keyed by job kind.
func (b *Broker) PublishEvent(ctx context.Context, e Event) error {
	e.Timestamp = time.Now().UnixMilli()
	body, _ := json.Marshal(e)
	return b.ch.PublishWithContext(ctx, ExchangeEvents, "event."+e.Kind, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Transient,
		Body:         body,
	})
}

func (b *Broker) Close() error {
	if b.conn != nil {
		return b.conn.Close()
	}
	return nil
}

// deathCount reads the number of prior dead-letterings from the x-death header
// RabbitMQ maintains, which is how we count retry attempts.
func deathCount(h amqp.Table) int {
	raw, ok := h["x-death"]
	if !ok {
		return 0
	}
	deaths, ok := raw.([]any)
	if !ok || len(deaths) == 0 {
		return 0
	}
	first, ok := deaths[0].(amqp.Table)
	if !ok {
		return 0
	}
	if c, ok := first["count"].(int64); ok {
		return int(c)
	}
	return 0
}
