package graph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
)

type NodeSet struct {
	Service *rag.Service
}

func (n NodeSet) Init(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "init", st, func(st *State) error {
		st.Start = time.Now()
		st.TraceID = strings.TrimSpace(st.Request.TraceID)
		if st.TraceID == "" {
			st.TraceID = observability.EnsureTraceID(ctx)
		}
		st.Request.TraceID = st.TraceID
		st.Profile = n.Service.Profile(st.Request.Profile)
		st.TopK = st.Request.TopK
		if st.TopK <= 0 {
			st.TopK = n.Service.TopK
		}
		if st.TopK <= 0 {
			st.TopK = 50
		}
		return nil
	})
}

func (n NodeSet) QueryRoute(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "query_route", st, func(st *State) error {
		route, warnings := n.Service.RouteQuery(ctx, st.Request)
		st.Warnings = append(st.Warnings, warnings...)
		if route == nil {
			return nil
		}
		st.Route = route
		switch route.Route {
		case rag.QueryRouteSingleRetrieval:
			st.Profile = rag.ProfileRealtime
		case rag.QueryRouteMultiStepRetrieval:
			st.Profile = rag.ProfileHighPrecision
		}
		return nil
	})
}

func (n NodeSet) SemanticCacheLookup(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "semantic_cache_lookup", st, func(st *State) error {
		if st.isDirectRoute() || n.Service.Cache == nil {
			return nil
		}
		embeddings, err := n.Service.Model.Embed(ctx, []string{st.Request.Query})
		if err != nil {
			return err
		}
		if len(embeddings) == 0 {
			return fmt.Errorf("embedding response is empty")
		}
		st.Embedding = embeddings[0]
		if cached, ok, warning := n.Service.LookupSemanticCache(ctx, st.Request, st.Embedding, st.TraceID, st.Profile, st.TopK, st.Start); ok {
			if st.Route != nil && cached.Route == nil {
				cached.Route = st.Route
			}
			st.Response = cached
			st.Cached = true
		} else if warning != "" {
			st.Warnings = append(st.Warnings, warning)
		}
		return nil
	})
}

func (n NodeSet) QueryRewrite(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "query_rewrite", st, func(st *State) error {
		if st.Cached || st.isDirectRoute() || st.isSingleRoute() || st.Profile != rag.ProfileHighPrecision || !n.Service.QueryRewriteEnabled {
			return nil
		}
		rewritten, warnings := n.Service.RewriteQuery(ctx, st.Request, st.Profile)
		st.Warnings = append(st.Warnings, warnings...)
		if rewritten != "" {
			st.RewrittenQuery = rewritten
		}
		return nil
	})
}

func (n NodeSet) MultiQuery(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "multi_query", st, func(st *State) error {
		if st.Cached || st.isDirectRoute() || st.isSingleRoute() || st.Profile != rag.ProfileHighPrecision || (n.Service.MultiQueryCount <= 1 && !n.Service.HyDEEnabled) {
			return nil
		}
		if st.RewrittenQuery == "" {
			st.RewrittenQuery = st.Request.Query
		}
		queries, warnings := n.Service.BuildRetrievalQueries(ctx, st.Request, st.Profile, st.RewrittenQuery)
		st.Warnings = append(st.Warnings, warnings...)
		st.RetrievalQueries = queries
		return nil
	})
}

func (n NodeSet) HybridRetrieve(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "hybrid_retrieve", st, func(st *State) error {
		if st.Cached || st.isDirectRoute() {
			return nil
		}
		retrievalQueries := st.RetrievalQueries
		if len(retrievalQueries) == 0 {
			query := st.Request.Query
			source := "query"
			if st.RewrittenQuery != "" {
				query = st.RewrittenQuery
				source = "rewrite"
			}
			retrievalQueries = []rag.RetrievalQuery{{Query: query, Source: source}}
		}
		results, queryVector, warnings, err := n.Service.RetrieveExpanded(ctx, st.Request, st.TopK, retrievalQueries, st.Embedding)
		st.Warnings = append(st.Warnings, warnings...)
		if err != nil {
			return err
		}
		st.Embedding = queryVector
		st.Results = results
		return nil
	})
}

func (n NodeSet) Rerank(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "ark_rerank", st, func(st *State) error {
		if st.Cached || len(st.Results) == 0 {
			return nil
		}
		st.Results = n.Service.ApplyRerank(ctx, st.Request.Query, st.Results)
		return nil
	})
}

func (n NodeSet) ContextPack(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "context_pack", st, func(st *State) error {
		if st.Cached || len(st.Results) == 0 {
			return nil
		}
		st.Context, st.Citations = n.Service.Packer.Pack(st.Results)
		return nil
	})
}

func (n NodeSet) PromptPrefixCache(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "prompt_prefix_cache", st, func(st *State) error {
		if st.Cached || len(st.Results) == 0 {
			return nil
		}
		system := n.systemPrompt(st.Profile)
		st.PromptText = n.Service.PromptStrategy.Apply([]prompt.Segment{
			{Name: "system", Stable: true, Content: system},
			{Name: "context", Stable: true, Content: st.Context},
			{Name: "question", Stable: false, Content: st.Request.Query},
		})
		return nil
	})
}

func (n NodeSet) Generate(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "ark_generate", st, func(st *State) error {
		if st.Cached {
			return nil
		}
		if st.isDirectRoute() {
			resp, err := n.Service.GenerateDirect(ctx, st.Request, st.Profile, st.TraceID, st.Start, st.Warnings, st.Route)
			if err != nil {
				return err
			}
			st.Response = resp
			return nil
		}
		if len(st.Results) == 0 {
			st.Response = rag.QueryResponse{
				Answer:      n.Service.NoContextAnswer,
				TraceID:     st.TraceID,
				CacheStatus: "miss",
				Profile:     st.Profile,
				Route:       st.Route,
				Warnings:    append(st.Warnings, "no_retrieved_context"),
				CreatedAt:   time.Now().UTC(),
				LatencyMS:   time.Since(st.Start).Milliseconds(),
			}
			return nil
		}
		system := n.systemPrompt(st.Profile)
		user := fmt.Sprintf("问题：%s\n\n上下文：\n%s", st.Request.Query, st.Context)
		answer, err := n.Service.Model.Chat(ctx, []ark.ChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user + "\n\n缓存稳定前缀：\n" + st.PromptText},
		})
		if err != nil {
			return err
		}
		citations, warnings := rag.ValidateCitations(st.Citations, st.Results)
		st.Warnings = append(st.Warnings, warnings...)
		st.Response = rag.QueryResponse{
			Answer:          rag.EnsureCitationHint(answer, citations),
			Citations:       citations,
			RetrievedChunks: st.Results,
			TraceID:         st.TraceID,
			CacheStatus:     "miss",
			Profile:         st.Profile,
			Route:           st.Route,
			Warnings:        st.Warnings,
			CreatedAt:       time.Now().UTC(),
			LatencyMS:       time.Since(st.Start).Milliseconds(),
		}
		return nil
	})
}

func (n NodeSet) SemanticCacheWrite(ctx context.Context, st State) (State, error) {
	return n.withSpan(ctx, "semantic_cache_write", st, func(st *State) error {
		if st.Cached || n.Service.Cache == nil || len(st.Response.Citations) == 0 {
			return nil
		}
		if warning := n.Service.StoreSemanticCache(ctx, st.Request, st.Embedding, st.Profile, st.TopK, st.Response); warning != "" {
			st.Response.Warnings = append(st.Response.Warnings, warning)
		}
		return nil
	})
}

func (n NodeSet) systemPrompt(profile rag.Profile) string {
	system := "你是一个严格基于给定上下文回答的 RAG 助手。回答必须使用中文，并在事实来自上下文时引用 chunk id。"
	if profile == rag.ProfileHighPrecision {
		system += " 当前为高精档，请更充分整合上下文。"
	}
	return system
}

func (n NodeSet) withSpan(ctx context.Context, name string, st State, fn func(*State) error) (State, error) {
	start := time.Now()
	err := fn(&st)
	latencyMS := time.Since(start).Milliseconds()
	span := NodeSpan{NodeName: name, LatencyMS: latencyMS}
	if err != nil {
		span.Error = err.Error()
		n.logNodeFailure(st, name, latencyMS, err)
	}
	st.Spans = append(st.Spans, span)
	collectSpan(ctx, span)
	return st, err
}

func (n NodeSet) logNodeFailure(st State, node string, latencyMS int64, err error) {
	if n.Service == nil || n.Service.Logger == nil || err == nil {
		return
	}
	n.Service.Logger.LogAttrs(context.Background(), slog.LevelError, "rag_graph_node_failed",
		slog.String("trace_id", st.TraceID),
		slog.String("tenant", st.Request.TenantID),
		slog.String("profile", string(st.Profile)),
		slog.String("node", node),
		slog.Int64("latency", latencyMS),
		slog.String("error", err.Error()),
	)
}
