# Ralph Loop Codex Skill

Generated from `orag.capabilities.v1` version `2026-07-05` with generator `manifest-first.v1` for Codex.

## Purpose
Use when an agent needs to run bounded ORAG Ralph Loop verification from a spec/task path and report a PASS/FAIL verdict with trace evidence.

## Trigger Conditions
- User asks to run Ralph Loop verification for an ORAG task or spec.
- Expected answer must include PASS/FAIL verdict, artifacts, and trace evidence.

## Anti-Triggers
- Do not use for general RAG queries.
- Do not use for unbounded autonomous implementation work.

## Mutual Exclusion
- Key: `ralph-loop`
- Ralph Loop verification is separate from self-check, diagnosis, and self-ops Skills.

## Capabilities
- `ralph_loop_run`: `ralph-loop` via `POST /v1/ralph-loop`, input `#/components/schemas/RalphLoopRequest`, output `#/components/schemas/RalphLoopResponse`, maturity `experimental`, risk `low`, side effect `read_only`

## Environment
- `ORAG_API_BASE_URL`
- `ORAG_API_TOKEN`
- `ORAG_TENANT_ID`

## Call Steps
1. Read the task or spec path.
2. Call ralph_loop_run with a bounded max_rounds value.
3. Report verdict, summary, artifacts, and trace ID.

## Example Prompts
- Run Ralph Loop for Task 1 in focused mode with at most one round.

## Example Request: `ralph_loop_run`
Run Ralph Loop for Task 1 in focused mode with at most one round, then report the verdict and trace ID.

```json
{
  "max_rounds": 1,
  "mode": "focused",
  "task_id": "Task 1",
  "task_spec_path": ".trae/specs/add-ralph-loop-mcp-skills/tasks.md"
}
```

## Expected Output Shape: `ralph_loop_run`
```json
{
  "status": "completed",
  "trace_id": "trace_ralph_loop_example",
  "verdict": "pass"
}
```

## Safety Boundaries
- Planned-only runtime boundary.
- Never print bearer tokens or tenant secrets.

## Failure Handling
- Surface API or MCP errors without retrying unboundedly.
- Return blocked when task scope is ambiguous.

## Codex Usage
- Read local task/spec or evidence files before invoking ORAG tools.
- Prefer MCP tools when available; use HTTP only when the matching `api/openapi.yaml` facet is implemented.
- Return verdict, summary, artifacts, and `trace_id` when present.
