"""S3-compatible object storage access (MinIO)."""

from __future__ import annotations

import io

from minio import Minio


class ObjectStore:
    def __init__(self, endpoint: str, access_key: str, secret_key: str, bucket: str, secure: bool):
        self._client = Minio(endpoint, access_key=access_key, secret_key=secret_key, secure=secure)
        self._bucket = bucket

    def get(self, key: str) -> bytes:
        response = self._client.get_object(self._bucket, key)
        try:
            return response.read()
        finally:
            response.close()
            response.release_conn()

    def put(self, key: str, content_type: str, data: bytes) -> int:
        result = self._client.put_object(
            self._bucket,
            key,
            io.BytesIO(data),
            length=len(data),
            content_type=content_type,
        )
        _ = result
        return len(data)
