// Package config loads the image worker's configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	RabbitURL     string
	Prefetch      int
	MaxRetries    int
	MetricsAddr   string
	ThumbnailSize int
	ResizeMaxDim  int

	PostgresDSN string

	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool
	S3Region    string

	OTelEnabled     bool
	OTelEndpoint    string
	OTelSampleRatio float64
	ServiceName     string
	ServiceVersion  string
}

func Load() (*Config, error) {
	c := &Config{
		RabbitURL:     env("RABBITMQ_URL", ""),
		Prefetch:      int(envInt("RABBITMQ_PREFETCH", 16)),
		MaxRetries:    int(envInt("RABBITMQ_MAX_RETRIES", 5)),
		MetricsAddr:   env("IMAGE_METRICS_ADDR", ":9102"),
		ThumbnailSize: int(envInt("IMAGE_THUMBNAIL_SIZE", 256)),
		ResizeMaxDim:  int(envInt("IMAGE_RESIZE_MAX_DIM", 1600)),
		PostgresDSN:   env("POSTGRES_DSN", ""),
		S3Endpoint:    env("S3_ENDPOINT", "minio:9000"),
		S3AccessKey:   env("S3_ACCESS_KEY", ""),
		S3SecretKey:   env("S3_SECRET_KEY", ""),
		S3Bucket:      env("S3_BUCKET", "mediaforge"),
		S3UseSSL:      envBool("S3_USE_SSL", false),
		S3Region:      env("S3_REGION", "us-east-1"),

		OTelEnabled:     envBool("OTEL_TRACES_ENABLED", false),
		OTelEndpoint:    env("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317"),
		OTelSampleRatio: envFloat("OTEL_TRACES_SAMPLE_RATIO", 1.0),
		ServiceName:     env("OTEL_SERVICE_NAME", "worker-image"),
		ServiceVersion:  env("SERVICE_VERSION", "dev"),
	}
	var missing []string
	for k, v := range map[string]string{
		"RABBITMQ_URL":  c.RabbitURL,
		"POSTGRES_DSN":  c.PostgresDSN,
		"S3_ACCESS_KEY": c.S3AccessKey,
		"S3_SECRET_KEY": c.S3SecretKey,
	} {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func envInt(k string, def int64) int64 {
	if v, ok := os.LookupEnv(k); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v, ok := os.LookupEnv(k); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v, ok := os.LookupEnv(k); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
