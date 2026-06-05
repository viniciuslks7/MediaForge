# MediaForge

> Distributed, event-driven media processing platform — Go · Python · TypeScript · RabbitMQ · Kubernetes

MediaForge ingests media (images and documents), fans the work out to a fleet of
horizontally-scalable workers over RabbitMQ, processes it (resize / thumbnail /
format-transcode for images, OCR text extraction for documents), and streams
real-time progress back to clients over WebSockets. It ships with
production-grade concerns baked in: dead-letter retries, idempotency, graceful
shutdown, Prometheus metrics, Grafana dashboards, Kubernetes manifests, a Helm
chart with HorizontalPodAutoscalers, and CI.

This is a **polyglot microservices** system on purpose — each service is written
in the language that fits its job:

| Service | Language | Role |
|---|---|---|
| `api-gateway` | **Go** | REST ingestion API, auth, object storage, job orchestration, publishes jobs |
| `worker-image` | **Go** | Consumes image jobs: resize, thumbnails, WebP transcode |
| `worker-ocr` | **Python** | Consumes OCR jobs: Tesseract text extraction + language detection |
| `realtime-gateway` | **TypeScript / Node** | WebSocket fan-out of job events to subscribed clients |

## Architecture

```
                         ┌──────────────────────────────────────────────┐
                         │                  Clients                       │
                         │   HTTP upload ───┐         ▲  WebSocket (live) │
                         └──────────────────┼─────────┼──────────────────┘
                                            │         │
                                   POST /v1/media      │ job events
                                            ▼         │
   ┌───────────────┐   metadata   ┌──────────────────┴───┐
   │  PostgreSQL   │◀────────────▶│      api-gateway      │ (Go)
   └───────────────┘              │  - validate + auth    │
   ┌───────────────┐  rate-limit  │  - store original     │
   │     Redis     │◀────────────▶│  - publish job        │
   └───────────────┘              └──────────┬───────────┘
   ┌───────────────┐  put/get                │ publish
   │  MinIO (S3)   │◀───────────────┐        ▼
   └───────▲───────┘                │   ┌──────────────────────────┐
           │                        │   │        RabbitMQ          │
           │                        │   │  exchange: media.jobs    │
           │ get/put results        │   │   ├─ image.process ──┐   │
           │                        │   │   └─ ocr.extract ───┐│   │
           │                        │   │  exchange: media.events ││ │
           │                        │   │  + DLX media.jobs.dlx  ││ │
           │                        │   └─────────┬────────────┘│ │
           │                        │             │             │ │
           │            ┌───────────┴───┐  ┌──────┴────────┐    │ │
           └────────────┤  worker-image │  │   worker-ocr  ├────┘ │
                        │     (Go)      │  │   (Python)    │      │
                        └───────┬───────┘  └──────┬────────┘      │
                                │ events          │ events        │
                                └────────┬────────┘               │
                                         ▼                        │
                              ┌────────────────────┐  consume     │
                              │  realtime-gateway  │◀─────────────┘
                              │   (TypeScript)     │  media.events
                              └────────────────────┘

   Observability: every service exposes /metrics → Prometheus → Grafana
```

See [`docs/architecture.md`](docs/architecture.md) for the deep dive (message
contracts, retry/DLQ semantics, idempotency, scaling model).

## Quick start (local, Docker Compose)

```bash
cp .env.example .env
docker compose up --build
```

Then:

| What | URL |
|---|---|
| API Gateway | http://localhost:8080 |
| WebSocket gateway | ws://localhost:8090/ws |
| RabbitMQ management | http://localhost:15672 (guest/guest) |
| MinIO console | http://localhost:9001 |
| Prometheus | http://localhost:9090 |
| Grafana | http://localhost:3000 (admin/admin) |
| Jaeger (traces) | http://localhost:16686 |

### Try it end-to-end

```bash
# 1. Submit an image for processing
curl -F "file=@./sample.jpg" -F "operations=resize,thumbnail,webp" \
     http://localhost:8080/v1/media

# => { "job_id": "9c1f...", "status": "pending", "kind": "image" }

# 2. Poll status (or subscribe over WebSocket for live updates)
curl http://localhost:8080/v1/media/9c1f...

# 3. Submit a document for OCR
curl -F "file=@./scan.png" -F "kind=ocr" http://localhost:8080/v1/media
```

Subscribe to live events:

```bash
# any wscat / websocat client
websocat ws://localhost:8090/ws
> {"type":"subscribe","job_id":"9c1f..."}
```

## Kubernetes

```bash
# Plain manifests (kustomize)
kubectl apply -k deploy/k8s/overlays/prod

# Or Helm, with autoscaling
helm install mediaforge deploy/helm/mediaforge \
  --set image.tag=$(git rev-parse --short HEAD)
```

Workers ship with `HorizontalPodAutoscaler` definitions driven by CPU and by
RabbitMQ queue depth (via the Prometheus adapter), so the fleet grows under load
and scales to a floor when idle.

## Repository layout

```
.
├── services/
│   ├── api-gateway/        Go    — ingestion API + orchestration
│   ├── worker-image/       Go    — image processing worker
│   ├── worker-ocr/         Python— OCR worker
│   └── realtime-gateway/   TS    — WebSocket fan-out
├── specs/                  Contract-first specs (single source of truth)
│   ├── schemas/            Canonical JSON Schemas (Job, Event, Artifact)
│   ├── asyncapi.yaml       Message contracts → $ref schemas
│   └── openapi.yaml        REST contracts    → $ref schemas
├── deploy/
│   ├── k8s/                Kustomize base + prod overlay
│   └── helm/mediaforge/    Helm chart (HPA, deployments, services)
├── observability/
│   ├── prometheus/         scrape config + alert rules
│   └── grafana/            dashboards + provisioning
├── docs/                   architecture & message contracts
├── docker-compose.yml      full local stack
└── .github/workflows/      CI: lint, test, build images
```

## Contract-first (spec-driven)

Because the platform is polyglot, the Job/Event/Artifact shapes used to live
duplicated across Go, Python and TypeScript. They now live **once**, as canonical
JSON Schemas under [`specs/schemas`](specs/), referenced by both the AsyncAPI
(messages) and OpenAPI (REST) documents **and** by a contract test in every
service:

| Service | Contract test | Validator |
|---|---|---|
| api-gateway (Go) | `internal/media/contract_schema_test.go` | santhosh-tekuri/jsonschema |
| worker-image (Go) | `internal/broker/contract_schema_test.go` | santhosh-tekuri/jsonschema |
| worker-ocr (Python) | `tests/test_contract.py` | jsonschema |
| realtime-gateway (TS) | `src/contract.test.ts` | ajv |

If any service diverges from the contract, its test fails in CI; the specs
themselves are linted by the `spec-lint` CI job (AsyncAPI CLI + Redocly). See
[`specs/README.md`](specs/README.md).

## Engineering highlights

- **Reliable messaging** — topic exchange with per-kind routing keys, quorum
  queues, publisher confirms, manual acks, a dead-letter exchange and a
  TTL-based retry queue with bounded attempts before parking on the DLQ.
- **Idempotency** — every job carries a UUID; workers no-op on already-completed
  jobs so redeliveries are safe.
- **Graceful shutdown** — all services drain in-flight work on `SIGTERM`
  (Kubernetes-friendly), cancelling contexts and `nack`-requeueing unfinished
  messages.
- **Observability-first** — RED metrics (Rate, Errors, Duration) on every hop,
  queue-depth gauges, a ready-made Grafana dashboard, and Prometheus alert rules.
- **12-factor config** — everything via environment variables, with sane
  defaults and an `.env.example`.

## License

MIT — see [LICENSE](LICENSE).
