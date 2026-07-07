# Engineering Runbook Scenario

## Role

Engineering and SRE teams use ORAG to search runbooks, incident notes, architecture docs, and API references during debugging.

## Why Use ORAG

Use ORAG when engineers need a fast answer grounded in internal operational knowledge and enough trace evidence to explain why a diagnostic answer was produced.

## When To Use It

Choose this scenario for incident triage, on-call handoff, architecture lookup, API troubleshooting, and postmortem knowledge reuse.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides runbook and escalation source material for the runnable demo.
- `main.go` loads `demo-data.md`, runs an in-process ORAG memory demo, and prints engineering usage dimensions.
- `expected-output.md` lists the observable success signals.
- Commands below use the public `pkg/memory` facade instead of duplicating raw API calls.

## Run

From the repository root, run the Go scenario demo:

```sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/scenarios/engineering-runbook
```

For read-only operational checks, inspect `examples/skills/self-check-diagnose-ops.md`.

## Demo Implementation

- `main.go` creates an in-memory ORAG client through `pkg/memory`.
- The demo imports `demo-data.md`, asks a latency-triage question, and prints answer, citations, trace metadata, and engineering usage dimensions.
- Use this as a code-level pattern before wiring the same flow to a live ORAG service and incident system.

## Usage Dimensions

- User: engineers, SREs, and on-call responders.
- Business problem: find actionable runbook steps during incidents.
- Input data: runbooks, postmortems, architecture notes, API troubleshooting docs.
- ORAG capabilities: knowledge retrieval, answer generation, trace metadata, cited source chunks.
- Success signal: incident notes include the answer, cited runbook section, and `trace_id`.

## Reused Assets

- `examples/scenarios/engineering-runbook/main.go`
- `examples/scenarios/engineering-runbook/demo-data.md`
- `examples/scenarios/internal/demo/demo.go`
- `pkg/memory/memory.go`
- `examples/skills/self-check-diagnose-ops.md`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
