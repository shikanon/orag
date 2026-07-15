# Customer Support Scenario

## Role

Customer support and pre-sales teams use ORAG to answer product, policy, and troubleshooting questions from maintained knowledge sources.

## Why Use ORAG

Use ORAG when support agents need grounded answers with citations, trace IDs, and repeatable evidence instead of manually searching product docs during every conversation.

## When To Use It

Choose this scenario for support consoles, internal service desks, pre-sales assistants, and FAQ bots that must import product material before answering customer questions.

## Scenario Files

- `sample-input.md` describes the representative user input.
- `demo-data.md` provides support-policy and escalation source material for the runnable demo.
- `main.go` loads `demo-data.md`, runs an in-process ORAG memory demo, and prints support-specific usage dimensions.
- `expected-output.md` lists the observable success signals.
- Commands below use the public `pkg/memory` facade instead of duplicating raw API calls.

## Run

From the repository root, run the Go scenario demo:

```sh
GOTOOLCHAIN=go1.26.5 CGO_ENABLED=0 GOFLAGS=-tags=stdjson,gjson go run ./examples/scenarios/customer-support
```

## Demo Implementation

- `main.go` creates an in-memory ORAG client through `pkg/memory`.
- The demo imports `demo-data.md`, asks a customer-escalation question, and prints answer, citations, trace metadata, and support usage dimensions.
- Use this as a code-level pattern before wiring the same flow to service-mode curl or a production API client.

## Usage Dimensions

- User: customer support, pre-sales, and service desk teams.
- Business problem: answer customer questions from trusted product knowledge.
- Input data: support policy, FAQ, troubleshooting notes, escalation rules.
- ORAG capabilities: document ingestion, retrieval QA, citations, trace evidence.
- Success signal: support can reply with a grounded answer and share `trace_id` for escalation.

## Reused Assets

- `examples/scenarios/customer-support/main.go`
- `examples/scenarios/customer-support/demo-data.md`
- `examples/scenarios/internal/demo/demo.go`
- `pkg/memory/memory.go`

## Expected Output

See `expected-output.md` for the success shape and verification cues.
