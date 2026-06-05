// Command worker is the MediaForge image-processing worker. It consumes image
// jobs from RabbitMQ, downloads the original from object storage, produces the
// requested variants, uploads them, records artifacts and emits live events.
package main

import (
	"context"
	"fmt"
	_ "image/gif"  // register gif decoder for inputs
	_ "image/jpeg" // register jpeg decoder for inputs
	_ "image/png"  // register png decoder for inputs
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "golang.org/x/image/webp" // register webp decoder for inputs

	"github.com/viniciusoliveira/mediaforge/worker-image/internal/broker"
	"github.com/viniciusoliveira/mediaforge/worker-image/internal/config"
	"github.com/viniciusoliveira/mediaforge/worker-image/internal/observability"
	"github.com/viniciusoliveira/mediaforge/worker-image/internal/processor"
	"github.com/viniciusoliveira/mediaforge/worker-image/internal/storage"
	"github.com/viniciusoliveira/mediaforge/worker-image/internal/store"
)

func main() {
	log := observability.Logger()

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	boot, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	shutdownTracing, err := observability.InitTracing(boot, observability.TracingConfig{
		Enabled:        cfg.OTelEnabled,
		Endpoint:       cfg.OTelEndpoint,
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		SampleRatio:    cfg.OTelSampleRatio,
	})
	if err != nil {
		log.Error("init tracing", "err", err)
		os.Exit(1)
	}
	defer func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer flushCancel()
		if err := shutdownTracing(flushCtx); err != nil {
			log.Error("shutdown tracing", "err", err)
		}
	}()

	db, err := store.New(boot, cfg.PostgresDSN)
	if err != nil {
		log.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	objects, err := storage.New(storage.Options{
		Endpoint: cfg.S3Endpoint, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
		Bucket: cfg.S3Bucket, UseSSL: cfg.S3UseSSL, Region: cfg.S3Region,
	})
	if err != nil {
		log.Error("connect object store", "err", err)
		os.Exit(1)
	}

	bus, err := broker.Connect(cfg.RabbitURL, cfg.Prefetch, cfg.MaxRetries, 10*time.Second)
	if err != nil {
		log.Error("connect rabbitmq", "err", err)
		os.Exit(1)
	}
	defer bus.Close()

	proc := processor.New(processor.Options{
		ThumbnailSize: cfg.ThumbnailSize,
		ResizeMaxDim:  cfg.ResizeMaxDim,
	})

	h := &handler{db: db, objects: objects, bus: bus, proc: proc, log: log}

	go observability.ServeMetrics(ctx, cfg.MetricsAddr, log)

	log.Info("image worker started", "queue", broker.QueueImage, "prefetch", cfg.Prefetch)
	if err := bus.Consume(ctx, broker.QueueImage, h.handle); err != nil && ctx.Err() == nil {
		log.Error("consume loop", "err", err)
		os.Exit(1)
	}
	log.Info("drained, bye")
}

type handler struct {
	db      *store.Store
	objects *storage.Store
	bus     *broker.Broker
	proc    *processor.Processor
	log     *slog.Logger
}

func (h *handler) handle(ctx context.Context, job broker.Job, attempt int) (err error) {
	start := time.Now()
	log := h.log.With("job_id", job.ID, "attempt", attempt)

	defer func() {
		observability.JobDuration.Observe(time.Since(start).Seconds())
		switch {
		case err == nil:
			observability.JobsProcessed.WithLabelValues("success").Inc()
		case attempt >= 5:
			observability.JobsProcessed.WithLabelValues("parked").Inc()
		default:
			observability.JobsProcessed.WithLabelValues("retry").Inc()
		}
	}()

	// Idempotency: skip jobs already completed by a previous (redelivered) attempt.
	if status, serr := h.db.Status(ctx, job.ID); serr == nil && status == "completed" {
		log.InfoContext(ctx, "job already completed, skipping")
		return nil
	}

	_ = h.db.SetStatus(ctx, job.ID, "processing", "")
	_ = h.bus.PublishEvent(ctx, broker.Event{JobID: job.ID, Kind: job.Kind, Status: "processing", Progress: 10, Message: "downloading source"})

	src, err := h.objects.Get(ctx, job.SourceKey)
	if err != nil {
		return h.fail(ctx, job, attempt, fmt.Errorf("download source: %w", err))
	}

	_ = h.bus.PublishEvent(ctx, broker.Event{JobID: job.ID, Kind: job.Kind, Status: "processing", Progress: 40, Message: "transforming"})

	results, err := h.proc.Process(src, job.Operations, processorParams(job))
	if err != nil {
		return h.fail(ctx, job, attempt, fmt.Errorf("process: %w", err))
	}

	for _, r := range results {
		key := fmt.Sprintf("artifacts/%s/%s", job.ID, artifactFilename(r))
		size, perr := h.objects.Put(ctx, key, r.ContentType, r.Bytes)
		if perr != nil {
			return h.fail(ctx, job, attempt, fmt.Errorf("upload %s: %w", r.Name, perr))
		}
		if aerr := h.db.AddArtifact(ctx, job.ID, r.Name, key, r.ContentType, size, r.Metadata); aerr != nil {
			return h.fail(ctx, job, attempt, fmt.Errorf("record %s: %w", r.Name, aerr))
		}
		observability.ArtifactsProduced.WithLabelValues(r.Name).Inc()
		_ = h.bus.PublishEvent(ctx, broker.Event{JobID: job.ID, Kind: job.Kind, Status: "processing", Progress: 80, Artifact: r.Name})
	}

	if err := h.db.SetStatus(ctx, job.ID, "completed", ""); err != nil {
		return h.fail(ctx, job, attempt, fmt.Errorf("finalize: %w", err))
	}
	_ = h.bus.PublishEvent(ctx, broker.Event{JobID: job.ID, Kind: job.Kind, Status: "completed", Progress: 100, Message: "done"})
	log.InfoContext(ctx, "job completed", "artifacts", len(results))
	return nil
}

// fail records the error and lets the broker decide retry vs parking based on
// the attempt count.
func (h *handler) fail(ctx context.Context, job broker.Job, attempt int, cause error) error {
	h.log.WarnContext(ctx, "job failed", "job_id", job.ID, "attempt", attempt, "err", cause)
	_ = h.db.SetStatus(ctx, job.ID, "failed", cause.Error())
	_ = h.bus.PublishEvent(ctx, broker.Event{JobID: job.ID, Kind: job.Kind, Status: "failed", Message: cause.Error()})
	return cause
}

// processorParams maps the job's optional wire params to the processor's params.
// A nil job.Params yields the zero value, which means "use defaults".
func processorParams(job broker.Job) processor.Params {
	if job.Params == nil {
		return processor.Params{}
	}
	return processor.Params{
		ResizeMaxDim:  job.Params.ResizeMaxDim,
		ThumbnailSize: job.Params.ThumbnailSize,
		ResizeFormat:  job.Params.ResizeFormat,
		Quality:       job.Params.Quality,
	}
}

func artifactFilename(r processor.Result) string {
	switch r.ContentType {
	case "image/png":
		return r.Name + ".png"
	case "image/webp":
		return r.Name + ".webp"
	default:
		return r.Name + ".jpg"
	}
}
