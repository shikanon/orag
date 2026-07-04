package eval

import (
	"time"

	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

type EvaluationMode string

const (
	EvaluationModeRuleBased EvaluationMode = "rule_based"
	EvaluationModeLLMJudge  EvaluationMode = "llm_judge"
	EvaluationModeHybrid    EvaluationMode = "hybrid"
)

type JudgeMetric string

const (
	JudgeMetricFaithfulness      JudgeMetric = "faithfulness"
	JudgeMetricGroundedness      JudgeMetric = "groundedness"
	JudgeMetricAnswerRelevance   JudgeMetric = "answer_relevance"
	JudgeMetricHallucination     JudgeMetric = "hallucination"
	JudgeMetricCompleteness      JudgeMetric = "completeness"
	JudgeMetricCitationSupport   JudgeMetric = "citation_support"
	JudgeMetricInstructionFollow JudgeMetric = "instruction_following"
	JudgeMetricSafety            JudgeMetric = "safety"
)

type JudgeConfig struct {
	Provider        string             `json:"provider,omitempty"`
	Model           string             `json:"model,omitempty"`
	PromptVersion   string             `json:"prompt_version,omitempty"`
	Metrics         []JudgeMetric      `json:"metrics,omitempty"`
	Rubric          JudgeRubric        `json:"rubric,omitempty"`
	Temperature     float64            `json:"temperature,omitempty"`
	MaxTokens       int                `json:"max_tokens,omitempty"`
	Repeat          int                `json:"repeat,omitempty"`
	PairwiseSwap    bool               `json:"pairwise_swap,omitempty"`
	StrictJSON      bool               `json:"strict_json,omitempty"`
	Ensemble        []JudgeModelConfig `json:"ensemble,omitempty"`
	Timeout         time.Duration      `json:"timeout,omitempty"`
	MaxRetries      int                `json:"max_retries,omitempty"`
	BackoffInitial  time.Duration      `json:"backoff_initial,omitempty"`
	BackoffMax      time.Duration      `json:"backoff_max,omitempty"`
	BackoffJitter   float64            `json:"backoff_jitter,omitempty"`
	MaxJudgeCalls   int                `json:"max_judge_calls,omitempty"`
	CircuitFailures int                `json:"circuit_failures,omitempty"`
	ExtraParams     map[string]any     `json:"extra_params,omitempty"`
	PromptTemplate  string             `json:"prompt_template,omitempty"`
}

type JudgeModelConfig struct {
	Provider    string         `json:"provider,omitempty"`
	Model       string         `json:"model,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
}

type JudgeRubric struct {
	Name     string            `json:"name,omitempty"`
	Version  string            `json:"version,omitempty"`
	Criteria []RubricCriterion `json:"criteria,omitempty"`
}

type RubricCriterion struct {
	Metric      JudgeMetric `json:"metric"`
	Description string      `json:"description,omitempty"`
	Weight      float64     `json:"weight,omitempty"`
	Scale       []string    `json:"scale,omitempty"`
}

type JudgeInput struct {
	TenantID         string            `json:"tenant_id,omitempty"`
	DatasetID        string            `json:"dataset_id,omitempty"`
	DatasetItemID    string            `json:"dataset_item_id,omitempty"`
	Query            string            `json:"query"`
	GroundTruth      string            `json:"ground_truth,omitempty"`
	ExpectedEvidence []string          `json:"expected_evidence,omitempty"`
	RelevantDocIDs   []string          `json:"relevant_doc_ids,omitempty"`
	Answer           string            `json:"answer"`
	Citations        []rag.Citation    `json:"citations,omitempty"`
	RetrievedChunks  []kb.SearchResult `json:"retrieved_chunks,omitempty"`
	TraceID          string            `json:"trace_id,omitempty"`
	CandidateID      string            `json:"candidate_id,omitempty"`
	Rubric           JudgeRubric       `json:"rubric,omitempty"`
}

type JudgeOutput struct {
	Scores        map[string]float64            `json:"scores,omitempty"`
	Labels        map[string]string             `json:"labels,omitempty"`
	Confidence    map[string]ConfidenceInterval `json:"confidence,omitempty"`
	Pass          bool                          `json:"pass"`
	Rationale     string                        `json:"rationale,omitempty"`
	Findings      []JudgeFinding                `json:"findings,omitempty"`
	RawResponse   string                        `json:"raw_response,omitempty"`
	ParsedJSON    map[string]any                `json:"parsed_json,omitempty"`
	TokenUsage    TokenUsage                    `json:"token_usage,omitempty"`
	CostUSD       float64                       `json:"cost_usd,omitempty"`
	JudgeModel    string                        `json:"judge_model,omitempty"`
	PromptVersion string                        `json:"prompt_version,omitempty"`
	RubricHash    string                        `json:"rubric_hash,omitempty"`
	ConfigHash    string                        `json:"config_hash,omitempty"`
	CreatedAt     time.Time                     `json:"created_at"`
}

type JudgeFinding struct {
	Metric   string `json:"metric,omitempty"`
	Label    string `json:"label,omitempty"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

type CandidateAnswer struct {
	ID              string            `json:"id,omitempty"`
	Answer          string            `json:"answer"`
	Citations       []rag.Citation    `json:"citations,omitempty"`
	RetrievedChunks []kb.SearchResult `json:"retrieved_chunks,omitempty"`
}

type PairwiseJudgeInput struct {
	TenantID         string          `json:"tenant_id,omitempty"`
	DatasetID        string          `json:"dataset_id,omitempty"`
	DatasetItemID    string          `json:"dataset_item_id,omitempty"`
	Query            string          `json:"query"`
	GroundTruth      string          `json:"ground_truth,omitempty"`
	ExpectedEvidence []string        `json:"expected_evidence,omitempty"`
	RelevantDocIDs   []string        `json:"relevant_doc_ids,omitempty"`
	AnswerA          CandidateAnswer `json:"answer_a"`
	AnswerB          CandidateAnswer `json:"answer_b"`
	Rubric           JudgeRubric     `json:"rubric,omitempty"`
}

type PairwiseJudgeOutput struct {
	Winner             string         `json:"winner"`
	Preference         string         `json:"preference,omitempty"`
	PreferenceStrength float64        `json:"preference_strength,omitempty"`
	Stable             bool           `json:"stable"`
	VoteCount          int            `json:"vote_count,omitempty"`
	Reasons            []JudgeFinding `json:"reasons,omitempty"`
	RawResponse        string         `json:"raw_response,omitempty"`
	ParsedJSON         map[string]any `json:"parsed_json,omitempty"`
	TokenUsage         TokenUsage     `json:"token_usage,omitempty"`
	CostUSD            float64        `json:"cost_usd,omitempty"`
	JudgeModel         string         `json:"judge_model,omitempty"`
	PromptVersion      string         `json:"prompt_version,omitempty"`
	RubricHash         string         `json:"rubric_hash,omitempty"`
	ConfigHash         string         `json:"config_hash,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}

type ConfidenceInterval struct {
	Mean   float64 `json:"mean"`
	Low    float64 `json:"low"`
	High   float64 `json:"high"`
	N      int     `json:"n"`
	Method string  `json:"method,omitempty"`
}

type QAGOutput struct {
	Score         float64            `json:"score"`
	Metrics       map[string]float64 `json:"metrics,omitempty"`
	Claims        []QAGClaim         `json:"claims,omitempty"`
	RawResponse   string             `json:"raw_response,omitempty"`
	ParsedJSON    map[string]any     `json:"parsed_json,omitempty"`
	TokenUsage    TokenUsage         `json:"token_usage,omitempty"`
	CostUSD       float64            `json:"cost_usd,omitempty"`
	JudgeModel    string             `json:"judge_model,omitempty"`
	PromptVersion string             `json:"prompt_version,omitempty"`
	RubricHash    string             `json:"rubric_hash,omitempty"`
	ConfigHash    string             `json:"config_hash,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
}

type QAGClaim struct {
	Claim    string `json:"claim"`
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer,omitempty"`
	Verdict  string `json:"verdict"`
	Evidence string `json:"evidence,omitempty"`
}

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}
