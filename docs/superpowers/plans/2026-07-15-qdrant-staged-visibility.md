# Qdrant Staged Visibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Prevent failed or partially activated ingestions from entering dense or sparse retrieval while preserving historical Qdrant compatibility and the previously active source version.

**Architecture:** PostgreSQL remains the authoritative chunk-visibility store. CompositeIndexer coordinates store, prepare, commit, abort, and finalize phases. Qdrant carries staged payload state, while dense retrieval validates every Qdrant candidate against searchable PostgreSQL chunks before returning it.

**Tech Stack:** Go 1.26, pgx v5, Qdrant gRPC client v1.11, PostgreSQL advisory transaction locks, Go testing, Docker Compose integration stack.

## Global Constraints

- Public HTTP and Go SDK ingestion signatures do not change.
- Historical Qdrant points without searchable or ingestion_job_id remain eligible for PostgreSQL authorization.
- PostgreSQL is the only authority for whether a chunk may enter retrieval.
- Visibility lookup errors fail closed; unchecked Qdrant candidates are never returned.
- Pre-commit failures produce failed jobs; post-commit cleanup failures produce succeeded jobs with warnings.
- New Qdrant points record searchable=false and ingestion_job_id at store time.
- The previously active source version remains searchable until PostgreSQL activation commits.
- Real PostgreSQL + Qdrant integration tests are required before merge.

---

### Task 1: Introduce the activation participant protocol

**Files:**
- Modify: internal/kb/composite.go
- Modify: internal/kb/types.go
- Modify: internal/kb/store_test.go
- Modify: internal/ingest/service_test.go

**Interfaces:**
- Produces: kb.ActivationParticipant
- Produces: kb.PostCommitCleanupWarning
- Produces: kb.ErrNonTransactionalCompositeIndexer
- Consumes: kb.Indexer, kb.Document, kb.Chunk

- [x] **Step 1: Write failing phase-order and abort tests**

Add this participant double to internal/kb/store_test.go:

~~~go
type recordingActivationParticipant struct {
    name        string
    events      *[]string
    storeErr    error
    prepareErr  error
    commitErr   error
    abortErr    error
    finalizeErr error
}

func (p recordingActivationParticipant) Store(context.Context, Document, []Chunk) error {
    *p.events = append(*p.events, p.name+":store")
    return p.storeErr
}
func (p recordingActivationParticipant) PrepareActivation(context.Context, Document, []Chunk) error {
    *p.events = append(*p.events, p.name+":prepare")
    return p.prepareErr
}
func (p recordingActivationParticipant) CommitActivation(context.Context, Document, []Chunk) error {
    *p.events = append(*p.events, p.name+":commit")
    return p.commitErr
}
func (p recordingActivationParticipant) AbortActivation(context.Context, Document, []Chunk) error {
    *p.events = append(*p.events, p.name+":abort")
    return p.abortErr
}
func (p recordingActivationParticipant) FinalizeActivation(context.Context, Document, []Chunk) error {
    *p.events = append(*p.events, p.name+":finalize")
    return p.finalizeErr
}
~~~

Test exact success order:

~~~go
func TestCompositeIndexerRunsActivationPhasesInOrder(t *testing.T) {
    events := []string{}
    indexer := CompositeIndexer{Indexers: []Indexer{
        recordingActivationParticipant{name: "postgres", events: &events},
        recordingActivationParticipant{name: "qdrant", events: &events},
    }}
    err := indexer.Store(context.Background(), Document{ID: "doc_1"}, []Chunk{{ID: "chk_1"}})
    if err != nil { t.Fatal(err) }
    want := []string{
        "postgres:store", "qdrant:store",
        "postgres:prepare", "qdrant:prepare",
        "postgres:commit", "qdrant:commit",
        "postgres:finalize", "qdrant:finalize",
    }
    if !reflect.DeepEqual(events, want) { t.Fatalf("events = %#v, want %#v", events, want) }
}
~~~

Add this fixed test matrix:

| Test | Setup | Required assertion |
| --- | --- | --- |
| TestCompositeIndexerAbortsStoredParticipantsInReverseOrder | qdrant prepareErr is prepare failed | errors.Is returns the sentinel and the final events are qdrant:abort, postgres:abort |
| TestCompositeIndexerJoinsAbortErrors | postgres abortErr is abort failed and qdrant commitErr is commit failed | errors.Is matches both sentinels |
| TestCompositeIndexerReturnsPostCommitCleanupWarning | qdrant finalizeErr is cleanup failed | errors.As returns *PostCommitCleanupWarning and no abort event exists |
| TestCompositeIndexerRejectsNonTransactionalIndexer | first indexer is a participant and second only implements Store | error is ErrNonTransactionalCompositeIndexer and events is empty |

- [x] **Step 2: Run the tests and verify RED**

Run:

~~~bash
go test ./internal/kb -run 'TestCompositeIndexer(RunsActivationPhasesInOrder|AbortsStoredParticipantsInReverseOrder|ReturnsPostCommitCleanupWarning|RejectsNonTransactionalIndexer)' -v
~~~

Expected: FAIL because ActivationParticipant, phased callbacks, and PostCommitCleanupWarning do not exist.

- [x] **Step 3: Implement the protocol**

Replace ActivatingIndexer in internal/kb/composite.go with:

~~~go
var ErrNonTransactionalCompositeIndexer = errors.New("composite indexer requires activation participants")

type ActivationParticipant interface {
    Indexer
    PrepareActivation(context.Context, Document, []Chunk) error
    CommitActivation(context.Context, Document, []Chunk) error
    AbortActivation(context.Context, Document, []Chunk) error
    FinalizeActivation(context.Context, Document, []Chunk) error
}

type PostCommitCleanupWarning struct{ Err error }

func (w *PostCommitCleanupWarning) Error() string {
    return "post-commit index cleanup failed: " + w.Err.Error()
}
func (w *PostCommitCleanupWarning) Unwrap() error { return w.Err }
~~~

CompositeIndexer.Store must preflight every non-nil indexer, stage participants, run each phase across all participants, abort stored participants in reverse order after store/prepare/commit failure, join errors with errors.Join, and wrap finalization errors in PostCommitCleanupWarning.

Move MemoryStore.Activate behavior to CommitActivation. Implement no-op prepare/finalize and AbortActivation that removes only pending candidate entries. Update the staged store and indexer doubles in internal/ingest/service_test.go to implement all participant methods.

- [x] **Step 4: Verify GREEN**

~~~bash
go test ./internal/kb ./internal/ingest -run 'TestCompositeIndexer|TestIngestFailedComposite|TestIngestSuccessfulComposite' -v
~~~

Expected: PASS. Failed replacements preserve the previous memory-store version.

- [x] **Step 5: Commit Task 1**

~~~bash
git add internal/kb/composite.go internal/kb/types.go internal/kb/store_test.go internal/ingest/service_test.go
git commit -m "feat(kb): coordinate staged index activation"
~~~

### Task 2: Make PostgreSQL the authoritative visibility participant

**Files:**
- Modify: internal/kb/types.go
- Modify: internal/storage/postgres/repository.go
- Modify: internal/storage/postgres/repository_test.go

**Interfaces:**
- Produces: kb.SearchableChunkFilter.FilterSearchableChunkIDs
- Implements: kb.ActivationParticipant on *postgres.Repository
- Consumes: knowledgeBaseTx and knowledgeBaseQueryer

- [x] **Step 1: Write failing visibility and activation tests**

Add this interface to internal/kb/types.go:

~~~go
type SearchableChunkFilter interface {
    FilterSearchableChunkIDs(
        ctx context.Context,
        tenantID string,
        knowledgeBaseID string,
        chunkIDs []string,
    ) (map[string]struct{}, error)
}
~~~

Add TestRepositoryFilterSearchableChunkIDsIsTenantScoped with fake rows chk_1 and chk_3. Assert chk_2 is absent and SQL contains all four fragments:

~~~go
for _, fragment := range []string{
    "tenant_id=$1",
    "knowledge_base_id=$2",
    "searchable",
    "id = ANY($3)",
} {
    if !strings.Contains(queryer.querySQL, fragment) {
        t.Fatalf("query missing %q: %s", fragment, queryer.querySQL)
    }
}
~~~

Add this fixed test matrix:

| Test | Fake transaction result | Required assertion |
| --- | --- | --- |
| TestRepositoryCommitActivationLocksSourceBeforeMutation | candidate exists | execSQLs[0] contains pg_advisory_xact_lock and its argument is tenant_1 NUL kb_1 NUL memory://doc.md |
| TestRepositoryCommitActivationScopesCandidate | candidate exists | candidate SQL contains tenant_id=$1, knowledge_base_id=$2, id=$3 |
| TestRepositoryCommitActivationDeletesOldAndActivatesCandidate | candidate exists | old deletes retain content_hash<>$4, update contains searchable=TRUE, Commit called once |
| TestRepositoryCommitActivationRejectsMissingCandidate | candidate does not exist | errors.Is matches kb.ErrActivationCandidateMissing, no delete/update SQL, Rollback called once |
| TestRepositoryAbortActivationDeletesOnlyPendingCandidate | transaction succeeds | first delete contains searchable=FALSE and second delete contains NOT EXISTS over chunks |
| TestRepositoryAbortActivationRollsBackOnError | first delete fails | Commit not called and Rollback called once |

Extend fakeKnowledgeBaseQueryer with queryArgs. Extend fakeKnowledgeBaseTx with a queued rows field so multiple QueryRow calls return deterministic lock/candidate results.

- [x] **Step 2: Verify RED**

~~~bash
go test ./internal/storage/postgres -run 'TestRepository(FilterSearchableChunkIDs|CommitActivation|AbortActivation)' -v
~~~

Expected: FAIL because the visibility method and participant phases are missing.

- [x] **Step 3: Implement active-chunk filtering**

Add:

~~~go
const searchableChunkIDsSQL =
    "SELECT id FROM chunks " +
    "WHERE tenant_id=$1 AND knowledge_base_id=$2 " +
    "AND searchable AND id = ANY($3)"

func (r *Repository) FilterSearchableChunkIDs(
    ctx context.Context,
    tenantID string,
    kbID string,
    chunkIDs []string,
) (map[string]struct{}, error) {
    active := make(map[string]struct{}, len(chunkIDs))
    if len(chunkIDs) == 0 { return active, nil }
    rows, err := r.knowledgeBaseQueryer().Query(ctx, searchableChunkIDsSQL, tenantID, kbID, chunkIDs)
    if err != nil { return nil, err }
    defer rows.Close()
    for rows.Next() {
        var id string
        if err := rows.Scan(&id); err != nil { return nil, err }
        active[id] = struct{}{}
    }
    if err := rows.Err(); err != nil { return nil, err }
    return active, nil
}
~~~

- [x] **Step 4: Implement PostgreSQL phases**

Define kb.ErrActivationCandidateMissing. Keep Store staged. PrepareActivation and FinalizeActivation return nil.

CommitActivation must:

1. begin one transaction;
2. execute SELECT pg_advisory_xact_lock(hashtextextended($1, 0)) with tenantID + NUL + kbID + NUL + sourceURI;
3. verify the candidate document with tenant and KB scope;
4. return ErrActivationCandidateMissing when absent;
5. delete older source versions using the current content-hash guard;
6. update the candidate chunks to searchable=TRUE;
7. commit.

AbortActivation uses exactly these predicates:

~~~sql
DELETE FROM chunks
WHERE tenant_id=$1 AND knowledge_base_id=$2
  AND document_id=$3 AND searchable=FALSE;

DELETE FROM documents d
WHERE d.tenant_id=$1 AND d.knowledge_base_id=$2 AND d.id=$3
  AND NOT EXISTS (
    SELECT 1 FROM chunks c
    WHERE c.tenant_id=d.tenant_id
      AND c.knowledge_base_id=d.knowledge_base_id
      AND c.document_id=d.id
  );
~~~

This preserves active chunks reused by an idempotent re-ingestion.

- [x] **Step 5: Verify GREEN**

~~~bash
go test ./internal/storage/postgres -run 'TestRepository(StoreStaged|FilterSearchableChunkIDs|CommitActivation|AbortActivation)' -v
~~~

Expected: PASS with tenant scope, advisory serialization, and rollback assertions.

- [x] **Step 6: Commit Task 2**

~~~bash
git add internal/kb/types.go internal/storage/postgres/repository.go internal/storage/postgres/repository_test.go
git commit -m "feat(postgres): authorize active ingestion chunks"
~~~

### Task 3: Stage and phase Qdrant vector mutations

**Files:**
- Modify: internal/storage/qdrant/client.go
- Modify: internal/storage/qdrant/payload.go
- Modify: internal/storage/qdrant/vector_store.go
- Modify: internal/storage/qdrant/vector_store_test.go
- Modify: internal/storage/qdrant/semantic_cache_test.go
- Modify: tests/integration/ingest_query_test.go

**Interfaces:**
- Extends: qdrantstore.PointsClient.SetPayload
- Implements: kb.ActivationParticipant on qdrantstore.VectorStore
- Produces payload keys: searchable and ingestion_job_id

- [x] **Step 1: Write failing payload and phase tests**

Record Upsert and SetPayload requests. Assert Store sends searchable=false and the chunk ingestion job ID:

~~~go
point := points.upsertReq.GetPoints()[0]
if point.GetPayload()["searchable"].GetBoolValue() {
    t.Fatal("staged point is searchable")
}
if got := point.GetPayload()["ingestion_job_id"].GetStringValue(); got != "job_1" {
    t.Fatalf("ingestion_job_id = %q", got)
}
~~~

Add this fixed test matrix:

| Test | Call | Required assertion |
| --- | --- | --- |
| TestPrepareActivationMarksDocumentSearchable | PrepareActivation | one SetPayload request, searchable=true, tenant/KB/document filters present |
| TestAbortActivationMarksDocumentUnsearchable | AbortActivation | one SetPayload request, searchable=false, tenant/KB/document filters present |
| TestCommitActivationDoesNotMutateQdrant | CommitActivation | no Upsert, SetPayload, or Delete requests |
| TestFinalizeActivationDeletesPreviousSourcePoints | FinalizeActivation | Delete filter contains tenant/KB/source and must-not document_id=doc_new |

Add SetPayload to every PointsClient fake, including failingPointsClient in integration tests.

- [x] **Step 2: Verify RED**

~~~bash
go test ./internal/storage/qdrant -run 'Test(VectorStoreStoresStagedPayload|PrepareActivation|AbortActivation|CommitActivation|FinalizeActivation)' -v
~~~

Expected: FAIL because payload fields, SetPayload, and phase methods are missing.

- [x] **Step 3: Extend the client and payload**

Add to PointsClient:

~~~go
SetPayload(
    ctx context.Context,
    in *qdrant.SetPayloadPoints,
    opts ...grpc.CallOption,
) (*qdrant.PointsOperationResponse, error)
~~~

Add boolValue and these fields to chunkPayload while retaining every existing field:

~~~go
"ingestion_job_id": stringValue(chunk.IngestionJobID),
"searchable":        boolValue(false),
~~~

- [x] **Step 4: Implement Qdrant phases**

Add a documentFilter scoped by tenant ID, knowledge-base ID, and document ID. Use:

~~~go
func (s VectorStore) setDocumentSearchable(
    ctx context.Context,
    doc kb.Document,
    searchable bool,
) error {
    wait := true
    _, err := s.Client.Points.SetPayload(ctx, &qdrant.SetPayloadPoints{
        CollectionName: s.Collection,
        Wait:           &wait,
        Payload:        map[string]*qdrant.Value{"searchable": boolValue(searchable)},
        PointsSelector: &qdrant.PointsSelector{
            PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
                Filter: documentFilter(doc.TenantID, doc.KnowledgeBaseID, doc.ID),
            },
        },
    })
    return err
}
~~~

Prepare maps to true, Abort maps to false, Commit is a no-op, and the old Activate source cleanup becomes FinalizeActivation.

- [x] **Step 5: Verify GREEN**

~~~bash
go test ./internal/storage/qdrant -run 'Test(VectorStoreStoresStagedPayload|PrepareActivation|AbortActivation|CommitActivation|FinalizeActivation|PayloadRoundTrip)' -v
~~~

Expected: PASS. All mutations are tenant, KB, and document/source scoped.

- [x] **Step 6: Commit Task 3**

~~~bash
git add internal/storage/qdrant/client.go internal/storage/qdrant/payload.go internal/storage/qdrant/vector_store.go internal/storage/qdrant/vector_store_test.go internal/storage/qdrant/semantic_cache_test.go tests/integration/ingest_query_test.go
git commit -m "feat(qdrant): stage vector visibility state"
~~~

### Task 4: Enforce the PostgreSQL barrier in dense retrieval

**Files:**
- Modify: internal/storage/qdrant/vector_store.go
- Modify: internal/storage/qdrant/vector_store_test.go

**Interfaces:**
- Consumes: kb.SearchableChunkFilter
- Produces: bounded, paged, fail-closed VectorStore.Retrieve

- [x] **Step 1: Write failing retrieval tests**

Create fixedSearchableChunkFilter with active IDs, a sentinel error, and recorded calls. Make the points fake return pages based on SearchPoints.GetOffset.

Add tests for:

- inactive candidates removed;
- true, false, and missing Qdrant payload states all require PostgreSQL authorization;
- nil visibility returns ErrVisibilityFilterRequired;
- filter error returns no results and preserves errors.Is;
- an inactive first page causes a second Qdrant page;
- scanning stops at max(limit*8, 256).

- [x] **Step 2: Verify RED**

~~~bash
go test ./internal/storage/qdrant -run 'TestRetrieve(RequiresVisibilityFilter|FiltersInactiveCandidates|FailsClosed|PagesForActiveResults|StopsAtScanCap)' -v
~~~

Expected: FAIL because retrieval returns unchecked candidates.

- [x] **Step 3: Implement bounded authorization**

Add:

~~~go
var ErrVisibilityFilterRequired =
    errors.New("qdrant vector retrieval requires a searchable chunk filter")

type VectorStore struct {
    Client     *Client
    Collection string
    Visibility kb.SearchableChunkFilter
}
~~~

Use pageSize=max(limit*2, 32), scanCap=max(limit*8, 256), and Qdrant Offset. For each page, extract IDs, call FilterSearchableChunkIDs once, append only authorized points in score order, and set ranks after filtering. Stop when TopK active points are collected, Qdrant is exhausted, or the cap is reached. Return no candidates when visibility lookup fails.

- [x] **Step 4: Verify GREEN and race safety**

~~~bash
go test ./internal/storage/qdrant -run TestRetrieve -v
go test -race ./internal/storage/qdrant
~~~

Expected: PASS. Offsets increase monotonically and never cross the cap.

- [x] **Step 5: Commit Task 4**

~~~bash
git add internal/storage/qdrant/vector_store.go internal/storage/qdrant/vector_store_test.go
git commit -m "feat(qdrant): authorize dense candidates in postgres"
~~~

### Task 5: Preserve job semantics and wire production dependencies

**Files:**
- Modify: internal/ingest/service.go
- Modify: internal/ingest/service_test.go
- Modify: internal/app/app.go
- Modify: internal/app/app_test.go

**Interfaces:**
- Consumes: *kb.PostCommitCleanupWarning
- Wires: qdrantstore.VectorStore.Visibility = repo

- [x] **Step 1: Write failing job and wiring tests**

Have an indexer return &kb.PostCommitCleanupWarning{Err: cleanupErr}. Assert Service.Ingest returns no error, result and stored job statuses are succeeded, document/chunk counts are present, and job.Error contains the cleanup warning.

Write TestBuildKnowledgeBackendWiresVectorVisibility against the wished-for helper; the helper does not exist until Step 4:

~~~go
func TestBuildKnowledgeBackendWiresVectorVisibility(t *testing.T) {
    client := &qdrantstore.Client{}
    repo := &postgres.Repository{}
    got := newPostgresVectorStore(client, "chunks", repo)
    if got.Client != client { t.Fatal("client was not preserved") }
    if got.Collection != "chunks" { t.Fatalf("collection = %q", got.Collection) }
    if got.Visibility != repo { t.Fatal("postgres visibility filter was not wired") }
}
~~~

The test calls the helper with a repository pointer and asserts the returned Visibility is that exact pointer, the client is unchanged, and the collection is unchanged.

- [x] **Step 2: Verify RED**

~~~bash
go test ./internal/ingest ./internal/app -run 'Test(IngestPostCommitCleanupWarningSucceeds|BuildKnowledgeBackendWiresVectorVisibility)' -v
~~~

Expected: FAIL because every indexer error currently fails the job and visibility is not wired.

- [x] **Step 3: Handle cleanup warnings**

Use:

~~~go
var indexWarnings []string
if err := s.Indexer.Store(ctx, doc, chunks); err != nil {
    var cleanupWarning *kb.PostCommitCleanupWarning
    if !errors.As(err, &cleanupWarning) {
        return fail(err)
    }
    indexWarnings = append(indexWarnings, cleanupWarning.Error())
}
~~~

Include indexWarnings with contextual, RAPTOR, and graph warnings before persisting the succeeded job.

- [x] **Step 4: Wire one vector-store value everywhere**

Implement newPostgresVectorStore, then make buildKnowledgeBackend use its return value for the composite indexer, dense retriever, and vector deleter:

~~~go
func newPostgresVectorStore(
    client *qdrantstore.Client,
    collection string,
    repo *postgres.Repository,
) qdrantstore.VectorStore {
    return qdrantstore.VectorStore{
        Client: client,
        Collection: collection,
        Visibility: repo,
    }
}
~~~

- [x] **Step 5: Verify GREEN**

~~~bash
go test ./internal/ingest ./internal/app -run 'Test(IngestPostCommitCleanupWarningSucceeds|BuildKnowledgeBackendWiresVectorVisibility|IngestFailedComposite)' -v
~~~

Expected: PASS. Pre-commit errors still fail; cleanup warnings succeed.

- [x] **Step 6: Commit Task 5**

~~~bash
git add internal/ingest/service.go internal/ingest/service_test.go internal/app/app.go internal/app/app_test.go
git commit -m "fix(ingest): distinguish committed cleanup warnings"
~~~

### Task 6: Prove the protocol with real PostgreSQL and Qdrant

**Files:**
- Modify: tests/integration/ingest_query_test.go
- Modify: tests/integration/helpers_test.go

**Interfaces:**
- Consumes: real PostgreSQL, real Qdrant, deterministic mock embedder
- Proves: failure invisibility, successful replacement, legacy compatibility, cleanup warnings, serialized replacement

- [x] **Step 1: Add integration participants and helpers**

Add:

~~~go
type failingCommitRepository struct {
    *postgres.Repository
    err error
}
func (r failingCommitRepository) CommitActivation(
    context.Context,
    kb.Document,
    []kb.Chunk,
) error {
    return r.err
}
~~~

Add these exact helpers:

~~~go
func integrationQueryVector(t *testing.T, ctx context.Context, app *core.App, text string) []float64
func integrationVectorStore(app *core.App, repo *postgres.Repository) qdrantstore.VectorStore
func countSearchableSourceChunks(t *testing.T, ctx context.Context, app *core.App, kbID, sourceURI string) int
func denseDocumentIDs(t *testing.T, ctx context.Context, store qdrantstore.VectorStore, req kb.SearchRequest) []string
func setQdrantDocumentPayload(t *testing.T, ctx context.Context, app *core.App, kbID, documentID string, payload map[string]*qdrant.Value)
func deleteQdrantDocumentPayloadKey(t *testing.T, ctx context.Context, app *core.App, kbID, documentID, key string)
~~~

integrationQueryVector calls app.Ingest.Embedder.Embed with one text and fails unless exactly one vector is returned. Every Qdrant payload helper filters by testTenantID, KB ID, and document ID and sets Wait=true.

- [x] **Step 2: Write the failing commit test**

TestFailedPostgresActivationDoesNotExposePreparedQdrantVectors performs:

1. successful old-source ingest;
2. same-source replacement using real Qdrant plus failingCommitRepository;
3. failed job assertion;
4. dense lookup with old and new embeddings;
5. no failed document ID in dense results;
6. old document remains authorized;
7. sparse retrieval returns only the old version.

Run:

~~~bash
make test-integration-up
ORAG_INTEGRATION_TESTS=1 DATABASE_URL="postgres://orag:orag@localhost:55432/orag_test?sslmode=disable" QDRANT_HOST=localhost QDRANT_GRPC_PORT=6634 QDRANT_COLLECTION=orag_chunks_test QDRANT_SEMANTIC_CACHE_COLLECTION=orag_semantic_cache_test go test ./tests/integration -run TestFailedPostgresActivationDoesNotExposePreparedQdrantVectors -v
~~~

Expected: FAIL before the full protocol is connected.

- [x] **Step 3: Add success and legacy tests**

Add:

- TestSuccessfulReplacementExposesOnlyPostgresAuthorizedVersion
- TestLegacyQdrantPointRequiresActivePostgresChunk

The legacy test removes searchable from a real point, proves it remains retrievable with an active PostgreSQL chunk, then makes that chunk inactive and proves dense retrieval filters the orphan.

- [x] **Step 4: Add cleanup-warning and concurrency tests**

Wrap a real vector participant so FinalizeActivation returns a sentinel error after real prepare. Assert ingestion succeeds with a warning and retrieval returns only the committed version.

Run two same-source replacements concurrently. Assert exactly one source document has searchable chunks and every dense result belongs to that active document. The winner is lock-acquisition order, not request creation time.

- [x] **Step 5: Verify the full integration package**

~~~bash
make test-integration
~~~

Expected: PASS for all ingestion, query, deletion, auth, and visibility tests.

- [x] **Step 6: Stop services and commit Task 6**

~~~bash
make test-integration-down
git add tests/integration/ingest_query_test.go tests/integration/helpers_test.go
git commit -m "test: prove cross-store ingestion visibility"
~~~

### Task 7: Document, validate, publish, and merge

**Files:**
- Modify: docs/architecture/rag-pipeline.md
- Modify: docs/operations/troubleshooting.md
- Modify: CHANGELOG.md
- Modify: ROADMAP.md
- Modify: ROADMAP_EN.md
- Modify: docs/superpowers/specs/2026-07-15-qdrant-staged-visibility-design.md
- Modify: docs/superpowers/plans/2026-07-15-qdrant-staged-visibility.md

**Interfaces:**
- Documents: visibility invariant, cleanup-warning behavior, operator diagnosis
- Publishes: an implementation PR closing issue #175 only after gates pass

- [x] **Step 1: Update project truth**

Document:

- PostgreSQL chunks.searchable authorizes dense and sparse results.
- Qdrant payload state is diagnostic, never sufficient authorization.
- Visibility lookup errors fail dense queries closed.
- A succeeded job can contain a post-commit cleanup warning.
- Operators inspect job ID, PostgreSQL chunk state, and Qdrant payload together.

Add an Unreleased changelog entry. Add a Stage 3 progress note to both Roadmaps linking issue #175 and the design without claiming the stage is complete. Set the spec status to Implemented and verified only after integration passes. Check completed plan boxes.

- [x] **Step 2: Run formatting and focused gates**

~~~bash
gofmt -w internal/kb internal/storage/postgres internal/storage/qdrant internal/ingest internal/app tests/integration
git diff --check
go test ./internal/kb ./internal/storage/postgres ./internal/storage/qdrant ./internal/ingest ./internal/app
go test -race ./internal/kb ./internal/ingest ./internal/storage/qdrant
~~~

Expected: all PASS and no formatting diff after gofmt.

- [x] **Step 3: Run repository-wide gates**

~~~bash
make agent-gate
npm --prefix console test -- --run
npm --prefix console run build
~~~

Expected: all PASS, including SDK consumer, OpenAPI, agent artifacts, and Console compatibility.

- [x] **Step 4: Re-run integration from a clean stack**

~~~bash
make test-integration-down
make test-integration-up
make test-integration
make test-integration-down
~~~

Expected: all PASS and no test containers or volumes left running.

- [x] **Step 5: Commit documentation**

~~~bash
git add CHANGELOG.md ROADMAP.md ROADMAP_EN.md docs/architecture/rag-pipeline.md docs/operations/troubleshooting.md docs/superpowers/specs/2026-07-15-qdrant-staged-visibility-design.md docs/superpowers/plans/2026-07-15-qdrant-staged-visibility.md
git commit -m "docs: record staged visibility guarantees"
~~~

- [ ] **Step 6: Rebase and publish**

~~~bash
git fetch origin
git rebase origin/main
git push -u origin codex/qdrant-staged-visibility
gh pr create --base main --head codex/qdrant-staged-visibility --title "fix: prevent failed ingestion vector visibility"
~~~

Use gh pr edit to add the actual test commands and Closes #175. Mark ready only after required checks pass.

- [ ] **Step 7: Merge and prove final state**

~~~bash
gh pr checks --watch
gh pr merge --squash --delete-branch
git switch main
git pull --ff-only origin main
git fetch --prune
git rev-list --left-right --count main...origin/main
git status --short --branch
~~~

Expected: checks PASS, PR merged, main...origin/main is 0 0, and the only allowed untracked path is .superpowers/.
