"""RabbitMQ adapter for the OCR worker.

Declares the same (idempotent) topology as the Go services, consumes ``q.ocr``
with bounded retries through the dead-letter + TTL retry loop, and publishes
lifecycle events on ``media.events``.
"""

from __future__ import annotations

import json
import logging
from collections.abc import Callable
from dataclasses import dataclass, field

import pika
from opentelemetry import trace
from opentelemetry.propagate import extract, inject
from opentelemetry.trace import SpanKind
from pika.adapters.blocking_connection import BlockingChannel
from pika.spec import Basic, BasicProperties

log = logging.getLogger("mediaforge.ocr.broker")
tracer = trace.get_tracer("mediaforge.ocr.broker")

EXCHANGE_JOBS = "media.jobs"
EXCHANGE_EVENTS = "media.events"
EXCHANGE_RETRY = "media.jobs.dlx"
EXCHANGE_PARKING = "media.jobs.parking"

QUEUE_IMAGE = "q.image"
QUEUE_OCR = "q.ocr"
QUEUE_RETRY = "q.retry"
QUEUE_PARKING = "q.parking"


@dataclass
class Job:
    """Wire contract mirroring the Go gateway's media.Job."""

    job_id: str
    kind: str
    source_key: str
    source_mime: str = ""
    size_bytes: int = 0
    operations: list[str] = field(default_factory=list)

    @staticmethod
    def from_bytes(body: bytes) -> Job:
        d = json.loads(body)
        return Job(
            job_id=d["job_id"],
            kind=d.get("kind", "ocr"),
            source_key=d["source_key"],
            source_mime=d.get("source_mime", ""),
            size_bytes=d.get("size_bytes", 0),
            operations=d.get("operations") or [],
        )


class PermanentError(Exception):
    """A failure no retry can fix (e.g. an unreadable payload).

    Mirrors the Go broker's PermanentError: the consumer parks the message on
    the first attempt instead of cycling it through the retry loop.
    """


# Handler raises to signal failure -> retry/park (PermanentError parks at once).
Handler = Callable[[Job, int], None]


class Broker:
    def __init__(self, url: str, prefetch: int, max_retries: int, retry_ttl_ms: int):
        self._params = pika.URLParameters(url)
        self._prefetch = prefetch
        self._max_retries = max_retries
        self._retry_ttl_ms = retry_ttl_ms
        self._conn = pika.BlockingConnection(self._params)
        self._ch = self._conn.channel()
        self._declare_topology()
        self._ch.basic_qos(prefetch_count=prefetch)

    def _declare_topology(self) -> None:
        for ex in (EXCHANGE_JOBS, EXCHANGE_EVENTS, EXCHANGE_RETRY, EXCHANGE_PARKING):
            self._ch.exchange_declare(ex, exchange_type="topic", durable=True)

        work_args = {"x-queue-type": "quorum", "x-dead-letter-exchange": EXCHANGE_RETRY}
        for queue, key in ((QUEUE_IMAGE, "image.process"), (QUEUE_OCR, "ocr.extract")):
            self._ch.queue_declare(queue, durable=True, arguments=work_args)
            self._ch.queue_bind(queue, EXCHANGE_JOBS, routing_key=key)

        retry_args = {"x-dead-letter-exchange": EXCHANGE_JOBS, "x-message-ttl": self._retry_ttl_ms}
        self._ch.queue_declare(QUEUE_RETRY, durable=True, arguments=retry_args)
        self._ch.queue_bind(QUEUE_RETRY, EXCHANGE_RETRY, routing_key="#")

        self._ch.queue_declare(QUEUE_PARKING, durable=True)
        self._ch.queue_bind(QUEUE_PARKING, EXCHANGE_PARKING, routing_key="#")

    def consume(self, queue: str, handler: Handler) -> None:
        """Block consuming ``queue`` until the connection is closed."""

        def _on_message(
            ch: BlockingChannel,
            method: Basic.Deliver,
            props: BasicProperties,
            body: bytes,
        ) -> None:
            attempt = _death_count(props.headers) + 1

            # Continue the distributed trace: extract the W3C context the gateway
            # injected into the message headers and open a consumer span.
            ctx = extract(props.headers or {})
            with tracer.start_as_current_span(
                f"consume {method.routing_key}",
                context=ctx,
                kind=SpanKind.CONSUMER,
                attributes={
                    "messaging.system": "rabbitmq",
                    "messaging.rabbitmq.destination.routing_key": method.routing_key or "",
                    "messaging.rabbitmq.delivery.attempt": attempt,
                },
            ) as span:
                try:
                    job = Job.from_bytes(body)
                except (ValueError, KeyError) as exc:
                    span.record_exception(exc)
                    self._park(method.routing_key, body, f"unmarshal: {exc}")
                    ch.basic_ack(method.delivery_tag)
                    return

                span.set_attribute("mediaforge.job.id", job.job_id)
                try:
                    handler(job, attempt)
                    ch.basic_ack(method.delivery_tag)
                except Exception as exc:  # noqa: BLE001 — broker decides retry vs park
                    span.record_exception(exc)
                    if isinstance(exc, PermanentError) or attempt >= self._max_retries:
                        self._park(method.routing_key, body, str(exc))
                        ch.basic_ack(method.delivery_tag)
                    else:
                        ch.basic_nack(method.delivery_tag, requeue=False)

        self._ch.basic_consume(queue, _on_message, auto_ack=False)
        self._ch.start_consuming()

    def _park(self, routing_key: str, body: bytes, reason: str) -> None:
        self._ch.basic_publish(
            EXCHANGE_PARKING,
            routing_key,
            body,
            properties=BasicProperties(
                content_type="application/json",
                delivery_mode=2,
                headers={"x-parking-reason": reason},
            ),
        )
        log.warning("parked message: %s", reason)

    def publish_event(self, event: dict) -> None:
        # Inject the current trace context so the realtime-gateway's fan-out
        # joins the same trace.
        headers: dict = {}
        inject(headers)
        self._ch.basic_publish(
            EXCHANGE_EVENTS,
            f"event.{event.get('kind', 'ocr')}",
            json.dumps(event),
            properties=BasicProperties(
                content_type="application/json", delivery_mode=1, headers=headers
            ),
        )

    def close(self) -> None:
        try:
            self._ch.stop_consuming()
        finally:
            self._conn.close()


def _death_count(headers: dict | None) -> int:
    if not headers:
        return 0
    deaths = headers.get("x-death")
    if not deaths:
        return 0
    first = deaths[0]
    return int(first.get("count", 0)) if isinstance(first, dict) else 0
