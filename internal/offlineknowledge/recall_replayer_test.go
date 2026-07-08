package offlineknowledge

import (
	"context"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/kb"
)

func TestRetrieverRecallReplayerBaselineWithFingerprints(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	doc := kb.Document{
		ID:              "doc_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		SourceURI:       "memory://orag.md",
		Title:           "orag.md",
		ContentHash:     "doc-hash-v1",
		CreatedAt:       time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC),
	}
	if err := store.Store(ctx, doc, []kb.Chunk{
		{
			ID:              "chunk_1",
			TenantID:        "tenant_1",
			KnowledgeBaseID: "kb_1",
			DocumentID:      "doc_1",
			Content:         "ORAG is a retrieval augmented generation framework.",
			SourceURI:       doc.SourceURI,
		},
		{
			ID:              "chunk_noise",
			TenantID:        "tenant_1",
			KnowledgeBaseID: "kb_1",
			DocumentID:      "doc_1",
			Content:         "Unrelated deployment notes.",
			SourceURI:       doc.SourceURI,
		},
	}); err != nil {
		t.Fatal(err)
	}
	replayer := NewRetrieverRecallReplayer(kb.SparseRetriever{Store: store}, NewChunkSourceMetadataReader(), 5)

	got, err := replayer.ReplayRecall(ctx, replayCluster("tenant_1", "kb_1", "What is ORAG?"))
	if err != nil {
		t.Fatalf("ReplayRecall() error = %v", err)
	}
	if len(got.BaselineRecallResults) != 1 {
		t.Fatalf("baseline results = %#v, want one hit", got.BaselineRecallResults)
	}
	item := got.BaselineRecallResults[0]
	if item.ChunkID != "chunk_1" || item.DocID != "doc_1" || item.Score <= 0 || !item.Matched {
		t.Fatalf("baseline item = %#v, want real chunk/doc/score", item)
	}
	if item.DocVersion != "doc-hash-v1" {
		t.Fatalf("DocVersion = %q, want document content hash", item.DocVersion)
	}
	wantHash := "sha256:" + stableHash("ORAG is a retrieval augmented generation framework.")
	if item.ChunkContentHash != wantHash {
		t.Fatalf("ChunkContentHash = %q, want %q", item.ChunkContentHash, wantHash)
	}
	if len(got.SourceFingerprints) != 1 || got.SourceFingerprints[0].ChunkContentHash != wantHash {
		t.Fatalf("SourceFingerprints = %#v, want replay fingerprint", got.SourceFingerprints)
	}
	if got.Metadata["replay_mode"] != "baseline" || got.Metadata["query"] != "What is ORAG?" {
		t.Fatalf("metadata = %#v, want baseline replay metadata", got.Metadata)
	}
}

func TestRetrieverRecallReplayerUsesExplicitSourceMetadata(t *testing.T) {
	retriever := &staticRetriever{results: []kb.SearchResult{{
		Chunk: kb.Chunk{
			ID:              "chunk_1",
			TenantID:        "tenant_1",
			KnowledgeBaseID: "kb_1",
			DocumentID:      "doc_1",
			Content:         "ORAG supports offline knowledge optimization.",
			Metadata: map[string]string{
				"doc_version":        "v42",
				"chunk_content_hash": "sha256:precomputed",
			},
		},
		Score: 0.83,
		Rank:  1,
	}}}
	replayer := NewRetrieverRecallReplayer(retriever, NewChunkSourceMetadataReader(), 3)

	got, err := replayer.ReplayRecall(context.Background(), replayCluster("tenant_1", "kb_1", "offline knowledge"))
	if err != nil {
		t.Fatalf("ReplayRecall() error = %v", err)
	}
	item := got.BaselineRecallResults[0]
	if item.DocVersion != "v42" || item.ChunkContentHash != "sha256:precomputed" {
		t.Fatalf("baseline item = %#v, want explicit metadata fingerprints", item)
	}
	if got.SourceFingerprints[0].DocVersion != "v42" || got.SourceFingerprints[0].ChunkContentHash != "sha256:precomputed" {
		t.Fatalf("fingerprints = %#v, want explicit metadata fingerprints", got.SourceFingerprints)
	}
}

func TestRetrieverRecallReplayerNoResults(t *testing.T) {
	replayer := NewRetrieverRecallReplayer(&staticRetriever{}, NewChunkSourceMetadataReader(), 3)

	got, err := replayer.ReplayRecall(context.Background(), replayCluster("tenant_1", "kb_1", "missing"))
	if err != nil {
		t.Fatalf("ReplayRecall() error = %v", err)
	}
	if len(got.BaselineRecallResults) != 0 || len(got.SourceFingerprints) != 0 {
		t.Fatalf("ReplayRecall() = %#v, want empty baseline and fingerprints", got)
	}
	if len(got.TraceSummaries) != 2 {
		t.Fatalf("TraceSummaries = %#v, want cluster traces preserved", got.TraceSummaries)
	}
}

func TestRetrieverRecallReplayerFiltersTenantAndKnowledgeBase(t *testing.T) {
	retriever := &staticRetriever{results: []kb.SearchResult{
		{
			Chunk: kb.Chunk{ID: "chunk_wrong_tenant", TenantID: "tenant_2", KnowledgeBaseID: "kb_1", DocumentID: "doc_2", Content: "ORAG private tenant data."},
			Score: 0.99,
			Rank:  1,
		},
		{
			Chunk: kb.Chunk{ID: "chunk_wrong_kb", TenantID: "tenant_1", KnowledgeBaseID: "kb_2", DocumentID: "doc_3", Content: "ORAG wrong kb data."},
			Score: 0.98,
			Rank:  2,
		},
		{
			Chunk: kb.Chunk{ID: "chunk_allowed", TenantID: "tenant_1", KnowledgeBaseID: "kb_1", DocumentID: "doc_1", Content: "ORAG allowed data."},
			Score: 0.97,
			Rank:  3,
		},
	}}
	replayer := NewRetrieverRecallReplayer(retriever, NewChunkSourceMetadataReader(), 5)

	got, err := replayer.ReplayRecall(context.Background(), replayCluster("tenant_1", "kb_1", "ORAG"))
	if err != nil {
		t.Fatalf("ReplayRecall() error = %v", err)
	}
	if len(got.BaselineRecallResults) != 1 || got.BaselineRecallResults[0].ChunkID != "chunk_allowed" {
		t.Fatalf("baseline results = %#v, want only tenant/kb scoped chunk", got.BaselineRecallResults)
	}
	if retriever.requests[0].TenantID != "tenant_1" || retriever.requests[0].KnowledgeBaseID != "kb_1" {
		t.Fatalf("retriever request = %#v, want tenant/kb scoped request", retriever.requests[0])
	}
}

func replayCluster(tenantID, kbID, question string) QuestionCluster {
	return QuestionCluster{
		ID:                "cluster_1",
		TenantID:          tenantID,
		KBID:              kbID,
		CanonicalQuestion: question,
		TraceIDs:          []string{"trace_1", "trace_2"},
	}
}

type staticRetriever struct {
	results  []kb.SearchResult
	requests []kb.SearchRequest
	err      error
}

func (r *staticRetriever) Retrieve(_ context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return nil, r.err
	}
	return append([]kb.SearchResult(nil), r.results...), nil
}
