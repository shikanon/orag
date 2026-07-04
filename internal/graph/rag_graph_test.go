package graph

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	if resp.Profile != rag.ProfileHighPrecision {
		t.Fatalf("high_precision profile = %q, want %q", resp.Profile, rag.ProfileHighPrecision)
	}
	if len(resp.Citations) == 0 {
		t.Fatalf("high_precision response citations is empty after cache miss")
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

func TestRAGGraphInvokeRepeatedTraceIDPersistsSeparateSpanBatches(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.Cache = nil
	g, err := NewRAGGraph(ctx, svc)
	if err != nil {
		t.Fatalf("NewRAGGraph() error = %v", err)
	}
	store := &capturingTraceStore{}
	g.TraceStore = store

	req := rag.QueryRequest{
		TenantID:        "tenant_default",
		TraceID:         "trace_graph_reused",
		KnowledgeBaseID: "kb_default",
		Query:           "qdrant vector search",
	}
	for i := 0; i < 2; i++ {
		resp, err := g.Invoke(ctx, req)
		if err != nil {
			t.Fatalf("Invoke(%d) error = %v", i+1, err)
		}
		if resp.TraceID != "trace_graph_reused" {
			t.Fatalf("Invoke(%d) trace_id = %q, want trace_graph_reused", i+1, resp.TraceID)
		}
	}

	if store.calls != 2 {
		t.Fatalf("StoreTrace() calls = %d, want 2", store.calls)
	}
	for i, traceID := range store.traceIDs {
		if traceID != "trace_graph_reused" {
			t.Fatalf("stored trace_id[%d] = %q, want trace_graph_reused", i, traceID)
		}
	}
	if len(store.spanBatches) != 2 || len(store.spanBatches[0]) == 0 || len(store.spanBatches[1]) == 0 {
		t.Fatalf("stored span batches = %#v, want non-empty spans per invocation", store.spanBatches)
	}
	if len(store.spanBatches[1]) != len(store.spanBatches[0]) {
		t.Fatalf("second span batch appears to include prior request spans: first=%d second=%d", len(store.spanBatches[0]), len(store.spanBatches[1]))
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

func TestRAGGraphInvokePersistsTraceOnNodeFailure(t *testing.T) {
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

func TestHighPrecisionMultiQueryAndHyDERunAdditionalRetrievals(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	model := &scriptedGraphModel{}
	retriever := &recordingGraphRetriever{}
	svc.Cache = nil
	svc.Model = model
	svc.Retriever = retriever
	svc.QueryRewriteEnabled = false
	svc.MultiQueryCount = 3
	svc.HyDEEnabled = true
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
	st, err = nodes.MultiQuery(ctx, st)
	if err != nil {
		t.Fatalf("MultiQuery() error = %v", err)
	}
	if len(st.RetrievalQueries) != 4 {
		t.Fatalf("retrieval queries = %d, want 4: %#v", len(st.RetrievalQueries), st.RetrievalQueries)
	}
	if !model.sawSystemPrompt("多查询") {
		t.Fatalf("multi-query generation chat was not called: %#v", model.systemPrompts)
	}
	if !model.sawSystemPrompt("HyDE") {
		t.Fatalf("HyDE generation chat was not called: %#v", model.systemPrompts)
	}
	if len(st.Warnings) != 0 {
		t.Fatalf("MultiQuery() warnings = %#v, want none", st.Warnings)
	}

	st, err = nodes.HybridRetrieve(ctx, st)
	if err != nil {
		t.Fatalf("HybridRetrieve() error = %v", err)
	}
	if len(retriever.requests) != 4 {
		t.Fatalf("retrieval calls = %d, want 4: %#v", len(retriever.requests), retriever.requests)
	}
	if len(st.Results) == 0 {
		t.Fatalf("expected fused retrieval results")
	}
}

type capturingTraceStore struct {
	traceID     string
	traceIDs    []string
	spans       []NodeSpan
	spanBatches [][]NodeSpan
	calls       int
}

func (s *capturingTraceStore) StoreTrace(_ context.Context, _, traceID, _ string, _ rag.Profile, _ int64, spans []NodeSpan) error {
	s.calls++
	s.traceID = traceID
	s.traceIDs = append(s.traceIDs, traceID)
	s.spans = append([]NodeSpan(nil), spans...)
	s.spanBatches = append(s.spanBatches, append([]NodeSpan(nil), spans...))
	return nil
}

type failingRetriever struct {
	err error
}

func (r failingRetriever) Retrieve(context.Context, kb.SearchRequest) ([]kb.SearchResult, error) {
	return nil, r.err
}

type recordingGraphRetriever struct {
	requests []kb.SearchRequest
}

func (r *recordingGraphRetriever) Retrieve(_ context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	r.requests = append(r.requests, req)
	id := fmt.Sprintf("chk_graph_%d", len(r.requests))
	return []kb.SearchResult{{
		Chunk: kb.Chunk{
			ID:         id,
			DocumentID: "doc_graph",
			Content:    req.Query + " context",
			SourceURI:  "memory://graph",
		},
		Score: 1,
		Rank:  1,
		From:  "stub",
	}}, nil
}

type scriptedGraphModel struct {
	systemPrompts []string
}

func (m *scriptedGraphModel) Chat(_ context.Context, messages []ark.ChatMessage) (string, error) {
	system := ""
	for _, message := range messages {
		if message.Role == "system" {
			system = message.Content
			break
		}
	}
	m.systemPrompts = append(m.systemPrompts, system)
	switch {
	case strings.Contains(system, "多查询"):
		return `["qdrant hybrid retrieval", "dense sparse fusion"]`, nil
	case strings.Contains(system, "HyDE"):
		return "qdrant vector search stores documents for hybrid retrieval", nil
	default:
		return "answer [chk_graph_1]", nil
	}
}

func (m *scriptedGraphModel) Embed(_ context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i, text := range texts {
		out[i] = []float64{float64(len(text)%7) + 1, float64(len(text)%5) + 1}
	}
	return out, nil
}

func (m *scriptedGraphModel) Rerank(_ context.Context, _ string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
	if topN <= 0 || topN > len(docs) {
		topN = len(docs)
	}
	out := make([]ark.RerankResult, 0, topN)
	for i := 0; i < topN; i++ {
		out = append(out, ark.RerankResult{Index: i, Score: 1 / float64(i+1)})
	}
	return out, nil
}

func (m *scriptedGraphModel) sawSystemPrompt(value string) bool {
	for _, prompt := range m.systemPrompts {
		if strings.Contains(prompt, value) {
			return true
		}
	}
	return false
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
