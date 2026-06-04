"""Contract tests: validate the OCR worker's payloads against the canonical
JSON Schemas in specs/schemas (the cross-language single source of truth)."""

from __future__ import annotations

import json
import time
from pathlib import Path

import pytest
from jsonschema import Draft202012Validator

from mediaforge_ocr.broker import Job

SCHEMA_DIR = Path(__file__).resolve().parents[3] / "specs" / "schemas"


def _validator(name: str) -> Draft202012Validator:
    schema = json.loads((SCHEMA_DIR / name).read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    return Draft202012Validator(schema)


def test_job_sample_conforms_and_decodes():
    """A canonical Job payload must satisfy the contract and decode cleanly."""
    wire = {
        "job_id": "9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b",
        "kind": "ocr",
        "status": "pending",
        "source_key": "uploads/9c1f/scan.png",
        "source_mime": "image/png",
        "size_bytes": 2048,
        "operations": [],
    }
    _validator("job.schema.json").validate(wire)

    job = Job.from_bytes(json.dumps(wire).encode())
    assert job.job_id == wire["job_id"]
    assert job.kind == "ocr"
    assert job.source_key == wire["source_key"]


def test_emitted_event_conforms():
    """The event dict the worker publishes must satisfy the Event contract."""
    event = {
        "job_id": "9c1f0c4a-2b3d-4e5f-8a9b-0c1d2e3f4a5b",
        "kind": "ocr",
        "status": "completed",
        "message": "42 words",
        "progress": 100,
        "artifact": "text",
        "ts": int(time.time() * 1000),
    }
    _validator("event.schema.json").validate(event)


def test_contract_actually_constrains():
    """A bad kind must be rejected — proves the schema is enforced."""
    from jsonschema import ValidationError

    bad = {
        "job_id": "x",
        "kind": "video",
        "status": "pending",
        "source_key": "uploads/x",
    }
    with pytest.raises(ValidationError):
        _validator("job.schema.json").validate(bad)
