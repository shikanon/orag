# Performance Baseline Contract Design

**Status:** Implemented and verified on 2026-07-17

**Roadmap:** Stage 3 — production-pilot baseline / observability and quality gates

## Decision

ORAG defines `orag.performance-baseline.v1`, a strict JSON report validated by a library and `oragctl benchmark-report --file`. The contract records deterministic Benchmark Pack provenance, fixed load parameters and the six Roadmap metric families. Comparison is deliberately conservative: every controlled input, including build revision and runtime-environment SHA, must match.

## Why this boundary

The project already proves that its Text RAG tutorial runs reproducibly with PostgreSQL, Qdrant, API and Console. It did not have a machine-checkable way to distinguish a controlled run from an arbitrary benchmark number. Publishing unqualified p95 or throughput values would be misleading because provider, hardware, network, cache state and build can change them.

## Validation and safety

The parser rejects unknown fields and multiple JSON values. A valid report requires a benchmark-tier deterministic mock run, two SHA-256 provenance fields, at least 20 measured requests, nonnegative timing/cost values, p95 no lower than p50, cache hit rate in `[0,1]`, and a throughput value that exactly matches document count and duration. No secrets, prompts, document content, tenant identifiers or raw traces belong in the report.

## Non-goals

This change does not fabricate a cross-hardware performance claim, collect production telemetry, or publish a real-provider benchmark. Those require a separately approved workload runner and explicitly described hardware/provider conditions.
