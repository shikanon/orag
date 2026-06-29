package graph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
)

type NodeSet struct {
	Service *rag.Service
}

func (n NodeSet) Init(ctx context.Context, st State) (State, error) {
	return n.withSpan("init", st, func(st *State) error {
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

func (n NodeSet) SemanticCacheLookup(ctx context.Context, st State) (State, error) {
	return n.withSpan("semantic_cache_lookup", st, func(st *State) error {
		if n.Service.Cache == nil {
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
		if cached, ok, warning := n.Service.LookupSemanticCache(ctx, st.Request, st.Embedding, st.TraceID, st.Profile, st.Start); ok {
			st.Response = cached
			st.Cached = true
		} else if warning != "" {
			st.Warnings = append(st.Warnings, warning)
		}
		return nil
	})
}

func (n NodeSet) QueryRewrite(ctx context.Context, st State) (State, error) {
	return n.withSpan("query_rewrite", st, func(st *State) error {
		if st.Cached || st.Profile != rag.ProfileHighPrecision || !n.Service.QueryRewriteEnabled {
			return nil
		}
		st.RewrittenQuery = st.Request.Query
		if n.Service.Model == nil {
			return nil
		}
		answer, err := n.Service.Model.Chat(ctx, []ark.ChatMessage{
			{Role: "system", Content: "你是 RAG 查询改写器。只输出一个适合检索的中文查询，不要解释。"},
			{Role: "user", Content: st.Request.Query},
		})
		if err != nil {
			st.Warnings = append(st.Warnings, "query rewrite failed: "+err.Error())
			return nil
		}
		answer = strings.TrimSpace(answer)
		if answer != "" && len([]rune(answer)) <= 240 {
			st.RewrittenQuery = answer
		}
		return nil
	})
}

func (n NodeSet) MultiQuery(_ context.Context, st State) (State, error) {
	return n.withSpan("multi_query", st, func(st *State) error {
		if st.Cached || st.Profile != rag.ProfileHighPrecision || n.Service.MultiQueryCount <= 1 {
			return nil
		}
		if st.RewrittenQuery == "" {
			st.RewrittenQuery = st.Request.Query
		}
		return nil
	})
}

func (n NodeSet) HybridRetrieve(ctx context.Context, st State) (State, error) {
	return n.withSpan("hybrid_retrieve", st, func(st *State) error {
		if st.Cached {
			return nil
		}
		query := st.Request.Query
		if st.RewrittenQuery != "" {
			query = st.RewrittenQuery
		}
		if len(st.Embedding) == 0 || st.RewrittenQuery != "" {
			embeddings, err := n.Service.Model.Embed(ctx, []string{query})
			if err != nil {
				return err
			}
			if len(embeddings) == 0 {
				return fmt.Errorf("embedding response is empty")
			}
			st.Embedding = embeddings[0]
		}
		searchReq := kb.SearchRequest{
			TenantID:        st.Request.TenantID,
			KnowledgeBaseID: st.Request.KnowledgeBaseID,
			Query:           query,
			Vector:          st.Embedding,
			TopK:            st.TopK,
		}
		var results []kb.SearchResult
		var err error
		if retriever, ok := n.Service.Retriever.(interface {
			RetrieveWithWarnings(context.Context, kb.SearchRequest) ([]kb.SearchResult, []string, error)
		}); ok {
			var warnings []string
			results, warnings, err = retriever.RetrieveWithWarnings(ctx, searchReq)
			st.Warnings = append(st.Warnings, warnings...)
		} else {
			results, err = n.Service.Retriever.Retrieve(ctx, searchReq)
		}
		if err != nil {
			return err
		}
		st.Results = results
		return nil
	})
}

func (n NodeSet) Rerank(ctx context.Context, st State) (State, error) {
	return n.withSpan("ark_rerank", st, func(st *State) error {
		if st.Cached || len(st.Results) == 0 {
			return nil
		}
		st.Results = n.Service.ApplyRerank(ctx, st.Request.Query, st.Results)
		return nil
	})
}

func (n NodeSet) ContextPack(_ context.Context, st State) (State, error) {
	return n.withSpan("context_pack", st, func(st *State) error {
		if st.Cached || len(st.Results) == 0 {
			return nil
		}
		st.Context, st.Citations = n.Service.Packer.Pack(st.Results)
		return nil
	})
}

func (n NodeSet) PromptPrefixCache(_ context.Context, st State) (State, error) {
	return n.withSpan("prompt_prefix_cache", st, func(st *State) error {
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
	return n.withSpan("ark_generate", st, func(st *State) error {
		if st.Cached {
			return nil
		}
		if len(st.Results) == 0 {
			st.Response = rag.QueryResponse{
				Answer:      n.Service.NoContextAnswer,
				TraceID:     st.TraceID,
				CacheStatus: "miss",
				Profile:     st.Profile,
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
			Warnings:        st.Warnings,
			CreatedAt:       time.Now().UTC(),
			LatencyMS:       time.Since(st.Start).Milliseconds(),
		}
		return nil
	})
}

func (n NodeSet) SemanticCacheWrite(ctx context.Context, st State) (State, error) {
	return n.withSpan("semantic_cache_write", st, func(st *State) error {
		if st.Cached || n.Service.Cache == nil || len(st.Response.Citations) == 0 {
			return nil
		}
		if warning := n.Service.StoreSemanticCache(ctx, st.Request, st.Embedding, st.Response); warning != "" {
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

func (n NodeSet) withSpan(name string, st State, fn func(*State) error) (State, error) {
	start := time.Now()
	err := fn(&st)
	latencyMS := time.Since(start).Milliseconds()
	span := NodeSpan{NodeName: name, LatencyMS: latencyMS}
	if err != nil {
		span.Error = err.Error()
		n.logNodeFailure(st, name, latencyMS, err)
	}
	st.Spans = append(st.Spans, span)
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
