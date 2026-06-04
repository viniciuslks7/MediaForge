#!/usr/bin/env python3
"""Generate a small sample PNG for smoke testing (no external deps)."""
import struct
import sys
import zlib


def make_png(path: str, width: int = 640, height: int = 480) -> None:
    raw = bytearray()
    for y in range(height):
        raw.append(0)  # filter type 0 for each scanline
        for x in range(width):
            raw += bytes((x % 256, y % 256, (x + y) % 256))

    def chunk(tag: bytes, data: bytes) -> bytes:
        body = tag + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body) & 0xFFFFFFFF)

    ihdr = struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)  # 8-bit RGB
    png = b"\x89PNG\r\n\x1a\n"
    png += chunk(b"IHDR", ihdr)
    png += chunk(b"IDAT", zlib.compress(bytes(raw), 9))
    png += chunk(b"IEND", b"")

    with open(path, "wb") as f:
        f.write(png)
    print(f"wrote {path} ({width}x{height})")


if __name__ == "__main__":
    out = sys.argv[1] if len(sys.argv) > 1 else "sample.png"
    make_png(out)
