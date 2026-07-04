package ingest

import (
	"context"
	"testing"

	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/ingest/parser"
	"github.com/shikanon/orag/internal/kb"
)

func TestLightweightGraphBuilderExtractsEntityRelations(t *testing.T) {
	builder := LightweightGraphBuilder{MaxEntitiesPerChunk: 4}
	doc := kb.Document{ID: "doc_graph", TenantID: "tenant_default", KnowledgeBaseID: "kb_default"}
	relations, warnings, err := builder.Build(context.Background(), GraphBuildRequest{
		Document: doc,
		Chunks: []kb.Chunk{{
			ID:      "chunk_1",
			Content: "Qdrant works with PostgreSQL FTS for hybrid retrieval.",
		}},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(relations) == 0 {
		t.Fatalf("expected extracted graph relation")
	}
	if relations[0].Subject == "" || relations[0].Object == "" || relations[0].SourceChunkID != "chunk_1" {
		t.Fatalf("relation = %#v", relations[0])
	}
}

func TestIngestStoresGraphRelationsWhenIndexerSupportsGraph(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	if err := store.PutKnowledgeBase(ctx, kb.KnowledgeBase{ID: "kb_default", TenantID: "tenant_default", Name: "Default"}); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		Parser:         parser.BasicParser{},
		Splitter:       chunker.Recursive{SizeTokens: 20, OverlapTokens: 0},
		Embedder:       fakeEmbedder{},
		GraphBuilder:   LightweightGraphBuilder{MaxEntitiesPerChunk: 4},
		KnowledgeBases: store,
		Indexer:        store,
		Jobs:           NewMemoryJobStore(),
	}

	_, err := svc.Ingest(ctx, Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://graph.md",
		Name:            "graph.md",
		Content:         []byte("Qdrant works with PostgreSQL FTS for hybrid retrieval."),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	results, err := store.ExpandGraph(ctx, kb.GraphExpansionRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Entities:        []string{"Qdrant"},
		Limit:           4,
	})
	if err != nil {
		t.Fatalf("ExpandGraph() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected stored graph expansion results")
	}
}
