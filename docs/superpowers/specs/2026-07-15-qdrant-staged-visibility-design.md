# Qdrant Staged Visibility Design

**Status:** Approved design direction

**Roadmap:** Stage 3 — production-pilot baseline / data consistency and execution safety

**Issue:** [#175 Failed ingestion can expose Qdrant vectors](https://github.com/shikanon/orag/issues/175)

## Summary

ORAG will make PostgreSQL the authoritative visibility boundary for ingested chunks while keeping Qdrant as the dense candidate index. New Qdrant points are staged with `searchable=false`, activation is split into prepare, commit, and finalize phases, and every dense result is checked against searchable PostgreSQL chunks before it can enter RAG context.

This protocol preserves existing Qdrant points that do not have a `searchable` payload, does not require a stop-the-world backfill, and prevents a failed or partially activated ingestion from becoming queryable through either sparse or dense retrieval.

## Problem

The production backend composes PostgreSQL and Qdrant in `kb.CompositeIndexer`. PostgreSQL stages chunks with `searchable=false`, but Qdrant currently upserts immediately searchable points. The composite then activates PostgreSQL and Qdrant independently.

That creates four correctness failures:

1. Qdrant points can be returned before the PostgreSQL activation transaction commits.
2. A PostgreSQL activation failure can leave dense vectors visible for a failed ingestion job.
3. Replacing a source can expose a mixture of old and new document versions.
4. Returning an error after one store has committed incorrectly labels visible data as a failed ingestion.

The required invariant is:

> A chunk may enter sparse or dense retrieval only when its PostgreSQL row is `searchable=true`. A failed ingestion must leave the previously active source version queryable and the failed version unqueryable.

## Goals

- Stage every newly written Qdrant point as not searchable.
- Preserve query compatibility with historical Qdrant points that have no `searchable` payload.
- Use one authoritative, tenant-scoped read barrier for dense and sparse visibility.
- Keep the previously active source version available until the new version commits.
- Make activation failures fail closed without requiring distributed transactions.
- Distinguish pre-commit failures from post-commit cleanup failures in ingestion job state.
- Prove the protocol with unit, concurrency, and real PostgreSQL + Qdrant integration tests.

## Non-goals

- A general distributed transaction framework.
- Qdrant collection-per-document or collection alias switching.
- A full persistent cleanup-job subsystem; retryable cross-store cleanup is tracked separately by issue #177.
- Changing the public HTTP or Go SDK ingestion contract.
- Backfilling every existing Qdrant point during deployment.

## Considered Approaches

### A. Qdrant staging plus PostgreSQL visibility barrier — selected

Qdrant stages new points, while PostgreSQL decides whether any dense candidate is active. The protocol adds one batched PostgreSQL lookup to dense retrieval and explicit activation phases.

This is selected because it closes the cross-store visibility gap without a disruptive migration and retains a single source of truth for active data.

### B. Staging plus compensating writes

On activation failure, ORAG would reset or delete Qdrant points. This is smaller, but a point can be visible before compensation runs and compensation can also fail. It does not satisfy the invariant under concurrent queries.

### C. Versioned Qdrant collections and aliases

Collection alias changes are useful for whole-index replacement, but ORAG activates individual tenant, knowledge-base, and source versions. Per-source collections would create excessive collections and operational overhead.

## Architecture

### PostgreSQL remains the source of truth

PostgreSQL already stores the document, chunk, tenant, knowledge base, source, and `searchable` state. It remains authoritative. Qdrant never independently decides that a chunk is safe to place in RAG context.

The `kb` package will define a narrow visibility interface:

```go
type SearchableChunkFilter interface {
	FilterSearchableChunkIDs(
		ctx context.Context,
		tenantID string,
		knowledgeBaseID string,
		chunkIDs []string,
	) (map[string]struct{}, error)
}
```

The PostgreSQL repository implements this with one tenant- and knowledge-base-scoped query over the candidate IDs and `searchable=true`. The Qdrant vector store requires this dependency when used as a retriever.

### Explicit activation participant protocol

The current single `Activate` callback cannot express cross-store ordering or post-commit cleanup. It will be replaced internally by an activation participant contract:

```go
type ActivationParticipant interface {
	Indexer
	PrepareActivation(ctx context.Context, doc Document, chunks []Chunk) error
	CommitActivation(ctx context.Context, doc Document, chunks []Chunk) error
	AbortActivation(ctx context.Context, doc Document, chunks []Chunk) error
	FinalizeActivation(ctx context.Context, doc Document, chunks []Chunk) error
}
```

Participants may implement a phase as a no-op, but the phase meanings are fixed:

- `Store`: persist a non-visible candidate version.
- `PrepareActivation`: make external data ready for commit without making it authoritative.
- `CommitActivation`: commit the authoritative PostgreSQL visibility change.
- `AbortActivation`: remove or hide the candidate after any pre-commit failure.
- `FinalizeActivation`: remove obsolete external data after the authoritative commit.

`CompositeIndexer` calls each phase for all participants before moving to the next phase. It records successfully stored participants and aborts them in reverse order after a store, prepare, or commit failure.

### Participant responsibilities

PostgreSQL:

- `Store` writes the new chunks as `searchable=false` and does not delete the active source version.
- `PrepareActivation` is a no-op.
- `CommitActivation` runs one transaction that removes the prior source version and sets the new document chunks to `searchable=true`.
- `AbortActivation` deletes only unsearchable chunks for the candidate document and removes the document row only when it has no searchable chunks.
- `FinalizeActivation` is a no-op.

Qdrant:

- `Store` upserts the new points with `searchable=false` and their `ingestion_job_id`.
- `PrepareActivation` sets `searchable=true` for points belonging to the candidate document, but does not delete prior source versions.
- `CommitActivation` is a no-op because Qdrant is not authoritative.
- `AbortActivation` resets the candidate document points to `searchable=false`; it does not delete them because an idempotent re-ingestion can reuse the currently active chunk IDs.
- `FinalizeActivation` deletes points for earlier documents with the same tenant, knowledge base, and source URI.

## Write and Activation Flow

1. The ingestion service creates a running job and builds the document and chunks.
2. PostgreSQL and Qdrant store the candidate version as non-visible.
3. Qdrant prepares the candidate by setting its payload to `searchable=true`; old points remain intact.
4. PostgreSQL commits the source replacement and marks the candidate chunks searchable in one transaction.
5. Qdrant finalizes by deleting old points for the same source.
6. The ingestion job becomes `succeeded`.

Before step 4, the PostgreSQL barrier rejects the prepared Qdrant candidates. After step 4, the barrier accepts the new candidates and rejects old candidates, even if step 5 has not completed.

This gives queries a single switch point: the PostgreSQL transaction commit.

## Dense Retrieval Flow

1. Qdrant searches with the tenant and knowledge-base filters. The payload state is not used as the authoritative search filter.
2. ORAG extracts candidate chunk IDs and calls `FilterSearchableChunkIDs` once for that batch.
3. PostgreSQL is the only authority: it may accept a point marked `true`, `false`, or missing when the matching chunk row is currently searchable.
4. Candidates absent from the returned active set are discarded before ranking or RAG context packing.
5. Retrieval continues through bounded Qdrant pages until it collects the requested active result count, Qdrant is exhausted, or the scan cap is reached.

The bounded paging policy is:

- Page size: `max(requestedLimit*2, 32)`.
- Scan cap: `max(requestedLimit*8, 256)` candidates.
- If the cap is reached, return the active results collected so far rather than fail open.

A visibility lookup error fails dense retrieval. Returning unchecked Qdrant results is prohibited.

## Legacy Compatibility

Existing Qdrant points do not contain `searchable` or `ingestion_job_id`.

- Missing `searchable` remains eligible for the PostgreSQL visibility check.
- PostgreSQL must still confirm that the corresponding chunk is searchable.
- Historical orphan Qdrant points without an active PostgreSQL chunk are filtered out.
- No synchronous backfill is required for rollout.
- New writes always include both payload fields, making future cleanup and diagnostics attributable to an ingestion job.

An idempotent re-ingestion may reuse the same deterministic document and chunk IDs. Its Qdrant upsert can temporarily set an already active point to `searchable=false`, but retrieval remains available because the matching PostgreSQL chunk is still searchable. If the re-ingestion fails, abort leaves the payload false while PostgreSQL continues to authorize the active chunk; a later successful prepare restores the diagnostic payload to true.

The public API, SDK, and stored PostgreSQL schema remain compatible.

## Failure Semantics

### Store or prepare failure

The composite aborts all participants that successfully stored the candidate. The ingestion job becomes `failed`. The old source remains active.

### PostgreSQL commit failure

The composite aborts the candidate. A failed Qdrant abort can leave prepared points, but the PostgreSQL barrier excludes new candidates because their chunks never became searchable. Reused active chunk IDs remain queryable because PostgreSQL still authorizes them. The ingestion job becomes `failed` and preserves all original and abort errors with `errors.Join`.

### Qdrant finalization failure

PostgreSQL has already committed, so the new version is active and the ingestion must not be labeled failed. `CompositeIndexer` returns a typed post-commit cleanup warning. The ingestion service records the warning on a `succeeded` job.

Old Qdrant points may remain temporarily, but the PostgreSQL barrier excludes them. Issue #177 will add persistent retryable cleanup so this storage leak is eventually removed.

### Visibility barrier failure

Dense retrieval returns an error. It must not return unchecked candidates. Sparse retrieval is unaffected because its SQL query already filters `searchable=true`.

## Concurrency

Two ingestions of the same source can otherwise race between prepare and commit. PostgreSQL `CommitActivation` will serialize source replacement using an advisory transaction lock derived from tenant ID, knowledge-base ID, and source URI before deleting the old version and activating the candidate.

After acquiring the lock it verifies that the candidate document and chunks still exist. A missing candidate returns a conflict error and triggers abort. Concurrent replacements may commit sequentially in lock-acquisition order, but every transaction leaves exactly one active source version and queries never observe a mixed or failed version. Choosing a winner by request creation time is outside this change.

Qdrant document IDs remain deterministic per content hash. All Qdrant mutations are scoped by tenant, knowledge base, and document ID, and source cleanup explicitly excludes the committed document ID.

## Error and Job Contract

The internal error categories are:

- pre-commit activation error: ingestion job `failed`;
- activation conflict: ingestion job `failed` with a conflict-class cause;
- post-commit cleanup warning: ingestion job `succeeded` with the warning recorded in the existing job warning/error field;
- visibility barrier error: query fails with the existing retrieval/internal error mapping and trace ID.

No HTTP success response may contain a failed job whose candidate is already authoritative, and no failed job candidate may be returned by retrieval.

## Testing Strategy

### Unit tests

- Qdrant staged payload contains `searchable=false` and `ingestion_job_id`.
- Qdrant payload states `true`, `false`, and missing all require PostgreSQL authorization before return.
- PostgreSQL active-chunk filtering is tenant and knowledge-base scoped.
- Dense retrieval discards Qdrant candidates not confirmed by PostgreSQL.
- Dense retrieval pages until it fills the requested active result count or reaches the cap.
- Visibility lookup failure returns an error and no candidates.
- Composite activation phases run in store, prepare, commit, finalize order.
- Pre-commit failure aborts successful participants in reverse order.
- Finalization failure becomes a typed post-commit warning.

### Concurrency tests

- Two replacements for the same source commit serially and leave exactly one active version.
- A query racing with Qdrant prepare and PostgreSQL commit sees either the old PostgreSQL-authorized version or the new one, never a failed candidate.

### Integration tests

Using the real PostgreSQL and Qdrant test stack:

1. Ingest and confirm an old source version through dense and sparse retrieval.
2. Stage a replacement, let Qdrant prepare, force PostgreSQL commit failure, and confirm both retrieval paths still return only the old version.
3. Run a successful replacement and confirm both paths return only the new version.
4. Insert a legacy Qdrant point without `searchable`, confirm it is returned only when PostgreSQL has a matching active chunk.
5. Force Qdrant finalization failure, confirm the job succeeds with a warning and retrieval returns only the PostgreSQL-authorized version.

### Regression gates

- `go test ./internal/kb ./internal/storage/postgres ./internal/storage/qdrant ./internal/ingest`
- `go test -race ./internal/kb ./internal/ingest`
- `go test -tags=integration ./tests/integration -run 'Test.*Ingest.*Visibility|Test.*Replacement' -v`
- `go test ./...`
- `go vet ./...`

## Rollout

1. Ship the payload and visibility barrier together; neither is safe as a standalone partial rollout.
2. Deploy without a Qdrant backfill. Historical points remain readable through the PostgreSQL barrier.
3. Monitor retrieval errors and ingestion cleanup warnings during the production pilot.
4. Use issue #177 to add durable cleanup retries before calling deletion and replacement recovery production-ready.
5. Update the Roadmap evidence only after the real integration and concurrency gates pass.

## Acceptance Criteria

- A failed ingestion cannot contribute a chunk to dense or sparse retrieval.
- A failed replacement preserves the previously active source version.
- A successful replacement exposes one PostgreSQL-authorized version across both retrieval paths.
- Existing Qdrant data remains queryable without mandatory backfill.
- Every new Qdrant point records staged visibility and ingestion-job lineage.
- Concurrent replacements commit serially, preserve one active source version, and never expose a mixed or failed version.
- Post-commit cleanup failures produce succeeded jobs with explicit warnings rather than false failed jobs.
- Unit, race, and real PostgreSQL + Qdrant integration tests prove the protocol.
