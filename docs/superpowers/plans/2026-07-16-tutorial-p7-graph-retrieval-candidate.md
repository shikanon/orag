# P7 Graph Retrieval Tutorial Candidate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver a direct-P0 P7 candidate that builds an independent lightweight graph index and changes only retrieval with GraphRetriever.

**Architecture:** Establish evaluator v4 around the explicit hybrid retriever, preventing app graph configuration from affecting P0–P6. P7 receives a fixed GraphBuilder during its separate candidate ingest and a GraphRetriever over that candidate KB during evaluation; it has no P0 index reuse.

**Tech Stack:** Go, PostgreSQL, Qdrant, OpenAPI, React/TypeScript, Vitest, Playwright and Docker Compose.

## Global Constraints

- P7 is `p7_graph_retrieval`, direct P0-only, Basic/800/120/realtime, and keeps the same model, dataset and Top-K.
- P7 must have a distinct candidate KB and execute indexing; GraphBuilder + GraphRetriever are the only inseparable module change.
- P0–P6 evaluator v4 uses explicit hybrid retrieval, disables graph, rerank, rewrite, HyDE, cache, Pipeline and router; P5 alone retains three-query expansion.
- Browser input contains only variant and idempotency key; graph parameters and storage stay server-owned.

---

### Task 1: Persist the Graph candidate contract

**Files:** `internal/tutorial/manifest.go`, `clone.go`, `run.go`, `run_definition.go`, `comparison.go`, `internal/storage/postgres/tutorial_run.go`, migration `000036_tutorial_p7_graph_candidate.sql`, and tutorial/storage tests.

- [ ] Add failing P7 manifest validation tests for exact Basic/800/120, graph strategy, enabled graph and no P0 index reuse.
- [ ] Add `graph_retrieval_enabled` to Pack candidate, public variant, run, definition fingerprint/matching and comparison validation; make P0–P6 require false.
- [ ] Add a non-null false-default migration and extend PostgreSQL INSERT/SELECT/scan order atomically.
- [ ] Add lifecycle and comparison tests proving a direct P0 parent, different KB, normal index stage and exact graph audit values.
- [ ] Run focused Go tests and commit the contract.

### Task 2: Clamp v4 baseline and wire fixed graph ingestion/evaluation

**Files:** `internal/app/app.go`, `internal/kb/graph_test.go`, `internal/tutorial/run_test.go`, relevant app tests.

- [ ] Add a test proving GraphRetriever adds graph-expanded results on a graph-enabled candidate KB while a hybrid baseline does not call graph expansion.
- [ ] Build tutorial baseline clones from the local `hybrid` value; preserve all v3 disabled features and set evaluator version `tutorial_eval_v4`.
- [ ] Register P7 candidate ingestor with `ingest.LightweightGraphBuilder{MaxEntitiesPerChunk: cfg.RAG.GraphRetrieval.MaxEntitiesPerChunk}` and candidate evaluator `kb.GraphRetriever{Base: hybrid, Store: graphStore, TopK: cfg.RAG.GraphRetrieval.TopK}`.
- [ ] Run app/tutorial/kb tests and commit evaluator wiring.

### Task 3: API, Console and controlled fixture

**Files:** `api/openapi.yaml`, generated Console schema, tutorial workbench, handlers, real E2E/spec script, `tests/fixtures/tutorial-packs/text-rag/1.0.7/quick`.

- [ ] Add read-only graph state to schemas and Console audit output; render P7 only by server ID/chapter.
- [ ] Create the P1–P7 fixture and map it transiently in the real test; assert P0→P7 has a distinct KB, graph enabled and comparable output.
- [ ] Run Console typecheck/unit/build and real tutorial E2E; commit the client surface.

### Task 4: Documentation and publication

**Files:** P7 tutorial Markdown/static page, README, hosted index, ROADMAP and CHANGELOG.

- [ ] Document independent graph index, entity relation expansion, explicit v4 baseline and non-inference of quality/cost/latency.
- [ ] Update roadmap remaining work to P8, execute full validation, open draft PR, wait for checks, merge, deploy hosted docs, curl-verify, synchronize main and remove the merged worktree.
