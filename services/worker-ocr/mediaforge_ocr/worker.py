"""OCR worker entrypoint: wires config, storage, db, broker and the OCR engine."""

from __future__ import annotations

import logging
import signal
import sys
import time

from . import ocr
from .broker import Broker, Job, PermanentError
from .config import Config
from .db import Database
from .metrics import JOB_DURATION, JOBS_PROCESSED, WORDS_EXTRACTED, serve
from .storage import ObjectStore
from .tracing import init_tracing, install_log_correlation

install_log_correlation()
logging.basicConfig(
    level=logging.INFO,
    format=(
        '{"level":"%(levelname)s","logger":"%(name)s",'
        '"trace_id":"%(trace_id)s","span_id":"%(span_id)s","msg":"%(message)s"}'
    ),
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
        except Exception as exc:
            terminal = isinstance(exc, PermanentError) or attempt >= self._cfg.max_retries
            outcome = "parked" if terminal else "retry"
            # The job row and the UI must never be left hanging in
            # "processing": record the failure before the broker decides
            # retry vs park (a retry flips it back to processing).
            self._fail(job, exc)
            raise
        finally:
            JOB_DURATION.observe(time.monotonic() - start)
            JOBS_PROCESSED.labels(outcome=outcome).inc()

    def _fail(self, job: Job, exc: Exception) -> None:
        try:
            self._db.set_status(job.job_id, "failed", str(exc))
            self._emit(job, "failed", 100, str(exc))
        except Exception:  # noqa: BLE001 — best-effort, never mask the cause
            log.exception("recording failure for job %s", job.job_id)

    def _process(self, job: Job, attempt: int) -> None:
        # Idempotency: a redelivered, already-completed job is a no-op.
        if self._db.status(job.job_id) == "completed":
            log.info("job %s already completed, skipping", job.job_id)
            return

        self._db.set_status(job.job_id, "processing")
        self._emit(job, "processing", 10, "downloading source")

        data = self._store.get(job.source_key)
        self._emit(job, "processing", 40, "running OCR")

        try:
            result = ocr.extract_text(
                data, self._cfg.languages, tesseract_cmd=self._cfg.tesseract_cmd
            )
        except PermanentError:
            raise
        except Exception as exc:
            # Extraction is deterministic for a given payload: an unreadable
            # file fails identically on every attempt, so park immediately.
            raise PermanentError(f"extract: {exc}") from exc

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

    tracer_provider = init_tracing(cfg)

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
        if tracer_provider is not None:
            tracer_provider.shutdown()
    log.info("drained, bye")
    return 0


if __name__ == "__main__":
    sys.exit(main())
