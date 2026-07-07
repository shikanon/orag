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
- `main.go` loads `demo-data.md`, runs an in-process ORAG memory demo, and prints agent-developer usage dimensions.
- `expected-output.md` lists the observable success signals.
- Commands below use the public `pkg/memory` facade instead of duplicating raw API calls.

## Run

From the repository root, run the Go scenario demo:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/scenarios/agent-developer
```

For copyable client configuration, inspect `examples/mcp/stdio-client-config.json`. For agent prompt packaging, inspect `examples/skills/README.md`.

## Demo Implementation

- `main.go` creates an in-memory ORAG client through `pkg/memory`.
- The demo imports `demo-data.md`, asks an agent tooling question, and prints answer, citations, trace metadata, and agent-developer usage dimensions.
- Use this as the code-level tool-response demo before running MCP discovery, self-check smoke, and `make agent-sync-check`.

## Usage Dimensions

- User: IDE agent, CLI agent, and MCP tool developers.
- Business problem: expose bounded ORAG verification and diagnostics as structured agent tools.
- Input data: MCP contracts, Skill prompts, authorization environment, safety boundaries.
- ORAG capabilities: tool discovery, Ralph Loop, self-check, trace evidence, Skill packaging.
- Success signal: agent output includes verdict-oriented guidance, artifacts, trace evidence, and approval boundaries.

## Reused Assets

- `examples/scenarios/agent-developer/main.go`
- `examples/scenarios/agent-developer/demo-data.md`
- `examples/scenarios/internal/demo/demo.go`
- `pkg/memory/memory.go`
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
