package http

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/ingest/chunkpreview"
)

const (
	chunkPreviewMaturityHeader = "X-Orag-Maturity"
	chunkPreviewMaturityValue  = "experimental"
)

type chunkPreviewRequest struct {
	Text             string `json:"text"`
	Strategy         string `json:"strategy"`
	SizeTokens       int    `json:"size_tokens"`
	OverlapTokens    int    `json:"overlap_tokens"`
	ParentChild      bool   `json:"parent_child"`
	ParentSizeTokens int    `json:"parent_size_tokens"`
	ChildSizeTokens  int    `json:"child_size_tokens"`
	SampleCount      int    `json:"sample_count"`
}

type chunkPreviewStrategySelectedResponse struct {
	Strategy           string  `json:"strategy"`
	Reason             string  `json:"reason"`
	Confidence         float64 `json:"confidence"`
	RecommendedSize    int     `json:"recommended_size"`
	RecommendedOverlap int     `json:"recommended_overlap"`
}

type chunkPreviewStatsResponse struct {
	TotalChunks      int            `json:"total_chunks"`
	TotalTokens      int            `json:"total_tokens"`
	AvgChunkTokens   float64        `json:"avg_chunk_tokens"`
	MinChunkTokens   int            `json:"min_chunk_tokens"`
	MaxChunkTokens   int            `json:"max_chunk_tokens"`
	SizeDistribution map[string]int `json:"size_distribution"`
	OrphanChunks     int            `json:"orphan_chunks"`
}

type chunkPreviewSampleChunkResponse struct {
	Index   int    `json:"index"`
	Content string `json:"content"`
	Section string `json:"section"`
	Tokens  int    `json:"tokens"`
	Offset  int    `json:"offset"`
}

type chunkPreviewResponse struct {
	StrategySelected chunkPreviewStrategySelectedResponse `json:"strategy_selected"`
	Stats            chunkPreviewStatsResponse            `json:"stats"`
	SampleChunks     []chunkPreviewSampleChunkResponse    `json:"sample_chunks"`
	Warnings         []string                             `json:"warnings"`
}

func (s *Server) chunkPreview(ctx context.Context, c *app.RequestContext) {
	_, ok := requestPrincipal(c)
	if !ok {
		writeError(c, consts.StatusForbidden, "forbidden", "request is not authorized")
		return
	}

	var req chunkPreviewRequest
	if !bindJSON(c, &req) {
		return
	}

	if strings.TrimSpace(req.Text) == "" {
		writeError(c, consts.StatusBadRequest, "invalid_request", "text is required")
		return
	}

	strategy := strings.TrimSpace(req.Strategy)
	if strategy == "" {
		strategy = chunkpreview.StrategyAuto
	}

	config := chunkpreview.Config{
		Strategy:         strategy,
		SizeTokens:       req.SizeTokens,
		OverlapTokens:    req.OverlapTokens,
		ParentChild:      req.ParentChild,
		ParentSizeTokens: req.ParentSizeTokens,
		ChildSizeTokens:  req.ChildSizeTokens,
		SampleCount:      req.SampleCount,
	}

	result := chunkpreview.Preview(req.Text, config)

	resp := chunkPreviewResponse{
		StrategySelected: chunkPreviewStrategySelectedResponse{
			Strategy:           result.StrategySelected.Strategy,
			Reason:             result.StrategySelected.Reason,
			Confidence:         result.StrategySelected.Confidence,
			RecommendedSize:    result.StrategySelected.RecommendedSize,
			RecommendedOverlap: result.StrategySelected.RecommendedOverlap,
		},
		Stats: chunkPreviewStatsResponse{
			TotalChunks:      result.Stats.TotalChunks,
			TotalTokens:      result.Stats.TotalTokens,
			AvgChunkTokens:   result.Stats.AvgChunkTokens,
			MinChunkTokens:   result.Stats.MinChunkTokens,
			MaxChunkTokens:   result.Stats.MaxChunkTokens,
			SizeDistribution: result.Stats.SizeDistribution,
			OrphanChunks:     result.Stats.OrphanChunks,
		},
		SampleChunks: make([]chunkPreviewSampleChunkResponse, 0, len(result.SampleChunks)),
		Warnings:     result.Warnings,
	}

	for _, sc := range result.SampleChunks {
		resp.SampleChunks = append(resp.SampleChunks, chunkPreviewSampleChunkResponse{
			Index:   sc.Index,
			Content: sc.Content,
			Section: sc.Section,
			Tokens:  sc.Tokens,
			Offset:  sc.Offset,
		})
	}

	setChunkPreviewMaturityHeader(c)
	c.JSON(consts.StatusOK, resp)
}

type chunkingImpactRequest struct {
	Strategy      string `json:"strategy"`
	SizeTokens    int    `json:"size_tokens"`
	OverlapTokens int    `json:"overlap_tokens"`
}

type chunkingImpactResponse struct {
	EstimatedAffectedDocuments int    `json:"estimated_affected_documents"`
	EstimatedChunkWrites       int    `json:"estimated_chunk_writes"`
	AffectsProductionPipeline  bool   `json:"affects_production_pipeline"`
	Note                       string `json:"note"`
}

func (s *Server) chunkingImpact(ctx context.Context, c *app.RequestContext) {
	kbID := c.Param("id")
	if _, ok := s.authorizedKnowledgeBase(ctx, c, kbID, auth.ActionResourceRead); !ok {
		return
	}

	var req chunkingImpactRequest
	if !bindJSON(c, &req) {
		return
	}

	resp := chunkingImpactResponse{
		EstimatedAffectedDocuments: 0,
		EstimatedChunkWrites:       0,
		AffectsProductionPipeline:  false,
		Note:                       "impact estimation requires document access, preview mode only",
	}

	setChunkPreviewMaturityHeader(c)
	c.JSON(consts.StatusOK, resp)
}

func setChunkPreviewMaturityHeader(c *app.RequestContext) {
	c.Header(chunkPreviewMaturityHeader, chunkPreviewMaturityValue)
}
