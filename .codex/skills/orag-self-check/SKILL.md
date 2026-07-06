# ORAG Self Check Codex Skill

Generated from `orag.capabilities.v1` version `2026-07-05` with generator `manifest-first.v1` for Codex.

## Purpose
Use for read-only ORAG health, contract, agent-sync, smoke, storage, config, and release checks.

## Trigger Conditions
- User asks to check ORAG health.
- User asks whether MCP or Skills are synchronized.
- User asks for release readiness or CI preflight checks.

## Anti-Triggers
- Do not perform root-cause diagnosis beyond reporting failed checks.
- Do not propose or apply writes; hand off to orag-self-diagnose or orag-self-ops when needed.

## Mutual Exclusion
- Key: `self-check`
- Only for checking state; diagnosis and writes belong to separate Skills.

## Capabilities
- `orag_check`: `self-check` via `POST /v1/self-check`, input `#/components/schemas/SelfCheckRequest`, output `#/components/schemas/SelfCheckResult`, risk `low`, side effect `read_only`

## Environment
- `ORAG_API_BASE_URL`
- `ORAG_API_TOKEN`
- `ORAG_TENANT_ID`

## Call Steps
1. Discover MCP tools.
2. Call orag_check with the requested scope and mode.
3. Report PASS, FAIL, or BLOCKED with stable check IDs and evidence.

## Example Prompts
- Check ORAG agent_sync in focused mode and report stale generated artifacts.

## Example Request: `orag_check`
Check whether ORAG MCP and Skill artifacts are synchronized.

```json
{
  "mode": "focused",
  "scope": "agent_sync"
}
```

## Expected Output Shape: `orag_check`
```json
{
  "schema_version": "orag.selfops.result.v1",
  "verdict": "pass"
}
```

## Safety Boundaries
- Read-only only.
- For agent_sync, state that make agent-sync-check remains the authoritative release gate.

## Failure Handling
- Return blocked when a check cannot complete.
- Preserve partial results and surface the trace ID.

## Codex Usage
- Read local task/spec or evidence files before invoking ORAG tools.
- Prefer MCP tools when available; use HTTP only when the matching `api/openapi.yaml` facet is implemented.
- Return verdict, summary, artifacts, and `trace_id` when present.
