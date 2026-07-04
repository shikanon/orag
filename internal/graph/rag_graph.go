package graph

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/rag"
)

type RAGGraph struct {
	Service    *rag.Service
	TraceStore TraceStore
	runner     compose.Runnable[State, State]
}

type TraceStore interface {
	StoreTrace(ctx context.Context, tenantID, traceID, query string, profile rag.Profile, latencyMS int64, spans []NodeSpan) error
}

type traceRecord struct {
	traceID   string
	profile   rag.Profile
	latencyMS int64
	spans     []NodeSpan
}

type spanCollectorKey struct{}

type spanCollector struct {
	mu    sync.Mutex
	spans []NodeSpan
}

func (c *spanCollector) add(span NodeSpan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.spans = append(c.spans, span)
}

func (c *spanCollector) snapshot() []NodeSpan {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]NodeSpan(nil), c.spans...)
}

func collectSpan(ctx context.Context, span NodeSpan) {
	collector, ok := ctx.Value(spanCollectorKey{}).(*spanCollector)
	if !ok || collector == nil {
		return
	}
	collector.add(span)
}

func NewRAGGraph(ctx context.Context, svc *rag.Service) (*RAGGraph, error) {
	nodes := NodeSet{Service: svc}
	graph := compose.NewGraph[State, State]()
	for _, item := range []struct {
		name string
		fn   func(context.Context, State) (State, error)
	}{
		{"init", nodes.Init},
		{"query_route", nodes.QueryRoute},
		{"semantic_cache_lookup", nodes.SemanticCacheLookup},
		{"query_rewrite", nodes.QueryRewrite},
		{"multi_query", nodes.MultiQuery},
		{"hybrid_retrieve", nodes.HybridRetrieve},
		{"ark_rerank", nodes.Rerank},
		{"context_pack", nodes.ContextPack},
		{"prompt_prefix_cache", nodes.PromptPrefixCache},
		{"ark_generate", nodes.Generate},
		{"semantic_cache_write", nodes.SemanticCacheWrite},
	} {
		if err := graph.AddLambdaNode(item.name, compose.InvokableLambda(item.fn)); err != nil {
			return nil, err
		}
	}
	edges := [][2]string{
		{compose.START, "init"},
		{"init", "query_route"},
		{"query_route", "semantic_cache_lookup"},
		{"semantic_cache_lookup", "query_rewrite"},
		{"query_rewrite", "multi_query"},
		{"multi_query", "hybrid_retrieve"},
		{"hybrid_retrieve", "ark_rerank"},
		{"ark_rerank", "context_pack"},
		{"context_pack", "prompt_prefix_cache"},
		{"prompt_prefix_cache", "ark_generate"},
		{"ark_generate", "semantic_cache_write"},
		{"semantic_cache_write", compose.END},
	}
	for _, edge := range edges {
		if err := graph.AddEdge(edge[0], edge[1]); err != nil {
			return nil, err
		}
	}
	runner, err := graph.Compile(ctx, compose.WithGraphName("orag_rag_graph"))
	if err != nil {
		return nil, err
	}
	return &RAGGraph{Service: svc, runner: runner}, nil
}

func (g *RAGGraph) Invoke(ctx context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
	start := time.Now()
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = observability.EnsureTraceID(ctx)
	}
	req.TraceID = traceID
	ctx = observability.WithTraceID(ctx, traceID)
	collector := &spanCollector{}
	ctx = context.WithValue(ctx, spanCollectorKey{}, collector)
	out, err := g.runner.Invoke(ctx, State{Request: req})
	if err != nil {
		_ = g.storeTrace(ctx, req, g.traceRecord(req, traceID, start, out, collector.snapshot(), true))
		return rag.QueryResponse{}, err
	}
	resp := out.Response
	if err := g.storeTrace(ctx, req, g.traceRecord(req, traceID, start, out, collector.snapshot(), false)); err != nil {
		resp.Warnings = append(resp.Warnings, "trace store failed: "+err.Error())
	}
	return resp, nil
}

func (g *RAGGraph) traceRecord(req rag.QueryRequest, fallbackTraceID string, fallbackStart time.Time, st State, fallbackSpans []NodeSpan, deriveLatency bool) traceRecord {
	traceID := strings.TrimSpace(st.TraceID)
	if traceID == "" {
		traceID = strings.TrimSpace(fallbackTraceID)
	}
	if traceID == "" {
		traceID = strings.TrimSpace(req.TraceID)
	}
	profile := st.Profile
	if profile == "" {
		profile = st.Response.Profile
	}
	if profile == "" {
		profile = g.profile(req.Profile)
	}
	latencyMS := st.Response.LatencyMS
	if deriveLatency && latencyMS == 0 {
		if !st.Start.IsZero() {
			latencyMS = time.Since(st.Start).Milliseconds()
		}
		if latencyMS == 0 && !fallbackStart.IsZero() {
			latencyMS = time.Since(fallbackStart).Milliseconds()
		}
	}
	spans := st.Spans
	if len(spans) == 0 {
		spans = fallbackSpans
	}
	return traceRecord{
		traceID:   traceID,
		profile:   profile,
		latencyMS: latencyMS,
		spans:     spans,
	}
}

func (g *RAGGraph) storeTrace(ctx context.Context, req rag.QueryRequest, rec traceRecord) error {
	if g.TraceStore == nil || rec.traceID == "" {
		return nil
	}
	return g.TraceStore.StoreTrace(ctx, req.TenantID, rec.traceID, req.Query, rec.profile, rec.latencyMS, rec.spans)
}

func (g *RAGGraph) profile(requested rag.Profile) rag.Profile {
	if g.Service == nil {
		return requested
	}
	return g.Service.Profile(requested)
}
