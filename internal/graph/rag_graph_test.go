package graph

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
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

func TestRAGGraphSemanticCacheIsolatesProfileAndTopK(t *testing.T) {
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
		Profile:         rag.ProfileRealtime,
		TopK:            8,
	}
	resp, err := g.Invoke(ctx, req)
	if err != nil {
		t.Fatalf("Invoke() realtime error = %v", err)
	}
	if resp.CacheStatus != "miss" {
		t.Fatalf("realtime first cache_status = %q, want miss", resp.CacheStatus)
	}

	cached, err := g.Invoke(ctx, req)
	if err != nil {
		t.Fatalf("Invoke() realtime cached error = %v", err)
	}
	if cached.CacheStatus != "hit" {
		t.Fatalf("same profile/top_k cache_status = %q, want hit", cached.CacheStatus)
	}

	highPrecision := req
	highPrecision.Profile = rag.ProfileHighPrecision
	resp, err = g.Invoke(ctx, highPrecision)
	if err != nil {
		t.Fatalf("Invoke() high_precision error = %v", err)
	}
	if resp.CacheStatus != "miss" {
		t.Fatalf("high_precision cache_status = %q, want miss", resp.CacheStatus)
	}

	differentTopK := req
	differentTopK.TopK = 16
	resp, err = g.Invoke(ctx, differentTopK)
	if err != nil {
		t.Fatalf("Invoke() different top_k error = %v", err)
	}
	if resp.CacheStatus != "miss" {
		t.Fatalf("different top_k cache_status = %q, want miss", resp.CacheStatus)
	}
}

func TestRAGGraphInvokeUsesRequestTraceIDInPersistence(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	g, err := NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatalf("NewRAGGraph() error = %v", err)
	}
	store := &capturingTraceStore{}
	g.TraceStore = store

	resp, err := g.Invoke(ctx, rag.QueryRequest{
		TenantID:        "tenant_default",
		TraceID:         "trace_graph_request",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if resp.TraceID != "trace_graph_request" {
		t.Fatalf("response trace_id = %q, want trace_graph_request", resp.TraceID)
	}
	if store.traceID != "trace_graph_request" {
		t.Fatalf("stored trace_id = %q, want trace_graph_request", store.traceID)
	}
	if len(store.spans) == 0 {
		t.Fatalf("expected persisted node spans")
	}
}

func TestRAGGraphFailureLogIncludesCorrelationFieldsWithoutSensitiveContent(t *testing.T) {
	var logs bytes.Buffer
	ctx := context.Background()
	svc := newTestService(t)
	svc.Cache = nil
	svc.Retriever = failingRetriever{err: errors.New("retrieval unavailable")}
	svc.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	g, err := NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatalf("NewRAGGraph() error = %v", err)
	}

	_, err = g.Invoke(ctx, rag.QueryRequest{
		TenantID:        "tenant_default",
		TraceID:         "trace_graph_failure",
		KnowledgeBaseID: "kb_default",
		Query:           "raw prompt should stay out of logs",
		Profile:         rag.ProfileHighPrecision,
	})
	if err == nil {
		t.Fatalf("Invoke() expected error")
	}

	line := logs.String()
	for _, want := range []string{
		`"msg":"rag_graph_node_failed"`,
		`"trace_id":"trace_graph_failure"`,
		`"tenant":"tenant_default"`,
		`"profile":"high_precision"`,
		`"node":"hybrid_retrieve"`,
		`"latency":`,
		`"error":"retrieval unavailable"`,
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("graph failure log missing %s: %s", want, line)
		}
	}
	for _, forbidden := range []string{"raw prompt", "document content", "model response"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("graph failure log leaked %q: %s", forbidden, line)
		}
	}
}

func TestRAGGraphInvokePersistsFailureSpans(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.Cache = nil
	retrieverErr := errors.New("retrieval unavailable")
	svc.Retriever = failingRetriever{err: retrieverErr}
	g, err := NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatalf("NewRAGGraph() error = %v", err)
	}
	store := &capturingTraceStore{}
	g.TraceStore = store

	rawQuery := "raw prompt should stay out of persisted spans"
	_, err = g.Invoke(ctx, rag.QueryRequest{
		TenantID:        "tenant_default",
		TraceID:         "trace_graph_failure_persisted",
		KnowledgeBaseID: "kb_default",
		Query:           rawQuery,
		Profile:         rag.ProfileHighPrecision,
	})
	if !errors.Is(err, retrieverErr) {
		t.Fatalf("Invoke() error = %v, want %v", err, retrieverErr)
	}
	if store.calls != 1 {
		t.Fatalf("StoreTrace() calls = %d, want 1", store.calls)
	}
	if store.traceID != "trace_graph_failure_persisted" {
		t.Fatalf("stored trace_id = %q, want trace_graph_failure_persisted", store.traceID)
	}
	if len(store.spans) == 0 {
		t.Fatalf("expected persisted failure spans")
	}

	foundRetrieveSpan := false
	for _, span := range store.spans {
		if strings.Contains(span.NodeName, rawQuery) || strings.Contains(span.Error, rawQuery) {
			t.Fatalf("persisted span leaked raw query: %+v", span)
		}
		if span.NodeName != "hybrid_retrieve" {
			continue
		}
		foundRetrieveSpan = true
		if span.LatencyMS < 0 {
			t.Fatalf("hybrid_retrieve latency = %d, want non-negative", span.LatencyMS)
		}
		if span.Error != "retrieval unavailable" {
			t.Fatalf("hybrid_retrieve error = %q, want retrieval unavailable", span.Error)
		}
	}
	if !foundRetrieveSpan {
		t.Fatalf("persisted spans missing hybrid_retrieve: %+v", store.spans)
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

type capturingTraceStore struct {
	traceID string
	spans   []NodeSpan
	calls   int
}

func (s *capturingTraceStore) StoreTrace(_ context.Context, _, traceID, _ string, _ rag.Profile, _ int64, spans []NodeSpan) error {
	s.calls++
	s.traceID = traceID
	s.spans = append([]NodeSpan(nil), spans...)
	return nil
}

type failingRetriever struct {
	err error
}

func (r failingRetriever) Retrieve(context.Context, kb.SearchRequest) ([]kb.SearchResult, error) {
	return nil, r.err
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
