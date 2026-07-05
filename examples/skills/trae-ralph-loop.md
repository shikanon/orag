# Trae Ralph Loop Skill Example

## Install

Trae workspace Skills are generated directly under the repository:

```sh
ls .trae/skills/ralph-loop/SKILL.md
```

If a separate workspace needs the Skill, copy the directory:

```sh
cp -R .trae/skills/ralph-loop /path/to/workspace/.trae/skills/
```

## Configure

Expose the ORAG API settings to Trae before invoking the Skill:

```sh
export ORAG_API_BASE_URL=http://localhost:8080
export ORAG_API_TOKEN=replace-with-token
export ORAG_TENANT_ID=tenant_default
```

## Prompt

```text
Run Ralph Loop verification for Task 5 in .trae/specs/add-ralph-loop-mcp-skills/tasks.md. Use focused mode and max_rounds=1. Return status, verdict, summary, artifacts, and trace_id only; do not reveal tokens.
```

## Expected Evidence

- Trae discovers `.trae/skills/ralph-loop/SKILL.md` from the workspace.
- The Skill builds the request with `task_spec_path`, `task_id`, `mode`, and `max_rounds`.
- The final response preserves the Ralph Loop `trace_id` for follow-up diagnostics.
