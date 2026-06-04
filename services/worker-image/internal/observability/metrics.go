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

func Logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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
