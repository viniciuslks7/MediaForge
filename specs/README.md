# MediaForge — API & Message Contracts (spec-first)

This directory is the **single source of truth** for the contracts that bind the
four services together. Because MediaForge is polyglot (Go, Python, TypeScript),
the same Job/Event/Artifact shapes used to live duplicated in each codebase and
were kept in sync by hand — a classic source of silent production drift.

Now the shapes live **once**, as canonical JSON Schemas, and everything else
references them:

```
specs/
├── schemas/
│   ├── job.schema.json        ← canonical Job   (media.jobs)
│   ├── event.schema.json      ← canonical Event (media.events)
│   └── artifact.schema.json   ← canonical Artifact (REST)
├── asyncapi.yaml              ← message contracts ($ref → schemas/)
└── openapi.yaml               ← REST contracts   ($ref → schemas/)
```

## How drift is prevented

Each service ships a **contract test** that validates its own
serialized/deserialized payloads against these schemas. If anyone changes a
field in one service in a way that diverges from the contract, that service's
contract test fails in CI:

| Service | Test | Validator |
|---|---|---|
| api-gateway (Go) | `internal/media/contract_schema_test.go` | santhosh-tekuri/jsonschema |
| worker-image (Go) | `internal/broker/contract_schema_test.go` | santhosh-tekuri/jsonschema |
| worker-ocr (Python) | `tests/test_contract.py` | jsonschema |
| realtime-gateway (TS) | `src/contract.test.ts` | ajv |

The specs themselves are also linted in CI (`spec-lint` job): AsyncAPI via the
AsyncAPI CLI and OpenAPI via Redocly.

## Working on a contract

1. Edit the relevant `schemas/*.json` (the source of truth).
2. Run the contract tests — they tell you which services no longer comply.
3. Update those services to match.
4. The AsyncAPI/OpenAPI docs update automatically (they `$ref` the schemas).

## Viewing the docs

```bash
# AsyncAPI HTML
npx @asyncapi/cli generate fromTemplate specs/asyncapi.yaml @asyncapi/html-template -o build/asyncapi

# OpenAPI preview
npx @redocly/cli preview-docs specs/openapi.yaml
```
