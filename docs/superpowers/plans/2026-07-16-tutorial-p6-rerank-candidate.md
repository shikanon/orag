# P6 Rerank Tutorial Candidate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a direct-P0, index-reusing P6 candidate that changes only server-owned rerank execution.

**Architecture:** Upgrade the isolated tutorial evaluator to v3, where P0–P5 set `DisableRerank=true`. P6 uses the same clone and P0 index but enables the existing reranker. Persist the immutable rerank state and reject comparisons whose stored audit contract cannot prove the one-variable delta.

**Tech Stack:** Go, PostgreSQL migrations, OpenAPI, React/TypeScript Console, Vitest, Playwright, Docker Compose, Qdrant.

## Global Constraints

- P6 ID is `p6_rerank_retrieval`; it is direct P0-only, uses Basic/800/120, `realtime`, hybrid retrieval, the same knowledge base/dataset/Top-K, and never accepts browser-owned settings.
- P6 starts at `run_evaluation`, reuses P0 index facts, and never reads Pack objects or indexes again.
- Tutorial evaluator v3 removes Pipeline, Cache and Query Router; disables Rewrite, HyDE and rerank for P0–P5; P5 retains only its fixed three-query expansion.
- P6 enables only existing rerank; zero-value production `rag.Service` behavior must remain unchanged.
- Historical v2 runs remain readable but cannot compare with v3 because the evaluator fingerprint changes.

---

### Task 1: Declare and persist the P6 rerank contract

**Files:**
- Modify: `internal/tutorial/manifest.go`, `internal/tutorial/clone.go`, `internal/tutorial/run.go`, `internal/tutorial/run_definition.go`, `internal/tutorial/comparison.go`
- Modify: `internal/storage/postgres/tutorial_run.go`
- Create: `migrations/000035_tutorial_p6_rerank_candidate.sql`
- Test: `internal/tutorial/manifest_test.go`, `internal/tutorial/clone_test.go`, `internal/tutorial/run_definition_test.go`, `internal/tutorial/run_test.go`, `internal/storage/postgres/tutorial_clone_test.go`

**Interfaces:**
- Add `TutorialP6RerankCandidateID`, `TutorialP6RerankChapter` and `RerankEnabled bool` to Pack candidate, public variant and persisted experiment run.
- `runtimeDefinition` derives P0–P5 `rerankEnabled=false`; P6 only accepts `rerank_enabled=true`.

- [ ] Add failing manifest tests for the exact P6 JSON declaration; reject false rerank, sparse retrieval, non-P0 index reuse, non-Basic/800/120 and nonzero multi-query count.
- [ ] Add strict P6 validation and make every earlier candidate require `rerank_enabled=false`.
- [ ] Add the non-null `rerank_enabled BOOLEAN NOT NULL DEFAULT FALSE` migration; extend PostgreSQL SELECT, INSERT and scanner in identical column order.
- [ ] Include rerank state in definition fingerprint, matching, public output and comparison checks; add lifecycle and invalid-comparison tests.
- [ ] Run `go test ./internal/tutorial ./internal/storage/postgres` and commit `feat: declare tutorial P6 rerank contract`.

### Task 2: Gate rerank without changing production defaults

**Files:**
- Modify: `internal/rag/service.go`
- Test: `internal/rag/service_test.go`

**Interfaces:**
- Add `DisableRerank bool` to `rag.Service`.
- `ApplyRerank` returns the exact retrieval order without model calls when true; false continues existing rerank behavior.

- [ ] Write a test using the scripted reranker: `DisableRerank=true` returns original `From`, rank and score, and records zero rerank calls.
- [ ] Add the early return before constructing rerank documents.
- [ ] Run `go test ./internal/rag` and commit `feat: allow tutorial rerank isolation`.

### Task 3: Wire evaluator v3 and direct-P0 P6 execution

**Files:**
- Modify: `internal/app/app.go`
- Test: `internal/tutorial/run_test.go`, `internal/app/app_test.go` or focused adjacent app test

**Interfaces:**
- Baseline tutorial clone sets `DisableRerank=true`; RuntimeEnvironment uses `EvaluatorVersion: "tutorial_eval_v3"`.
- P4 sparse and P5 multi-query clones retain `DisableRerank=true`; P6 clone only flips it to false.

- [ ] Extend the P0-to-candidate lifecycle test: P6 rejects absent P0, starts at evaluation, inherits index facts and KB, persists `rerank_enabled=true`, invokes only its evaluator and does no ingest.
- [ ] Configure P6 evaluator and v3 fingerprint; assert P0–P5 remain rerank-disabled through their audit values.
- [ ] Run `go test ./internal/app ./internal/tutorial ./internal/rag` and commit `feat: run P6 on fixed tutorial evaluator v3`.

### Task 4: Expose P6 and cover the controlled walkthrough

**Files:**
- Modify: `api/openapi.yaml`, `console/src/api/schema.d.ts`, `console/src/features/tutorials/tutorial-experiment-workbench.tsx`, `console/src/test/handlers.ts`
- Modify: `console/e2e/real-backend-tutorial-clone.spec.ts`, `scripts/console-real-backend-tutorial-clone-e2e.sh`
- Create: `tests/fixtures/tutorial-packs/text-rag/1.0.6/quick/manifest.json`, `tests/fixtures/tutorial-packs/text-rag/1.0.6/quick/corpus/service.json`

- [ ] Add read-only `rerank_enabled` to variant and run schemas, regenerate Console types, and update mock P0/P6 records.
- [ ] Render the immutable rerank state, P6 label and direct-P0 index-reuse explanation; add no client controls.
- [ ] Add a P1–P6 fixture, temporarily map it in the real harness, then assert P0→P6 renders `rerank_enabled=true`, P0 parent reuse and comparable metrics.
- [ ] Run Console typecheck, unit tests, build and `make console-real-tutorial-clone-e2e`; commit `feat: expose tutorial P6 rerank walkthrough`.

### Task 5: Document, publish and verify

**Files:**
- Create: `docs/tutorials/p6-rerank-candidate.md`, `docs-site/tutorials/p6-rerank.html`
- Modify: `README.md`, `docs-site/index.html`, `ROADMAP.md`, `CHANGELOG.md`

- [ ] Document v3 rerank isolation, direct P0 lineage, inherited index facts and the absence of inferred quality/cost/latency claims.
- [ ] Update remaining roadmap scope to P7–P8 and link the hosted P6 page.
- [ ] Run `go test ./...`, `go vet ./...`, `make openapi-validate`, Console typecheck/unit/build, real tutorial E2E and `./scripts/build-docs-site.sh /tmp/orag-docs-p6-check`.
- [ ] Commit, push, create a draft PR, wait for all checks, merge, deploy the merged static site to `www.tensorbytes.com/orag/`, curl-verify P6 and OpenAPI, synchronize local `main`, then remove the merged worktree/branch.
