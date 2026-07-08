package offlineknowledge

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrCodexInvalidEnum        = errors.New("codex response invalid enum")
	ErrCodexMissingEvidence    = errors.New("codex response answer item missing evidence")
	ErrCodexActionItemMismatch = errors.New("codex response action item type mismatch")
	ErrCodexStepBudgetExceeded = errors.New("codex deep search step budget exceeded")
	ErrCodexUnknownTool        = errors.New("codex tool is not allowed")
	ErrCodexQuotaExceeded      = errors.New("codex tool quota exceeded")
)

type CodexAnalyzer interface {
	AnalyzeCodex(ctx context.Context, request CodexAnalyzeRequest) (CodexAnalyzeResponse, error)
}

type CodexAnalyzeRequest struct {
	TenantID              string               `json:"tenant_id"`
	KBID                  string               `json:"kb_id"`
	CanonicalQuestion     string               `json:"canonical_question"`
	SampleQuestions       []string             `json:"sample_questions"`
	BaselineRecallResults []BaselineRecallItem `json:"baseline_recall_results"`
	TraceSummaries        []TraceSummary       `json:"trace_summaries"`
	Metadata              map[string]any       `json:"metadata,omitempty"`
	Constraints           CodexConstraints     `json:"constraints"`
}

type BaselineRecallItem struct {
	TraceID          string  `json:"trace_id,omitempty"`
	ChunkID          string  `json:"chunk_id"`
	DocID            string  `json:"doc_id"`
	DocVersion       string  `json:"doc_version,omitempty"`
	ChunkContentHash string  `json:"chunk_content_hash,omitempty"`
	Rank             int     `json:"rank"`
	Score            float64 `json:"score"`
	Matched          bool    `json:"matched"`
}

type TraceSummary struct {
	TraceID         string        `json:"trace_id"`
	Query           string        `json:"query"`
	Answer          string        `json:"answer,omitempty"`
	RetrievedChunks []string      `json:"retrieved_chunks,omitempty"`
	Latency         time.Duration `json:"latency,omitempty"`
	HasError        bool          `json:"has_error,omitempty"`
	Error           string        `json:"error,omitempty"`
}

type CodexConstraints struct {
	ReadOnlyTools      []ReadOnlyToolName  `json:"read_only_tools"`
	Quota              ToolQuota           `json:"quota"`
	RequireEvidence    bool                `json:"require_evidence"`
	MaxDeepSearchSteps int                 `json:"max_deep_search_steps"`
	AllowedItemTypes   []ItemType          `json:"allowed_item_types,omitempty"`
	AllowedActions     []RecommendedAction `json:"allowed_actions,omitempty"`
}

type CodexAnalyzeResponse struct {
	ItemType          ItemType          `json:"item_type"`
	RecommendedAction RecommendedAction `json:"recommended_action"`
	RecallQuality     RecallQuality     `json:"recall_quality"`
	FailureType       FailureType       `json:"failure_type,omitempty"`
	Confidence        float64           `json:"confidence"`
	FinalAnswer       string            `json:"final_answer,omitempty"`
	Evidence          []Evidence        `json:"evidence,omitempty"`
	MissingEvidence   []string          `json:"missing_evidence,omitempty"`
	DeepSearchSteps   []DeepSearchStep  `json:"deep_search_steps,omitempty"`
	ToolUsage         []ToolUsage       `json:"tool_usage,omitempty"`
	Metadata          map[string]any    `json:"metadata,omitempty"`
}

type RecommendedAction string

const (
	RecommendedActionCreateAnswerItem       RecommendedAction = "create_answer_item"
	RecommendedActionCreateQueryRewriteItem RecommendedAction = "create_query_rewrite_item"
	RecommendedActionCreateKnowledgeGapItem RecommendedAction = "create_knowledge_gap_item"
	RecommendedActionNeedsReview            RecommendedAction = "needs_review"
	RecommendedActionReject                 RecommendedAction = "reject"
)

type ReadOnlyToolName string

const (
	ReadOnlyToolSearchChunksByText ReadOnlyToolName = "search_chunks_by_text"
	ReadOnlyToolSearchChunksVector ReadOnlyToolName = "search_chunks_by_vector"
	ReadOnlyToolGetChunkNeighbors  ReadOnlyToolName = "get_chunk_neighbors"
	ReadOnlyToolGetDocumentChunks  ReadOnlyToolName = "get_document_chunks"
	ReadOnlyToolGetGraphChunks     ReadOnlyToolName = "get_related_graph_chunks"
	ReadOnlyToolLookupEvalResults  ReadOnlyToolName = "get_eval_results_by_question"
	ReadOnlyToolLookupExistingItem ReadOnlyToolName = "get_existing_optimization_items"
	ReadOnlyToolReplayRecall       ReadOnlyToolName = "replay_recall_with_query"
)

type ToolQuota struct {
	MaxTokens          int           `json:"max_tokens"`
	MaxRowsPerCall     int           `json:"max_rows_per_call"`
	MaxQPSPerTenant    int           `json:"max_qps_per_tenant"`
	MaxTimeout         time.Duration `json:"max_timeout"`
	MaxDeepSearchSteps int           `json:"max_deep_search_steps"`
}

type ToolUsage struct {
	Tool    ReadOnlyToolName `json:"tool"`
	Tokens  int              `json:"tokens"`
	Rows    int              `json:"rows"`
	QPS     int              `json:"qps"`
	Timeout time.Duration    `json:"timeout"`
	Steps   int              `json:"steps"`
}

type ToolGuard struct {
	quota ToolQuota
}

func NewToolGuard(quota ToolQuota) ToolGuard {
	return ToolGuard{quota: quota}
}

func ValidateCodexResponse(response CodexAnalyzeResponse, quota ToolQuota) error {
	if !isValidItemType(response.ItemType) {
		return fmt.Errorf("%w: item_type %q", ErrCodexInvalidEnum, response.ItemType)
	}
	if !isValidRecommendedAction(response.RecommendedAction) {
		return fmt.Errorf("%w: recommended_action %q", ErrCodexInvalidEnum, response.RecommendedAction)
	}
	if err := validateRecommendedActionItemType(response.RecommendedAction, response.ItemType); err != nil {
		return err
	}
	if !isValidRecallQuality(response.RecallQuality) {
		return fmt.Errorf("%w: recall_quality %q", ErrCodexInvalidEnum, response.RecallQuality)
	}
	if response.FailureType != "" && !isValidFailureType(response.FailureType) {
		return fmt.Errorf("%w: failure_type %q", ErrCodexInvalidEnum, response.FailureType)
	}
	if response.Confidence < 0 || response.Confidence > 1 {
		return fmt.Errorf("%w: confidence %.4f", ErrCodexInvalidEnum, response.Confidence)
	}
	if response.ItemType == ItemTypeAnswer && len(response.Evidence) == 0 {
		return ErrCodexMissingEvidence
	}
	if quota.MaxDeepSearchSteps > 0 && len(response.DeepSearchSteps) > quota.MaxDeepSearchSteps {
		return fmt.Errorf("%w: got %d, max %d", ErrCodexStepBudgetExceeded, len(response.DeepSearchSteps), quota.MaxDeepSearchSteps)
	}

	guard := NewToolGuard(quota)
	for _, step := range response.DeepSearchSteps {
		if step.Step <= 0 {
			return fmt.Errorf("%w: deep_search_steps step %d", ErrCodexInvalidEnum, step.Step)
		}
		if !isReadOnlyTool(ReadOnlyToolName(step.Tool)) {
			return fmt.Errorf("%w: %q", ErrCodexUnknownTool, step.Tool)
		}
	}
	for _, usage := range response.ToolUsage {
		if err := guard.Validate(usage); err != nil {
			return err
		}
	}
	return nil
}

func (g ToolGuard) Validate(usage ToolUsage) error {
	if !isReadOnlyTool(usage.Tool) {
		return fmt.Errorf("%w: %q", ErrCodexUnknownTool, usage.Tool)
	}
	if usage.Tokens < 0 || usage.Rows < 0 || usage.QPS < 0 || usage.Timeout < 0 || usage.Steps < 0 {
		return fmt.Errorf("%w: negative usage for %q", ErrCodexQuotaExceeded, usage.Tool)
	}
	if g.quota.MaxTokens > 0 && usage.Tokens > g.quota.MaxTokens {
		return fmt.Errorf("%w: tool %q tokens %d, max %d", ErrCodexQuotaExceeded, usage.Tool, usage.Tokens, g.quota.MaxTokens)
	}
	if g.quota.MaxRowsPerCall > 0 && usage.Rows > g.quota.MaxRowsPerCall {
		return fmt.Errorf("%w: tool %q rows %d, max %d", ErrCodexQuotaExceeded, usage.Tool, usage.Rows, g.quota.MaxRowsPerCall)
	}
	if g.quota.MaxQPSPerTenant > 0 && usage.QPS > g.quota.MaxQPSPerTenant {
		return fmt.Errorf("%w: tool %q qps %d, max %d", ErrCodexQuotaExceeded, usage.Tool, usage.QPS, g.quota.MaxQPSPerTenant)
	}
	if g.quota.MaxTimeout > 0 && usage.Timeout > g.quota.MaxTimeout {
		return fmt.Errorf("%w: tool %q timeout %s, max %s", ErrCodexQuotaExceeded, usage.Tool, usage.Timeout, g.quota.MaxTimeout)
	}
	if g.quota.MaxDeepSearchSteps > 0 && usage.Steps > g.quota.MaxDeepSearchSteps {
		return fmt.Errorf("%w: tool %q steps %d, max %d", ErrCodexQuotaExceeded, usage.Tool, usage.Steps, g.quota.MaxDeepSearchSteps)
	}
	return nil
}

func isValidItemType(itemType ItemType) bool {
	switch itemType {
	case ItemTypeAnswer, ItemTypeQueryRewrite, ItemTypeKnowledgeGap:
		return true
	default:
		return false
	}
}

func isValidRecommendedAction(action RecommendedAction) bool {
	switch action {
	case RecommendedActionCreateAnswerItem,
		RecommendedActionCreateQueryRewriteItem,
		RecommendedActionCreateKnowledgeGapItem,
		RecommendedActionNeedsReview,
		RecommendedActionReject:
		return true
	default:
		return false
	}
}

func validateRecommendedActionItemType(action RecommendedAction, itemType ItemType) error {
	expected, constrained := recommendedActionItemType(action)
	if !constrained || itemType == expected {
		return nil
	}
	return fmt.Errorf("%w: action %q requires item_type %q, got %q", ErrCodexActionItemMismatch, action, expected, itemType)
}

func recommendedActionItemType(action RecommendedAction) (ItemType, bool) {
	switch action {
	case RecommendedActionCreateAnswerItem:
		return ItemTypeAnswer, true
	case RecommendedActionCreateQueryRewriteItem:
		return ItemTypeQueryRewrite, true
	case RecommendedActionCreateKnowledgeGapItem:
		return ItemTypeKnowledgeGap, true
	default:
		return "", false
	}
}

func isValidRecallQuality(quality RecallQuality) bool {
	switch quality {
	case RecallQualityHit,
		RecallQualityPartialHit,
		RecallQualityMiss,
		RecallQualityBadAnswer,
		RecallQualityNoAnswerInKB,
		RecallQualityAmbiguous,
		RecallQualityDuplicate:
		return true
	default:
		return false
	}
}

func isValidFailureType(failureType FailureType) bool {
	switch failureType {
	case FailureTypeKeywordMismatch,
		FailureTypeSemanticGap,
		FailureTypeChunkBoundary,
		FailureTypeRerankError,
		FailureTypeGraphMissing,
		FailureTypeGenerationError,
		FailureTypeKnowledgeGap,
		FailureTypeUnclearQuestion:
		return true
	default:
		return false
	}
}

func isReadOnlyTool(tool ReadOnlyToolName) bool {
	switch tool {
	case ReadOnlyToolSearchChunksByText,
		ReadOnlyToolSearchChunksVector,
		ReadOnlyToolGetChunkNeighbors,
		ReadOnlyToolGetDocumentChunks,
		ReadOnlyToolGetGraphChunks,
		ReadOnlyToolLookupEvalResults,
		ReadOnlyToolLookupExistingItem,
		ReadOnlyToolReplayRecall:
		return true
	default:
		return false
	}
}
