// Command api is the MediaForge ingestion gateway. It accepts uploads, stores
// originals, persists job records and publishes work onto RabbitMQ.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/broker"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/config"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/httpapi"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/observability"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/storage"
	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/store"
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

	// --- dependencies ---
	bootCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	shutdownTracing, err := observability.InitTracing(bootCtx, observability.TracingConfig{
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

	db, err := store.New(bootCtx, cfg.PostgresDSN)
	if err != nil {
		log.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	objects, err := storage.New(bootCtx, storage.Options{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Bucket:    cfg.S3Bucket,
		UseSSL:    cfg.S3UseSSL,
		Region:    cfg.S3Region,
	})
	if err != nil {
		log.Error("connect object store", "err", err)
		os.Exit(1)
	}

	bus, err := broker.Connect(cfg.RabbitURL, 10*time.Second)
	if err != nil {
		log.Error("connect rabbitmq", "err", err)
		os.Exit(1)
	}
	defer bus.Close()

	limiter, err := store.NewRateLimiter(cfg.RedisAddr, 60, time.Minute)
	if err != nil {
		log.Warn("rate limiter unavailable, continuing without it", "err", err)
	}

	srv := &httpapi.Server{
		Jobs:           db,
		Objects:        objects,
		Bus:            bus,
		Log:            log,
		MaxUploadBytes: cfg.MaxUploadBytes,
		AuthToken:      cfg.AuthToken,
	}
	if limiter != nil {
		srv.Limiter = limiter
	}

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// --- run with graceful shutdown ---
	go func() {
		log.Info("api gateway listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown", "err", err)
	}
	log.Info("bye")
}
