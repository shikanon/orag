# Reproducible Performance Baseline Runner Design

**Status:** Implemented and verified on 2026-07-17

**Roadmap:** Phase 3 — production-pilot baseline / observability and quality gates

## Decision

Add a deterministic, dependency-free `oragctl benchmark-run` command that executes
a small versioned Text RAG workload through the public `github.com/shikanon/orag`
SDK and writes a valid `orag.performance-baseline.v1` JSON report. The public SDK
returns validated report JSON bytes rather than an `internal/*` report type. The command
never reads model credentials or uses ambient service configuration: it always
creates an embedded client from `orag.MockConfig()`.

The runner owns a public, checked-in workload containing documents, query/evidence
pairs and a fixed evaluator input. It measures real local wall-clock durations for
ingestion, warmup/measurement queries and evaluation. The fixed inputs, load
parameters, build revision and runtime-environment fingerprint make each output
auditable; they do not make absolute timings portable across hardware.

## Interface and data flow

`oragctl benchmark-run --output report.json --build-revision <revision>` is the
primary entry point. It uses 10 warmup requests, 20 measured requests and
concurrency 1 unless an explicit supported load override is supplied. The command:

1. builds the fixed `text-rag/mock-baseline-v1` workload;
2. creates a mock SDK client and knowledge base, then ingests every workload
   document while timing that phase;
3. creates a deterministic evaluation dataset whose relevant document IDs are the
   IDs returned by actual ingestion;
4. runs warmup and measured queries, records each response duration and cache
   status, then derives p50, p95 and cache-hit rate from observed responses;
5. runs the evaluation through the same SDK client and records its duration;
6. creates and validates the internal report, returns canonical indented JSON,
   and lets the CLI write it with restrictive file permissions.

The report's data fingerprint is SHA-256 over the canonical workload bytes. Its
runtime-environment fingerprint is SHA-256 over an explicit JSON record of the
runner schema, Go version, GOOS/GOARCH, mock model identifiers and fixed RAG
configuration. `model_calls` is an explicitly documented deterministic accounting
estimate: one model pipeline invocation for each warmup, measured and evaluator
query; `cost_usd` remains zero because the mock provider has no billable calls.

## Safety and correctness

The command rejects an empty output path, blank build revision, invalid load
values, cancellation and any SDK failure; it does not leave a partially-written
output file. The workload contains no user material, secrets, tenant identifiers
or provider endpoints. A report must continue to pass the existing strict parser
before it is written.

Only local mock results can be produced by this command. Documentation must call
them a reproducible local baseline and must require disclosed hardware/provider
conditions before any public cross-run comparison or production claim.

## Testing and release evidence

Unit tests cover workload fingerprint stability, percentile rounding, cache-rate
derivation, report validity, option rejection and cancellation. CLI tests create a
temporary report, re-parse it and verify that no output appears after invalid
arguments. A contract test checks that the Make target and documentation expose
both generation and verification. `make agent-gate` remains the complete local
gate.

## Non-goals

This does not publish a hardware-neutral performance number, execute a real
provider benchmark, replace the PostgreSQL/Qdrant benchmark E2E, or claim the
independent deployment and 30-day pilot evidence required for the Phase 3 exit.
