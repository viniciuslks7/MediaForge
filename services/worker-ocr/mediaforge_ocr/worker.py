"""OCR worker entrypoint: wires config, storage, db, broker and the OCR engine."""

from __future__ import annotations

import logging
import signal
import sys
import time

from . import ocr
from .broker import Broker, Job
from .config import Config
from .db import Database
from .metrics import JOB_DURATION, JOBS_PROCESSED, WORDS_EXTRACTED, serve
from .storage import ObjectStore

logging.basicConfig(
    level=logging.INFO,
    format='{"level":"%(levelname)s","logger":"%(name)s","msg":"%(message)s"}',
)
log = logging.getLogger("mediaforge.ocr")


class Worker:
    def __init__(self, cfg: Config, db: Database, store: ObjectStore, broker: Broker):
        self._cfg = cfg
        self._db = db
        self._store = store
        self._broker = broker

    def handle(self, job: Job, attempt: int) -> None:
        start = time.monotonic()
        outcome = "success"
        try:
            self._process(job, attempt)
        except Exception:
            outcome = "parked" if attempt >= self._cfg.max_retries else "retry"
            raise
        finally:
            JOB_DURATION.observe(time.monotonic() - start)
            JOBS_PROCESSED.labels(outcome=outcome).inc()

    def _process(self, job: Job, attempt: int) -> None:
        # Idempotency: a redelivered, already-completed job is a no-op.
        if self._db.status(job.job_id) == "completed":
            log.info("job %s already completed, skipping", job.job_id)
            return

        self._db.set_status(job.job_id, "processing")
        self._emit(job, "processing", 10, "downloading source")

        data = self._store.get(job.source_key)
        self._emit(job, "processing", 40, "running OCR")

        result = ocr.extract_text(
            data, self._cfg.languages, tesseract_cmd=self._cfg.tesseract_cmd
        )

        text_key = f"artifacts/{job.job_id}/text.txt"
        size = self._store.put(text_key, "text/plain; charset=utf-8", result.text.encode("utf-8"))
        self._db.add_artifact(
            job.job_id,
            "text",
            text_key,
            "text/plain; charset=utf-8",
            size,
            {
                "word_count": result.word_count,
                "char_count": result.char_count,
                "mean_confidence": result.mean_confidence,
                "languages": result.languages,
                **result.metadata,
            },
        )
        WORDS_EXTRACTED.inc(result.word_count)
        self._emit(job, "processing", 80, "stored text", artifact="text")

        self._db.set_status(job.job_id, "completed")
        self._emit(job, "completed", 100, f"{result.word_count} words")
        log.info(
            "job %s completed (%d words, conf=%.1f)",
            job.job_id,
            result.word_count,
            result.mean_confidence,
        )

    def _emit(
        self, job: Job, status: str, progress: int, message: str = "", artifact: str = ""
    ) -> None:
        self._broker.publish_event(
            {
                "job_id": job.job_id,
                "kind": job.kind,
                "status": status,
                "progress": progress,
                "message": message,
                "artifact": artifact,
                "ts": int(time.time() * 1000),
            }
        )


def main() -> int:
    cfg = Config.load()

    serve(cfg.metrics_port)
    log.info("metrics listening on :%d", cfg.metrics_port)

    db = Database(cfg.postgres_dsn)
    store = ObjectStore(
        cfg.s3_endpoint, cfg.s3_access_key, cfg.s3_secret_key, cfg.s3_bucket, cfg.s3_use_ssl
    )
    broker = Broker(cfg.rabbit_url, cfg.prefetch, cfg.max_retries, cfg.retry_ttl_ms)
    worker = Worker(cfg, db, store, broker)

    def _shutdown(_signum, _frame):  # noqa: ANN001
        log.info("signal received, draining")
        broker.close()

    signal.signal(signal.SIGTERM, _shutdown)
    signal.signal(signal.SIGINT, _shutdown)

    log.info("ocr worker started, consuming q.ocr")
    try:
        broker.consume("q.ocr", worker.handle)
    except KeyboardInterrupt:
        pass
    finally:
        db.close()
    log.info("drained, bye")
    return 0


if __name__ == "__main__":
    sys.exit(main())
