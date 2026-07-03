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
	"github.com/shikanon/orag/internal/observability"
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

func TestRAGGraphInvokeStartsInjectedTracerAndKeepsNodeSpans(t *testing.T) {
	ctx := context.Background()
	tracer := &recordingGraphTracer{}
	restore := observability.SetTracer(tracer)
	defer restore()

	svc := newTestService(t)
	g, err := NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatalf("NewRAGGraph() error = %v", err)
	}
	store := &capturingTraceStore{}
	g.TraceStore = store

	resp, err := g.Invoke(ctx, rag.QueryRequest{
		TenantID:        "tenant_default",
		TraceID:         "trace_graph_bridge",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if resp.TraceID != "trace_graph_bridge" {
		t.Fatalf("response trace_id = %q, want trace_graph_bridge", resp.TraceID)
	}
	if store.traceID != "trace_graph_bridge" {
		t.Fatalf("stored trace_id = %q, want trace_graph_bridge", store.traceID)
	}
	if len(store.spans) == 0 {
		t.Fatalf("expected persisted graph node spans")
	}
	if len(tracer.names) == 0 {
		t.Fatalf("expected injected tracer to start graph node spans")
	}
	if tracer.names[0] != "init" {
		t.Fatalf("first tracer span = %q, want init; spans=%v", tracer.names[0], tracer.names)
	}
	if len(tracer.ended) != len(tracer.names) {
		t.Fatalf("ended tracer spans = %d, want %d", len(tracer.ended), len(tracer.names))
	}
}

func TestRAGGraphInvokeAddsStructuredTraceStoreWarning(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	g, err := NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatalf("NewRAGGraph() error = %v", err)
	}
	g.TraceStore = &capturingTraceStore{err: errors.New("database unavailable")}

	resp, err := g.Invoke(ctx, rag.QueryRequest{
		TenantID:        "tenant_default",
		TraceID:         "trace_graph_store_warning",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(resp.Warnings) != 1 || !strings.Contains(resp.Warnings[0], "trace store failed") {
		t.Fatalf("warnings = %#v, want compatibility trace store warning", resp.Warnings)
	}
	if len(resp.TraceWarnings) != 1 {
		t.Fatalf("trace_warnings = %#v, want one structured warning", resp.TraceWarnings)
	}
	if got := resp.TraceWarnings[0].Code; got != rag.WarningCodeTraceStoreFailed {
		t.Fatalf("trace warning code = %q, want %q", got, rag.WarningCodeTraceStoreFailed)
	}
	if !strings.Contains(resp.TraceWarnings[0].Message, "database unavailable") {
		t.Fatalf("trace warning message = %q, want database unavailable", resp.TraceWarnings[0].Message)
	}
	if resp.TraceSummary == nil || resp.TraceSummary.NodeCount == 0 || resp.TraceSummary.SlowestNode == "" {
		t.Fatalf("trace summary = %#v, want node count and slowest node", resp.TraceSummary)
	}
}

func TestRAGGraphInvokePersistsPartialTraceOnNodeFailure(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.Cache = nil
	svc.Retriever = failingRetriever{err: errors.New("retrieval unavailable")}
	g, err := NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatalf("NewRAGGraph() error = %v", err)
	}
	store := &capturingTraceStore{}
	g.TraceStore = store

	_, err = g.Invoke(ctx, rag.QueryRequest{
		TenantID:        "tenant_default",
		TraceID:         "trace_graph_partial_failure",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
		Profile:         rag.ProfileHighPrecision,
	})
	if err == nil || !strings.Contains(err.Error(), "retrieval unavailable") {
		t.Fatalf("Invoke() error = %v, want retrieval unavailable", err)
	}
	if store.traceID != "trace_graph_partial_failure" {
		t.Fatalf("stored trace_id = %q, want trace_graph_partial_failure", store.traceID)
	}
	if store.profile != rag.ProfileHighPrecision {
		t.Fatalf("stored profile = %q, want high_precision", store.profile)
	}
	if store.latencyMS < 0 {
		t.Fatalf("stored latency_ms = %d, want non-negative", store.latencyMS)
	}
	if len(store.spans) == 0 {
		t.Fatalf("expected persisted partial spans")
	}
	last := store.spans[len(store.spans)-1]
	if last.NodeName != "hybrid_retrieve" {
		t.Fatalf("last span node = %q, want hybrid_retrieve; spans=%v", last.NodeName, store.spans)
	}
	if !strings.Contains(last.Error, "retrieval unavailable") {
		t.Fatalf("last span error = %q, want retrieval unavailable", last.Error)
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
	traceID   string
	profile   rag.Profile
	latencyMS int64
	spans     []NodeSpan
	err       error
}

func (s *capturingTraceStore) StoreTrace(_ context.Context, _, traceID, _ string, profile rag.Profile, latencyMS int64, spans []NodeSpan) error {
	s.traceID = traceID
	s.profile = profile
	s.latencyMS = latencyMS
	s.spans = spans
	return s.err
}

type recordingGraphTracer struct {
	names []string
	ended []error
}

func (t *recordingGraphTracer) StartSpan(ctx context.Context, name string) (context.Context, observability.Span) {
	t.names = append(t.names, name)
	return ctx, recordingGraphSpan{tracer: t}
}

type recordingGraphSpan struct {
	tracer *recordingGraphTracer
}

func (s recordingGraphSpan) End(err error) {
	s.tracer.ended = append(s.tracer.ended, err)
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
