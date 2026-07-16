# Text RAG Official Replay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement task-by-task.

**Goal:** Publish a server-validated, offline, read-only `text-rag` official Replay.

**Architecture:** Embed one validated snapshot in `internal/tutorial`; expose it by a public read-only tutorial endpoint; render it separately from clone/Live Run in Console. The snapshot contains aggregated metrics and hashes only.

### Task 1: Snapshot parser and catalog contract

**Files:** create `internal/tutorial/replay.go`, `internal/tutorial/replay_test.go`, `internal/tutorial/testdata/text-rag-replay-v1.json`; modify `internal/tutorial/catalog.json` and catalog tests.

- [x] Write tests that accept the canonical text snapshot and reject unknown fields, invalid SHA-256, credentials/object keys, wrong P0/P8 context-pack contract, and non-text templates.
- [x] Implement strict JSON decode, canonical SHA-256 and a `Catalog.Replay(templateID)` lookup. Keep visual/video unavailable.
- [x] Run `go test ./internal/tutorial -run Replay -count=1`; commit `feat: add validated text tutorial replay snapshot`.

### Task 2: Read-only API

**Files:** modify `internal/http/tutorials.go`, `internal/http/router.go`, `internal/http/*_test.go`, `api/openapi.yaml`.

- [x] Add `GET /v1/tutorials/{template_id}/replay` with public tutorial read semantics; return 404 for unavailable snapshots and no mutation route.
- [x] Add schema fields for snapshot identity, Pack/environment/build hashes, generated timestamp, P0/P8 metrics and index facts.
- [x] Run `go test ./internal/http ./tests/contract -run 'Tutorial.*Replay|OpenAPI'`; commit `feat: expose read-only tutorial replay`.

### Task 3: Console read-only Replay page

**Files:** modify `console/src/api/client.ts`, `console/src/features/tutorials/tutorial-detail.tsx`, routing/tests/handlers; generate `schema.d.ts`.

- [x] Replace “Replay 即将开放” with fixed-environment Replay summary when API returns a snapshot; render P0/P8 facts without clone controls.
- [x] Add MSW and component tests proving no secret/object coordinates render.
- [x] Run Console typecheck/tests/build; commit `feat: render official text tutorial replay`.

### Task 4: Documentation and release checks

**Files:** modify README/ROADMAP/API/tutorial docs and docs-site; add hosted Replay page.

- [x] Document offline, immutable and non-Live semantics; retain visual/video pending status.
- [ ] Run full Go, vet, OpenAPI, Console and hosted-doc builds; create PR, merge after CI, deploy and verify `/orag/`.
