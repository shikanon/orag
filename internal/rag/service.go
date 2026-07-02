package rag

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
)

type Pipeline interface {
	Invoke(ctx context.Context, req QueryRequest) (QueryResponse, error)
}

type Model interface {
	ark.ChatGenerator
	ark.Embedder
	ark.Reranker
}

type Service struct {
	Retriever              kb.Retriever
	Model                  Model
	Cache                  SemanticCacheStore
	Packer                 ContextPacker
	PromptStrategy         prompt.CacheStrategy
	DefaultProfile         Profile
	NoContextAnswer        string
	TopK                   int
	Pipeline               Pipeline
	SemanticCacheThreshold float64

	QueryRewriteEnabled bool
	MultiQueryCount     int
	HyDEEnabled         bool
	Logger              *slog.Logger
}

func (s *Service) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	start := time.Now()
	ctx, req, traceID := ensureRequestTrace(ctx, req)
	if s.Pipeline != nil {
		resp, err := s.Pipeline.Invoke(ctx, req)
		if err != nil {
			s.logFailure(ctx, req, s.Profile(req.Profile), traceID, "rag_pipeline", start, err)
		}
		return resp, err
	}
	return s.Execute(ctx, req)
}

func (s *Service) Execute(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	start := time.Now()
	ctx, req, traceID := ensureRequestTrace(ctx, req)
	profile := s.Profile(req.Profile)
	topK := req.TopK
	if topK <= 0 {
		topK = s.TopK
	}
	if topK <= 0 {
		topK = 50
	}
	embeddings, err := s.Model.Embed(ctx, []string{req.Query})
	if err != nil {
		s.logFailure(ctx, req, profile, traceID, "ark_embed", start, err)
		return QueryResponse{}, err
	}
	if len(embeddings) == 0 {
		err := fmt.Errorf("embedding response is empty")
		s.logFailure(ctx, req, profile, traceID, "ark_embed", start, err)
		return QueryResponse{}, err
	}
	var warnings []string
	if cached, ok, warning := s.LookupSemanticCache(ctx, req, embeddings[0], traceID, profile, topK, start); ok {
		return cached, nil
	} else if warning != "" {
		warnings = append(warnings, warning)
	}
	searchReq := kb.SearchRequest{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Vector:          embeddings[0],
		TopK:            topK,
	}
	var results []kb.SearchResult
	if retriever, ok := s.Retriever.(interface {
		RetrieveWithWarnings(context.Context, kb.SearchRequest) ([]kb.SearchResult, []string, error)
	}); ok {
		var retrievalWarnings []string
		searchReq.TopK = req.TopK
		if req.TopK <= 0 {
			searchReq.DenseTopK = topK
			searchReq.SparseTopK = topK
		}
		results, retrievalWarnings, err = retriever.RetrieveWithWarnings(ctx, searchReq)
		warnings = append(warnings, retrievalWarnings...)
	} else {
		results, err = s.Retriever.Retrieve(ctx, searchReq)
	}
	if err != nil {
		s.logFailure(ctx, req, profile, traceID, "hybrid_retrieve", start, err)
		return QueryResponse{}, err
	}
	if len(results) == 0 {
		return QueryResponse{
			Answer:      s.NoContextAnswer,
			TraceID:     traceID,
			CacheStatus: "miss",
			Profile:     profile,
			Warnings:    append(warnings, "no_retrieved_context"),
			CreatedAt:   time.Now().UTC(),
			LatencyMS:   time.Since(start).Milliseconds(),
		}, nil
	}
	results = s.ApplyRerank(ctx, req.Query, results)
	contextText, citations := s.Packer.Pack(results)
	system := "你是一个严格基于给定上下文回答的 RAG 助手。回答必须使用中文，并在事实来自上下文时引用 chunk id。"
	if profile == ProfileHighPrecision {
		system += " 当前为高精档，请更充分整合上下文。"
	}
	user := fmt.Sprintf("问题：%s\n\n上下文：\n%s", req.Query, contextText)
	promptText := s.PromptStrategy.Apply([]prompt.Segment{
		{Name: "system", Stable: true, Content: system},
		{Name: "context", Stable: true, Content: contextText},
		{Name: "question", Stable: false, Content: req.Query},
	})
	answer, err := s.Model.Chat(ctx, []ark.ChatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user + "\n\n缓存稳定前缀：\n" + promptText},
	})
	if err != nil {
		s.logFailure(ctx, req, profile, traceID, "ark_generate", start, err)
		return QueryResponse{}, err
	}
	citations, validationWarnings := ValidateCitations(citations, results)
	warnings = append(warnings, validationWarnings...)
	resp := QueryResponse{
		Answer:          EnsureCitationHint(answer, citations),
		Citations:       citations,
		RetrievedChunks: results,
		TraceID:         traceID,
		CacheStatus:     "miss",
		Profile:         profile,
		Warnings:        warnings,
		CreatedAt:       time.Now().UTC(),
		LatencyMS:       time.Since(start).Milliseconds(),
	}
	if warning := s.StoreSemanticCache(ctx, req, embeddings[0], profile, topK, resp); warning != "" {
		resp.Warnings = append(resp.Warnings, warning)
	}
	return resp, nil
}

func (s *Service) logFailure(ctx context.Context, req QueryRequest, profile Profile, traceID, node string, start time.Time, err error) {
	if s.Logger == nil || err == nil {
		return
	}
	s.Logger.LogAttrs(ctx, slog.LevelError, "rag_failure",
		slog.String("trace_id", traceID),
		slog.String("tenant", req.TenantID),
		slog.String("profile", string(profile)),
		slog.String("node", node),
		slog.Int64("latency", time.Since(start).Milliseconds()),
		slog.String("error", err.Error()),
	)
}

func ensureRequestTrace(ctx context.Context, req QueryRequest) (context.Context, QueryRequest, string) {
	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = observability.EnsureTraceID(ctx)
	}
	req.TraceID = traceID
	return observability.WithTraceID(ctx, traceID), req, traceID
}

func (s *Service) LookupSemanticCache(ctx context.Context, req QueryRequest, vector []float64, traceID string, profile Profile, topK int, start time.Time) (QueryResponse, bool, string) {
	if s.Cache == nil || strings.TrimSpace(req.Query) == "" {
		return QueryResponse{}, false, ""
	}
	cached, ok, err := s.Cache.Lookup(ctx, SemanticCacheLookupRequest{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Vector:          vector,
		Threshold:       s.SemanticCacheThreshold,
		Profile:         profile,
		TopK:            topK,
	})
	if err != nil {
		return QueryResponse{}, false, "semantic cache lookup failed: " + err.Error()
	}
	if !ok {
		return QueryResponse{}, false, ""
	}
	if cachedProfile := cacheProfile(cached.Profile); cachedProfile != cacheProfile(profile) {
		return QueryResponse{}, false, ""
	}
	cached.CacheStatus = "hit"
	cached.TraceID = traceID
	cached.LatencyMS = time.Since(start).Milliseconds()
	cached.CreatedAt = time.Now().UTC()
	return cached, true, ""
}

func (s *Service) StoreSemanticCache(ctx context.Context, req QueryRequest, vector []float64, profile Profile, topK int, resp QueryResponse) string {
	if s.Cache == nil || len(resp.Citations) == 0 {
		return ""
	}
	cacheProfile := resp.Profile
	if cacheProfile == "" {
		cacheProfile = profile
	}
	if cacheProfile == "" {
		cacheProfile = s.Profile(req.Profile)
	}
	if resp.Profile == "" {
		resp.Profile = cacheProfile
	}
	if err := s.Cache.Store(ctx, SemanticCacheEntry{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Query:           req.Query,
		Vector:          vector,
		Profile:         cacheProfile,
		TopK:            topK,
		Response:        resp,
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		return "semantic cache write failed: " + err.Error()
	}
	return ""
}

func (s *Service) Profile(requested Profile) Profile {
	if requested != "" {
		return requested
	}
	if s.DefaultProfile != "" {
		return s.DefaultProfile
	}
	return ProfileRealtime
}

func cacheProfile(profile Profile) Profile {
	if profile == "" {
		return ProfileRealtime
	}
	return profile
}

func (s *Service) ApplyRerank(ctx context.Context, query string, results []kb.SearchResult) []kb.SearchResult {
	docs := make([]ark.RerankDocument, len(results))
	for i, result := range results {
		docs[i] = ark.RerankDocument{ID: result.Chunk.ID, Content: result.Chunk.Content}
	}
	reranked, err := s.Model.Rerank(ctx, query, docs, s.Packer.TopN)
	if err != nil || len(reranked) == 0 {
		return results
	}
	out := make([]kb.SearchResult, 0, len(reranked))
	for rank, item := range reranked {
		if item.Index < 0 || item.Index >= len(results) {
			continue
		}
		result := results[item.Index]
		result.Rank = rank + 1
		result.Score = item.Score
		result.From = "ark_rerank"
		out = append(out, result)
	}
	return out
}

func EnsureCitationHint(answer string, citations []Citation) string {
	if len(citations) == 0 || strings.Contains(answer, citations[0].ChunkID) {
		return answer
	}
	return answer + " [" + citations[0].ChunkID + "]"
}
