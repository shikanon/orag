# ORAG Skill Examples

These examples show how to install and prompt generated ORAG Skills for Codex, Claude Code, and Trae.

## Common Environment

Set these values in the agent runtime before using any generated Skill that calls the ORAG API:

```sh
export ORAG_API_BASE_URL=http://localhost:8080
export ORAG_API_TOKEN=replace-with-token
export ORAG_TENANT_ID=tenant_default
```

The generated Skills are API client descriptions derived from the capability manifest. They report structured verdicts, artifacts, and `trace_id` values without printing bearer tokens or full request headers.

## Self-check, Diagnose, and Self-ops

Use `self-check-diagnose-ops.md` for the three mutually exclusive operational Skills:

| Skill | Boundary | Typical prompt |
| --- | --- | --- |
| `orag-self-check` | Read-only status checks only. | "Run focused ORAG agent_sync self-check and report stale generated artifacts." |
| `orag-self-diagnose` | Read-only diagnosis from symptoms, trace IDs, logs, or failed command evidence. | "Diagnose why make agent-sync-check failed and recommend verification commands." |
| `orag-self-ops` | Dry-run maintenance plans and explicitly approved low-risk writes. | "Create a dry-run plan to regenerate stale agent artifacts; do not apply yet." |

Do not use `orag-self-check` for root-cause analysis, do not use `orag-self-diagnose` for writes, and do not use `orag-self-ops` without explicit user approval for apply.

## Example Guides

| Client | Guide | Generated source |
| --- | --- | --- |
| All clients | `self-check-diagnose-ops.md` | `.codex/skills/orag-self-check/SKILL.md`, `.claude/skills/orag-self-diagnose/SKILL.md`, `.trae/skills/orag-self-ops/SKILL.md` |
| Codex | `codex-ralph-loop.md` | `.codex/skills/ralph-loop/SKILL.md` |
| Claude Code | `claude-code-ralph-loop.md` | `.claude/skills/ralph-loop/SKILL.md` |
| Trae | `trae-ralph-loop.md` | `.trae/skills/ralph-loop/SKILL.md` |
