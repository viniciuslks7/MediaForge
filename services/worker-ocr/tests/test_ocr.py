import pytest

from mediaforge_ocr.broker import Job, PermanentError, _death_count
from mediaforge_ocr.config import Config
from mediaforge_ocr.ocr import is_pdf, summarize
from mediaforge_ocr.worker import Worker


def test_summarize_counts_words_and_chars():
    text = "Hello, world! OCR is fun."
    words, chars = summarize(text)
    assert words == 5  # Hello world OCR is fun
    assert chars == len(text)


def test_summarize_empty():
    assert summarize("") == (0, 0)


def test_summarize_handles_unicode():
    words, _ = summarize("olá mundo こんにちは")
    assert words == 3


def test_death_count_none():
    assert _death_count(None) == 0
    assert _death_count({}) == 0


def test_death_count_reads_first_entry():
    headers = {"x-death": [{"count": 3, "queue": "q.retry"}]}
    assert _death_count(headers) == 3


def test_death_count_malformed():
    assert _death_count({"x-death": ["not-a-dict"]}) == 0


def test_is_pdf_magic():
    assert is_pdf(b"%PDF-1.7 rest of file")
    assert is_pdf(b"  \n%PDF-1.4")  # leading whitespace tolerated
    assert not is_pdf(b"\x89PNG\r\n\x1a\n")
    assert not is_pdf(b"")


# ---- failure path: a failing job must never stay "processing" ----


class _FakeDB:
    def __init__(self):
        self.statuses: list[tuple[str, str, str]] = []

    def status(self, _job_id):
        return "pending"

    def set_status(self, job_id, status, error=""):
        self.statuses.append((job_id, status, error))


class _FakeStore:
    def get(self, _key):
        return b"not an image at all"


class _FakeBroker:
    def __init__(self):
        self.events: list[dict] = []

    def publish_event(self, event):
        self.events.append(event)


def _worker():
    cfg = Config(
        rabbit_url="", prefetch=1, max_retries=5, retry_ttl_ms=0,
        postgres_dsn="", s3_endpoint="", s3_access_key="", s3_secret_key="",
        s3_bucket="", s3_use_ssl=False, metrics_port=0, languages="eng",
        tesseract_cmd="", otel_enabled=False, otel_endpoint="",
        otel_sample_ratio=1.0, service_name="test", service_version="test",
    )
    db, store, broker = _FakeDB(), _FakeStore(), _FakeBroker()
    return Worker(cfg, db, store, broker), db, broker


def test_handle_marks_job_failed_and_emits_event():
    worker, db, broker = _worker()
    job = Job(job_id="j1", kind="ocr", source_key="uploads/j1/junk.bin")

    # Undecodable payload: extraction fails deterministically -> PermanentError.
    with pytest.raises(PermanentError):
        worker.handle(job, attempt=1)

    assert ("j1", "failed", db.statuses[-1][2]) == db.statuses[-1]
    assert db.statuses[-1][2] != ""  # the cause is recorded on the job row
    failed = [e for e in broker.events if e["status"] == "failed"]
    assert failed, "a failed event must reach the realtime stream"
