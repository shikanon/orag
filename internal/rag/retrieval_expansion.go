package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
)

const (
	maxGeneratedQueryRunes = 240
	maxSearchQueryRunes    = 2000
	maxHyDEDocumentRunes   = 2000
)

type RetrievalQuery struct {
	Query         string
	EmbeddingText string
	Source        string
}

func (s *Service) RewriteQuery(ctx context.Context, req QueryRequest, profile Profile) (string, []string) {
	if profile != ProfileHighPrecision || !s.QueryRewriteEnabled {
		return "", nil
	}
	base := cleanRetrievalText(req.Query, maxSearchQueryRunes)
	if s.Model == nil {
		return base, nil
	}
	answer, err := s.Model.Chat(ctx, []ark.ChatMessage{
		{Role: "system", Content: "你是 RAG 查询改写器。只输出一个适合检索的中文查询，不要解释。"},
		{Role: "user", Content: req.Query},
	})
	if err != nil {
		return base, []string{"query rewrite failed: " + err.Error()}
	}
	if rewritten := cleanRetrievalText(answer, maxGeneratedQueryRunes); rewritten != "" {
		return rewritten, nil
	}
	return base, nil
}

func (s *Service) BuildRetrievalQueries(ctx context.Context, req QueryRequest, profile Profile, rewrittenQuery string) ([]RetrievalQuery, []string) {
	base := cleanRetrievalText(rewrittenQuery, maxSearchQueryRunes)
	source := "rewrite"
	if base == "" {
		base = cleanRetrievalText(req.Query, maxSearchQueryRunes)
		source = "query"
	}
	var queries []RetrievalQuery
	addRetrievalQuery(&queries, RetrievalQuery{Query: base, Source: source})
	if profile != ProfileHighPrecision {
		return queries, nil
	}

	var warnings []string
	if s.MultiQueryCount > 1 {
		if s.Model == nil {
			warnings = append(warnings, "multi-query generation skipped: model unavailable")
		} else {
			generated, err := s.generateMultiQueries(ctx, req.Query, base, s.MultiQueryCount-1)
			if err != nil {
				warnings = append(warnings, "multi-query generation failed: "+err.Error())
			}
			for _, query := range generated {
				if len(queries) >= s.MultiQueryCount {
					break
				}
				addRetrievalQuery(&queries, RetrievalQuery{Query: query, Source: "multi_query"})
			}
		}
		if len(queries) < s.MultiQueryCount {
			warnings = append(warnings, fmt.Sprintf("multi-query expansion produced %d/%d retrieval queries", len(queries), s.MultiQueryCount))
		}
	}

	if s.HyDEEnabled {
		if s.Model == nil {
			warnings = append(warnings, "HyDE generation skipped: model unavailable")
		} else {
			doc, err := s.generateHyDEDocument(ctx, req.Query)
			if err != nil {
				warnings = append(warnings, "HyDE generation failed: "+err.Error())
			} else if doc == "" {
				warnings = append(warnings, "HyDE generation produced empty document")
			} else if !addRetrievalQuery(&queries, RetrievalQuery{Query: base, EmbeddingText: doc, Source: "hyde"}) {
				warnings = append(warnings, "HyDE generation produced duplicate retrieval document")
			}
		}
	}

	return queries, warnings
}

func (s *Service) RetrieveExpanded(ctx context.Context, req QueryRequest, topK int, queries []RetrievalQuery, queryVector []float64) ([]kb.SearchResult, []float64, []string, error) {
	if topK <= 0 {
		topK = s.TopK
	}
	if topK <= 0 {
		topK = 50
	}
	if len(queryVector) == 0 {
		vector, err := s.embedOne(ctx, req.Query)
		if err != nil {
			return nil, nil, nil, err
		}
		queryVector = vector
	}
	if len(queries) == 0 {
		queries = []RetrievalQuery{{Query: req.Query, Source: "query"}}
	}

	var warnings []string
	resultSets := make([][]kb.SearchResult, 0, len(queries))
	for _, query := range queries {
		searchQuery := cleanRetrievalText(query.Query, maxSearchQueryRunes)
		if searchQuery == "" {
			continue
		}
		embeddingText := strings.TrimSpace(query.EmbeddingText)
		if embeddingText == "" {
			embeddingText = searchQuery
		}
		vector := queryVector
		if normalizeRetrievalText(embeddingText) != normalizeRetrievalText(req.Query) {
			embedded, err := s.embedOne(ctx, embeddingText)
			if err != nil {
				return nil, queryVector, warnings, err
			}
			vector = embedded
		}
		searchReq := kb.SearchRequest{
			TenantID:        req.TenantID,
			KnowledgeBaseID: req.KnowledgeBaseID,
			Query:           searchQuery,
			Vector:          vector,
			TopK:            topK,
		}
		results, retrievalWarnings, err := s.retrieveOne(ctx, req, searchReq, topK)
		warnings = append(warnings, retrievalWarnings...)
		if err != nil {
			return nil, queryVector, warnings, err
		}
		resultSets = append(resultSets, results)
	}
	if len(resultSets) == 0 {
		return nil, queryVector, warnings, nil
	}
	if len(resultSets) == 1 {
		return resultSets[0], queryVector, warnings, nil
	}
	return kb.RRF(resultSets, 60, topK), queryVector, warnings, nil
}

func (s *Service) generateMultiQueries(ctx context.Context, originalQuery, baseQuery string, count int) ([]string, error) {
	answer, err := s.Model.Chat(ctx, []ark.ChatMessage{
		{Role: "system", Content: "你是 RAG 多查询生成器。输出 JSON 字符串数组，每个元素是一条不同检索查询，不要解释。"},
		{Role: "user", Content: fmt.Sprintf("原始问题：%s\n基础检索查询：%s\n生成 %d 条互补查询。", originalQuery, baseQuery, count)},
	})
	if err != nil {
		return nil, err
	}
	return parseGeneratedQueries(answer, count), nil
}

func (s *Service) generateHyDEDocument(ctx context.Context, query string) (string, error) {
	answer, err := s.Model.Chat(ctx, []ark.ChatMessage{
		{Role: "system", Content: "你是 RAG HyDE 生成器。基于问题写一段可能出现在相关文档中的简短假设答案，只输出答案。"},
		{Role: "user", Content: query},
	})
	if err != nil {
		return "", err
	}
	return cleanRetrievalText(answer, maxHyDEDocumentRunes), nil
}

func (s *Service) embedOne(ctx context.Context, text string) ([]float64, error) {
	if s.Model == nil {
		return nil, fmt.Errorf("rag model is nil")
	}
	embeddings, err := s.Model.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("embedding response is empty")
	}
	return embeddings[0], nil
}

func (s *Service) retrieveOne(ctx context.Context, req QueryRequest, searchReq kb.SearchRequest, topK int) ([]kb.SearchResult, []string, error) {
	if retriever, ok := s.Retriever.(interface {
		RetrieveWithWarnings(context.Context, kb.SearchRequest) ([]kb.SearchResult, []string, error)
	}); ok {
		searchReq.TopK = req.TopK
		if req.TopK <= 0 {
			searchReq.DenseTopK = topK
			searchReq.SparseTopK = topK
		}
		return retriever.RetrieveWithWarnings(ctx, searchReq)
	}
	results, err := s.Retriever.Retrieve(ctx, searchReq)
	return results, nil, err
}

func parseGeneratedQueries(raw string, limit int) []string {
	raw = stripCodeFence(strings.TrimSpace(raw))
	var decoded []string
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return cleanQueryList(decoded, limit)
	}
	var wrapped map[string][]string
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil {
		for _, key := range []string{"queries", "query", "items"} {
			if values := cleanQueryList(wrapped[key], limit); len(values) > 0 {
				return values
			}
		}
	}
	lines := strings.Split(raw, "\n")
	return cleanQueryList(lines, limit)
}

func cleanQueryList(values []string, limit int) []string {
	out := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for _, value := range values {
		cleaned := cleanRetrievalText(value, maxGeneratedQueryRunes)
		if cleaned == "" {
			continue
		}
		key := normalizeRetrievalText(cleaned)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cleaned)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func addRetrievalQuery(queries *[]RetrievalQuery, query RetrievalQuery) bool {
	query.Query = cleanRetrievalText(query.Query, maxSearchQueryRunes)
	query.EmbeddingText = cleanRetrievalText(query.EmbeddingText, maxHyDEDocumentRunes)
	if query.Query == "" {
		return false
	}
	key := normalizeRetrievalText(query.Query) + "\x00" + normalizeRetrievalText(query.EmbeddingText)
	for _, existing := range *queries {
		existingKey := normalizeRetrievalText(existing.Query) + "\x00" + normalizeRetrievalText(existing.EmbeddingText)
		if existingKey == key {
			return false
		}
	}
	*queries = append(*queries, query)
	return true
}

func cleanRetrievalText(text string, maxRunes int) string {
	text = strings.Trim(strings.TrimSpace(stripListMarker(text)), "\"'`")
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	if maxRunes > 0 && len([]rune(text)) > maxRunes {
		return ""
	}
	return text
}

func normalizeRetrievalText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(text))), " ")
}

func stripCodeFence(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return text
}

func stripListMarker(text string) string {
	text = strings.TrimLeft(strings.TrimSpace(text), "-*• \t")
	runes := []rune(text)
	i := 0
	for i < len(runes) && unicode.IsDigit(runes[i]) {
		i++
	}
	if i > 0 && i < len(runes) {
		switch runes[i] {
		case '.', ')', '、':
			return strings.TrimSpace(string(runes[i+1:]))
		}
	}
	return text
}
