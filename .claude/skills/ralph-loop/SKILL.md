---
name: ralph-loop
description: "Use when an agent needs to run bounded ORAG Ralph Loop verification from a spec/task path and report a PASS/FAIL verdict with trace evidence."
allowed-tools: Read, Bash(curl:*)
---

# Ralph Loop Claude Code Skill

Generated from `x-orag-agent-capabilities` version `1` for Claude Code.

## Purpose
Use when an agent needs to run bounded ORAG Ralph Loop verification from a spec/task path and report a PASS/FAIL verdict with trace evidence.

## Trigger Conditions
- Use when an agent needs a bounded ORAG Ralph Loop verification run from a task/spec path.
- Use when the expected answer must include a PASS/FAIL verdict, artifacts, and trace evidence.
- Do not use for general RAG queries, ingestion, or unbounded autonomous code changes.

## Parameters
- API endpoint: `POST /v1/ralph-loop`
- Operation ID: `runRalphLoop`
- MCP tool: `ralph_loop_run`
- Input schema: `#/components/schemas/RalphLoopRequest`
- Output schema: `#/components/schemas/RalphLoopResponse`
- Error schema: `#/components/schemas/ErrorResponse`

## Environment
- `ORAG_API_BASE_URL`
- `ORAG_API_TOKEN`
- `ORAG_TENANT_ID`

## Call Steps
1. Confirm `ORAG_API_BASE_URL`, `ORAG_API_TOKEN`, and `ORAG_TENANT_ID` are available.
2. Build a request body with `task_spec_path`, `task_id`, `mode`, and `max_rounds`.
3. Send `Authorization: Bearer ${ORAG_API_TOKEN}`, `X-ORAG-Tenant-ID: ${ORAG_TENANT_ID}`, and optional `X-Trace-ID`.
4. Report `status`, `verdict`, `summary`, `artifacts`, and `trace_id` from the response.

## Example Prompt
Run Ralph Loop for `Task 1` in `focused` mode with at most `1` round(s), then report the verdict and trace ID.

## Example Request
```json
{
  "max_rounds": 1,
  "mode": "focused",
  "task_id": "Task 1",
  "task_spec_path": ".trae/specs/add-ralph-loop-mcp-skills/tasks.md"
}
```

## Expected Output Shape
```json
{
  "status": "completed",
  "trace_id": "trace_ralph_loop_example",
  "verdict": "pass"
}
```

## Safety Boundaries
- Treat this Skill as an API client description; it does not implement the Ralph Loop runtime handler.
- Never print bearer tokens, tenant secrets, or full request headers in the final answer.
- Stop and surface the API error when `#/components/schemas/ErrorResponse` or HTTP status indicates failure.

## Claude Code Usage
- Prefer `Read` for local task/spec context and `Bash(curl:*)` only for the ORAG API call.
- Do not modify repository files unless the user explicitly asks for implementation work.
