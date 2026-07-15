# Retryable Knowledge-Base Deletion Design

**Status:** Implemented and verified on 2026-07-15

**Roadmap:** Stage 3 — production-pilot baseline / data consistency and execution safety

**Issue:** [#177 KB delete leaves non-retryable orphan vectors](https://github.com/shikanon/orag/issues/177)

## Summary

ORAG will make knowledge-base deletion retryable without introducing a new persistence model. For an existing tenant-scoped knowledge base, the application deletes semantic-cache entries first, then Qdrant document vectors, and deletes PostgreSQL metadata only after every external cleanup succeeds.

The still-present PostgreSQL knowledge-base row is the durable retry token. A transient external cleanup failure returns `deleted=false` with the original error, keeps the knowledge base addressable by the same DELETE request, and allows an identical retry after a process restart.

## Problem

`knowledgeBaseStore.DeleteKnowledgeBase` currently deletes PostgreSQL metadata before Qdrant vectors and semantic-cache entries. If either external deletion fails, the API returns an error after the authoritative knowledge-base row has disappeared. A later DELETE treats the knowledge base as missing and skips cleanup, permanently orphaning external points.

The required invariant is:

> A failed knowledge-base deletion must retain a durable, tenant-scoped retry path. PostgreSQL metadata may disappear only after every configured external index cleanup has succeeded.

## Goals

- Make a repeated DELETE retry external cleanup after a transient failure.
- Keep tenant and knowledge-base scoping on every cleanup call.
- Preserve missing and wrong-tenant DELETE behavior: `deleted=false`, no cleanup calls.
- Make every cleanup step safe to invoke repeatedly.
- Return a truthful `deleted` value: true only after PostgreSQL metadata is removed.
- Prove retry behavior with unit tests and real PostgreSQL + Qdrant integration tests.

## Non-goals

- A general background cleanup-job framework.
- Hiding a knowledge base behind a deletion tombstone while cleanup is pending.
- An asynchronous DELETE API or new public deletion-status endpoint.
- Reusing a deleted knowledge-base ID concurrently with an in-flight deletion.
- Changing the public HTTP or Go SDK method signatures.

## Selected Approach

The approved approach is cleanup-before-metadata:

1. Confirm the tenant-scoped knowledge base exists.
2. Delete semantic-cache points when a semantic-cache deleter is configured.
3. Delete primary Qdrant vectors when a vector deleter is configured.
4. Delete PostgreSQL knowledge-base metadata and its relational children.
5. Return `deleted=true` only when step 4 reports success.

Semantic cache is removed before document vectors because losing cache entries only forces recomputation. If vector cleanup then fails, the still-visible knowledge base can continue through PostgreSQL sparse retrieval without replaying stale cached answers. Qdrant filter deletes are idempotent, so an unknown network outcome is handled by repeating the same scoped delete.

## Failure Semantics

| Failure point | Metadata retained | Return value | Retry behavior |
| --- | --- | --- | --- |
| Existence lookup | Yes | `false, error` | Repeat lookup and deletion |
| Semantic-cache cleanup | Yes | `false, error` | Retry semantic cache, vectors, metadata |
| Vector cleanup | Yes | `false, error` | Retry semantic cache idempotently, then vectors and metadata |
| PostgreSQL metadata delete | Yes unless the repository committed despite an error | repository `deleted`, error | Retry all external cleanup idempotently, then metadata |
| All steps succeed | No | `true, nil` | Later DELETE returns `false, nil` and performs no cleanup |

Pre-metadata cleanup errors preserve the original error for `errors.Is` and never claim that deletion succeeded. Cleanup is sequential; a failed step prevents later mutation in that attempt.

## Concurrency

Two DELETE requests for the same tenant and knowledge-base ID may both pass the existence check. Both can safely repeat external filter deletes. PostgreSQL remains the final arbiter: one request deletes the row and returns true; the other may receive false after its idempotent cleanup. No request may report true before metadata deletion succeeds.

Cross-instance create/delete coordination for immediate ID reuse is outside this issue. Callers should treat knowledge-base IDs as non-reusable identifiers.

## Operational Behavior

While cleanup is failing, the knowledge-base metadata remains visible. Semantic-cache or vector data may already be absent, so queries can temporarily recompute answers or fall back to sparse retrieval. The DELETE error is the operator signal to retry; no hidden orphan state is created.

Operators can safely resend the same tenant-scoped DELETE after restoring Qdrant connectivity. Successful retry removes PostgreSQL children, document vectors, and semantic-cache points.

## Testing Strategy

### Unit tests

- Successful deletion calls semantic cache, vectors, then metadata in exact order.
- Wrong-tenant and missing deletion call no cleanup dependency.
- First semantic-cache failure retains metadata and a second attempt retries it.
- First vector failure retains metadata and a second attempt repeats both external cleanups.
- Metadata failure occurs after external cleanup, retains metadata, and a second attempt repeats cleanup before deletion.
- Nil cleanup dependencies are skipped without changing metadata semantics.
- Cleanup errors preserve `errors.Is` and return `deleted=false`.

### Integration tests

With real PostgreSQL and Qdrant:

1. Create a knowledge base and ingest vectors.
2. Wrap the vector deleter to fail once without deleting.
3. Confirm the first DELETE returns an error, PostgreSQL metadata remains, and Qdrant points remain.
4. Retry DELETE through the same application store.
5. Confirm metadata, relational children, document vectors, and semantic-cache points are gone.

## Rollout

This is an internal ordering and return-semantics change with no schema migration. Existing clients already handle DELETE errors. After rollout, clients should interpret `deleted=false, error` as retryable when the cause is a transient external cleanup failure.

## Acceptance Criteria

- A transient Qdrant vector or semantic-cache cleanup failure cannot remove the only retry handle.
- Repeating the same DELETE invokes failed cleanup again and can complete successfully.
- `deleted=true` is returned only after PostgreSQL metadata deletion succeeds.
- Missing and wrong-tenant DELETE requests remain side-effect free.
- Real PostgreSQL + Qdrant tests prove orphan points are removed by retry.
