# ORAG Examples

This directory is the smoke-test entry point for trying ORAG core capabilities in service mode.
The curl examples exercise the public HTTP API end-to-end with shared state, helpers, and actionable failures.

## Prerequisites

- Go 1.26. Use `GOTOOLCHAIN=go1.26.4` when the local default toolchain is older.
- Docker, when using `scripts/dev-up.sh` to start PostgreSQL and Qdrant.
- `curl` on `PATH` for every service-mode script.
- A running ORAG API service at `BASE_URL`, defaulting to `http://localhost:8080`.
- Default demo credentials `ADMIN_USERNAME=admin` and `ADMIN_PASSWORD=admin`, or equivalent environment variables accepted by the running service.

## Commands

Start the local dependencies and API service:

```sh
./scripts/dev-up.sh
make migrate
make run
```

Wait until the API reports readiness:

```sh
./scripts/wait-ready.sh
```

Run the service-mode curl examples in order:

```sh
./examples/curl/05_health_ready.sh
./examples/curl/00_login.sh
./examples/curl/10_create_kb.sh
./examples/curl/20_upload_doc.sh
./examples/curl/25_upload_file.sh
./examples/curl/30_query.sh
./examples/curl/35_query_stream.sh
./examples/curl/36_trace_lookup.sh
./examples/curl/40_eval.sh
./examples/curl/45_optimize.sh
```

Stop local dependencies when finished:

```sh
./scripts/dev-down.sh
```

Validate this examples index, script paths, and endpoint drift against `api/openapi.yaml`:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run TestExamples -v
```

## Service/Curl Examples

| Script | Covered module | What it does | Expected output |
| --- | --- | --- | --- |
| `examples/curl/05_health_ready.sh` | Health and readiness | Calls `/healthz` and `/readyz` before stateful examples. | JSON status for process health and dependency readiness. |
| `examples/curl/00_login.sh` | Auth | Logs in and stores a bearer token under `.orag-demo/token`. | JSON containing `access_token`. |
| `examples/curl/10_create_kb.sh` | Knowledge base | Creates a demo knowledge base and stores `.orag-demo/kb_id`. | JSON containing the knowledge base `id`. |
| `examples/curl/20_upload_doc.sh` | Document import | Imports sample content into the current knowledge base through `/v1/knowledge-bases/{id}/documents:import`. | JSON containing `document`, `chunks`, and `job`. |
| `examples/curl/25_upload_file.sh` | Document upload | Uploads a generated Markdown file through multipart `/v1/knowledge-bases/{id}/documents`. | JSON containing `document`, `chunks`, and `job`. |
| `examples/curl/30_query.sh` | Query | Sends a normal RAG query against the current knowledge base and stores `.orag-demo/trace_id`. | JSON containing an answer, citations, and `trace_id`. |
| `examples/curl/35_query_stream.sh` | SSE query | Sends a streaming RAG query to `/v1/query:stream`. | Server-Sent Events such as `trace`, `chunk`, `citations`, and `done`. |
| `examples/curl/36_trace_lookup.sh` | Trace list/detail | Lists recent traces and fetches one trace detail. | JSON trace list followed by a trace record. |
| `examples/curl/40_eval.sh` | Dataset and evaluation | Creates a dataset item and runs an evaluation. | JSON containing evaluation metrics and an evaluation `id`. |
| `examples/curl/45_optimize.sh` | Optimization | Runs profile/top-k optimization for the current dataset and knowledge base. | JSON containing optimization candidates and the selected best result. |

The curl scripts share state and helpers through `examples/curl/lib.sh`. State is stored in `.orag-demo` by default and can be redirected with `STATE_DIR`.
Scripts fail fast with actionable messages when `curl` is missing, the service is not reachable, the bearer token is missing, or required IDs have not been created yet.

## Go Examples

The Go memory example at `examples/go/memory/main.go` demonstrates dependency-free library-style usage through the public `pkg/memory` facade. It creates an in-memory ORAG client, ingests sample content, runs a query, and prints trace/response metadata without PostgreSQL, Qdrant, or Ark.

Run it directly:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/go/memory
```

Or run the example package test:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./examples/go/memory -v
```

Expected output includes `document_id=doc_`, `trace_id=trace_example_memory`, `cache_status=disabled`, trace summary fields, and citation counts.

## Covered Modules

- Service scripts cover health/ready checks, auth, knowledge base creation, JSON document import, multipart document upload, normal query, SSE query, trace list/detail, dataset creation, evaluation, and optimization.
- The Go memory example covers in-process document ingestion, querying, citations, trace lookup, and response metadata through `pkg/memory`.
- Existing repository scripts cover local dependency startup, readiness polling, and dependency shutdown.
- The contract test validates important script paths and public HTTP endpoints against `api/openapi.yaml` to catch example drift.
