# Optimizer Single-Flight Implementation Plan

> **Issue:** [#176](https://github.com/shikanon/orag/issues/176)
>
> **Design:** [Optimizer single-flight state transition design](../specs/2026-07-15-optimizer-singleflight-design.md)

**Goal:** Prevent duplicate optimizer runners and candidate executions across ORAG replicas by making execution acquisition an atomic persisted state transition.

**Architecture:** Extend the optimizer repository with compare-and-swap operations implemented by both PostgreSQL and the in-memory repository. Gate resume, run acquisition, and candidate execution on those operations, map conflicts to HTTP 409, and prove the invariant with concurrent unit and real-store tests.

**Tech stack:** Go, pgx/PostgreSQL, Hertz HTTP, OpenAPI, existing integration test stack.

---

### Task 1: Specify conflict behavior with failing tests

- [x] **Step 1: Add optimizer service concurrency tests**

Files:
- Modify: `internal/optimizer/service_test.go`

Add tests that race multiple `Resume` and `RunPending` calls against a blocking runner. Assert exactly one accepted acquisition and one runner call. Add table coverage for resumable and non-resumable run states.

Run:

```bash
go test ./internal/optimizer -run 'TestService(ConcurrentResume|ConcurrentRunPending|ResumeState)' -count=1
```

Expected: FAIL because repository CAS and conflict behavior do not exist.

- [x] **Step 2: Add HTTP conflict test**

Files:
- Modify: `internal/http/router_test.go`

Assert that a resume against a running optimization returns `409` with `optimization_state_conflict` and does not invoke a second runner.

Run:

```bash
go test ./internal/http -run TestOptimizationResumeConflict -count=1
```

Expected: FAIL with the current `202`/blind update behavior.

- [x] **Step 3: Commit the red tests**

```bash
git add internal/optimizer/service_test.go internal/http/router_test.go
git commit -m "test: expose duplicate optimizer runners"
```

### Task 2: Implement repository compare-and-swap

- [x] **Step 1: Extend the optimizer repository contract**

Files:
- Modify: `internal/optimizer/service.go`
- Modify: `internal/optimizer/memory_repository.go`
- Modify: test repositories in `internal/optimizer/service_test.go`

Add run and candidate CAS methods returning `(bool, error)`. Implement atomic status comparison under the memory repository mutex.

- [x] **Step 2: Add PostgreSQL CAS tests**

Files:
- Modify: `internal/storage/postgres/repository_test.go`

Cover expected-status predicates, tenant/run/candidate guards, one-row success, zero-row miss, and error propagation.

Run:

```bash
go test ./internal/storage/postgres -run 'TestRepositoryCompareAndSwapOptimization' -count=1
```

Expected: FAIL before the PostgreSQL methods exist.

- [x] **Step 3: Implement PostgreSQL CAS**

Files:
- Modify: `internal/storage/postgres/optimizer.go`

Reuse the existing aggregate encoding, add `status = $expected` predicates, and derive the boolean result from `RowsAffected()`.

Run:

```bash
go test ./internal/storage/postgres -run 'TestRepositoryCompareAndSwapOptimization' -count=1
```

Expected: PASS.

- [x] **Step 4: Commit repository support**

```bash
git add internal/optimizer/service.go internal/optimizer/memory_repository.go internal/optimizer/service_test.go internal/storage/postgres/optimizer.go internal/storage/postgres/repository_test.go
git commit -m "feat(optimizer): add atomic execution claims"
```

### Task 3: Gate service execution and expose HTTP conflict

- [x] **Step 1: Implement resumable-state and run acquisition gates**

Files:
- Modify: `internal/optimizer/service.go`

Allow only `failed`, `canceled`, and `budget_stopped` resume sources. CAS the selected source to `queued`, then CAS `queued` to `running` inside execution. Return an optimizer conflict sentinel wrapped with `apperrors.CodeConflict` on a disallowed state or CAS miss.

- [x] **Step 2: Implement phase-aware candidate acquisition**

Files:
- Modify: `internal/optimizer/service.go`

CAS selection candidates from `queued`/`failed` and holdout candidates from `scored` before rate limiting or external runner calls. Never reclaim `running`.

- [x] **Step 3: Map and document conflict responses**

Files:
- Modify: `internal/http/router.go`
- Modify: `internal/http/router_test.go`
- Modify: `api/openapi.yaml`

Map optimizer conflicts to `409 optimization_state_conflict` and add the response to the resume operation.

Run:

```bash
go test ./internal/optimizer ./internal/http -count=1
```

Expected: PASS, including the new race and state-table tests.

- [x] **Step 4: Run race detection and commit service behavior**

```bash
go test -race ./internal/optimizer -count=1
git add internal/optimizer/service.go internal/optimizer/service_test.go internal/http/router.go internal/http/router_test.go api/openapi.yaml
git commit -m "fix(optimizer): prevent duplicate resume runners"
```

### Task 4: Prove real PostgreSQL concurrency

- [x] **Step 1: Add a real-store integration test**

Files:
- Modify: `tests/integration/ingest_query_test.go` or the optimizer integration test file selected by the existing suite

Create one queued run and candidate, launch multiple PostgreSQL CAS claimers, and assert exactly one run winner and one candidate winner. Verify the persisted states are `running`.

- [x] **Step 2: Run targeted integration coverage**

```bash
make test-integration
```

Expected: PASS with real PostgreSQL and the existing Qdrant-backed suite.

- [x] **Step 3: Commit integration proof**

```bash
git add tests/integration
git commit -m "test: prove optimizer single-flight in postgres"
```

### Task 5: Update operator and Roadmap documentation

- [x] **Step 1: Document conflict behavior**

Files:
- Modify: `docs/operations/troubleshooting.md`
- Modify: `CHANGELOG.md`

Explain that `409` means another execution owns the run or the run is not resumable, and direct operators to fetch the current state rather than retrying aggressively.

- [x] **Step 2: Record Stage 3 progress**

Files:
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`
- Modify: `docs/superpowers/specs/2026-07-15-optimizer-singleflight-design.md`

Link Issue #176 and this design from the Stage 3 progress paragraph. Mark the design implemented only after all validation passes; keep Stage 3 explicitly incomplete.

- [x] **Step 3: Commit documentation**

```bash
git add CHANGELOG.md ROADMAP.md ROADMAP_EN.md docs/operations/troubleshooting.md docs/superpowers/specs/2026-07-15-optimizer-singleflight-design.md docs/superpowers/plans/2026-07-15-optimizer-singleflight.md
git commit -m "docs: record optimizer single-flight guarantees"
```

### Task 6: Validate, publish, merge, and clean up

- [x] **Step 1: Run repository gates**

```bash
gofmt -w internal/optimizer internal/storage/postgres internal/http tests/integration
git diff --check
go test ./internal/optimizer ./internal/storage/postgres ./internal/http -count=1
go test -race ./internal/optimizer -count=1
make agent-gate
make test-integration
```

If Console files or generated contract consumers change, also run the Console install, unit, and production build gates required by the repository.

- [x] **Step 2: Review the final diff and commit validation state**

```bash
git status --short
git diff --stat main...HEAD
git diff --check
```

Update checked boxes and validation evidence only after the commands succeed.

- [ ] **Step 3: Push and open a ready PR**

```bash
git push -u origin codex/optimizer-singleflight
gh pr create --base main --head codex/optimizer-singleflight --title "fix: make optimizer execution single-flight" --body-file /tmp/orag-pr-176.md
```

The PR body must list actual validation commands and contain `Closes #176`.

- [ ] **Step 4: Merge and prove final state**

```bash
gh pr checks --watch
gh pr merge --squash --delete-branch
git -C /Users/bytedance/Documents/orag fetch origin --prune
git -C /Users/bytedance/Documents/orag pull --ff-only origin main
git -C /Users/bytedance/Documents/orag rev-list --left-right --count main...origin/main
```

Verify Issue #176 is closed, remove only this feature worktree/local branch, and leave unrelated worktrees and `.superpowers/` untouched.
