# Agent and MCP Integration Scenario

## Role

Agent developers and automation platform teams use ORAG MCP and Skill assets to integrate RAG verification into agent workflows.

## Why Use ORAG

Use ORAG MCP and generated Skills when agent clients need structured operational tools instead of ad-hoc shell commands.

## When To Use It

Choose this for IDE agents, CLI agents, and automation that must report verdicts, artifacts, and trace evidence.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `expected-output.md` lists the observable success signals.
- Commands below reference maintained examples instead of duplicating raw API calls.

## Run

From the repository root, start the API first when the scenario uses service-mode curl scripts:

```sh
./scripts/dev-up.sh
make migrate
make run
./scripts/wait-ready.sh
```

Then run or inspect the scenario-specific commands:

```sh
head -n 2 examples/mcp/ralph-loop-stdio-smoke.jsonl \
| GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./cmd/orag-mcp --openapi api/openapi.yaml
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make agent-sync-check
```

For copyable client configuration, inspect `examples/mcp/stdio-client-config.json`. For generated Skill prompts, inspect `examples/skills/README.md`.

## Reused Assets

- `examples/mcp/README.md`
- `examples/mcp/stdio-client-config.json`
- `examples/mcp/ralph-loop-stdio-smoke.jsonl`
- `examples/skills/README.md`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
