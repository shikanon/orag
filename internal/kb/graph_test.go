package kb

import (
	"context"
	"testing"
)

func TestMemoryStorePersistsGraphRelationsAndExpandsByEntity(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	doc := Document{ID: "doc_graph", TenantID: "tenant_default", KnowledgeBaseID: "kb_default", SourceURI: "memory://graph"}
	chunks := []Chunk{
		{ID: "chunk_a", TenantID: doc.TenantID, KnowledgeBaseID: doc.KnowledgeBaseID, DocumentID: doc.ID, Content: "Qdrant stores vectors"},
		{ID: "chunk_b", TenantID: doc.TenantID, KnowledgeBaseID: doc.KnowledgeBaseID, DocumentID: doc.ID, Content: "PostgreSQL stores FTS indexes"},
	}
	if err := store.Store(ctx, doc, chunks); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	if err := store.StoreGraphRelations(ctx, []GraphRelation{{
		TenantID:        doc.TenantID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		DocumentID:      doc.ID,
		SourceChunkID:   "chunk_a",
		TargetChunkID:   "chunk_b",
		Subject:         "Qdrant",
		Predicate:       "co_occurs_with",
		Object:          "PostgreSQL",
		Weight:          1,
	}}); err != nil {
		t.Fatalf("StoreGraphRelations() error = %v", err)
	}

	results, err := store.ExpandGraph(ctx, GraphExpansionRequest{
		TenantID:        doc.TenantID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		Entities:        []string{"qdrant"},
		Limit:           4,
	})
	if err != nil {
		t.Fatalf("ExpandGraph() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want both linked chunks: %#v", len(results), results)
	}
}

func TestGraphRetrieverAddsRelatedChunks(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	doc := Document{ID: "doc_graph", TenantID: "tenant_default", KnowledgeBaseID: "kb_default", SourceURI: "memory://graph"}
	if err := store.Store(ctx, doc, []Chunk{
		{ID: "chunk_seed", TenantID: doc.TenantID, KnowledgeBaseID: doc.KnowledgeBaseID, DocumentID: doc.ID, Content: "Qdrant vectors"},
		{ID: "chunk_related", TenantID: doc.TenantID, KnowledgeBaseID: doc.KnowledgeBaseID, DocumentID: doc.ID, Content: "PostgreSQL FTS"},
	}); err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	_ = store.StoreGraphRelations(ctx, []GraphRelation{{
		TenantID:        doc.TenantID,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		DocumentID:      doc.ID,
		SourceChunkID:   "chunk_seed",
		TargetChunkID:   "chunk_related",
		Subject:         "Qdrant",
		Predicate:       "co_occurs_with",
		Object:          "PostgreSQL",
		Weight:          1,
	}})

	retriever := GraphRetriever{
		Base:  fixedRetriever{results: []SearchResult{{Chunk: Chunk{ID: "chunk_seed", Content: "Qdrant vectors"}, Score: 1, Rank: 1, From: "base"}}},
		Store: store,
		TopK:  4,
	}
	results, err := retriever.Retrieve(ctx, SearchRequest{TenantID: doc.TenantID, KnowledgeBaseID: doc.KnowledgeBaseID, Query: "Qdrant relation", TopK: 4})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want seed plus graph expansion: %#v", len(results), results)
	}
	if results[1].Chunk.ID != "chunk_related" || results[1].From != "graph" {
		t.Fatalf("graph result = %#v", results[1])
	}
}

type fixedRetriever struct {
	results []SearchResult
	err     error
}

func (r fixedRetriever) Retrieve(context.Context, SearchRequest) ([]SearchResult, error) {
	return r.results, r.err
}
