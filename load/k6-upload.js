// k6 load test for the MediaForge ingestion path.
//
// Ramps virtual users posting image jobs at the gateway to exercise throughput,
// publisher confirms, and — when run against the k8s deployment — the
// worker-image HorizontalPodAutoscaler that scales on RabbitMQ queue depth.
//
// Run:
//   k6 run load/k6-upload.js
//   BASE_URL=http://localhost:8080 API_TOKEN=dev-local-token k6 run load/k6-upload.js
//
// Watch while it runs: Grafana (queue depth + worker replicas) and Jaeger
// (per-request traces). The thresholds below fail the run if the gateway can't
// keep error rate and p95 latency within budget.

import http from 'k6/http';
import { check, sleep } from 'k6';
import encoding from 'k6/encoding';
import { Rate } from 'k6/metrics';

const accepted = new Rate('submit_accepted');

const BASE = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.API_TOKEN || 'dev-local-token';

// A minimal valid 1x1 PNG, decoded once per VU init. Keeps the test self
// contained — no binary fixture committed to the repo.
const PNG = encoding.b64decode(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
);

export const options = {
  // Open model ramp: warm up, sustain, drain. Tune targets to your hardware.
  stages: [
    { duration: '30s', target: 20 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'], // <1% of submissions may fail
    http_req_duration: ['p(95)<800'], // 95th percentile under 800ms
    submit_accepted: ['rate>0.99'], // ~all return 202
  },
};

export default function () {
  const res = http.post(
    `${BASE}/v1/media`,
    {
      file: http.file(PNG, 'load.png', 'image/png'),
      kind: 'image',
      operations: 'resize,thumbnail,webp',
    },
    { headers: { Authorization: `Bearer ${TOKEN}` }, tags: { name: 'submit' } },
  );

  const ok = res.status === 202;
  accepted.add(ok);
  check(res, {
    'status is 202': () => ok,
    'has job_id': (r) => {
      try {
        return Boolean(r.json('job_id'));
      } catch {
        return false;
      }
    },
  });

  sleep(1);
}
