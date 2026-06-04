"""OCR extraction and a pure text-summarisation helper.

The Tesseract call is isolated in :func:`extract_text` so the surrounding logic
(:func:`summarize`) stays pure and unit-testable without the binary installed.
"""

from __future__ import annotations

import io
import re
from dataclasses import dataclass, field

import pytesseract
from PIL import Image

_WORD_RE = re.compile(r"\b[\w'-]+\b", re.UNICODE)


@dataclass
class OCRResult:
    text: str
    word_count: int
    char_count: int
    mean_confidence: float
    languages: str
    metadata: dict = field(default_factory=dict)


def summarize(text: str) -> tuple[int, int]:
    """Return ``(word_count, char_count)`` for the extracted text.

    Pure function — the unit tests exercise this directly.
    """
    words = _WORD_RE.findall(text)
    return len(words), len(text)


def extract_text(
    image_bytes: bytes, languages: str, *, tesseract_cmd: str = "tesseract"
) -> OCRResult:
    """Run Tesseract OCR over ``image_bytes`` and return a structured result."""
    if tesseract_cmd:
        pytesseract.pytesseract.tesseract_cmd = tesseract_cmd

    image = Image.open(io.BytesIO(image_bytes))
    image.load()

    text = pytesseract.image_to_string(image, lang=languages)
    confidence = _mean_confidence(image, languages)
    word_count, char_count = summarize(text)

    return OCRResult(
        text=text,
        word_count=word_count,
        char_count=char_count,
        mean_confidence=confidence,
        languages=languages,
        metadata={
            "image_width": image.width,
            "image_height": image.height,
            "image_mode": image.mode,
        },
    )


def _mean_confidence(image: Image.Image, languages: str) -> float:
    """Average per-word confidence reported by Tesseract (0..100)."""
    try:
        data = pytesseract.image_to_data(
            image, lang=languages, output_type=pytesseract.Output.DICT
        )
    except Exception:  # noqa: BLE001 — confidence is best-effort
        return 0.0
    confidences = [
        int(c) for c in data.get("conf", []) if str(c).lstrip("-").isdigit() and int(c) >= 0
    ]
    if not confidences:
        return 0.0
    return round(sum(confidences) / len(confidences), 2)
