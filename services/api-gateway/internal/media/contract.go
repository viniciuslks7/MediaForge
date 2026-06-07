// Package media defines the domain types and the wire contracts shared between
// the gateway and the workers. These structs ARE the message format on the bus,
// so changes here are changes to the inter-service protocol — keep them stable
// and additive.
package media

import "time"

// Kind identifies what pipeline a job flows through. The string value is also
// used as the RabbitMQ routing key suffix (media.jobs -> image.process, etc.).
type Kind string

const (
	KindImage Kind = "image"
	KindOCR   Kind = "ocr"
)

// RoutingKey maps a Kind to its RabbitMQ routing key on the media.jobs exchange.
func (k Kind) RoutingKey() string {
	switch k {
	case KindOCR:
		return "ocr.extract"
	default:
		return "image.process"
	}
}

// Valid reports whether k is a Kind the platform knows how to route.
func (k Kind) Valid() bool {
	return k == KindImage || k == KindOCR
}

// Status is the lifecycle state of a job, persisted in Postgres and emitted on
// the media.events exchange.
type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Operation is an image transformation requested by the client.
type Operation string

const (
	OpResize    Operation = "resize"
	OpThumbnail Operation = "thumbnail"
	OpWebP      Operation = "webp"
	OpGrayscale Operation = "grayscale"
)

// JobParams carries optional per-job overrides for the image pipeline. A nil
// *JobParams (the common case) means "use the worker's configured defaults".
// Mirrors the "params" object in specs/schemas/job.schema.json.
type JobParams struct {
	ResizeMaxDim  int    `json:"resize_max_dim,omitempty"`
	ThumbnailSize int    `json:"thumbnail_size,omitempty"`
	ResizeFormat  string `json:"resize_format,omitempty"` // jpeg | png | webp
	Quality       int    `json:"quality,omitempty"`       // 1..100, lossy formats
}

// Job is the persisted record of a piece of work and the payload published to
// the workers on media.jobs.
type Job struct {
	ID         string      `json:"job_id"`
	Kind       Kind        `json:"kind"`
	Status     Status      `json:"status"`
	SourceKey  string      `json:"source_key"`  // object key of the uploaded original
	SourceMIME string      `json:"source_mime"` // detected content type
	SizeBytes  int64       `json:"size_bytes"`
	Operations []Operation `json:"operations,omitempty"` // image jobs only
	Params     *JobParams  `json:"params,omitempty"`     // optional per-job overrides
	Attempt    int         `json:"attempt"`              // delivery attempt, set by the broker layer
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// Artifact is a single output produced by a worker (a thumbnail, a webp, the
// extracted text blob, ...). Stored in Postgres and surfaced via the API.
type Artifact struct {
	JobID       string         `json:"job_id"`
	Name        string         `json:"name"` // logical name: "thumbnail", "webp", "text"
	ObjectKey   string         `json:"object_key"`
	ContentType string         `json:"content_type"`
	SizeBytes   int64          `json:"size_bytes"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Event is emitted by any service on the media.events exchange whenever a job
// changes state. The realtime-gateway fans these out to subscribed WebSocket
// clients keyed by JobID.
type Event struct {
	JobID     string `json:"job_id"`
	Kind      Kind   `json:"kind"`
	Status    Status `json:"status"`
	Message   string `json:"message,omitempty"`
	Progress  int    `json:"progress"` // 0..100
	Artifact  string `json:"artifact,omitempty"`
	Timestamp int64  `json:"ts"` // unix millis
}
