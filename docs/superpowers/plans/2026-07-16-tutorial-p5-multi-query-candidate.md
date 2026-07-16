# P5 Multi-query Tutorial Candidate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a direct-P0, index-reusing P5 candidate that changes only fixed realtime multi-query expansion.

**Architecture:** Build a tutorial v2 evaluator baseline from a shallow `rag.Service` clone with ambient pipeline/cache/router/rewrite/HyDE removed. P0–P4 use that fixed baseline; P5 uses the same clone but enables exactly three generated retrieval queries. Persist the expansion contract and require it in comparisons.

**Tech Stack:** Go, PostgreSQL migrations, OpenAPI, React/TypeScript Console, Vitest, Playwright, Docker Compose, Qdrant.

## Global Constraints

- P5 ID is `p5_multi_query_retrieval`; it is direct P0-only and never accepts browser-owned runtime settings.
- P5 retains Basic/800/120, `realtime`, identical P0 knowledge base/dataset/Top-K and Hybrid retrieval.
- P5 reuses P0 index facts and begins at `run_evaluation`; it must never call Pack ingestion.
- P5 enables exactly 3 multi-queries; it must not enable Rewrite, HyDE, cache, Pipeline, or Query Router.
- Evaluator version changes to `tutorial_eval_v2`; old runs remain readable but never compare across versions.

---

### Task 1: Declare P5 and persist the expansion contract

**Files:**
- Modify: `internal/tutorial/manifest.go`, `internal/tutorial/clone.go`, `internal/tutorial/run.go`, `internal/tutorial/run_definition.go`
- Modify: `internal/storage/postgres/tutorial_run.go`
- Create: `migrations/000034_tutorial_p5_multi_query_candidate.sql`
- Test: `internal/tutorial/manifest_test.go`, `internal/tutorial/clone_test.go`, `internal/tutorial/run_definition_test.go`, `internal/storage/postgres/tutorial_clone_test.go`

**Interfaces:**
- Produces `TutorialP5MultiQueryCandidateID`, `TutorialQueryExpansionNone`, `TutorialQueryExpansionMultiQuery`.
- Adds `MultiQueryCount int` to `RuntimeCandidate`/`ExperimentVariant` and `MultiQueryCount int`, `QueryExpansionMode string` to `ExperimentRun`.

- [ ] Add strict P5 manifest test accepting only Basic/800/120, Hybrid, P0-index reuse and `multi_query_count: 3`; assert wrong count, sparse strategy, contextual retrieval or fresh index fails.
- [ ] Add constants and fields, then make P1–P4 require zero count and P5 require exactly three.
- [ ] Add migration with non-null defaults `query_expansion_mode='none'`, `multi_query_count=0`; update SELECT, INSERT and scan order atomically.
- [ ] Extend runtime definition/fingerprint/match logic and public experiment output; add migration and fingerprint tests.
- [ ] Run `go test ./internal/tutorial ./internal/storage/postgres` and commit `feat: declare tutorial P5 multi-query contract`.

### Task 2: Make multi-query explicitly available to the fixed realtime tutorial evaluator

**Files:**
- Modify: `internal/rag/service.go`, `internal/rag/retrieval_expansion.go`
- Test: `internal/rag/service_test.go`

**Interfaces:**
- Adds `MultiQueryForRealtime bool` to `rag.Service`.
- `BuildRetrievalQueries` expands only when profile is `high_precision` or this flag is true; Rewrite and HyDE keep their current high-precision-only gates.

- [ ] Add a failing realtime test with `MultiQueryForRealtime=true`, `MultiQueryCount=3`, Rewrite/HyDE disabled; assert exactly original + generated queries and no rewrite/HyDE model prompts.
- [ ] Add the boolean field and replace the profile-only early return with an expansion-enabled predicate.
- [ ] Run `go test ./internal/rag` and commit `feat: allow server-owned realtime multi-query expansion`.

### Task 3: Wire tutorial evaluator v2 and P5 lifecycle

**Files:**
- Modify: `internal/app/app.go`, `internal/tutorial/run.go`, `internal/tutorial/comparison.go`
- Test: `internal/app/app_test.go` or focused wiring test, `internal/tutorial/run_test.go`

**Interfaces:**
- Add an app-local helper or adjacent construction that produces a baseline `rag.Service` clone with `Pipeline`, `Cache`, `QueryRouter` nil; Rewrite/HyDE false; one query; `MultiQueryForRealtime=false`.
- Configure `LiveRunService` with a v2 `eval.Runner` and `RuntimeEnvironment{EvaluatorVersion: "tutorial_eval_v2"}`.
- Register P4 sparse and P5 hybrid candidate evaluators from that same baseline clone.

- [ ] Write lifecycle tests: P5 rejects absent P0, starts directly in evaluation, stores inherited P0 facts, uses the same KB, records Hybrid/multi-query/3, and performs no ingestion.
- [ ] Update comparison tests: P5 requires P0 index reuse and exact expansion facts; invalid count/mode or different KB is non-comparable.
- [ ] Wire the v2 baseline evaluator; create P5 clone with only `MultiQueryCount=3` and `MultiQueryForRealtime=true`; ensure P4 clone remains sparse and has expansion disabled.
- [ ] Run `go test ./internal/app ./internal/tutorial ./internal/rag` and commit `feat: run P5 on fixed tutorial evaluator v2`.

### Task 4: Expose P5 through the API, Console and controlled walkthrough

**Files:**
- Modify: `api/openapi.yaml`, `console/src/api/schema.d.ts`, `console/src/features/tutorials/tutorial-experiment-workbench.tsx`, `console/src/test/handlers.ts`
- Modify: `console/e2e/real-backend-tutorial-clone.spec.ts`, `scripts/console-real-backend-tutorial-clone-e2e.sh`
- Create: `tests/fixtures/tutorial-packs/text-rag/1.0.5/quick/manifest.json`, `tests/fixtures/tutorial-packs/text-rag/1.0.5/quick/corpus/service.json`

- [ ] Add read-only variant/run properties for `multi_query_count` and `query_expansion_mode`; regenerate `console/src/api/schema.d.ts` with `npm --prefix console run api:generate`.
- [ ] Add P5 labels, inherited-index explanation and read-only expansion audit to Console; add completed P5 mock run/comparison.
- [ ] Add controlled 1.0.5 fixture with P1–P5 declarations and matching checksum corpus; update walkthrough temporary mapping and assert P0→P5 UI audit facts with strict locators.
- [ ] Run Console unit tests, build, and `make console-real-tutorial-clone-e2e`; commit `feat: expose tutorial P5 multi-query walkthrough`.

### Task 5: Document, publish, and verify

**Files:**
- Create: `docs/tutorials/p5-multi-query-candidate.md`, `docs-site/tutorials/p5-multi-query.html`
- Modify: `README.md`, `docs/README.md`, `docs-site/index.html`, `ROADMAP.md`, `CHANGELOG.md`

- [ ] Document v2 evaluator isolation, direct P0 lineage, exact three-query expansion, P0-index reuse and no inferred quality/cost/latency claims.
- [ ] Link the hosted page from `docs-site/index.html`; update roadmap remaining scope to P6–P8.
- [ ] Run `go test ./...`, `go vet ./...`, `make openapi-validate`, Console build, `make console-real-tutorial-clone-e2e`, and `./scripts/build-docs-site.sh /tmp/orag-docs-p5-check`.
- [ ] Commit docs, push branch, open PR, wait for all checks, merge, deploy the merged static site to `www.tensorbytes.com/orag/`, curl-verify the P5 page/OpenAPI, then remove the merged worktree and branch.
