"""OCR extraction and a pure text-summarisation helper.

The Tesseract call is isolated in :func:`extract_text` so the surrounding logic
(:func:`summarize`, :func:`is_pdf`) stays pure and unit-testable without the
binary installed. PDFs are rasterised page-by-page with poppler (pdf2image)
before going through Tesseract.
"""

from __future__ import annotations

import io
import re
from dataclasses import dataclass, field

import pytesseract
from pdf2image import convert_from_bytes
from PIL import Image

_WORD_RE = re.compile(r"\b[\w'-]+\b", re.UNICODE)

# Rasterising a PDF costs ~1s/page; cap the work a single upload can demand.
MAX_PDF_PAGES = 10
PDF_RENDER_DPI = 200


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


def is_pdf(data: bytes) -> bool:
    """Detect a PDF payload by its magic bytes (pure, unit-testable)."""
    return data.lstrip()[:5] == b"%PDF-"


def extract_text(
    source_bytes: bytes, languages: str, *, tesseract_cmd: str = "tesseract"
) -> OCRResult:
    """Run Tesseract OCR over an image or PDF payload."""
    if tesseract_cmd:
        pytesseract.pytesseract.tesseract_cmd = tesseract_cmd

    if is_pdf(source_bytes):
        return _extract_pdf(source_bytes, languages)

    image = Image.open(io.BytesIO(source_bytes))
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


def _extract_pdf(pdf_bytes: bytes, languages: str) -> OCRResult:
    """Rasterise up to MAX_PDF_PAGES pages and OCR each one."""
    pages = convert_from_bytes(
        pdf_bytes, dpi=PDF_RENDER_DPI, first_page=1, last_page=MAX_PDF_PAGES
    )

    texts: list[str] = []
    confidences: list[float] = []
    for page in pages:
        texts.append(pytesseract.image_to_string(page, lang=languages))
        confidences.append(_mean_confidence(page, languages))

    text = "\n\f\n".join(texts)  # form feed between pages, like tesseract's own PDF mode
    word_count, char_count = summarize(text)
    mean_conf = round(sum(confidences) / len(confidences), 2) if confidences else 0.0

    return OCRResult(
        text=text,
        word_count=word_count,
        char_count=char_count,
        mean_confidence=mean_conf,
        languages=languages,
        metadata={"page_count": len(pages), "source": "pdf"},
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
