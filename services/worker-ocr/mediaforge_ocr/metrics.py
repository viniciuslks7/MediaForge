"""Prometheus metrics for the OCR worker."""

from __future__ import annotations

from prometheus_client import Counter, Histogram, start_http_server

JOBS_PROCESSED = Counter(
    "mediaforge_ocr_worker_jobs_processed_total",
    "OCR jobs processed, partitioned by outcome.",
    ["outcome"],  # success | retry | parked
)

JOB_DURATION = Histogram(
    "mediaforge_ocr_worker_job_duration_seconds",
    "End-to-end OCR job processing time.",
)

WORDS_EXTRACTED = Counter(
    "mediaforge_ocr_worker_words_extracted_total",
    "Total number of words extracted across all documents.",
)


def serve(port: int) -> None:
    """Start the Prometheus metrics HTTP server (non-blocking)."""
    start_http_server(port)
