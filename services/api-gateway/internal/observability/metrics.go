// Package observability wires Prometheus metrics and a structured logger used
// across the gateway. Metrics follow the RED method (Rate, Errors, Duration).
package observability

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTPRequests counts inbound HTTP requests by route, method and status.
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mediaforge",
		Subsystem: "api",
		Name:      "http_requests_total",
		Help:      "Total HTTP requests handled by the API gateway.",
	}, []string{"route", "method", "status"})

	// HTTPDuration observes request latency by route.
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "mediaforge",
		Subsystem: "api",
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request latency in seconds.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"route"})

	// JobsPublished counts jobs successfully published to the bus by kind.
	JobsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mediaforge",
		Subsystem: "api",
		Name:      "jobs_published_total",
		Help:      "Total media jobs published to RabbitMQ.",
	}, []string{"kind"})

	// UploadBytes observes the size of accepted uploads.
	UploadBytes = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "mediaforge",
		Subsystem: "api",
		Name:      "upload_bytes",
		Help:      "Size of accepted uploads in bytes.",
		Buckets:   prometheus.ExponentialBuckets(1024, 4, 8),
	})
)

// Logger returns a JSON structured logger at info level.
func Logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// MetricsHandler exposes the Prometheus scrape endpoint.
func MetricsHandler() http.Handler { return promhttp.Handler() }
