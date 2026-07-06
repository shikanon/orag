package ingest

import (
	"context"
	"errors"
	"strings"
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

func TestIngestGraphRelationFailureSucceedsWithWarning(t *testing.T) {
	ctx := context.Background()
	graphErr := errors.New("graph relation write failed")
	indexer := &graphFailingIndexer{
		MemoryStore: kb.NewMemoryStore(),
		err:         graphErr,
	}
	if err := indexer.PutKnowledgeBase(ctx, kb.KnowledgeBase{ID: "kb_default", TenantID: "tenant_default", Name: "Default"}); err != nil {
		t.Fatal(err)
	}
	jobs := NewMemoryJobStore()
	svc := &Service{
		Parser:         parser.BasicParser{},
		Splitter:       chunker.Recursive{SizeTokens: 20, OverlapTokens: 0},
		Embedder:       fakeEmbedder{},
		GraphBuilder:   fixedRelationGraphBuilder{},
		KnowledgeBases: indexer,
		Indexer:        indexer,
		Jobs:           jobs,
	}

	res, err := svc.Ingest(ctx, Request{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://graph-warning.md",
		Name:            "graph-warning.md",
		Content:         []byte("Qdrant relation failure visible marker"),
	})
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if res.Job.Status != JobStatusSucceeded {
		t.Fatalf("job status = %q", res.Job.Status)
	}
	assertGraphWarning(t, res.Job.Error, graphErr)
	stored, ok, err := jobs.GetJob(ctx, "tenant_default", res.Job.ID)
	if err != nil || !ok {
		t.Fatalf("job lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != JobStatusSucceeded {
		t.Fatalf("stored job status = %q", stored.Status)
	}
	assertGraphWarning(t, stored.Error, graphErr)

	results, err := (kb.SparseRetriever{Store: indexer}).Retrieve(ctx, kb.SearchRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "visible marker",
		TopK:            8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Chunk.DocumentID != res.Document.ID {
		t.Fatalf("successful ingestion is not searchable: %#v", results)
	}
}

type graphFailingIndexer struct {
	*kb.MemoryStore
	err error
}

func (i *graphFailingIndexer) StoreGraphRelations(context.Context, []kb.GraphRelation) error {
	return i.err
}

type fixedRelationGraphBuilder struct{}

func (fixedRelationGraphBuilder) Build(_ context.Context, req GraphBuildRequest) ([]kb.GraphRelation, []string, error) {
	if len(req.Chunks) == 0 {
		return nil, nil, nil
	}
	return []kb.GraphRelation{{
		TenantID:        req.Document.TenantID,
		KnowledgeBaseID: req.Document.KnowledgeBaseID,
		DocumentID:      req.Document.ID,
		SourceChunkID:   req.Chunks[0].ID,
		TargetChunkID:   req.Chunks[0].ID,
		Subject:         "Qdrant",
		Predicate:       "co_occurs_with",
		Object:          "PostgreSQL",
		Weight:          1,
	}}, nil, nil
}

func assertGraphWarning(t *testing.T, got string, want error) {
	t.Helper()
	if !strings.Contains(got, "graph indexing failed") || !strings.Contains(got, want.Error()) {
		t.Fatalf("job error = %q, want graph indexing warning containing %q", got, want)
	}
}
