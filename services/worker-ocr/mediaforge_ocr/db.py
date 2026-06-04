"""PostgreSQL access for job state and artifacts (psycopg 3)."""

from __future__ import annotations

import json

import psycopg
from psycopg_pool import ConnectionPool


class Database:
    def __init__(self, dsn: str):
        self._pool = ConnectionPool(dsn, min_size=1, max_size=5, kwargs={"autocommit": True})

    def close(self) -> None:
        self._pool.close()

    def status(self, job_id: str) -> str | None:
        with self._pool.connection() as conn:
            row = conn.execute("SELECT status FROM jobs WHERE id = %s", (job_id,)).fetchone()
            return row[0] if row else None

    def set_status(self, job_id: str, status: str, error: str = "") -> None:
        with self._pool.connection() as conn:
            conn.execute(
                "UPDATE jobs SET status = %s, error = NULLIF(%s,''), updated_at = now() "
                "WHERE id = %s",
                (status, error, job_id),
            )

    def add_artifact(
        self,
        job_id: str,
        name: str,
        object_key: str,
        content_type: str,
        size_bytes: int,
        metadata: dict,
    ) -> None:
        with self._pool.connection() as conn:
            conn.execute(
                """
                INSERT INTO artifacts (job_id, name, object_key, content_type, size_bytes, metadata)
                VALUES (%s, %s, %s, %s, %s, %s)
                ON CONFLICT (job_id, name) DO UPDATE
                  SET object_key = EXCLUDED.object_key,
                      content_type = EXCLUDED.content_type,
                      size_bytes = EXCLUDED.size_bytes,
                      metadata = EXCLUDED.metadata
                """,
                (job_id, name, object_key, content_type, size_bytes, json.dumps(metadata)),
            )


# Re-exported for callers that want to catch connection errors explicitly.
DatabaseError = psycopg.Error
