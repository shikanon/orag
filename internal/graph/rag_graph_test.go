package graph

import (
	"context"
	"testing"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
)

func TestRAGGraphInvokeAndCacheHit(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	g, err := NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatalf("NewRAGGraph() error = %v", err)
	}

	req := rag.QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}
	resp, err := g.Invoke(ctx, req)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if resp.CacheStatus != "miss" {
		t.Fatalf("first response cache_status = %q, want miss", resp.CacheStatus)
	}
	if len(resp.Citations) == 0 {
		t.Fatalf("first response citations is empty")
	}

	cached, err := g.Invoke(ctx, req)
	if err != nil {
		t.Fatalf("Invoke() cached error = %v", err)
	}
	if cached.CacheStatus != "hit" {
		t.Fatalf("second response cache_status = %q, want hit", cached.CacheStatus)
	}
	if cached.TraceID == resp.TraceID {
		t.Fatalf("cached response should get a new trace id")
	}
}

func TestHighPrecisionRewriteNodeRuns(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	nodes := NodeSet{Service: svc}
	st, err := nodes.Init(ctx, State{Request: rag.QueryRequest{
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
		Profile:         rag.ProfileHighPrecision,
	}})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	st, err = nodes.QueryRewrite(ctx, st)
	if err != nil {
		t.Fatalf("QueryRewrite() error = %v", err)
	}
	if st.RewrittenQuery == "" {
		t.Fatalf("expected rewritten query")
	}
	if got := st.Spans[len(st.Spans)-1].NodeName; got != "query_rewrite" {
		t.Fatalf("last span = %q, want query_rewrite", got)
	}
}

func newTestService(t *testing.T) *rag.Service {
	t.Helper()
	ctx := context.Background()
	store := kb.NewMemoryStore()
	err := store.Store(ctx, kb.Document{
		ID:              "doc_1",
		TenantID:        "tenant_default",
		KnowledgeBaseID: "kb_default",
		SourceURI:       "memory://doc",
		Title:           "Doc",
	}, []kb.Chunk{
		{
			ID:              "chk_1",
			TenantID:        "tenant_default",
			KnowledgeBaseID: "kb_default",
			DocumentID:      "doc_1",
			Content:         "qdrant vector search stores dense embeddings for retrieval",
			SourceURI:       "memory://doc",
			Section:         "intro",
		},
	})
	if err != nil {
		t.Fatalf("store seed: %v", err)
	}
	model := ark.NewClient(ark.Config{EmbeddingDimensions: 8}, nil)
	return &rag.Service{
		Retriever: kb.HybridRetriever{
			Dense:  kb.DenseRetriever{Store: store},
			Sparse: kb.SparseRetriever{Store: store},
			RRFK:   60,
			TopN:   8,
		},
		Model:               model,
		Cache:               rag.NewSemanticCache(10),
		Packer:              rag.ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:      prompt.NewStrategy("auto"),
		DefaultProfile:      rag.ProfileRealtime,
		NoContextAnswer:     "no context",
		TopK:                8,
		QueryRewriteEnabled: true,
		MultiQueryCount:     2,
	}
}
