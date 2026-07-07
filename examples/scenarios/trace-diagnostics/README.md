# Trace and Diagnostics Scenario

## Role

Support, engineering, and operations teams use ORAG trace data to explain and debug RAG answers.

## Why Use ORAG

Use ORAG traces and read-only diagnostics when an answer looks wrong, slow, or hard to explain.

## When To Use It

Use this scenario after running a normal or streaming query and saving `.orag-demo/trace_id`.

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
./examples/curl/36_trace_lookup.sh
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson make mcp-self-check-smoke
```

For agent-guided diagnosis prompts, inspect `examples/skills/self-check-diagnose-ops.md`.

## Reused Assets

- `examples/curl/36_trace_lookup.sh`
- `examples/mcp/self-check-stdio-smoke.jsonl`
- `examples/skills/self-check-diagnose-ops.md`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
