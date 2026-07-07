---
name: orag-self-diagnose
description: "Use persisted trace evidence as read-only input for diagnosis; missing or unavailable traces must be reported as blocked."
allowed-tools: Read, Bash(curl:*)
---

# ORAG Self Diagnose Claude Code Skill

Generated from `orag.capabilities.v1` version `2026-07-05` with generator `manifest-first.v1` for Claude Code.

## Purpose
Use persisted trace evidence as read-only input for diagnosis; missing or unavailable traces must be reported as blocked.

## Trigger Conditions
- User provides symptoms, trace IDs, logs, or failed command evidence.
- User asks for root-cause analysis or recommended verification actions.

## Anti-Triggers
- Do not execute write operations.
- Do not claim release readiness; use orag-self-check for check-only requests.

## Mutual Exclusion
- Key: `self-diagnose`
- Diagnosis interprets evidence; self-check only gathers status, and self-ops handles authorized write plans.

## Capabilities
- `orag_diagnose`: `diagnose` via `POST /v1/diagnostics/diagnose`, input `#/components/schemas/DiagnoseRequest`, output `#/components/schemas/DiagnoseResult`, risk `low`, side effect `read_only`
- `orag_runbook_suggest`: `runbook-suggest` via `POST /v1/diagnostics/runbooks/suggest`, input `#/components/schemas/RunbookSuggestRequest`, output `#/components/schemas/RunbookSuggestResponse`, risk `low`, side effect `read_only`
- `orag_trace_lookup`: `trace-lookup` via `GET /v1/traces/{trace_id}`, input `#/components/schemas/TraceLookupRequest`, output `#/components/schemas/TraceLookupResponse`, risk `low`, side effect `read_only`

## Environment
- `ORAG_API_BASE_URL`
- `ORAG_API_TOKEN`
- `ORAG_TENANT_ID`

## Call Steps
1. Collect symptom, trace, and command evidence.
2. Call the diagnostic MCP tool.
3. Report findings, severity, recommended actions, and verification commands.

## Example Prompts
- Look up trace trace_req; if found, summarize failing stages, otherwise report blocked.

## Example Request: `orag_diagnose`
Diagnose why make agent-sync-check failed and recommend the next verification command.

```json
{
  "allow_commands": false,
  "scope": "mcp",
  "symptom": "make agent-sync-check failed"
}
```

## Expected Output Shape: `orag_diagnose`
```json
{
  "severity": "warning",
  "verdict": "pass"
}
```


## Example Request: `orag_runbook_suggest`
Suggest a runbook for storage readiness failures.

```json
{
  "scope": "storage",
  "verdict": "blocked"
}
```

## Expected Output Shape: `orag_runbook_suggest`
```json
{
  "runbook": "docs/operations/troubleshooting.md",
  "verdict": "pass"
}
```


## Example Request: `orag_trace_lookup`
Look up trace trace_req and summarize the failed stage when evidence exists.

```json
{
  "trace_id": "trace_req"
}
```

## Expected Output Shape: `orag_trace_lookup`
```json
{
  "found": true,
  "trace_id": "trace_req",
  "verdict": "pass"
}
```

## Safety Boundaries
- Read-only only.
- If a write is required, recommend switching to orag-self-ops dry-run planning.

## Failure Handling
- Return blocked when evidence is insufficient.
- For trace lookup, return found=false and verdict=blocked when the trace is missing or the trace store is unavailable.
- Preserve trace IDs and failed command output as evidence.

## Claude Code Usage
- Prefer `Read` for local context and MCP/HTTP calls only for the listed ORAG capabilities.
- Do not modify repository files unless the user explicitly asks for implementation work.
