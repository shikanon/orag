# Retryable Knowledge-Base Deletion Implementation Plan

> **For agentic workers:** Implement each task with test-driven development. Do not change production behavior before the task's RED test fails for the intended reason.

**Goal:** Make knowledge-base deletion safely retryable after transient Qdrant vector or semantic-cache cleanup failures.

**Architecture:** `knowledgeBaseStore` retains the tenant-scoped PostgreSQL knowledge-base row as the durable retry handle. It deletes semantic cache, then document vectors, then PostgreSQL metadata. External deletes are idempotent; `deleted=true` is returned only after metadata deletion succeeds.

**Tech Stack:** Go 1.26, Qdrant gRPC client v1.11, pgx v5, Docker Compose integration stack.

## Global Constraints

- Public HTTP and Go SDK signatures do not change.
- Missing or wrong-tenant knowledge bases cause no external cleanup.
- Cleanup failures return `deleted=false` and preserve `errors.Is`.
- PostgreSQL metadata is the final mutation in a successful deletion attempt.
- Semantic cache is cleaned before document vectors.
- Real PostgreSQL + Qdrant retry coverage is required before merge.

---

### Task 1: Reverse deletion order and prove synchronous retry

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`

**Interfaces:**
- Consumes: `kb.KnowledgeBaseRepository`
- Consumes: `knowledgeBaseVectorDeleter`
- Consumes: `knowledgeBaseSemanticCacheDeleter`

- [x] **Step 1: Write failing order and retry tests**

Replace metadata-first assertions with these fixed contracts:

| Test | Setup | Required assertion |
| --- | --- | --- |
| `TestKnowledgeBaseStoreDeleteKnowledgeBaseCleansExternalIndexesBeforeMetadata` | all dependencies succeed | calls are semantic cache, vectors, metadata and return is `true,nil` |
| `TestKnowledgeBaseStoreDeleteKnowledgeBaseRetriesVectorCleanup` | vector deleter fails once | first return is `false,error`, metadata exists; second call repeats cache/vector and deletes metadata |
| `TestKnowledgeBaseStoreDeleteKnowledgeBaseRetriesSemanticCacheCleanup` | semantic deleter fails once | first attempt stops before vectors/metadata; second completes all steps |
| `TestKnowledgeBaseStoreDeleteKnowledgeBaseRetriesAfterMetadataFailure` | metadata delete fails once | first attempt cleans both external indexes and retains metadata; second repeats both and deletes metadata |
| `TestKnowledgeBaseStoreDeleteKnowledgeBaseSkipsMissingOrWrongTenant` | repository lookup misses | no cleanup or metadata calls and return is `false,nil` |

Extend the cleanup fakes with queued errors so the same instance can fail once and then succeed.

- [x] **Step 2: Verify RED**

```bash
go test ./internal/app -run 'TestKnowledgeBaseStoreDeleteKnowledgeBase(CleansExternalIndexesBeforeMetadata|Retries|SkipsMissingOrWrongTenant)' -v
```

Expected: tests fail because metadata is currently deleted first and cleanup failures report `deleted=true`.

- [x] **Step 3: Implement cleanup-before-metadata**

`knowledgeBaseStore.DeleteKnowledgeBase` must:

1. verify tenant-scoped existence;
2. call semantic-cache deletion when configured;
3. call vector deletion when configured;
4. call `primary.DeleteKnowledgeBase`;
5. return the repository result.

Any external cleanup error returns `false, err` immediately. Do not wrap the error unless the original remains discoverable through `errors.Is`.

- [x] **Step 4: Verify GREEN and app regressions**

```bash
go test ./internal/app -run 'TestKnowledgeBaseStoreDeleteKnowledgeBase' -v
go test ./internal/app ./internal/http
```

Expected: PASS. HTTP missing/wrong-tenant semantics remain unchanged.

- [x] **Step 5: Commit Task 1**

```bash
git add internal/app/app.go internal/app/app_test.go
git commit -m "fix(app): make knowledge base deletion retryable"
```

### Task 2: Prove retry against real PostgreSQL and Qdrant

**Files:**
- Modify: `tests/integration/ingest_query_test.go`

**Interfaces:**
- Consumes: real `postgres.Repository`
- Consumes: real `qdrantstore.VectorStore`
- Proves: a failed external cleanup leaves a retryable metadata row and a retry removes all stores

- [x] **Step 1: Add a fail-once vector deleter and test**

Add a wrapper that records calls, returns a sentinel on the first `DeleteKnowledgeBaseVectors`, and delegates to the real vector store on later calls.

`TestDeleteKnowledgeBaseRetriesFailedQdrantCleanup` must:

1. create an isolated knowledge base;
2. ingest a document and semantic-cache entry;
3. construct a `knowledgeBaseStore`-equivalent deletion path through an exported/testable application helper;
4. fail the first vector cleanup;
5. assert `deleted=false`, the sentinel is preserved, PostgreSQL metadata and Qdrant points remain;
6. retry deletion;
7. assert `deleted=true` and PostgreSQL rows, Qdrant vectors, and semantic-cache points are gone.

If the current unexported facade prevents integration construction, extract a narrow exported constructor or behavior-bearing type without exposing storage-specific implementation details.

- [x] **Step 2: Verify RED**

```bash
make test-integration-up
go clean -testcache
ORAG_INTEGRATION_TESTS=1 DATABASE_URL="postgres://orag:orag@localhost:55432/orag_test?sslmode=disable" QDRANT_HOST=localhost QDRANT_GRPC_PORT=6634 QDRANT_COLLECTION=orag_chunks_test QDRANT_SEMANTIC_CACHE_COLLECTION=orag_semantic_cache_test go test ./tests/integration -run TestDeleteKnowledgeBaseRetriesFailedQdrantCleanup -v
```

Expected: FAIL before the production deletion ordering is wired into the testable facade.

- [x] **Step 3: Implement the smallest testability seam**

Prefer exercising the same facade used by `App.KBStore`. Do not duplicate the deletion algorithm in integration-only code.

- [x] **Step 4: Verify the full integration package from a clean stack**

```bash
go clean -testcache
make test-integration
make test-integration-down
```

Expected: PASS and no test containers or volumes remain.

- [x] **Step 5: Commit Task 2**

```bash
git add tests/integration/ingest_query_test.go internal/app/app.go internal/app/app_test.go
git commit -m "test: prove retryable knowledge base cleanup"
```

### Task 3: Update operator and roadmap truth

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `ROADMAP.md`
- Modify: `ROADMAP_EN.md`
- Modify: `docs/operations/troubleshooting.md`
- Modify: `docs/superpowers/specs/2026-07-15-kb-delete-retry-design.md`
- Modify: `docs/superpowers/plans/2026-07-15-kb-delete-retry.md`

- [x] **Step 1: Document behavior**

Document:

- DELETE returns an error with `deleted=false` while external cleanup is incomplete;
- the metadata row remains the durable retry handle;
- retrying the same tenant/KB DELETE is safe;
- partial cleanup can temporarily remove cache or dense vectors while sparse metadata remains;
- operators restore Qdrant and resend DELETE.

Add an Unreleased changelog entry. Add a Stage 3 progress note to both Roadmaps linking Issue #177 and the design without claiming Stage 3 complete. Set the design status to Implemented and verified only after the real integration test passes. Check completed plan boxes.

- [x] **Step 2: Commit Task 3**

```bash
git add CHANGELOG.md ROADMAP.md ROADMAP_EN.md docs/operations/troubleshooting.md docs/superpowers/specs/2026-07-15-kb-delete-retry-design.md docs/superpowers/plans/2026-07-15-kb-delete-retry.md
git commit -m "docs: record retryable knowledge base deletion"
```

### Task 4: Validate, publish, merge, and clean up

- [ ] **Step 1: Run focused and repository-wide gates**

```bash
gofmt -w internal/app tests/integration
git diff --check
go test ./internal/app ./internal/http
go test -race ./internal/app
make agent-gate
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console test -- --run
PATH="/Users/bytedance/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin:$PATH" npm --prefix console run build
```

- [ ] **Step 2: Re-run integration without cache**

```bash
go clean -testcache
make test-integration-up
make test-integration
make test-integration-down
```

- [ ] **Step 3: Rebase and publish**

```bash
git fetch origin
git rebase origin/main
git push -u origin codex/kb-delete-retry
gh pr create --base main --head codex/kb-delete-retry --title "fix: make knowledge base deletion retryable"
```

The PR body must list the actual commands and contain `Closes #177`.

- [ ] **Step 4: Merge and prove final state**

```bash
gh pr checks --watch
gh pr merge --squash --delete-branch
git -C /Users/bytedance/Documents/orag pull --ff-only origin main
git -C /Users/bytedance/Documents/orag rev-list --left-right --count main...origin/main
```

Expected: PR merged, Issue #177 closed, main is `0 0`, test services are stopped, and the feature worktree/local branch are removed without touching `.superpowers/`.
