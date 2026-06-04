// Package store updates job state and records produced artifacts in Postgres.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 8
	cfg.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// Status reads the current status of a job. Used for idempotency: a worker
// skips jobs already in a terminal state.
func (s *Store) Status(ctx context.Context, jobID string) (string, error) {
	var status string
	err := s.pool.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1`, jobID).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("read status: %w", err)
	}
	return status, nil
}

// SetStatus transitions a job and stamps updated_at, optionally recording an error.
func (s *Store) SetStatus(ctx context.Context, jobID, status, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE jobs SET status = $2, error = NULLIF($3,''), updated_at = now() WHERE id = $1`,
		jobID, status, errMsg)
	if err != nil {
		return fmt.Errorf("set status: %w", err)
	}
	return nil
}

// AddArtifact upserts a produced artifact (idempotent on job_id+name).
func (s *Store) AddArtifact(ctx context.Context, jobID, name, key, contentType string, size int64, meta map[string]any) error {
	m, _ := json.Marshal(meta)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO artifacts (job_id, name, object_key, content_type, size_bytes, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (job_id, name) DO UPDATE
		  SET object_key = EXCLUDED.object_key,
		      content_type = EXCLUDED.content_type,
		      size_bytes = EXCLUDED.size_bytes,
		      metadata = EXCLUDED.metadata`,
		jobID, name, key, contentType, size, m)
	if err != nil {
		return fmt.Errorf("add artifact: %w", err)
	}
	return nil
}
