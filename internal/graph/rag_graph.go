package graph

import (
	"context"

	"github.com/cloudwego/eino/compose"
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

func NewRAGGraph(ctx context.Context, svc *rag.Service) (*RAGGraph, error) {
	nodes := NodeSet{Service: svc}
	graph := compose.NewGraph[State, State]()
	for _, item := range []struct {
		name string
		fn   func(context.Context, State) (State, error)
	}{
		{"init", nodes.Init},
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
		{"init", "semantic_cache_lookup"},
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
	out, err := g.runner.Invoke(ctx, State{Request: req})
	if err != nil {
		return rag.QueryResponse{}, err
	}
	resp := out.Response
	if g.TraceStore != nil && resp.TraceID != "" {
		if err := g.TraceStore.StoreTrace(ctx, req.TenantID, resp.TraceID, req.Query, resp.Profile, resp.LatencyMS, out.Spans); err != nil {
			resp.Warnings = append(resp.Warnings, "trace store failed: "+err.Error())
		}
	}
	return resp, nil
}
