package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	qdrant "github.com/qdrant/go-client/qdrant"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

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

func TestDeleteKnowledgeBaseCleansPostgresAndQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	kbID := fmt.Sprintf("kb_delete_%d", time.Now().UnixNano())
	now := time.Now().UTC()
	app.KBStore.PutKnowledgeBase(kb.KnowledgeBase{
		ID:          kbID,
		TenantID:    testTenantID,
		Name:        "integration delete",
		Description: "temporary integration test knowledge base",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

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

	deleted, err := app.KBStore.DeleteKnowledgeBase(ctx, testTenantID, kbID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase deleted=false, want true")
	}
	if _, ok := app.KBStore.GetKnowledgeBase(testTenantID, kbID); ok {
		t.Fatal("deleted knowledge base is still readable")
	}
	for _, table := range []string{"chunks", "documents", "ingestion_jobs", "knowledge_bases"} {
		if count := countPostgresRows(t, ctx, app, table, kbID); count != 0 {
			t.Fatalf("%s rows after delete = %d", table, count)
		}
	}
	if count := countQdrantPoints(t, ctx, app, kbID); count != 0 {
		t.Fatalf("qdrant points after delete = %d", count)
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

func countQdrantPoints(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	exact := true
	resp, err := app.Qdrant.Points.Count(ctx, &qdrant.CountPoints{
		CollectionName: app.Config.Qdrant.Collection,
		Filter:         integrationKnowledgeBaseFilter(kbID),
		Exact:          &exact,
	})
	if err != nil {
		t.Fatal(err)
	}
	return int(resp.GetResult().GetCount())
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

func citedDocument(resp rag.QueryResponse, documentID string) bool {
	for _, citation := range resp.Citations {
		if citation.DocumentID == documentID {
			return true
		}
	}
	return false
}
