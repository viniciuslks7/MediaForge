#!/usr/bin/env bash
# End-to-end smoke test: submit a job and poll until it completes.
# Requires a running stack (make up) and: curl, jq, python3 (to make a sample).
set -euo pipefail

API="${API:-http://localhost:8080}"
SAMPLE="${SAMPLE:-/tmp/mediaforge-sample.png}"

if [[ ! -f "$SAMPLE" ]]; then
  echo "==> generating sample image at $SAMPLE"
  python3 "$(dirname "$0")/make_sample.py" "$SAMPLE"
fi

echo "==> submitting $SAMPLE"
resp=$(curl -fsS -F "file=@${SAMPLE}" -F "operations=resize,thumbnail,webp" "${API}/v1/media")
echo "$resp" | jq .
job_id=$(echo "$resp" | jq -r .job_id)

echo "==> polling job ${job_id}"
for i in $(seq 1 30); do
  status=$(curl -fsS "${API}/v1/media/${job_id}" | jq -r .job.status)
  echo "    [$i] status=${status}"
  if [[ "$status" == "completed" ]]; then
    echo "==> done. artifacts:"
    curl -fsS "${API}/v1/media/${job_id}" | jq '.artifacts[] | {name, content_type, size_bytes, url}'
    exit 0
  fi
  if [[ "$status" == "failed" ]]; then
    echo "!! job failed"
    exit 1
  fi
  sleep 1
done

echo "!! timed out waiting for completion"
exit 1
