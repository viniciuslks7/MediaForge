from mediaforge_ocr.broker import _death_count
from mediaforge_ocr.ocr import summarize


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
