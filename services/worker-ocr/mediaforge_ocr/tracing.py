"""OpenTelemetry tracing setup for the OCR worker.

Exports spans over OTLP/HTTP (no grpcio dependency) to the collector. The W3C
trace-context propagator is OpenTelemetry's default, so context extracted from
RabbitMQ headers continues the trace the gateway started.
"""

from __future__ import annotations

import logging

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.sdk.trace.sampling import ParentBased, TraceIdRatioBased

from .config import Config

log = logging.getLogger("mediaforge.ocr.tracing")


def install_log_correlation() -> None:
    """Tag every log record with trace_id/span_id from the active span.

    Uses a log-record factory so the fields always exist (empty when there is no
    active span), making logs and traces cross-navigable without risking a
    KeyError in the format string. Safe to call when tracing export is disabled.
    """
    old_factory = logging.getLogRecordFactory()

    def factory(*args, **kwargs):  # noqa: ANN002, ANN003
        record = old_factory(*args, **kwargs)
        sc = trace.get_current_span().get_span_context()
        if sc.is_valid:
            record.trace_id = format(sc.trace_id, "032x")
            record.span_id = format(sc.span_id, "016x")
        else:
            record.trace_id = ""
            record.span_id = ""
        return record

    logging.setLogRecordFactory(factory)


def init_tracing(cfg: Config) -> TracerProvider | None:
    """Install a global tracer provider exporting over OTLP/HTTP.

    Returns the provider (call ``.shutdown()`` on exit) or ``None`` when tracing
    is disabled.
    """
    if not cfg.otel_enabled or not cfg.otel_endpoint:
        return None

    endpoint = cfg.otel_endpoint.rstrip("/")
    if not endpoint.endswith("/v1/traces"):
        endpoint = f"{endpoint}/v1/traces"

    resource = Resource.create(
        {
            "service.name": cfg.service_name,
            "service.version": cfg.service_version,
        }
    )
    provider = TracerProvider(
        resource=resource,
        sampler=ParentBased(TraceIdRatioBased(cfg.otel_sample_ratio or 1.0)),
    )
    provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint)))
    trace.set_tracer_provider(provider)
    log.info("tracing enabled, exporting to %s", endpoint)
    return provider
