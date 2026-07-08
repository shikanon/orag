package offlineknowledge

import (
	"encoding/json"
	"time"
)

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

type ItemStatus string

const (
	ItemStatusCandidate          ItemStatus = "candidate"
	ItemStatusEvidenceValidating ItemStatus = "evidence_validating"
	ItemStatusNeedsReview        ItemStatus = "needs_review"
	ItemStatusVerified           ItemStatus = "verified"
	ItemStatusShadowEnabled      ItemStatus = "shadow_enabled"
	ItemStatusRegressionPassed   ItemStatus = "regression_passed"
	ItemStatusRegressionFailed   ItemStatus = "regression_failed"
	ItemStatusPublished          ItemStatus = "published"
	ItemStatusKnowledgeGap       ItemStatus = "knowledge_gap"
	ItemStatusRejected           ItemStatus = "rejected"
	ItemStatusStale              ItemStatus = "stale"
	ItemStatusDeprecated         ItemStatus = "deprecated"
)

type ItemType string

const (
	ItemTypeAnswer       ItemType = "answer_item"
	ItemTypeQueryRewrite ItemType = "query_rewrite_item"
	ItemTypeKnowledgeGap ItemType = "knowledge_gap_item"
)

type RecallQuality string

const (
	RecallQualityHit          RecallQuality = "hit"
	RecallQualityPartialHit   RecallQuality = "partial_hit"
	RecallQualityMiss         RecallQuality = "miss"
	RecallQualityBadAnswer    RecallQuality = "bad_answer"
	RecallQualityNoAnswerInKB RecallQuality = "no_answer_in_kb"
	RecallQualityAmbiguous    RecallQuality = "ambiguous"
	RecallQualityDuplicate    RecallQuality = "duplicate"
)

type FailureType string

const (
	FailureTypeKeywordMismatch FailureType = "keyword_mismatch"
	FailureTypeSemanticGap     FailureType = "semantic_gap"
	FailureTypeChunkBoundary   FailureType = "chunk_boundary"
	FailureTypeRerankError     FailureType = "rerank_error"
	FailureTypeGraphMissing    FailureType = "graph_missing"
	FailureTypeGenerationError FailureType = "generation_error"
	FailureTypeKnowledgeGap    FailureType = "knowledge_gap"
	FailureTypeUnclearQuestion FailureType = "unclear_question"
)

type SourceFingerprint struct {
	DocID            string `json:"doc_id"`
	DocVersion       string `json:"doc_version"`
	ChunkID          string `json:"chunk_id"`
	ChunkContentHash string `json:"chunk_content_hash"`
}

type Evidence struct {
	ChunkID  string `json:"chunk_id"`
	DocID    string `json:"doc_id"`
	Quote    string `json:"quote"`
	Supports string `json:"supports"`
}

type DeepSearchStep struct {
	Step        int    `json:"step"`
	Tool        string `json:"tool"`
	Query       string `json:"query"`
	Observation string `json:"observation"`
	Decision    string `json:"decision"`
}

type OfflineKnowledgeRun struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	KBID        string         `json:"kb_id"`
	Status      RunStatus      `json:"status"`
	WindowStart time.Time      `json:"window_start"`
	WindowEnd   time.Time      `json:"window_end"`
	ConfigHash  string         `json:"config_hash"`
	ConfigJSON  map[string]any `json:"config_json,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  time.Time      `json:"finished_at,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type QuestionCluster struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	RunID              string    `json:"run_id"`
	KBID               string    `json:"kb_id"`
	CanonicalQuestion  string    `json:"canonical_question"`
	NormalizedQuestion string    `json:"normalized_question"`
	QuestionHash       string    `json:"question_hash"`
	EmbeddingRef       string    `json:"embedding_ref,omitempty"`
	EmbeddingJSON      []float64 `json:"embedding_json,omitempty"`
	OccurrenceCount    int       `json:"occurrence_count"`
	SampleQuestions    []string  `json:"sample_questions"`
	TraceIDs           []string  `json:"trace_ids"`
	LongTail           bool      `json:"long_tail,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type OptimizationItem struct {
	ID                 string              `json:"id"`
	TenantID           string              `json:"tenant_id"`
	RunID              string              `json:"run_id"`
	KBID               string              `json:"kb_id"`
	QuestionClusterID  string              `json:"question_cluster_id"`
	ItemType           ItemType            `json:"item_type"`
	Status             ItemStatus          `json:"status"`
	CanonicalQuestion  string              `json:"canonical_question"`
	FinalAnswer        string              `json:"final_answer,omitempty"`
	RecallQuality      RecallQuality       `json:"recall_quality"`
	FailureType        FailureType         `json:"failure_type,omitempty"`
	Confidence         float64             `json:"confidence"`
	SourceFingerprints []SourceFingerprint `json:"source_fingerprints"`
	Evidence           []Evidence          `json:"evidence"`
	DeepSearchSteps    []DeepSearchStep    `json:"deep_search_steps"`
	EvalReportJSON     json.RawMessage     `json:"eval_report_json,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	PublishedAt        time.Time           `json:"published_at,omitempty"`
}

type ShadowRetrievalEvent struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	KBID              string    `json:"kb_id"`
	ItemID            string    `json:"item_id"`
	TraceID           string    `json:"trace_id"`
	Query             string    `json:"query"`
	Matched           bool      `json:"matched"`
	Injected          bool      `json:"injected"`
	Rank              int       `json:"rank,omitempty"`
	Score             float64   `json:"score,omitempty"`
	RecallLift        float64   `json:"recall_lift,omitempty"`
	AnswerLift        float64   `json:"answer_lift,omitempty"`
	HallucinationRisk float64   `json:"hallucination_risk,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

var itemStatusTransitions = map[ItemStatus]map[ItemStatus]struct{}{
	ItemStatusCandidate: {
		ItemStatusEvidenceValidating: {},
	},
	ItemStatusEvidenceValidating: {
		ItemStatusRejected:     {},
		ItemStatusKnowledgeGap: {},
		ItemStatusNeedsReview:  {},
		ItemStatusVerified:     {},
	},
	ItemStatusNeedsReview: {
		ItemStatusVerified: {},
		ItemStatusRejected: {},
	},
	ItemStatusVerified: {
		ItemStatusShadowEnabled: {},
		ItemStatusRejected:      {},
	},
	ItemStatusShadowEnabled: {
		ItemStatusRegressionPassed: {},
		ItemStatusRegressionFailed: {},
	},
	ItemStatusRegressionFailed: {
		ItemStatusNeedsReview: {},
		ItemStatusRejected:    {},
		ItemStatusVerified:    {},
	},
	ItemStatusRegressionPassed: {
		ItemStatusPublished:  {},
		ItemStatusRejected:   {},
		ItemStatusDeprecated: {},
	},
	ItemStatusPublished: {
		ItemStatusStale:      {},
		ItemStatusDeprecated: {},
	},
	ItemStatusStale: {
		ItemStatusEvidenceValidating: {},
		ItemStatusDeprecated:         {},
	},
	ItemStatusRejected: {
		ItemStatusCandidate: {},
	},
	ItemStatusKnowledgeGap: {
		ItemStatusCandidate: {},
	},
}

func CanTransition(from, to ItemStatus) bool {
	next, ok := itemStatusTransitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}
