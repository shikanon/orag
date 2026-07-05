# Ralph Loop Skill Examples

These examples show how to install and prompt the generated Ralph Loop Skills for Codex, Claude Code, and Trae.

## Common Environment

Set these values in the agent runtime before using any Ralph Loop Skill:

```sh
export ORAG_API_BASE_URL=http://localhost:8080
export ORAG_API_TOKEN=replace-with-token
export ORAG_TENANT_ID=tenant_default
```

The generated Skills are API client descriptions. They read local task/spec context, call `POST /v1/ralph-loop`, and report `status`, `verdict`, `summary`, `artifacts`, and `trace_id`.

## Example Guides

| Client | Guide | Generated source |
| --- | --- | --- |
| Codex | `codex-ralph-loop.md` | `.codex/skills/ralph-loop/SKILL.md` |
| Claude Code | `claude-code-ralph-loop.md` | `.claude/skills/ralph-loop/SKILL.md` |
| Trae | `trae-ralph-loop.md` | `.trae/skills/ralph-loop/SKILL.md` |
