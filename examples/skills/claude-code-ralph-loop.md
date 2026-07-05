# Claude Code Ralph Loop Skill Example

## Install

Copy the generated Skill directory into the Claude Code Skill location used by the target workspace:

```sh
cp -R .claude/skills/ralph-loop /path/to/claude/skills/
```

The generated frontmatter allows `Read` and `Bash(curl:*)`, which is sufficient for reading task context and calling the ORAG API.

## Configure

Expose the ORAG API settings to Claude Code:

```sh
export ORAG_API_BASE_URL=http://localhost:8080
export ORAG_API_TOKEN=replace-with-token
export ORAG_TENANT_ID=tenant_default
```

## Prompt

```text
Use the Ralph Loop Skill for .trae/specs/add-ralph-loop-mcp-skills/tasks.md Task 5. Run in focused mode with max_rounds=1, then return the PASS/FAIL verdict, summary, artifact list, and trace_id. Stop if the API returns an error.
```

## Expected Evidence

- The Skill uses `Read` for local task/spec context.
- The only shell network action is a `curl` call to `POST /v1/ralph-loop`.
- Secrets remain out of logs and final responses.
