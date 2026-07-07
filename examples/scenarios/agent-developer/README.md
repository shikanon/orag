# Agent Developer Scenario

## Role

Agent developers use ORAG to expose RAG verification, diagnostics, and operational evidence as MCP tools and agent Skills.

## Why Use ORAG

Use ORAG when an IDE agent, CLI agent, or automation workflow needs structured tool calls, bounded verification, trace evidence, and reusable prompts instead of ad-hoc shell scripts.

## When To Use It

Choose this scenario for MCP client integration, Ralph Loop verification, read-only self-check flows, generated Skill packaging, and agent workflows that must report evidence.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides agent integration requirements for the runnable demo.
- `run.sh` runs local MCP discovery, self-check smoke, and agent asset synchronization checks.
- `expected-output.md` lists the observable success signals.
- Commands below reference maintained examples instead of duplicating raw API calls.

## Run

From the repository root, inspect or run the local MCP discovery smoke:

```sh
./examples/scenarios/agent-developer/run.sh
```

For copyable client configuration, inspect `examples/mcp/stdio-client-config.json`. For agent prompt packaging, inspect `examples/skills/README.md`.

## Demo Implementation

- `run.sh` reads `demo-data.md` as the role contract for agent integration.
- The demo verifies MCP stdio discovery, read-only self-check smoke, and generated asset sync.
- Override `GOTOOLCHAIN`, `GOFLAGS`, or `CGO_ENABLED` to match the local Go environment.

## Reused Assets

- `examples/mcp/README.md`
- `examples/mcp/stdio-client-config.json`
- `examples/mcp/ralph-loop-stdio-smoke.jsonl`
- `examples/mcp/self-check-stdio-smoke.jsonl`
- `examples/skills/README.md`
- `examples/skills/codex-ralph-loop.md`
- `examples/skills/claude-code-ralph-loop.md`
- `examples/skills/trae-ralph-loop.md`
- `examples/skills/self-check-diagnose-ops.md`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
