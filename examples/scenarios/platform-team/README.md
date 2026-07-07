# Platform Team Scenario

## Role

Platform teams use ORAG as a shared RAG capability layer for multiple applications, tenants, and agent clients.

## Why Use ORAG

Use ORAG when a platform needs one maintained service for authentication, knowledge-base lifecycle, ingestion, query, trace, evaluation, optimization, and generated agent-facing assets.

## When To Use It

Choose this scenario when building an internal AI platform, offering RAG as a service to business teams, or validating that API, MCP, and Skill assets stay aligned.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides shared-service readiness material for the runnable demo.
- `main.go` loads `demo-data.md`, runs an in-process ORAG memory demo, and prints platform usage dimensions.
- `expected-output.md` lists the observable success signals.
- Commands below use the public `pkg/memory` facade instead of duplicating raw API calls.

## Run

From the repository root, run the Go scenario demo:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/scenarios/platform-team
```

## Demo Implementation

- `main.go` creates an in-memory ORAG client through `pkg/memory`.
- The demo imports `demo-data.md`, asks a shared-service readiness question, and prints answer, citations, trace metadata, and platform usage dimensions.
- Use this as the code-level onboarding demo before running live API smoke, evaluation, optimization, and `make agent-sync-check`.

## Usage Dimensions

- User: AI platform teams and infrastructure owners.
- Business problem: provide a reusable RAG capability to multiple application teams.
- Input data: onboarding guide, readiness checklist, quality gates, agent asset rules.
- ORAG capabilities: knowledge base lifecycle, query, trace, evaluation, optimization, MCP and Skill sync.
- Success signal: application teams receive a documented service path with quality and trace evidence.

## Reused Assets

- `examples/scenarios/platform-team/main.go`
- `examples/scenarios/platform-team/demo-data.md`
- `examples/scenarios/internal/demo/demo.go`
- `pkg/memory/memory.go`
- `examples/mcp/README.md`
- `examples/skills/README.md`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
