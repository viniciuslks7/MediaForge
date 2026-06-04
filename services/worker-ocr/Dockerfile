# OCR worker — needs the Tesseract binary + language data at runtime.
FROM python:3.12-slim AS base

# System deps: tesseract OCR engine with English + Portuguese language packs.
RUN apt-get update && apt-get install -y --no-install-recommends \
        tesseract-ocr \
        tesseract-ocr-eng \
        tesseract-ocr-por \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install Python deps first for layer caching.
COPY pyproject.toml ./
RUN pip install --no-cache-dir .

COPY mediaforge_ocr ./mediaforge_ocr

ENV OCR_TESSERACT_CMD=tesseract
EXPOSE 9103

# Run as non-root.
RUN useradd --create-home --uid 10001 appuser
USER appuser

ENTRYPOINT ["python", "-m", "mediaforge_ocr.worker"]
