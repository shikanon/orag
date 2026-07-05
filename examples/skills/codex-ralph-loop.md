# Codex Ralph Loop Skill Example

## Install

Copy the generated Skill directory into the Codex workspace or user Skill path expected by the local Codex client:

```sh
cp -R .codex/skills/ralph-loop /path/to/codex/skills/
```

Keep `.codex/skills/ralph-loop/SKILL.md` generated from `api/openapi.yaml`; do not edit the copied file by hand unless the client requires local-only metadata.

## Configure

Expose the ORAG API settings to the Codex runtime:

```sh
export ORAG_API_BASE_URL=http://localhost:8080
export ORAG_API_TOKEN=replace-with-token
export ORAG_TENANT_ID=tenant_default
```

## Prompt

```text
Use the Ralph Loop Skill to verify Task 5 from .trae/specs/add-ralph-loop-mcp-skills/tasks.md in focused mode with max_rounds=1. Report status, verdict, summary, artifacts, and trace_id. Do not print bearer tokens or full request headers.
```

## Expected Evidence

- The Skill reads `.trae/specs/add-ralph-loop-mcp-skills/tasks.md` before calling the API.
- The API call targets `${ORAG_API_BASE_URL}/v1/ralph-loop`.
- The final answer includes the Ralph Loop verdict and trace evidence.
