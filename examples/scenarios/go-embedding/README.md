# In-process Go Embedding Scenario

## Role

Go application developers use ORAG memory mode to embed a dependency-free RAG workflow in a service or test.

## Why Use ORAG

Use ORAG in-process memory mode when a Go service needs local, dependency-free RAG behavior for tests, demos, or embedded workflows.

## When To Use It

Choose this for SDK exploration before wiring PostgreSQL, Qdrant, or external model providers.

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
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/go/memory
GOTOOLCHAIN=go1.26.4 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go test ./examples/go/memory -v
```

## Reused Assets

- `examples/go/memory/main.go`
- `examples/go/memory/main_test.go`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
