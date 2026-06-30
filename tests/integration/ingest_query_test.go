package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/ingest"
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
