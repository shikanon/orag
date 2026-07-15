# Product Team Scenario

## Role

Product teams use ORAG to validate whether a knowledge assistant answers the right questions with enough quality evidence to support release decisions.

## Why Use ORAG

Use ORAG when product managers need to compare answer quality, inspect citations, collect trace evidence, and tune retrieval settings before shipping an AI feature.

## When To Use It

Choose this scenario for feature discovery, launch readiness reviews, prompt or retrieval experiments, and product-quality regression checks.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides launch-readiness review material for the runnable demo.
- `main.go` loads `demo-data.md`, runs an in-process ORAG memory demo, and prints product usage dimensions.
- `expected-output.md` lists the observable success signals.
- Commands below use the public `pkg/memory` facade instead of duplicating raw API calls.

## Run

From the repository root, run the Go scenario demo:

```sh
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/scenarios/product-team
```

## Demo Implementation

- `main.go` creates an in-memory ORAG client through `pkg/memory`.
- The demo imports `demo-data.md`, asks a launch-readiness question, and prints answer, citations, trace metadata, and product usage dimensions.
- Use this as the code-level review demo before running live service evaluation and optimization scripts.

## Usage Dimensions

- User: product managers and AI feature owners.
- Business problem: decide whether a knowledge assistant is ready to launch.
- Input data: launch criteria, representative questions, evaluation set, optimization candidates.
- ORAG capabilities: grounded QA, citations, trace evidence, quality metrics, retrieval tuning.
- Success signal: product can explain launch, hold, or tune decisions with evidence.

## Reused Assets

- `examples/scenarios/product-team/main.go`
- `examples/scenarios/product-team/demo-data.md`
- `examples/scenarios/internal/demo/demo.go`
- `pkg/memory/memory.go`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
