-- MediaForge schema. Applied idempotently by the gateway on boot.

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
