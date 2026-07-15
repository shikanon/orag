package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	qdrant "github.com/qdrant/go-client/qdrant"
	core "github.com/shikanon/orag/internal/app"
	oraghttp "github.com/shikanon/orag/internal/http"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
	qdrantstore "github.com/shikanon/orag/internal/storage/qdrant"
	"google.golang.org/grpc"
)

type failingCommitRepository struct {
	*postgres.Repository
	err       error
	storedDoc kb.Document
}

func (r *failingCommitRepository) Store(ctx context.Context, doc kb.Document, chunks []kb.Chunk) error {
	r.storedDoc = doc
	return r.Repository.Store(ctx, doc, chunks)
}

func (r *failingCommitRepository) CommitActivation(context.Context, kb.Document, []kb.Chunk) error {
	return r.err
}

type failingFinalizeVectorStore struct {
	qdrantstore.VectorStore
	err error
}

func (s failingFinalizeVectorStore) FinalizeActivation(ctx context.Context, doc kb.Document, chunks []kb.Chunk) error {
	return errors.Join(s.VectorStore.FinalizeActivation(ctx, doc, chunks), s.err)
}

func TestFailedPostgresActivationDoesNotExposePreparedQdrantVectors(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	kbID := createIntegrationKnowledgeBase(t, ctx, app, "failed-activation")
	source := "integration://replace-failed"
	oldContent := "old source remains authoritative after failed postgres activation"
	old := ingestIntegrationDocument(t, ctx, app, kbID, source, oldContent)

	repo := postgres.NewRepository(app.Postgres)
	repo.StageChunks = true
	commitErr := errors.New("forced postgres activation failure")
	failingRepo := &failingCommitRepository{Repository: repo, err: commitErr}
	vectors := integrationVectorStore(app, repo)
	app.Ingest.Indexer = kb.CompositeIndexer{Indexers: []kb.Indexer{failingRepo, vectors}}
	failedContent := "new source must remain invisible after failed postgres activation"
	failed, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID: testTenantID, KnowledgeBaseID: kbID, SourceURI: source,
		Name: "replacement.md", Content: []byte(failedContent),
	})
	if !errors.Is(err, commitErr) || failed.Job.Status != ingest.JobStatusFailed {
		t.Fatalf("failed ingest = %#v, %v", failed.Job, err)
	}
	failedDocID := failingRepo.storedDoc.ID
	for _, query := range []string{oldContent, failedContent} {
		ids := denseDocumentIDs(t, ctx, vectors, kb.SearchRequest{
			TenantID: testTenantID, KnowledgeBaseID: kbID, Vector: integrationQueryVector(t, ctx, app, query), TopK: 8,
		})
		if containsString(ids, failedDocID) {
			t.Fatalf("failed document %s is dense-visible: %#v", failedDocID, ids)
		}
	}
	oldIDs := denseDocumentIDs(t, ctx, vectors, kb.SearchRequest{
		TenantID: testTenantID, KnowledgeBaseID: kbID, Vector: integrationQueryVector(t, ctx, app, oldContent), TopK: 8,
	})
	if !containsString(oldIDs, old.Document.ID) {
		t.Fatalf("old document is not dense-visible: %#v", oldIDs)
	}
	sparse, err := postgres.NewFTSRetriever(repo).Retrieve(ctx, kb.SearchRequest{
		TenantID: testTenantID, KnowledgeBaseID: kbID, Query: "old source remains authoritative", TopK: 8,
	})
	if err != nil || len(sparse) == 0 || sparse[0].Chunk.DocumentID != old.Document.ID {
		t.Fatalf("sparse old version = %#v, %v", sparse, err)
	}
}

func TestSuccessfulReplacementExposesOnlyPostgresAuthorizedVersion(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	kbID := createIntegrationKnowledgeBase(t, ctx, app, "successful-replacement")
	source := "integration://replace-success"
	old := ingestIntegrationDocument(t, ctx, app, kbID, source, "old successful replacement content")
	current := ingestIntegrationDocument(t, ctx, app, kbID, source, "new successful replacement content")
	repo := postgres.NewRepository(app.Postgres)
	store := integrationVectorStore(app, repo)

	ids := denseDocumentIDs(t, ctx, store, kb.SearchRequest{
		TenantID: testTenantID, KnowledgeBaseID: kbID,
		Vector: integrationQueryVector(t, ctx, app, "new successful replacement content"), TopK: 8,
	})
	if len(ids) == 0 || !containsString(ids, current.Document.ID) || containsString(ids, old.Document.ID) {
		t.Fatalf("replacement dense IDs = %#v, old=%s current=%s", ids, old.Document.ID, current.Document.ID)
	}
	if got := countSearchableSourceChunks(t, ctx, app, kbID, source); got != len(current.Chunks) {
		t.Fatalf("searchable source chunks = %d, want %d", got, len(current.Chunks))
	}
}

func TestLegacyQdrantPointRequiresActivePostgresChunk(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	kbID := createIntegrationKnowledgeBase(t, ctx, app, "legacy-point")
	content := "legacy qdrant payload remains postgres authorized"
	result := ingestIntegrationDocument(t, ctx, app, kbID, "integration://legacy", content)
	repo := postgres.NewRepository(app.Postgres)
	store := integrationVectorStore(app, repo)
	deleteQdrantDocumentPayloadKey(t, ctx, app, kbID, result.Document.ID, "searchable")
	req := kb.SearchRequest{TenantID: testTenantID, KnowledgeBaseID: kbID, Vector: integrationQueryVector(t, ctx, app, content), TopK: 8}
	if ids := denseDocumentIDs(t, ctx, store, req); !containsString(ids, result.Document.ID) {
		t.Fatalf("legacy active point filtered: %#v", ids)
	}
	if _, err := app.Postgres.Exec(ctx, `UPDATE chunks SET searchable=FALSE WHERE tenant_id=$1 AND knowledge_base_id=$2 AND document_id=$3`, testTenantID, kbID, result.Document.ID); err != nil {
		t.Fatal(err)
	}
	if ids := denseDocumentIDs(t, ctx, store, req); containsString(ids, result.Document.ID) {
		t.Fatalf("legacy inactive point passed barrier: %#v", ids)
	}
}

func TestQdrantFinalizationFailureSucceedsWithWarning(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	kbID := createIntegrationKnowledgeBase(t, ctx, app, "cleanup-warning")
	source := "integration://cleanup-warning"
	_ = ingestIntegrationDocument(t, ctx, app, kbID, source, "old cleanup warning content")
	repo := postgres.NewRepository(app.Postgres)
	repo.StageChunks = true
	vectors := integrationVectorStore(app, repo)
	cleanupErr := errors.New("forced qdrant finalization failure")
	app.Ingest.Indexer = kb.CompositeIndexer{Indexers: []kb.Indexer{repo, failingFinalizeVectorStore{VectorStore: vectors, err: cleanupErr}}}
	current, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID: testTenantID, KnowledgeBaseID: kbID, SourceURI: source,
		Name: "cleanup.md", Content: []byte("new cleanup warning content"),
	})
	if err != nil || current.Job.Status != ingest.JobStatusSucceeded || !strings.Contains(current.Job.Error, cleanupErr.Error()) {
		t.Fatalf("cleanup warning ingest = %#v, %v", current.Job, err)
	}
	ids := denseDocumentIDs(t, ctx, vectors, kb.SearchRequest{
		TenantID: testTenantID, KnowledgeBaseID: kbID,
		Vector: integrationQueryVector(t, ctx, app, "new cleanup warning content"), TopK: 8,
	})
	if len(ids) == 0 || !containsString(ids, current.Document.ID) {
		t.Fatalf("committed document missing after cleanup warning: %#v", ids)
	}
}

func TestConcurrentReplacementsLeaveOneDenseVisibleVersion(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	kbID := createIntegrationKnowledgeBase(t, ctx, app, "concurrent-replacement")
	source := "integration://concurrent-replacement"
	_ = ingestIntegrationDocument(t, ctx, app, kbID, source, "initial concurrent source content")
	contents := []string{"concurrent replacement alpha", "concurrent replacement beta"}
	type outcome struct {
		result ingest.Result
		err    error
	}
	outcomes := make(chan outcome, len(contents))
	var start sync.WaitGroup
	start.Add(1)
	for _, content := range contents {
		content := content
		go func() {
			start.Wait()
			result, err := app.Ingest.Ingest(ctx, ingest.Request{
				TenantID: testTenantID, KnowledgeBaseID: kbID, SourceURI: source,
				Name: "concurrent.md", Content: []byte(content),
			})
			outcomes <- outcome{result: result, err: err}
		}()
	}
	start.Done()
	for range contents {
		outcome := <-outcomes
		if outcome.err != nil || outcome.result.Job.Status != ingest.JobStatusSucceeded {
			t.Fatalf("concurrent ingest = %#v, %v", outcome.result.Job, outcome.err)
		}
	}
	var activeDocID string
	if err := app.Postgres.QueryRow(ctx, `
		SELECT document_id FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND source_uri=$3 AND searchable
		GROUP BY document_id`, testTenantID, kbID, source).Scan(&activeDocID); err != nil {
		t.Fatal(err)
	}
	repo := postgres.NewRepository(app.Postgres)
	store := integrationVectorStore(app, repo)
	visible := 0
	for _, content := range contents {
		ids := denseDocumentIDs(t, ctx, store, kb.SearchRequest{
			TenantID: testTenantID, KnowledgeBaseID: kbID, Vector: integrationQueryVector(t, ctx, app, content), TopK: 8,
		})
		for _, id := range ids {
			if id != activeDocID {
				t.Fatalf("dense result %s is not active %s: %#v", id, activeDocID, ids)
			}
			visible++
		}
	}
	if visible == 0 {
		t.Fatalf("active document %s has no dense point after concurrent replacement", activeDocID)
	}
}

func createIntegrationKnowledgeBase(t *testing.T, ctx context.Context, app *core.App, suffix string) string {
	t.Helper()
	kbID := fmt.Sprintf("kb_%s_%d", suffix, time.Now().UnixNano())
	now := time.Now().UTC()
	if err := app.KBStore.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID: kbID, TenantID: testTenantID, Name: suffix, Description: "temporary integration test",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = app.KBStore.DeleteKnowledgeBase(context.Background(), testTenantID, kbID) })
	return kbID
}

func ingestIntegrationDocument(t *testing.T, ctx context.Context, app *core.App, kbID, sourceURI, content string) ingest.Result {
	t.Helper()
	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID: testTenantID, KnowledgeBaseID: kbID, SourceURI: sourceURI,
		Name: "integration.md", Content: []byte(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.Status != ingest.JobStatusSucceeded {
		t.Fatalf("ingest job = %#v", result.Job)
	}
	return result
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestIngestQueryWithPostgresQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	marker := fmt.Sprintf("oragint%d", time.Now().UnixNano())
	content := fmt.Sprintf("The marker %s describes ORAG using Qdrant vector retrieval and PostgreSQL sparse retrieval.", marker)
	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        testTenantID,
		KnowledgeBaseID: testKBID,
		SourceURI:       "integration://" + marker,
		Name:            marker + ".md",
		Content:         []byte(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.Status != ingest.JobStatusSucceeded || result.Job.DocumentID != result.Document.ID || result.Job.ChunkCount == 0 {
		t.Fatalf("unexpected ingest job: %#v", result.Job)
	}

	job, ok, err := app.Ingest.Jobs.GetJob(ctx, testTenantID, result.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || job.DocumentID != result.Document.ID || job.ChunkCount == 0 {
		t.Fatalf("stored job ok=%v job=%#v", ok, job)
	}

	req := rag.QueryRequest{
		TenantID:        testTenantID,
		KnowledgeBaseID: testKBID,
		Query:           "Which retrieval stores are described by marker " + marker + "?",
		Profile:         rag.ProfileRealtime,
		TopK:            8,
	}
	resp, err := app.RAG.Query(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answer == "" || resp.TraceID == "" {
		t.Fatalf("unexpected query response: %#v", resp)
	}
	if !retrievedDocument(resp, result.Document.ID) {
		t.Fatalf("retrieved chunks do not contain document %s: %#v", result.Document.ID, resp.RetrievedChunks)
	}
	if len(resp.Citations) > 0 && !citedDocument(resp, result.Document.ID) {
		t.Fatalf("citations do not contain document %s: %#v", result.Document.ID, resp.Citations)
	}

	cached, err := app.RAG.Query(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Answer == "" || cached.TraceID == "" {
		t.Fatalf("unexpected cached query response: %#v", cached)
	}
	if cached.CacheStatus == "hit" && len(cached.Citations) == 0 {
		t.Fatalf("cache hit did not replay citations: %#v", cached)
	}
}

func TestIngestMissingKnowledgeBaseWithPostgresQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        testTenantID,
		KnowledgeBaseID: "kb_missing",
		SourceURI:       "integration://missing-kb",
		Name:            "missing.md",
		Content:         []byte("missing knowledge bases must not reach postgres foreign keys"),
	})
	if !errors.Is(err, ingest.ErrKnowledgeBaseNotFound) {
		t.Fatalf("Ingest() error = %v, want ErrKnowledgeBaseNotFound", err)
	}
	if result.Job.ID != "" {
		t.Fatalf("unexpected job for missing knowledge base: %#v", result.Job)
	}
}

func TestFailedQdrantIngestKeepsPostgresChunksUnsearchable(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	kbID := fmt.Sprintf("kb_failed_qdrant_%d", time.Now().UnixNano())
	now := time.Now().UTC()
	if err := app.KBStore.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:          kbID,
		TenantID:    testTenantID,
		Name:        "integration failed qdrant",
		Description: "temporary integration test knowledge base",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = app.KBStore.DeleteKnowledgeBase(context.Background(), testTenantID, kbID) })

	repo := postgres.NewRepository(app.Postgres)
	repo.StageChunks = true
	qdrantErr := errors.New("qdrant upsert failed")
	vectors := qdrantstore.VectorStore{
		Client:     &qdrantstore.Client{Points: failingPointsClient{err: qdrantErr}},
		Collection: app.Config.Qdrant.Collection,
	}
	app.Ingest.Indexer = kb.CompositeIndexer{Indexers: []kb.Indexer{repo, vectors}}

	marker := fmt.Sprintf("failedqdrantmarker%d", time.Now().UnixNano())
	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        testTenantID,
		KnowledgeBaseID: kbID,
		SourceURI:       "integration://" + marker,
		Name:            marker + ".md",
		Content:         []byte("The marker " + marker + " must remain hidden after a failed qdrant write."),
	})
	if !errors.Is(err, qdrantErr) {
		t.Fatalf("Ingest() error = %v, want %v", err, qdrantErr)
	}
	if result.Job.Status != ingest.JobStatusFailed {
		t.Fatalf("job status = %q", result.Job.Status)
	}
	job, ok, err := app.Ingest.Jobs.GetJob(ctx, testTenantID, result.Job.ID)
	if err != nil || !ok {
		t.Fatalf("job lookup ok=%v err=%v", ok, err)
	}
	if job.Status != ingest.JobStatusFailed {
		t.Fatalf("stored job status = %q", job.Status)
	}
	if count := countPostgresRows(t, ctx, app, "chunks", kbID); count != 0 {
		t.Fatalf("staged postgres chunks after qdrant rollback = %d, want 0", count)
	}
	if count := countSearchablePostgresChunks(t, ctx, app, kbID); count != 0 {
		t.Fatalf("searchable postgres chunks after qdrant failure = %d", count)
	}

	results, err := postgres.NewFTSRetriever(repo).Retrieve(ctx, kb.SearchRequest{
		TenantID:        testTenantID,
		KnowledgeBaseID: kbID,
		Query:           marker,
		TopK:            8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("failed ingest chunks are searchable through postgres FTS: %#v", results)
	}
}

func TestHTTPIngestMissingKnowledgeBaseWithPostgresQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h := oraghttp.NewServer(app).Hertz().Engine
	token := loginHTTPToken(t, h, app.Config.Auth.AdminDefaultUsername, app.Config.Auth.AdminDefaultPassword)
	missingKB := fmt.Sprintf("kb_missing_http_%d", time.Now().UnixNano())

	status, body := performIntegrationJSON(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents:import", `{"name":"missing.md","source_uri":"integration://missing-http","content":"missing knowledge bases must return 404"}`, token)
	assertMissingKBHTTPResponse(t, status, body)
	assertNoPostgresIngestRows(t, ctx, app, missingKB)

	status, body = performIntegrationUpload(t, h, "/v1/knowledge-bases/"+missingKB+"/documents", "missing.md", "missing knowledge bases must return 404", token)
	assertMissingKBHTTPResponse(t, status, body)
	assertNoPostgresIngestRows(t, ctx, app, missingKB)
}

func TestDeleteKnowledgeBaseCleansPostgresAndQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	kbID := fmt.Sprintf("kb_delete_%d", time.Now().UnixNano())
	now := time.Now().UTC()
	if err := app.KBStore.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:          kbID,
		TenantID:    testTenantID,
		Name:        "integration delete",
		Description: "temporary integration test knowledge base",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        testTenantID,
		KnowledgeBaseID: kbID,
		SourceURI:       "integration://" + kbID,
		Name:            kbID + ".md",
		Content:         []byte("This marker verifies knowledge base deletion clears vectors and rows."),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.ChunkCount == 0 {
		t.Fatalf("expected chunks before delete: %#v", result.Job)
	}
	if count := countPostgresRows(t, ctx, app, "chunks", kbID); count == 0 {
		t.Fatal("expected postgres chunks before delete")
	}
	if count := countQdrantPoints(t, ctx, app, kbID); count == 0 {
		t.Fatal("expected qdrant points before delete")
	}
	cacheVector := make([]float64, app.Config.Ark.EmbeddingDimensions)
	cacheVector[0] = 0.42
	if err := app.RAG.Cache.Store(ctx, rag.SemanticCacheEntry{
		TenantID:        testTenantID,
		KnowledgeBaseID: kbID,
		Query:           "cache cleanup " + kbID,
		Vector:          cacheVector,
		Profile:         rag.ProfileRealtime,
		TopK:            8,
		Response: rag.QueryResponse{
			Answer:  "cached answer for deleted knowledge base",
			Profile: rag.ProfileRealtime,
			RetrievedChunks: []kb.SearchResult{{
				Chunk: kb.Chunk{
					ID:              "cached_" + kbID,
					TenantID:        testTenantID,
					KnowledgeBaseID: kbID,
					DocumentID:      result.Document.ID,
					Content:         "cached deleted content",
				},
			}},
		},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if count := countQdrantSemanticCachePoints(t, ctx, app, kbID); count == 0 {
		t.Fatal("expected qdrant semantic cache points before delete")
	}
	datasetID := "ds_" + kbID
	optimizationRunID := "opt_" + kbID
	candidateID := "cand_" + kbID
	harnessID := "harness_" + kbID
	if _, err := app.Postgres.Exec(ctx, `
		INSERT INTO datasets(id, tenant_id, name, kind, version)
		VALUES($1, $2, $3, $4, $5)`,
		datasetID, testTenantID, "delete integration dataset", "golden", kbID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Postgres.Exec(ctx, `
		INSERT INTO optimization_runs(id, tenant_id, dataset_id, knowledge_base_id, status)
		VALUES($1, $2, $3, $4, $5)`,
		optimizationRunID, testTenantID, datasetID, kbID, "queued"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Postgres.Exec(ctx, `
		INSERT INTO optimization_candidates(id, optimization_run_id, config, status)
		VALUES($1, $2, '{}'::jsonb, $3)`,
		candidateID, optimizationRunID, "queued"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Postgres.Exec(ctx, `
		INSERT INTO harness_runs(id, tenant_id, candidate_id, harness_type)
		VALUES($1, $2, $3, $4)`,
		harnessID, testTenantID, candidateID, "integration"); err != nil {
		t.Fatal(err)
	}
	if count := countOptimizationRuns(t, ctx, app, kbID); count == 0 {
		t.Fatal("expected optimization runs before delete")
	}
	if count := countOptimizationCandidates(t, ctx, app, kbID); count == 0 {
		t.Fatal("expected optimization candidates before delete")
	}
	if count := countHarnessRunsForCandidate(t, ctx, app, candidateID); count == 0 {
		t.Fatal("expected harness runs before delete")
	}

	deleted, err := app.KBStore.DeleteKnowledgeBase(ctx, testTenantID, kbID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase deleted=false, want true")
	}
	if _, ok, err := app.KBStore.GetKnowledgeBase(context.Background(), testTenantID, kbID); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("deleted knowledge base is still readable")
	}
	for _, table := range []string{"chunks", "documents", "ingestion_jobs", "knowledge_bases"} {
		if count := countPostgresRows(t, ctx, app, table, kbID); count != 0 {
			t.Fatalf("%s rows after delete = %d", table, count)
		}
	}
	if count := countOptimizationCandidates(t, ctx, app, kbID); count != 0 {
		t.Fatalf("optimization_candidates rows after delete = %d", count)
	}
	if count := countOptimizationRuns(t, ctx, app, kbID); count != 0 {
		t.Fatalf("optimization_runs rows after delete = %d", count)
	}
	if count := countHarnessRunsForCandidate(t, ctx, app, candidateID); count != 0 {
		t.Fatalf("harness_runs rows after delete = %d", count)
	}
	if count := countQdrantPoints(t, ctx, app, kbID); count != 0 {
		t.Fatalf("qdrant points after delete = %d", count)
	}
	if count := countQdrantSemanticCachePoints(t, ctx, app, kbID); count != 0 {
		t.Fatalf("qdrant semantic cache points after delete = %d", count)
	}
}

func countPostgresRows(t *testing.T, ctx context.Context, app *core.App, table, kbID string) int {
	t.Helper()
	var where string
	switch table {
	case "chunks", "documents", "ingestion_jobs":
		where = "tenant_id=$1 AND knowledge_base_id=$2"
	case "knowledge_bases":
		where = "tenant_id=$1 AND id=$2"
	default:
		t.Fatalf("unexpected table %q", table)
	}
	var count int
	if err := app.Postgres.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", table, where), testTenantID, kbID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func countOptimizationRuns(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	var count int
	if err := app.Postgres.QueryRow(ctx, `
		SELECT count(*)
		FROM optimization_runs
		WHERE tenant_id=$1 AND knowledge_base_id=$2`, testTenantID, kbID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func countOptimizationCandidates(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	var count int
	if err := app.Postgres.QueryRow(ctx, `
		SELECT count(*)
		FROM optimization_candidates c
		JOIN optimization_runs r ON r.id = c.optimization_run_id
		WHERE r.tenant_id=$1 AND r.knowledge_base_id=$2`, testTenantID, kbID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func countHarnessRunsForCandidate(t *testing.T, ctx context.Context, app *core.App, candidateID string) int {
	t.Helper()
	var count int
	if err := app.Postgres.QueryRow(ctx, `
		SELECT count(*)
		FROM harness_runs
		WHERE tenant_id=$1 AND candidate_id=$2`, testTenantID, candidateID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func countQdrantPoints(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	return countQdrantPointsInCollection(t, ctx, app, app.Config.Qdrant.Collection, kbID)
}

func countQdrantSemanticCachePoints(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	return countQdrantPointsInCollection(t, ctx, app, app.Config.Qdrant.SemanticCacheCollection, kbID)
}

func countQdrantPointsInCollection(t *testing.T, ctx context.Context, app *core.App, collection, kbID string) int {
	t.Helper()
	exact := true
	resp, err := app.Qdrant.Points.Count(ctx, &qdrant.CountPoints{
		CollectionName: collection,
		Filter:         integrationKnowledgeBaseFilter(kbID),
		Exact:          &exact,
	})
	if err != nil {
		t.Fatal(err)
	}
	return int(resp.GetResult().GetCount())
}

func countSearchablePostgresChunks(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	var count int
	if err := app.Postgres.QueryRow(ctx, `
		SELECT count(*) FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND searchable`,
		testTenantID, kbID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func integrationKnowledgeBaseFilter(kbID string) *qdrant.Filter {
	return &qdrant.Filter{Must: []*qdrant.Condition{
		integrationMatchKeyword("tenant_id", testTenantID),
		integrationMatchKeyword("knowledge_base_id", kbID),
	}}
}

func integrationMatchKeyword(key, value string) *qdrant.Condition {
	return &qdrant.Condition{ConditionOneOf: &qdrant.Condition_Field{Field: &qdrant.FieldCondition{
		Key: key,
		Match: &qdrant.Match{MatchValue: &qdrant.Match_Keyword{
			Keyword: value,
		}},
	}}}
}

func retrievedDocument(resp rag.QueryResponse, documentID string) bool {
	for _, result := range resp.RetrievedChunks {
		if result.Chunk.DocumentID == documentID {
			return true
		}
	}
	return false
}

type failingPointsClient struct {
	err error
}

func (c failingPointsClient) Upsert(context.Context, *qdrant.UpsertPoints, ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	return nil, c.err
}

func (c failingPointsClient) SetPayload(context.Context, *qdrant.SetPayloadPoints, ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	return nil, c.err
}

func (c failingPointsClient) Search(context.Context, *qdrant.SearchPoints, ...grpc.CallOption) (*qdrant.SearchResponse, error) {
	return nil, c.err
}

func (c failingPointsClient) Delete(context.Context, *qdrant.DeletePoints, ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	return nil, c.err
}

func (c failingPointsClient) Count(context.Context, *qdrant.CountPoints, ...grpc.CallOption) (*qdrant.CountResponse, error) {
	return nil, c.err
}

func loginHTTPToken(t *testing.T, h *route.Engine, username, password string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		t.Fatal(err)
	}
	status, resp := performIntegrationJSON(h, "POST", "/v1/auth/login", string(body), "")
	if status != 200 {
		t.Fatalf("login status = %d body=%s", status, resp)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.AccessToken == "" {
		t.Fatal("missing access token")
	}
	return parsed.AccessToken
}

func performIntegrationJSON(h *route.Engine, method, path, body, token string) (int, string) {
	headers := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	var reqBody *ut.Body
	if body != "" {
		reqBody = &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
	}
	w := ut.PerformRequest(h, method, path, reqBody, headers...)
	result := w.Result()
	return result.StatusCode(), string(result.Body())
}

func performIntegrationUpload(t *testing.T, h *route.Engine, path, filename, content, token string) (int, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	headers := []ut.Header{{Key: "Content-Type", Value: writer.FormDataContentType()}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	w := ut.PerformRequest(h, "POST", path, &ut.Body{Body: bytes.NewReader(body.Bytes()), Len: body.Len()}, headers...)
	result := w.Result()
	return result.StatusCode(), string(result.Body())
}

func assertMissingKBHTTPResponse(t *testing.T, status int, body string) {
	t.Helper()
	if status != 404 {
		t.Fatalf("missing knowledge base status = %d body=%s", status, body)
	}
	if !strings.Contains(body, `"code":"knowledge_base_not_found"`) {
		t.Fatalf("unexpected missing knowledge base body: %s", body)
	}
	if strings.Contains(body, `"code":"ingest_failed"`) {
		t.Fatalf("missing knowledge base returned ingest_failed: %s", body)
	}
}

func assertNoPostgresIngestRows(t *testing.T, ctx context.Context, app *core.App, kbID string) {
	t.Helper()
	for _, table := range []string{"ingestion_jobs", "documents", "chunks"} {
		if count := countPostgresRows(t, ctx, app, table, kbID); count != 0 {
			t.Fatalf("%s rows for missing knowledge base %s = %d", table, kbID, count)
		}
	}
}

func citedDocument(resp rag.QueryResponse, documentID string) bool {
	for _, citation := range resp.Citations {
		if citation.DocumentID == documentID {
			return true
		}
	}
	return false
}
