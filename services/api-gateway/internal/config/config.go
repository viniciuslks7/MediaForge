// Package config loads service configuration from the environment (12-factor),
// applying defaults and failing fast on anything required but missing.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr       string
	MaxUploadBytes int64
	AuthToken      string

	PostgresDSN string
	RedisAddr   string

	RabbitURL      string
	RabbitPrefetch int

	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool
	S3Region    string
}

// Load reads configuration from the process environment.
func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:       env("API_HTTP_ADDR", ":8080"),
		MaxUploadBytes: envInt64("API_MAX_UPLOAD_BYTES", 25<<20),
		AuthToken:      env("API_AUTH_TOKEN", ""),
		PostgresDSN:    env("POSTGRES_DSN", ""),
		RedisAddr:      env("REDIS_ADDR", "redis:6379"),
		RabbitURL:      env("RABBITMQ_URL", ""),
		RabbitPrefetch: int(envInt64("RABBITMQ_PREFETCH", 16)),
		S3Endpoint:     env("S3_ENDPOINT", "minio:9000"),
		S3AccessKey:    env("S3_ACCESS_KEY", ""),
		S3SecretKey:    env("S3_SECRET_KEY", ""),
		S3Bucket:       env("S3_BUCKET", "mediaforge"),
		S3UseSSL:       envBool("S3_USE_SSL", false),
		S3Region:       env("S3_REGION", "us-east-1"),
	}

	var missing []string
	for k, v := range map[string]string{
		"POSTGRES_DSN":  c.PostgresDSN,
		"RABBITMQ_URL":  c.RabbitURL,
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

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
