# Ralph Loop MCP and Skill Integration

Ralph Loop is exposed to agent clients through two derived integration surfaces:

- MCP stdio server: `cmd/orag-mcp` exposes the generated `ralph_loop_run` tool.
- Agent Skills: `.codex/skills/ralph-loop`, `.claude/skills/ralph-loop`, and `.trae/skills/ralph-loop` describe the same API contract for tool-capable coding agents.

Both surfaces are generated from `api/openapi.yaml` and its `x-orag-agent-capabilities` extension. Do not hand-edit generated MCP or Skill artifacts; update OpenAPI first and run the sync command.

## Contract Source

| Artifact | Path | Source |
| --- | --- | --- |
| MCP tool schema | `.mcp/tools/ralph-loop.json` | `api/openapi.yaml` |
| Codex Skill | `.codex/skills/ralph-loop/SKILL.md` | `api/openapi.yaml` |
| Claude Code Skill | `.claude/skills/ralph-loop/SKILL.md` | `api/openapi.yaml` |
| Trae Skill | `.trae/skills/ralph-loop/SKILL.md` | `api/openapi.yaml` |

Regenerate artifacts after changing the capability manifest:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync
```

Check for drift without writing files:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check
```

## Start MCP Server

The MCP server uses stdio and reads JSON-RPC requests from stdin. Run it from the repository root so the default OpenAPI path resolves correctly:

```sh
ORAG_API_BASE_URL=http://localhost:8080 \
ORAG_API_TOKEN=replace-with-token \
ORAG_TENANT_ID=tenant_default \
ORAG_MCP_TIMEOUT=30s \
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson \
go run ./cmd/orag-mcp --openapi api/openapi.yaml
```

Runtime environment:

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `ORAG_API_BASE_URL` | No | `http://localhost:8080` | Base URL for the ORAG API that implements `POST /v1/ralph-loop`. |
| `ORAG_API_TOKEN` | Yes | None | Bearer token sent to the downstream API. |
| `ORAG_TENANT_ID` | Yes | None | Tenant header sent as `X-ORAG-Tenant-ID`. |
| `ORAG_MCP_TIMEOUT` | No | `30s` | Downstream HTTP timeout parsed with Go duration syntax. |

To discover tools without calling the downstream API:

```sh
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
| GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./cmd/orag-mcp --openapi api/openapi.yaml
```

The `tools/list` response includes `ralph_loop_run`, its input schema, output schema, auth annotations, and example request metadata.

## MCP Client Configuration

Use `examples/mcp/stdio-client-config.json` as a copyable MCP client configuration. It starts the local server with `go run ./cmd/orag-mcp` and passes the ORAG API settings through environment variables.

Use `examples/mcp/ralph-loop-stdio-smoke.jsonl` as a minimal JSON-RPC transcript for manual stdio smoke checks. The `initialize` and `tools/list` lines can run without a live API; the `tools/call` line requires a running ORAG API and valid token.

## Skill Installation

Generated Skill directories are ready to copy into agent-specific skill locations:

| Client | Generated source | Example guide |
| --- | --- | --- |
| Codex | `.codex/skills/ralph-loop/SKILL.md` | `examples/skills/codex-ralph-loop.md` |
| Claude Code | `.claude/skills/ralph-loop/SKILL.md` | `examples/skills/claude-code-ralph-loop.md` |
| Trae | `.trae/skills/ralph-loop/SKILL.md` | `examples/skills/trae-ralph-loop.md` |

Each Skill expects these runtime values to be available to the agent before it calls the API:

```sh
export ORAG_API_BASE_URL=http://localhost:8080
export ORAG_API_TOKEN=replace-with-token
export ORAG_TENANT_ID=tenant_default
```

The Skill is an API client description only. It does not implement the Ralph Loop runtime handler and should stop when `/v1/ralph-loop` returns an error.

## Common Errors

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `missing required ORAG MCP configuration` | `ORAG_API_TOKEN` or `ORAG_TENANT_ID` is empty. | Export both variables before `tools/call`. |
| `ORAG_API_BASE_URL is invalid` | Base URL lacks scheme or host. | Use a full URL such as `http://localhost:8080`. |
| `downstream_auth_error` or `invalid_bearer_token` | Token is missing, expired, or from another tenant. | Login again and pass the fresh token as `ORAG_API_TOKEN`. |
| `downstream_rate_limited` | ORAG API returned HTTP 429. | Retry after the server-provided backoff window. |
| `downstream_timeout` | API call exceeded `ORAG_MCP_TIMEOUT`. | Increase `ORAG_MCP_TIMEOUT` or inspect the API trace ID. |
| `invalid_tool_arguments` | `mode`, `max_rounds`, or required fields do not match the schema. | Re-check `.mcp/tools/ralph-loop.json` or the `tools/list` response. |
| `agent artifacts are out of sync` | Generated MCP/Skill outputs differ from OpenAPI. | Run `make agent-sync`, review changes, then run `make agent-sync-check`. |

## Local Verification

Run the Task 5 verification set from the repository root:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./internal/mcp ./internal/agentskills ./internal/agentsync ./cmd/oragctl -v
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run 'TestOpenAPI|TestExamples' -v
```

Optional live MCP call verification requires a running ORAG API that implements `POST /v1/ralph-loop`:

```sh
ORAG_API_BASE_URL=http://localhost:8080 \
ORAG_API_TOKEN=replace-with-token \
ORAG_TENANT_ID=tenant_default \
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson \
go run ./cmd/orag-mcp --openapi api/openapi.yaml < examples/mcp/ralph-loop-stdio-smoke.jsonl
```

For live failures, report the MCP JSON-RPC error code, downstream `trace_id`, command, and artifact paths instead of printing tokens or full request headers.
