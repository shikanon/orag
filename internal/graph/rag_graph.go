package graph

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/rag"
)

var ErrRunnableNotConfigured = errors.New("rag graph runnable is not configured")

type RAGGraph struct {
	Service    *rag.Service
	TraceStore TraceStore
	Metrics    *observability.Metrics
	runner     compose.Runnable[State, State]
}

type TraceStore interface {
	StoreTrace(ctx context.Context, input TraceInput) error
}

// TraceInput captures the server-resolved execution lineage together with the
// query result. The lineage fields are never accepted from the public query
// request and therefore remain trustworthy for replay and release audits.
type TraceInput struct {
	TenantID        string
	KnowledgeBaseID string
	TraceID         string
	Query           string
	Profile         rag.Profile
	LatencyMS       int64
	Answer          string
	RetrievedChunks []string
	Spans           []NodeSpan

	ProjectID         string
	PipelineID        string
	PipelineVersionID string
	ReleaseID         string
	Environment       string
	DatasetID         string
	EvaluationRunID   string
	RequestedTopK     int
	RequestedProfile  rag.Profile
}

type traceRecord struct {
	traceID         string
	profile         rag.Profile
	answer          string
	retrievedChunks []string
	latencyMS       int64
	spans           []NodeSpan
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
	return g.InvokeCompiled(ctx, g.runner, req)
}

// InvokeCompiled executes a compiled, server-owned pipeline definition with
// the same trace and metric behavior as the default graph. Production release
// execution uses this path so a frozen version cannot bypass trace lineage.
func (g *RAGGraph) InvokeCompiled(ctx context.Context, runner compose.Runnable[State, State], req rag.QueryRequest) (rag.QueryResponse, error) {
	if runner == nil {
		return rag.QueryResponse{}, ErrRunnableNotConfigured
	}
	start := time.Now()
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = observability.EnsureTraceID(ctx)
	}
	req.TraceID = traceID
	ctx = observability.WithTraceID(ctx, traceID)
	collector := &spanCollector{}
	ctx = context.WithValue(ctx, spanCollectorKey{}, collector)
	out, err := runner.Invoke(ctx, State{Request: req})
	if err != nil {
		_ = g.storeTrace(ctx, req, g.traceRecord(req, traceID, start, out, collector.snapshot(), true))
		return rag.QueryResponse{}, err
	}
	resp := rag.NormalizeQueryResponse(out.Response)
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
		traceID:         traceID,
		profile:         profile,
		answer:          storableAnswer(st.Response),
		retrievedChunks: storableRetrievedChunks(st.Response),
		latencyMS:       latencyMS,
		spans:           spans,
	}
}

func (g *RAGGraph) storeTrace(ctx context.Context, req rag.QueryRequest, rec traceRecord) error {
	if g.TraceStore == nil || rec.traceID == "" {
		return nil
	}
	start := time.Now()
	err := g.TraceStore.StoreTrace(ctx, TraceInput{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		TraceID:         rec.traceID,
		Query:           req.Query,
		Profile:         rec.profile,
		LatencyMS:       rec.latencyMS,
		Answer:          rec.answer,
		RetrievedChunks: rec.retrievedChunks,
		Spans:           rec.spans,

		ProjectID:         req.ProjectID,
		PipelineID:        req.PipelineID,
		PipelineVersionID: req.PipelineVersionID,
		ReleaseID:         req.ReleaseID,
		Environment:       req.Environment,
		DatasetID:         req.DatasetID,
		EvaluationRunID:   req.EvaluationRunID,
		RequestedTopK:     req.TopK,
		RequestedProfile:  req.Profile,
	})
	if g.Metrics != nil {
		outcome := "success"
		if err != nil {
			outcome = "error"
		}
		g.Metrics.ObserveTraceStore(outcome, time.Since(start).Milliseconds())
	}
	return err
}

func storableAnswer(resp rag.QueryResponse) string {
	return resp.Answer
}

func storableRetrievedChunks(resp rag.QueryResponse) []string {
	out := make([]string, 0, len(resp.RetrievedChunks))
	for _, result := range resp.RetrievedChunks {
		if result.Chunk.ID != "" {
			out = append(out, result.Chunk.ID)
		}
	}
	return out
}

func (g *RAGGraph) profile(requested rag.Profile) rag.Profile {
	if g.Service == nil {
		return requested
	}
	return g.Service.Profile(requested)
}
