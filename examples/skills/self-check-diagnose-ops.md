# Self-check, Diagnose, and Self-ops Skill Examples

## Purpose

These examples show when to use the three generated ORAG operational Skills and when not to use them. The Skills are mutually exclusive:

| Skill | Use when | Do not use when |
| --- | --- | --- |
| `orag-self-check` | The user asks for read-only health, contract, agent-sync, smoke, storage, config, or release checks. | The user asks for root-cause analysis or an operational write. |
| `orag-self-diagnose` | The user provides symptoms, trace IDs, logs, or failed command evidence and wants findings plus verification commands. | The user only asks whether a gate passes, or asks to apply a fix. |
| `orag-self-ops` | The user asks for a dry-run maintenance plan or explicitly authorizes a low-risk operational action. | The user has not approved writes or only needs read-only status. |

| Skill | Primary MCP tools |
| --- | --- |
| `orag-self-check` | `orag_check` |
| `orag-self-diagnose` | `orag_trace_lookup`, `orag_diagnose`, `orag_runbook_suggest` |
| `orag-self-ops` | `orag_maintenance_plan`, `orag_apply_low_risk_action`, `orag_create_remediation_issue` |

## Install

Generated Skill sources are written for each supported agent:

```sh
ls .codex/skills/orag-self-check/SKILL.md
ls .codex/skills/orag-self-diagnose/SKILL.md
ls .codex/skills/orag-self-ops/SKILL.md
ls .claude/skills/orag-self-check/SKILL.md
ls .claude/skills/orag-self-diagnose/SKILL.md
ls .claude/skills/orag-self-ops/SKILL.md
ls .trae/skills/orag-self-check/SKILL.md
ls .trae/skills/orag-self-diagnose/SKILL.md
ls .trae/skills/orag-self-ops/SKILL.md
```

Keep generated Skill files in sync with the capability manifest by running:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check
```

## Common Environment

Expose these values when a Skill calls the ORAG API. Local MCP read-only tools may not need every value, but generated Skills document the same auth contract for consistency.

```sh
export ORAG_API_BASE_URL=http://localhost:8080
export ORAG_API_TOKEN=replace-with-token
export ORAG_TENANT_ID=tenant_default
```

## Self-check Prompt

```text
Use the ORAG self-check Skill to run a focused agent_sync check. Report the verdict, stable check IDs, evidence, trace_id, and explicitly state that make agent-sync-check remains the authoritative CI gate.
```

Expected tool input:

```json
{
  "scope": "agent_sync",
  "mode": "focused"
}
```

## Diagnose Prompt

```text
Use the ORAG self-diagnose Skill to explain why make agent-sync-check failed with "generated content differs". Do not perform writes. Return findings, severity, recommended actions, verification commands, artifacts, and trace_id.
```

Expected tool input:

```json
{
  "scope": "agent_sync",
  "symptom": "make agent-sync-check failed",
  "failed_command": "make agent-sync-check",
  "failed_command_exit_code": 1,
  "failed_command_output": "generated content differs",
  "allow_commands": false
}
```

## Self-ops Prompt

```text
Use the ORAG self-ops Skill to create a dry-run maintenance plan for stale agent artifacts. Do not apply the plan unless I explicitly approve the low-risk action after reviewing snapshot hashes, preconditions, idempotency key, lock key, rollback, and verification commands.
```

Expected dry-run input:

```json
{
  "scope": "agent_artifacts",
  "dry_run": true
}
```

Expected apply input only after explicit user approval:

```json
{
  "plan_id": "plan_20260705_001",
  "approved": true
}
```

## Safety Notes

- `orag-self-check` and `orag-self-diagnose` are read-only.
- `orag-self-ops` starts with dry-run planning and blocks writes without explicit approval.
- Apply must re-check snapshot hashes and preconditions; drift returns `verdict=blocked`.
- Do not print bearer tokens, request headers, raw prompts, document content, or model responses in final reports.
