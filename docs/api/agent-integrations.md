# Agent Integration Guide

ORAG exposes Agent-facing capabilities through a manifest-first pipeline. The capability manifest is the single source of truth for behavior semantics, safety boundaries, risk level, operations metadata, MCP annotations, Skill triggers, and generation metadata. OpenAPI remains the HTTP facet only; do not infer Skill behavior, call ordering, or operational safety rules from OpenAPI alone.

## Generated Surfaces

| Artifact | Path | Source | Purpose |
| --- | --- | --- | --- |
| OpenAPI contract | `api/openapi.yaml` | Capability manifest HTTP facet plus HTTP handlers | Public HTTP schema and contract validation. |
| OpenAPI facet snapshot | `.mcp/openapi-facet.json` | Capability manifest | Static drift evidence for Agent-facing HTTP facets. |
| MCP tool schemas | `.mcp/tools/*.json` | Capability manifest | Stdio tool discovery for Ralph Loop, self-check, diagnosis, and self-ops. |
| Codex Skills | `.codex/skills/*/SKILL.md` | Capability manifest | Agent-specific Skill instructions and safety boundaries. |
| Claude Code Skills | `.claude/skills/*/SKILL.md` | Capability manifest | Agent-specific Skill instructions and allowed behavior. |
| Trae Skills | `.trae/skills/*/SKILL.md` | Capability manifest | Workspace Skill instructions and trigger boundaries. |

Generated outputs include `ralph-loop`, `orag-self-check`, `orag-self-diagnose`, and `orag-self-ops`. Do not hand-edit generated MCP or Skill artifacts; update the manifest-backed generator and regenerate artifacts instead.

## Regeneration and Drift Gates

Regenerate artifacts after changing a capability, template, OpenAPI facet, or Skill behavior boundary:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync
```

Check for drift without writing files:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check
```

`make agent-sync-check` is the authoritative CI/static drift gate. Runtime MCP probes such as `orag_check(scope=agent_sync, mode=focused)` are convenience checks for Agents and humans; a passing runtime probe does not replace the static gate in CI.

## Start MCP Server

The MCP server uses stdio and reads JSON-RPC requests from stdin. Run it from the repository root so the OpenAPI contract and generated `.mcp/tools/*.json` artifacts resolve correctly:

```sh
ORAG_API_BASE_URL=http://localhost:8080 ORAG_API_TOKEN=replace-with-token ORAG_TENANT_ID=tenant_default ORAG_MCP_TIMEOUT=30s GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./cmd/orag-mcp --openapi api/openapi.yaml
```

Runtime environment:

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `ORAG_API_BASE_URL` | No | `http://localhost:8080` | Base URL for HTTP-backed Agent capabilities such as `ralph_loop_run`. |
| `ORAG_API_TOKEN` | Required for HTTP-backed tools | None | Bearer token sent to downstream ORAG APIs. |
| `ORAG_TENANT_ID` | Required for HTTP-backed tools | None | Tenant header sent as `X-ORAG-Tenant-ID`. |
| `ORAG_MCP_TIMEOUT` | No | `30s` | Downstream HTTP timeout parsed with Go duration syntax. |

Local self-check, diagnosis, and dry-run self-ops tools are handled inside `cmd/orag-mcp` when their generated MCP artifacts are present. HTTP-backed tools still require valid API configuration.

## MCP Discovery and Smoke

Discover generated tools without calling the downstream API:

```sh
printf '%s
'   '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'   '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./cmd/orag-mcp --openapi api/openapi.yaml
```

Run the focused self-check stdio smoke:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make mcp-self-check-smoke
```

The self-check smoke reads `examples/mcp/self-check-stdio-smoke.jsonl`, calls `orag_check(scope=agent_sync, mode=focused)`, and expects `structuredContent.verdict`, stable check IDs, evidence, `trace_id`, and `runtime_gate_warning`.

Optional Ralph Loop live verification requires a running ORAG API that implements `POST /v1/ralph-loop`:

```sh
ORAG_API_BASE_URL=http://localhost:8080 ORAG_API_TOKEN=replace-with-token ORAG_TENANT_ID=tenant_default GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./cmd/orag-mcp --openapi api/openapi.yaml < examples/mcp/ralph-loop-stdio-smoke.jsonl
```

## Operational MCP Tools

| Tool | Skill | Side effect | Boundary |
| --- | --- | --- | --- |
| `orag_check` | `orag-self-check` | Read-only | Returns health, contract, agent-sync, smoke, storage, config, release, or all check results. |
| `orag_trace_lookup` | `orag-self-diagnose` | Read-only | Looks up trace evidence for diagnosis. |
| `orag_diagnose` | `orag-self-diagnose` | Read-only | Interprets symptoms, trace IDs, logs, and failed command evidence. |
| `orag_runbook_suggest` | `orag-self-diagnose` | Read-only | Maps diagnosis scope and verdict to runbooks and verification commands. |
| `orag_maintenance_plan` | `orag-self-ops` | Dry-run | Creates a plan with snapshot hashes, preconditions, idempotency key, lock key, rollback, and verification commands. |
| `orag_apply_low_risk_action` | `orag-self-ops` | Write | Applies only explicitly approved low-risk actions after fresh precondition checks. |
| `orag_create_remediation_issue` | `orag-self-ops` | Write | Creates an approved remediation issue from findings when implemented by the target runtime. |

Self-ops apply paths use TOCTOU protection: snapshot hashes and preconditions are recaptured before write actions, idempotency keys prevent duplicate application, and single-flight locks block concurrent applies. If state drifts, the result returns `verdict=blocked` and the plan must be regenerated.

## Skill Installation

Generated Skill directories are ready to copy into agent-specific locations:

| Client | Generated source | Example guide |
| --- | --- | --- |
| Codex | `.codex/skills/ralph-loop/SKILL.md` | `examples/skills/codex-ralph-loop.md` |
| Claude Code | `.claude/skills/ralph-loop/SKILL.md` | `examples/skills/claude-code-ralph-loop.md` |
| Trae | `.trae/skills/ralph-loop/SKILL.md` | `examples/skills/trae-ralph-loop.md` |
| All supported clients | `orag-self-check`, `orag-self-diagnose`, `orag-self-ops` under each client Skill root | `examples/skills/self-check-diagnose-ops.md` |

Operational Skill trigger boundaries are mutually exclusive:

| Skill | Trigger | Anti-trigger |
| --- | --- | --- |
| `orag-self-check` | Read-only checks and release preflight status. | Root-cause analysis or write actions. |
| `orag-self-diagnose` | Symptoms, trace IDs, logs, failed commands, and runbook suggestions. | Pure pass/fail gate checks or write actions. |
| `orag-self-ops` | Dry-run maintenance planning and explicitly approved low-risk apply. | Read-only status checks or unapproved writes. |

## Common Errors

| Symptom | Likely cause | Fix |
| --- | --- | --- |
| `missing required ORAG MCP configuration` | `ORAG_API_TOKEN` or `ORAG_TENANT_ID` is empty for an HTTP-backed tool. | Export both variables before the HTTP-backed `tools/call`. |
| `ORAG_API_BASE_URL is invalid` | Base URL lacks scheme or host. | Use a full URL such as `http://localhost:8080`. |
| `downstream_auth_error` or `invalid_bearer_token` | Token is missing, expired, or from another tenant. | Login again and pass the fresh token as `ORAG_API_TOKEN`. |
| `downstream_rate_limited` | ORAG API returned HTTP 429. | Retry after the server-provided backoff window. |
| `downstream_timeout` | API call exceeded `ORAG_MCP_TIMEOUT`. | Increase `ORAG_MCP_TIMEOUT` or inspect the API trace ID. |
| `invalid_tool_arguments` | Tool arguments do not match the generated schema. | Re-check `.mcp/tools/*.json` or the `tools/list` response. |
| `agent artifacts are out of sync` | Generated MCP/Skill/OpenAPI facet outputs differ from the manifest. | Run `make agent-sync`, review changes, then run `make agent-sync-check`. |
| `verdict=blocked` from self-ops apply | Snapshot or preconditions drifted, approval is missing, or a single-flight lock is active. | Regenerate the dry-run plan or wait for the active apply to finish. |

## Local Verification

Run the focused Task 6 verification set from the repository root:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-artifact-tests
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make mcp-self-check-smoke
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./tests/contract -run 'TestOpenAPI|TestExamples' -v
```

For live failures, report the MCP JSON-RPC error code, downstream `trace_id`, command, and artifact paths instead of printing tokens, raw prompts, document content, model responses, or full request headers.
