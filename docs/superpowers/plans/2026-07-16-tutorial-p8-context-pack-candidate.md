# Tutorial P8 Context Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver an auditable direct-P0 P8 candidate that only reduces the server-owned Context Pack from five evidence chunks to three.

**Architecture:** Evaluator v5 fixes the tutorial baseline Packer to `TopN: 5, MaxTokens: 6000`; P8 reuses the compatible P0 index and runs an evaluator clone with `TopN: 3`. Durable run fields, fingerprints and comparison validation prove this is the sole P8 delta.

**Tech Stack:** Go, PostgreSQL migrations, OpenAPI, React/TypeScript Console, Vite/Vitest/Playwright, Docker Compose, Qdrant.

## Global Constraints

- P8 ID/chapter is exactly `p8_context_pack`.
- P8 accepts only Basic/800/120, hybrid retrieval, P0-index reuse, no contextual retrieval, no multi-query, no rerank, no graph retrieval, `context_pack_top_n=3`, `context_pack_max_tokens=6000`.
- v5 P0–P7 record Context Pack `5`/`6000`; P8 records `3`/`6000`.
- P8 must never accept browser-owned packer, retriever, index, model, cache, profile or Top-K settings.
- Keep Pipeline, semantic cache, router, rewrite, HyDE, GraphBuilder and RAPTOR disabled for tutorial evaluation; retain the existing P5, P6 and P7 isolated exceptions only.
- Do not claim quality, actual context-token use, latency or cost from configured pack values.

---

### Task 1: Declare and persist the immutable P8 contract

**Files:**
- Modify: `internal/tutorial/manifest.go`, `internal/tutorial/clone.go`, `internal/tutorial/run.go`, `internal/tutorial/run_definition.go`, `internal/tutorial/comparison.go`
- Modify: `internal/storage/postgres/tutorial_run.go`
- Create: `migrations/000037_tutorial_p8_context_pack_candidate.sql`
- Test: `internal/tutorial/manifest_test.go`, `internal/tutorial/clone_test.go`, `internal/tutorial/run_definition_test.go`, `internal/tutorial/run_test.go`, `internal/storage/postgres/tutorial_clone_test.go`

**Interfaces:**
- Produces `TutorialP8ContextPackCandidateID`, `TutorialP8ContextPackChapter`, `ContextPackTopN`, and `ContextPackMaxTokens` on manifest candidates, public variants and runs.
- Produces exact v5 comparison predicates for P0/P8.

- [ ] **Step 1: Add failing P8 manifest and comparison tests**

Add a valid P8 declaration and reject a changed Top-N, token limit, strategy, reuse flag, rerank, graph or multi-query setting. Add a comparison fixture where P0 has `5/6000`, P8 has `3/6000`, shares the P0 KB and is comparable; mutate each Context Pack value and assert it becomes non-comparable.

- [ ] **Step 2: Run focused tests and verify failure**

Run: `go test ./internal/tutorial ./internal/storage/postgres`

Expected: failure because P8 and Context Pack fields are not declared.

- [ ] **Step 3: Implement the exact P8 contract**

Add the two integer fields and make P1–P7 reject manifest-supplied Context Pack values. Initialize every runtime definition with `contextPackTopN: 5` and `contextPackMaxTokens: 6000`; override only P8 from the exact manifest candidate. Include both fields in run creation, public output, definition fingerprint/matching and legacy baseline handling.

Extend SQL columns, insert arguments and scanner in lockstep. Migration up adds non-null `context_pack_top_n INTEGER NOT NULL DEFAULT 0` and `context_pack_max_tokens INTEGER NOT NULL DEFAULT 0`; migration down drops both columns. Require baseline `5/6000`, P1–P7 `5/6000`, and P8 `3/6000` in comparison validation.

- [ ] **Step 4: Run focused tests and verify pass**

Run: `go test ./internal/tutorial ./internal/storage/postgres`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tutorial internal/storage/postgres migrations/000037_tutorial_p8_context_pack_candidate.sql
git commit -m "feat: declare tutorial P8 context pack contract"
```

### Task 2: Wire evaluator v5 and prove Context Pack isolation

**Files:**
- Modify: `internal/app/app.go`
- Test: `internal/rag/context_pack_test.go`, `internal/app/app_test.go` or the focused existing app tests

**Interfaces:**
- Consumes the persisted P8 candidate ID from Task 1.
- Produces a P0–P7 tutorial evaluator with `rag.ContextPacker{TopN: 5, MaxTokens: 6000}` and a P8 evaluator clone with `TopN: 3, MaxTokens: 6000}`.

- [ ] **Step 1: Add failing packer behavior tests**

Construct five distinct `kb.SearchResult` values and assert `rag.ContextPacker{TopN: 5, MaxTokens: 6000}` emits five citations while `{TopN: 3, MaxTokens: 6000}` emits exactly the first three. Preserve the existing content order and citation source fields.

- [ ] **Step 2: Run the focused test and verify failure**

Run: `go test ./internal/rag -run ContextPack`

Expected: failure until the explicit P8 behavior test is added.

- [ ] **Step 3: Register v5 evaluators**

In `app.New`, set the baseline tutorial RAG Packer explicitly to `rag.ContextPacker{TopN: 5, MaxTokens: 6000}` after copying the production service. Build `contextPackTutorialRAG` from that clone and only set `Packer.TopN = 3`. Register it under `TutorialP8ContextPackCandidateID`; update `RuntimeEnvironment.EvaluatorVersion` to `tutorial_eval_v5`.

Do not configure a P8 ingestor: its manifest requires P0-index reuse and must enter evaluation directly. Preserve explicit hybrid retriever, disabled cache/Pipeline/router/rewrite/HyDE, disabled GraphBuilder/RAPTOR ingestion, P5 multi-query, P6 rerank and P7 graph evaluator wiring.

- [ ] **Step 4: Run application and packer tests**

Run: `go test ./internal/rag ./internal/app ./internal/tutorial`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go internal/rag/context_pack_test.go internal/tutorial
git commit -m "feat: run tutorial P8 on fixed context pack"
```

### Task 3: Expose P8 and verify the controlled walkthrough

**Files:**
- Modify: `api/openapi.yaml`, `console/src/api/schema.d.ts`, `console/src/features/tutorials/tutorial-experiment-workbench.tsx`, `console/src/test/handlers.ts`
- Modify: `console/e2e/real-backend-tutorial-clone.spec.ts`, `scripts/console-real-backend-tutorial-clone-e2e.sh`
- Create: `tests/fixtures/tutorial-packs/text-rag/1.0.8/quick/manifest.json`, `tests/fixtures/tutorial-packs/text-rag/1.0.8/quick/corpus/service.json`
- Test: Console typecheck, unit tests, build and real tutorial clone E2E

**Interfaces:**
- OpenAPI exposes read-only `context_pack_top_n` and `context_pack_max_tokens` on tutorial variants and runs.
- Console renders immutable Context Pack values and P8's P0-index-reuse explanation without a tuning control.

- [ ] **Step 1: Add API and Console P8 fixtures**

Add P8 to mock variants/runs/comparisons with shared P0 KB, `3/6000` facts and a completed standard evaluation. Add a P8 Console assertion for the displayed context package and the “复用兼容 P0 索引” stage.

- [ ] **Step 2: Update public schemas and UI**

Add the two read-only integer properties, regenerate `schema.d.ts`, show them in variant cards and run audit, map P8 title/description/button, and never add client parameters.

- [ ] **Step 3: Create the immutable fixture and real-browser assertion**

Copy only the P7 fixture assets into version `1.0.8`, update manifest version/license labels and append exact P8 candidate JSON. Temporarily map only the fixture in the existing harness, run P0 then P8, and assert `Context Pack` displays `3 / 6000`, P0 index reuse, and a comparable result.

- [ ] **Step 4: Validate surfaces**

Run:

```bash
make openapi-validate
cd console && npm run typecheck && npm test -- --run && npm run build
cd .. && make console-real-tutorial-clone-e2e
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add api console scripts tests/fixtures
git commit -m "feat: expose tutorial P8 context pack walkthrough"
```

### Task 4: Document, publish, merge and deploy

**Files:**
- Create: `docs/tutorials/p8-context-pack-candidate.md`, `docs-site/tutorials/p8-context-pack.html`
- Modify: `docs-site/index.html`, `README.md`, `ROADMAP.md`, `CHANGELOG.md`

- [ ] **Step 1: Write documentation**

Explain direct P0 lineage, P0 `5/6000` versus P8 `3/6000`, reused P0 index, evaluator-v5 isolation and the absence of quality/cost/latency claims. Update roadmap remaining scope so P8 is complete and retain only real unresolved roadmap items.

- [ ] **Step 2: Build and check hosted artifacts**

Run: `./scripts/build-docs-site.sh /tmp/orag-docs-p8`

Expected: output includes `tutorials/p8-context-pack.html` and `openapi.yaml` with both Context Pack fields.

- [ ] **Step 3: Run final backend checks**

Run:

```bash
go test ./...
make vet
git diff --check
```

Expected: PASS and no whitespace errors.

- [ ] **Step 4: Publish and merge**

Push `codex/tutorial-p8-context-pack-candidate`, create a draft PR, wait for every required GitHub Actions check, mark ready, merge through the GitHub API, and fast-forward local `main` to the merge commit.

- [ ] **Step 5: Deploy and verify docs**

Build from merged `main`, compare local and remote SHA-256 before extraction, atomically switch `/var/www/orag-docs`, reload Nginx, then curl-check `https://www.tensorbytes.com/orag/`, the P8 page and OpenAPI fields. Remove the merged P8 worktree and branch.
