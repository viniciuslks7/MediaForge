"""MediaForge OCR worker.

Consumes ``ocr.extract`` jobs from RabbitMQ, downloads the source document from
object storage, runs Tesseract OCR, stores the extracted text as an artifact and
emits lifecycle events — mirroring the Go image worker's contract and retry
semantics so the two are interchangeable from the platform's point of view.
"""

__version__ = "0.1.0"
