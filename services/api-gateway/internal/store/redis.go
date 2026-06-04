package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter is a fixed-window per-client limiter backed by Redis. Simple,
// distributed and good enough for protecting the ingestion endpoint.
type RateLimiter struct {
	rdb    *redis.Client
	limit  int
	window time.Duration
}

// NewRateLimiter connects to Redis and returns a limiter allowing `limit`
// requests per `window` per key.
func NewRateLimiter(addr string, limit int, window time.Duration) (*RateLimiter, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	return &RateLimiter{rdb: rdb, limit: limit, window: window}, nil
}

// Allow reports whether the caller identified by key may proceed.
func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	bucket := fmt.Sprintf("rl:%s:%d", key, time.Now().Unix()/int64(r.window.Seconds()))
	n, err := r.rdb.Incr(ctx, bucket).Result()
	if err != nil {
		return true, err // fail open: don't reject traffic on limiter outage
	}
	if n == 1 {
		_ = r.rdb.Expire(ctx, bucket, r.window).Err()
	}
	return n <= int64(r.limit), nil
}
