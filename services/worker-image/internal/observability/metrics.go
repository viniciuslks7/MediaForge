// Package observability exposes the worker's Prometheus metrics and logger.
package observability

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/trace"
)

var (
	JobsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mediaforge", Subsystem: "image_worker", Name: "jobs_processed_total",
		Help: "Image jobs processed, partitioned by outcome.",
	}, []string{"outcome"}) // success | retry | parked

	JobDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "mediaforge", Subsystem: "image_worker", Name: "job_duration_seconds",
		Help: "End-to-end image job processing time.", Buckets: prometheus.DefBuckets,
	})

	ArtifactsProduced = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mediaforge", Subsystem: "image_worker", Name: "artifacts_produced_total",
		Help: "Artifacts produced by variant.",
	}, []string{"variant"})
)

// Logger returns a JSON structured logger. It is wrapped so any log emitted with
// a context carrying an active span is tagged with trace_id and span_id, making
// logs and traces cross-navigable. Use the *Context log methods to propagate it.
func Logger() *slog.Logger {
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(traceHandler{base})
}

// traceHandler decorates each record with the trace/span IDs from the context.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}

// ServeMetrics starts a metrics+health server and shuts it down on ctx cancel.
func ServeMetrics(ctx context.Context, addr string, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sc)
	}()

	log.Info("metrics listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("metrics server", "err", err)
	}
}
