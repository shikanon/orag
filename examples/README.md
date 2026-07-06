# ORAG Examples

This directory is the smoke-test entry point for trying ORAG core capabilities in service mode.
The curl examples exercise the public HTTP API end-to-end with shared state, helpers, and actionable failures.

## Prerequisites

- Go 1.26. Use `GOTOOLCHAIN=go1.26.4` when the local default toolchain is older.
- Docker, when using `scripts/dev-up.sh` to start PostgreSQL and Qdrant.
- `curl` on `PATH` for every service-mode script.
- A running ORAG API service at `BASE_URL`, defaulting to `http://localhost:8080`.
- Default demo credentials `ADMIN_USERNAME=admin` and `ADMIN_PASSWORD=admin`, or equivalent environment variables accepted by the running service.
- Ralph Loop MCP live calls require `ORAG_API_BASE_URL`, `ORAG_API_TOKEN`, and `ORAG_TENANT_ID`.
- Local self-check MCP smoke uses generated `.mcp/tools/*.json` artifacts and can run without a live downstream API.

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

Run the Ralph Loop MCP discovery smoke without a live downstream API:

```sh
head -n 2 examples/mcp/ralph-loop-stdio-smoke.jsonl \
| GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./cmd/orag-mcp --openapi api/openapi.yaml
```

Run the focused self-check MCP stdio smoke:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make mcp-self-check-smoke
```

Validate generated MCP/Skill artifacts are in sync with `api/openapi.yaml`:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check
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

## MCP and Skill Examples

| File | Covered module | What it does | Expected output |
| --- | --- | --- | --- |
| `examples/mcp/README.md` | MCP stdio | Documents local discovery, focused self-check, and live `tools/call` smoke commands. | `initialize` and `tools/list` JSON-RPC responses; optional live `structuredContent`. |
| `examples/mcp/stdio-client-config.json` | MCP client config | Shows a copyable `mcpServers.orag-ralph-loop` config that starts `go run ./cmd/orag-mcp`. | A client can discover `ralph_loop_run` through stdio. |
| `examples/mcp/ralph-loop-stdio-smoke.jsonl` | MCP JSON-RPC | Provides initialize, `tools/list`, and `tools/call` request lines. | Discovery responses without live API; live call response with verdict and trace. |
| `examples/mcp/self-check-stdio-smoke.jsonl` | MCP self-check smoke | Provides initialize, `tools/list`, and focused `orag_check(scope=agent_sync)` request lines. | `structuredContent.verdict`, stable check IDs, evidence, trace, and CI gate warning. |
| `examples/skills/README.md` | Skill overview | Links Codex, Claude Code, Trae, and operational Skill usage examples. | Agent-specific setup path, shared environment, and mutual-exclusion boundaries. |
| `examples/skills/self-check-diagnose-ops.md` | Operational Skills | Shows `orag-self-check`, `orag-self-diagnose`, and `orag-self-ops` prompts and safety boundaries. | Prompt examples stay read-only unless apply is explicitly approved. |
| `examples/skills/codex-ralph-loop.md` | Codex Skill | Shows install, environment, prompt, and expected evidence. | Codex calls `/v1/ralph-loop` and reports verdict evidence. |
| `examples/skills/claude-code-ralph-loop.md` | Claude Code Skill | Shows install, allowed tools, prompt, and evidence expectations. | Claude Code uses `Read` plus `curl` only. |
| `examples/skills/trae-ralph-loop.md` | Trae Skill | Shows workspace install, prompt, and evidence expectations. | Trae discovers `.trae/skills/ralph-loop/SKILL.md`. |

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
- The MCP examples cover stdio initialize, tool discovery, copyable client configuration, a focused `orag_check` smoke, and an optional live `ralph_loop_run` tool call.
- The Skill examples cover Codex, Claude Code, Trae, and the mutually exclusive `orag-self-check`, `orag-self-diagnose`, and `orag-self-ops` boundaries.
- Existing repository scripts cover local dependency startup, readiness polling, and dependency shutdown.
- The contract test validates important script paths and public HTTP endpoints against `api/openapi.yaml` to catch example drift.
