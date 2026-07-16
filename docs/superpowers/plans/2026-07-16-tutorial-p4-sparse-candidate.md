# Tutorial P4 Sparse-Retrieval Candidate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver `p4_sparse_retrieval` as a direct-P0 candidate that reuses P0's index and evaluates only pure server-owned sparse retrieval.

**Architecture:** A strict Pack declaration and durable run fields prove the strategy and inherited index. P4 skips indexing and runs a server-built sparse `eval.Runner`; P0-P3 remain unchanged. Comparison requires direct P0 lineage and matching reused index identity.

**Tech Stack:** Go, PostgreSQL, OpenAPI 3.1, React/TypeScript, Docker Compose, Playwright.

## Global Constraints

- P4 is `sparse`, never Hybrid/RRF; P5 owns later fusion work.
- P4 reuses only a compatible completed P0 knowledge base and never reads or writes Pack objects.
- Browser input remains only Pack variant plus idempotency key.
- P4 exposes persisted evaluation results, not quality/cost/latency claims.

---

### Task 1: Declare and persist the P4 contract

**Files:**
- Modify: `internal/tutorial/manifest.go`, `internal/tutorial/clone.go`, `internal/tutorial/run.go`, `internal/tutorial/run_definition.go`, `internal/tutorial/clone_memory.go`
- Modify: `internal/storage/postgres/tutorial_run.go`, `internal/storage/postgres/tutorial_clone_test.go`
- Add: `migrations/000033_tutorial_p4_sparse_candidate.sql`
- Test: `internal/tutorial/manifest_test.go`, `internal/tutorial/clone_test.go`, `internal/tutorial/run_definition_test.go`

**Produces:** `RuntimeCandidate.RetrievalStrategy string`, `RuntimeCandidate.ReuseBaselineIndex bool`, `ExperimentRun.RetrievalStrategy string`, `ExperimentRun.ReusedBaselineIndex bool`.

- [ ] **Step 1: Write failing exact-manifest tests.**

Add the accepted P4 candidate:

```go
RuntimeCandidate{ID: TutorialP4SparseCandidateID, Chapter: TutorialP4SparseChapter,
  ParserMethod: "basic", ChunkSizeTokens: TutorialBaselineChunkSizeTokens,
  ChunkOverlapTokens: TutorialBaselineChunkOverlapTokens,
  RetrievalStrategy: TutorialRetrievalStrategySparse, ReuseBaselineIndex: true}
```

Reject hybrid P4, false reuse, contextual retrieval, and changed splitter. Assert the public variant exposes both fields.

- [ ] **Step 2: Run the failure check.**

Run `go test ./internal/tutorial -run 'Test.*P4' -count=1`; expect missing P4 fields/constants.

- [ ] **Step 3: Implement exact declaration and fingerprints.**

Add constants `TutorialP4SparseCandidateID`, `TutorialP4SparseChapter`, `TutorialRetrievalStrategyHybrid`, and `TutorialRetrievalStrategySparse`. Validate P4 as Basic/800/120/sparse/reuse=true/contextual=false; P1-P3 retain empty declaration fields. Derive `hybrid` for P0-P3 and `sparse` for P4, and include strategy/reuse in the definition fingerprint, equality test, and public projections.

- [ ] **Step 4: Persist the fields in memory and PostgreSQL.**

Create migration:

```sql
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN retrieval_strategy TEXT NOT NULL DEFAULT 'hybrid',
    ADD COLUMN reused_baseline_index BOOLEAN NOT NULL DEFAULT FALSE;
```

Add the reverse drops, PostgreSQL columns/arguments/scans, and memory clone preservation. Add a migration-fragment test.

- [ ] **Step 5: Verify and commit.**

Run `go test ./internal/tutorial ./internal/storage/postgres -run 'Test.*(P4|Manifest|Variant|Migration|Definition)' -count=1`, then commit:

```bash
git add internal/tutorial internal/storage/postgres migrations/000033_tutorial_p4_sparse_candidate.sql
git commit -m "feat: declare tutorial P4 sparse candidate"
```

### Task 2: Reuse P0 and select candidate evaluators

**Files:**
- Modify: `internal/tutorial/run.go`, `internal/tutorial/run_definition.go`, `internal/tutorial/run_test.go`

**Produces:** `ConfigureCandidateEvaluators(environment RuntimeEnvironment, evaluators map[string]RuntimeEvaluator)` and P4 direct-evaluation execution.

- [ ] **Step 1: Write failing P4 lifecycle tests.**

Use recording ingestor/evaluators. Assert P4 before P0 returns `ErrBaselineRequired`; after P0, P4 has P0's `KnowledgeBaseID`, `ReusedBaselineIndex == true`, inherited index facts, and `Stage == ExperimentRunStageEvaluate`. Execute P4 and assert zero P4 ingestor calls and one sparse evaluator call. Assert missing P4 evaluator returns `ErrRuntimeUnavailable`.

- [ ] **Step 2: Run the failure check.**

Run `go test ./internal/tutorial -run 'TestLiveRun.*P4' -count=1`; expect P4 to index or lack a candidate evaluator.

- [ ] **Step 3: Build immutable baseline reuse.**

After `FindCompletedBaseline`, for `ReuseBaselineIndex` replace only the definition knowledge base with `baseline.KnowledgeBaseID`, recompute its definition fingerprint, copy baseline index facts into the new run, set reuse true, and queue `ExperimentRunStageEvaluate`. P1-P3 retain derived KB and index stage.

- [ ] **Step 4: Resolve evaluator only in evaluation.**

Add a cloned app-owned `candidateEvaluators` map. Move `ingestorFor` inside the index-stage switch. In the evaluation case select `candidateEvaluators[run.Variant]` when present, otherwise the baseline evaluator, and fail with `ErrRuntimeUnavailable` if nil. No HTTP input changes.

- [ ] **Step 5: Verify and commit.**

Run `go test ./internal/tutorial -run 'TestLiveRun.*(P4|Baseline|Candidate)' -count=1`, then commit:

```bash
git add internal/tutorial/run.go internal/tutorial/run_definition.go internal/tutorial/run_test.go
git commit -m "feat: reuse P0 index for tutorial P4"
```

### Task 3: Wire pure sparse evaluation and comparison checks

**Files:**
- Modify: `internal/app/app.go`, `internal/app/app_test.go`, `internal/tutorial/comparison.go`, `internal/tutorial/run_test.go`

**Produces:** server-owned P4 sparse `eval.Runner` and P4 comparison rule.

- [ ] **Step 1: Write failing sparse-wiring/comparison tests.**

Assert P4 evaluator uses sparse retrieval, `Pipeline == nil`, and `Cache == nil`. Assert comparisons reject a P4 run with a derived KB, `hybrid` strategy, false reuse, changed parser/splitter, or a non-direct parent.

- [ ] **Step 2: Run the failure check.**

Run `go test ./internal/app ./internal/tutorial -run 'Test.*(P4|Sparse|Comparison)' -count=1`; expect no P4 sparse evaluator/rule.

- [ ] **Step 3: Wire P4 without mutating production RAG.**

In `app.New`, make a shallow `ragSvc` copy, then set `Retriever = backend.sparse`, `Pipeline = nil`, and `Cache = nil`. Construct `eval.Runner{RAG: &sparseTutorialRAG, Datasets: datasets, Repository: backend.evalRepo}` and register only it under P4. Keep P0-P3 on `evalRunner`.

- [ ] **Step 4: Enforce comparison invariants.**

Require P0 `hybrid`; P4 Basic/800/120, `sparse`, reuse true, direct baseline ID, same knowledge base, and inherited index values equal to P0. Reuse existing index metrics; do not add quality/cost/latency metrics.

- [ ] **Step 5: Verify and commit.**

Run `go test ./internal/app ./internal/tutorial -run 'Test.*(P4|Sparse|Comparison)' -count=1`, then commit:

```bash
git add internal/app/app.go internal/app/app_test.go internal/tutorial/comparison.go internal/tutorial/run_test.go
git commit -m "feat: evaluate tutorial P4 with sparse retrieval"
```

### Task 4: API, Console, fixture, docs, and real walkthrough

**Files:**
- Modify: `api/openapi.yaml`, `console/src/api/schema.d.ts`, `console/src/features/tutorials/tutorial-experiment-workbench.tsx`, `console/src/test/handlers.ts`, `console/src/features/tutorials/tutorials.test.tsx`
- Add: `tests/fixtures/tutorial-packs/text-rag/1.0.4/quick/corpus/service.json`, `tests/fixtures/tutorial-packs/text-rag/1.0.4/quick/manifest.json`, `docs/tutorials/p4-sparse-retrieval-candidate.md`
- Modify: `scripts/console-real-backend-tutorial-clone-e2e.sh`, `console/e2e/real-backend-tutorial-clone.spec.ts`, `README.md`, `docs/README.md`, `ROADMAP.md`, `CHANGELOG.md`

- [ ] **Step 1: Write failing HTTP and Console tests.**

Mock P4 declaration/run/comparison. Assert P4 says sparse and “reused P0 index,” and there are no controls for retriever, rank limit, RRF, or storage. Assert P0 then P4 returns sparse strategy, reuse true, P0 KB ID, and comparable true.

- [ ] **Step 2: Extend the public read model.**

Add read-only OpenAPI fields:

```yaml
retrieval_strategy: {type: string, readOnly: true, enum: [hybrid, sparse]}
reused_baseline_index: {type: boolean, readOnly: true}
```

Run `npm --prefix console run api:generate`.

- [ ] **Step 3: Render P4 and add immutable fixture.**

Label P4 only from API ID/chapter; render strategy and P0 index reuse as server facts. Create checked 1.0.4 Pack with P1-P4 declarations, update the temporary catalog mapping, and change Playwright to run P0 then P4 and assert sparse/reuse/comparison output.

- [ ] **Step 4: Document and verify.**

Document direct P0 lineage, sparse evaluator, reused index, and P5 non-overlap. Run:

```bash
make fmt
go test ./...
go vet ./...
ORAG_NODE_BIN="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node" PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console test -- --run
ORAG_NODE_BIN="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node" PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console run build
ORAG_NODE_BIN="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node" make console-real-tutorial-clone-e2e
```

- [ ] **Step 5: Commit.**

```bash
git add api console tests/fixtures/tutorial-packs/text-rag/1.0.4 scripts docs README.md ROADMAP.md CHANGELOG.md
git commit -m "feat: publish tutorial P4 sparse walkthrough"
```

### Task 5: Publish and deploy

- [ ] **Step 1: Review.** Run `git diff origin/main...HEAD --check`, `git status --short`, and `go vet ./...`.
- [ ] **Step 2: Publish.** Push `codex/tutorial-p4-sparse-candidate`, open a `main` PR, and wait for every GitHub check.
- [ ] **Step 3: Merge.** Merge with a merge commit and fast-forward `/Users/bytedance/Documents/orag` to `origin/main`, preserving `.superpowers/` only.
- [ ] **Step 4: Deploy.** Build the merged docs site, upload `/var/www/orag-docs-releases/<commit>`, atomically repoint `/var/www/orag-docs`, validate/reload nginx, and verify the home, P4 page, and API Reference over HTTPS.
- [ ] **Step 5: Clean up.** Remove this worktree and branch only after deployment verification; preserve all unrelated worktrees.
