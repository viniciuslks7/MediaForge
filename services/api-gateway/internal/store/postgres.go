// Package store is the PostgreSQL persistence layer for jobs and artifacts.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/viniciusoliveira/mediaforge/api-gateway/internal/media"
)

var ErrNotFound = errors.New("job not found")

type Store struct {
	pool *pgxpool.Pool
}

// New opens a pgx pool against dsn and verifies connectivity.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// schema is applied idempotently on boot. Kept in sync with
// migrations/0001_init.sql.
const schema = `
CREATE TABLE IF NOT EXISTS jobs (
    id          UUID PRIMARY KEY,
    kind        TEXT        NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'pending',
    source_key  TEXT        NOT NULL,
    source_mime TEXT        NOT NULL,
    size_bytes  BIGINT      NOT NULL DEFAULT 0,
    operations  JSONB       NOT NULL DEFAULT '[]',
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_jobs_status     ON jobs (status);
CREATE INDEX IF NOT EXISTS idx_jobs_kind       ON jobs (kind);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs (created_at DESC);
CREATE TABLE IF NOT EXISTS artifacts (
    id           BIGSERIAL PRIMARY KEY,
    job_id       UUID        NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    object_key   TEXT        NOT NULL,
    content_type TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL DEFAULT 0,
    metadata     JSONB       NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (job_id, name)
);
CREATE INDEX IF NOT EXISTS idx_artifacts_job_id ON artifacts (job_id);
`

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

func (s *Store) Close() { s.pool.Close() }

// CreateJob persists a new job in the pending state.
func (s *Store) CreateJob(ctx context.Context, j *media.Job) error {
	ops, _ := json.Marshal(j.Operations)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO jobs (id, kind, status, source_key, source_mime, size_bytes, operations)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		j.ID, j.Kind, j.Status, j.SourceKey, j.SourceMIME, j.SizeBytes, ops)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

// GetJob loads a job and its artifacts by id.
func (s *Store) GetJob(ctx context.Context, id string) (*media.Job, []media.Artifact, error) {
	var j media.Job
	var ops []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, kind, status, source_key, source_mime, size_bytes, operations, created_at, updated_at
		FROM jobs WHERE id = $1`, id).
		Scan(&j.ID, &j.Kind, &j.Status, &j.SourceKey, &j.SourceMIME, &j.SizeBytes, &ops, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("select job: %w", err)
	}
	_ = json.Unmarshal(ops, &j.Operations)

	rows, err := s.pool.Query(ctx, `
		SELECT job_id, name, object_key, content_type, size_bytes, metadata, created_at
		FROM artifacts WHERE job_id = $1 ORDER BY created_at`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("select artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []media.Artifact
	for rows.Next() {
		var a media.Artifact
		var meta []byte
		if err := rows.Scan(&a.JobID, &a.Name, &a.ObjectKey, &a.ContentType, &a.SizeBytes, &meta, &a.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("scan artifact: %w", err)
		}
		_ = json.Unmarshal(meta, &a.Metadata)
		artifacts = append(artifacts, a)
	}
	return &j, artifacts, rows.Err()
}
