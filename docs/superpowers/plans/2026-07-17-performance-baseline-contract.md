# Performance Baseline Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make performance-baseline inputs and result metrics machine-verifiable before publishing benchmark figures.

**Architecture:** A small internal Go package owns a strict JSON schema, semantic validation, comparability and fingerprinting. `oragctl` exposes file validation; documentation connects it to the deterministic Text RAG Benchmark Pack flow.

**Tech Stack:** Go standard library JSON/SHA-256/time packages, existing `oragctl`, Markdown documentation.

## Global Constraints

- Reports must never contain credentials, prompts, documents, tenant identifiers, or raw traces.
- Only deterministic mock benchmark-tier runs are accepted by this v1 contract.
- A report is comparable only when every workload, environment, build and load dimension matches.

### Task 1: Add report parser and semantic validator

**Files:**
- Create: `internal/benchmark/report.go`
- Test: `internal/benchmark/report_test.go`

- [ ] Define schema, provenance, load and metrics structs.
- [ ] Reject unknown/multiple JSON values, uncontrolled provenance, invalid load, inconsistent p50/p95, invalid cache rate and non-recomputable throughput.
- [ ] Add `Comparable` and SHA-256 `Fingerprint` helpers.
- [ ] Run `go test ./internal/benchmark`; expect PASS.

### Task 2: Expose a stable verification command

**Files:**
- Modify: `cmd/oragctl/main.go`, `cmd/oragctl/main_test.go`, `Makefile`

- [ ] Add `oragctl benchmark-report --file REPORT.json` and print its verified ID and fingerprint.
- [ ] Add `make benchmark-report-verify BENCHMARK_REPORT=REPORT.json`.
- [ ] Run `go test ./cmd/oragctl`; expect PASS.

### Task 3: Publish the operational contract

**Files:**
- Create: `docs/benchmarks/performance-baseline-contract.md`
- Modify: `ROADMAP.md`, `ROADMAP_EN.md`, `CHANGELOG.md`

- [ ] Document metric definitions, validation rules, reproducibility boundary and non-claim policy.
- [ ] Link it from both Roadmaps and changelog without claiming a platform-independent performance result.
- [ ] Run `./scripts/build-docs-site.sh`; expect PASS.
