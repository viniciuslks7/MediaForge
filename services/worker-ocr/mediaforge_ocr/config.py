"""Environment-driven configuration (12-factor)."""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Config:
    rabbit_url: str
    prefetch: int
    max_retries: int
    retry_ttl_ms: int

    postgres_dsn: str

    s3_endpoint: str
    s3_access_key: str
    s3_secret_key: str
    s3_bucket: str
    s3_use_ssl: bool

    metrics_port: int
    languages: str
    tesseract_cmd: str

    otel_enabled: bool
    otel_endpoint: str
    otel_sample_ratio: float
    service_name: str
    service_version: str

    @staticmethod
    def load() -> Config:
        cfg = Config(
            rabbit_url=_req("RABBITMQ_URL"),
            prefetch=int(os.getenv("RABBITMQ_PREFETCH", "16")),
            max_retries=int(os.getenv("RABBITMQ_MAX_RETRIES", "5")),
            retry_ttl_ms=int(os.getenv("RABBITMQ_RETRY_TTL_MS", "10000")),
            postgres_dsn=_req("POSTGRES_DSN"),
            s3_endpoint=os.getenv("S3_ENDPOINT", "minio:9000"),
            s3_access_key=_req("S3_ACCESS_KEY"),
            s3_secret_key=_req("S3_SECRET_KEY"),
            s3_bucket=os.getenv("S3_BUCKET", "mediaforge"),
            s3_use_ssl=os.getenv("S3_USE_SSL", "false").lower() == "true",
            metrics_port=int(os.getenv("OCR_METRICS_PORT", "9103")),
            languages=os.getenv("OCR_LANGUAGES", "eng"),
            tesseract_cmd=os.getenv("OCR_TESSERACT_CMD", "tesseract"),
            otel_enabled=os.getenv("OTEL_TRACES_ENABLED", "false").lower() == "true",
            otel_endpoint=os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel-collector:4318"),
            otel_sample_ratio=float(os.getenv("OTEL_TRACES_SAMPLE_RATIO", "1.0")),
            service_name=os.getenv("OTEL_SERVICE_NAME", "worker-ocr"),
            service_version=os.getenv("SERVICE_VERSION", "dev"),
        )
        return cfg


def _req(key: str) -> str:
    value = os.getenv(key, "").strip()
    if not value:
        raise RuntimeError(f"missing required env: {key}")
    return value
