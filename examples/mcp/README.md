# Ralph Loop MCP Examples

This directory contains copyable stdio examples for connecting an MCP client to the local ORAG Ralph Loop server.

## Files

| File | Purpose |
| --- | --- |
| `stdio-client-config.json` | Example MCP client configuration that starts `go run ./cmd/orag-mcp`. |
| `ralph-loop-stdio-smoke.jsonl` | JSON-RPC transcript for initialize, tool discovery, and one `ralph_loop_run` call. |

## Discovery Smoke

Run the discovery-only portion without a live ORAG API:

```sh
head -n 2 examples/mcp/ralph-loop-stdio-smoke.jsonl \
| GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./cmd/orag-mcp --openapi api/openapi.yaml
```

Expected output:

- `initialize` returns protocol version `2024-11-05`.
- `tools/list` returns one tool named `ralph_loop_run`.

## Live Tool Call

The third JSON-RPC line calls the downstream ORAG API, so it requires a running API and valid auth:

```sh
ORAG_API_BASE_URL=http://localhost:8080 \
ORAG_API_TOKEN=replace-with-token \
ORAG_TENANT_ID=tenant_default \
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson \
go run ./cmd/orag-mcp --openapi api/openapi.yaml < examples/mcp/ralph-loop-stdio-smoke.jsonl
```

Expected live output includes a `tools/call` result with `structuredContent.status`, `structuredContent.verdict`, and `_meta.trace_id`.
