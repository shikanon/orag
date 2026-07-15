package eval

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

type Runner struct {
	RAG        *rag.Service
	Datasets   *dataset.Service
	Repository Repository
	Judge      Judge
	QAG        QAGJudge
	Pairwise   PairwiseJudge
}

type Repository interface {
	StoreEvaluationRun(ctx context.Context, tenantID string, result RunResult) error
	StoreEvaluationResult(ctx context.Context, runID, datasetItemID, answer string, metrics map[string]float64) error
	GetEvaluationRun(ctx context.Context, tenantID, id string) (RunResult, bool, error)
	StoreJudgeRun(ctx context.Context, tenantID string, run JudgeRunRecord) error
	StoreJudgeResult(ctx context.Context, result JudgeResultRecord) error
	StorePairwiseJudgeResult(ctx context.Context, result PairwiseJudgeResultRecord) error
	StoreJudgeCalibrationRun(ctx context.Context, tenantID string, run JudgeCalibrationRunRecord) error
	GetEvaluationDetail(ctx context.Context, tenantID, id string, options EvaluationDetailOptions) (EvaluationDetail, bool, error)
}

type ProjectRepository interface {
	GetEvaluationRunInProject(ctx context.Context, tenantID, projectID, id string) (RunResult, bool, error)
}

type RunRequest struct {
	TenantID           string               `json:"-"`
	ProjectID          string               `json:"-"`
	DatasetID          string               `json:"dataset_id"`
	KnowledgeBaseID    string               `json:"knowledge_base_id"`
	Profile            rag.Profile          `json:"profile"`
	TopK               int                  `json:"top_k,omitempty"`
	ScopedShadowItemID string               `json:"scoped_shadow_item_id,omitempty"`
	Split              dataset.DatasetSplit `json:"split,omitempty"`
	HoldoutGate        *HoldoutGateConfig   `json:"holdout_gate,omitempty"`
	Judge              *JudgeConfig         `json:"judge,omitempty"`
	QAG                *JudgeConfig         `json:"qag,omitempty"`
	Pairwise           *JudgeConfig         `json:"pairwise,omitempty"`
}

type RunResult struct {
	ID                    string                  `json:"id"`
	ProjectID             string                  `json:"project_id,omitempty"`
	DatasetID             string                  `json:"dataset_id"`
	Profile               string                  `json:"profile"`
	Total                 int                     `json:"total"`
	HitRate               float64                 `json:"hit_rate"`
	Accuracy              float64                 `json:"accuracy"`
	WeightedSampleCount   float64                 `json:"weighted_sample_count,omitempty"`
	UnweightedSampleCount int                     `json:"unweighted_sample_count,omitempty"`
	Split                 dataset.DatasetSplit    `json:"split,omitempty"`
	SplitSummary          map[string]SplitSummary `json:"split_summary,omitempty"`
	MissingSplit          bool                    `json:"missing_split,omitempty"`
	HoldoutGate           HoldoutGateResult       `json:"holdout_gate,omitempty"`
	Metrics               map[string]float64      `json:"metrics,omitempty"`
	CreatedAt             time.Time               `json:"created_at"`
}

type SplitSummary struct {
	UnweightedSampleCount int     `json:"unweighted_sample_count"`
	WeightedSampleCount   float64 `json:"weighted_sample_count"`
}

type EvaluationDetailOptions struct {
	IncludeItems    bool
	IncludeJudge    bool
	IncludePairwise bool
}

type EvaluationDetail struct {
	Run             RunResult                   `json:"run"`
	Items           []EvaluationItemDetail      `json:"items,omitempty"`
	JudgeRuns       []JudgeRunRecord            `json:"judge_runs,omitempty"`
	JudgeResults    []JudgeResultRecord         `json:"judge_results,omitempty"`
	PairwiseResults []PairwiseJudgeResultRecord `json:"pairwise_judge_results,omitempty"`
	CalibrationRuns []JudgeCalibrationRunRecord `json:"judge_calibration_runs,omitempty"`
}

type EvaluationItemDetail struct {
	RunID         string             `json:"run_id"`
	DatasetItemID string             `json:"dataset_item_id"`
	Answer        string             `json:"answer"`
	Metrics       map[string]float64 `json:"metrics,omitempty"`
}

type JudgeRunRecord struct {
	ID              string             `json:"id"`
	EvaluationRunID string             `json:"evaluation_run_id"`
	Provider        string             `json:"judge_provider,omitempty"`
	Model           string             `json:"judge_model,omitempty"`
	PromptVersion   string             `json:"prompt_version,omitempty"`
	RubricHash      string             `json:"rubric_hash,omitempty"`
	PromptHash      string             `json:"prompt_hash,omitempty"`
	ConfigHash      string             `json:"judge_params_hash,omitempty"`
	Mode            string             `json:"mode"`
	ComparisonMode  string             `json:"comparison_mode,omitempty"`
	Rubric          JudgeRubric        `json:"rubric,omitempty"`
	JudgeParams     JudgeConfig        `json:"judge_params,omitempty"`
	Ensemble        []JudgeModelConfig `json:"ensemble,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
}

type JudgeResultRecord struct {
	ID            string                        `json:"id"`
	JudgeRunID    string                        `json:"judge_run_id"`
	DatasetItemID string                        `json:"dataset_item_id"`
	CandidateID   string                        `json:"candidate_id,omitempty"`
	Scores        map[string]float64            `json:"scores,omitempty"`
	Labels        map[string]string             `json:"labels,omitempty"`
	Pass          bool                          `json:"pass"`
	Rationale     string                        `json:"rationale,omitempty"`
	Findings      []JudgeFinding                `json:"findings,omitempty"`
	RawResponse   string                        `json:"raw_response,omitempty"`
	ParsedJSON    map[string]any                `json:"parsed_json,omitempty"`
	Confidence    map[string]ConfidenceInterval `json:"confidence,omitempty"`
	TokenUsage    TokenUsage                    `json:"token_usage,omitempty"`
	CostUSD       float64                       `json:"cost_usd,omitempty"`
	CreatedAt     time.Time                     `json:"created_at"`
}

type PairwiseJudgeResultRecord struct {
	ID            string         `json:"id"`
	JudgeRunID    string         `json:"judge_run_id"`
	DatasetItemID string         `json:"dataset_item_id"`
	CandidateAID  string         `json:"candidate_a_id"`
	CandidateBID  string         `json:"candidate_b_id"`
	Winner        string         `json:"winner"`
	Preference    string         `json:"preference,omitempty"`
	Stable        bool           `json:"stable"`
	Reasons       []JudgeFinding `json:"reasons,omitempty"`
	RawResponse   string         `json:"raw_response,omitempty"`
	ParsedJSON    map[string]any `json:"parsed_json,omitempty"`
	TokenUsage    TokenUsage     `json:"token_usage,omitempty"`
	CostUSD       float64        `json:"cost_usd,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

type JudgeCalibrationRunRecord struct {
	ID                string             `json:"id"`
	DatasetID         string             `json:"dataset_id"`
	JudgeConfigHash   string             `json:"judge_config_hash"`
	HumanScoreVersion string             `json:"human_score_version,omitempty"`
	Spearman          float64            `json:"spearman"`
	CohenKappa        float64            `json:"cohen_kappa"`
	SampleCount       int                `json:"sample_count"`
	Metrics           map[string]float64 `json:"metrics,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
}

const (
	PrimaryMetricDeterministicAnswerMatch = "deterministic_answer_match"
	PrimaryMetricPairwiseAccuracy         = "pairwise_accuracy"
)

func (r Runner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if strings.TrimSpace(req.DatasetID) == "" {
		return RunResult{}, apperrors.New(apperrors.CodeValidation, "dataset_id is required")
	}
	if strings.TrimSpace(req.KnowledgeBaseID) == "" {
		return RunResult{}, apperrors.New(apperrors.CodeValidation, "knowledge_base_id is required")
	}
	datasetItem, ok, err := r.Datasets.Get(ctx, req.TenantID, req.DatasetID)
	if err != nil {
		return RunResult{}, err
	} else if !ok {
		return RunResult{}, apperrors.Wrap(apperrors.CodeNotFound, "dataset not found", dataset.ErrDatasetNotFound)
	}
	if req.ProjectID != "" && datasetItem.ProjectID != "" && datasetItem.ProjectID != req.ProjectID {
		return RunResult{}, apperrors.New(apperrors.CodeValidation, "dataset and knowledge base must belong to the same project")
	}
	if datasetItem.ProjectID != "" {
		req.ProjectID = datasetItem.ProjectID
	}
	allItems, err := r.Datasets.Items(ctx, req.TenantID, req.DatasetID)
	if err != nil {
		if errors.Is(err, dataset.ErrDatasetNotFound) {
			return RunResult{}, apperrors.Wrap(apperrors.CodeNotFound, "dataset not found", err)
		}
		return RunResult{}, err
	}
	splitSummary := summarizeSplits(allItems)
	items := allItems
	requestedSplit := dataset.NormalizeSplit(req.Split)
	if requestedSplit != "" {
		items, err = r.Datasets.ItemsBySplit(ctx, req.TenantID, req.DatasetID, requestedSplit)
		if err != nil {
			if errors.Is(err, dataset.ErrDatasetNotFound) {
				return RunResult{}, apperrors.Wrap(apperrors.CodeNotFound, "dataset not found", err)
			}
			return RunResult{}, err
		}
	}
	if len(items) == 0 {
		if requestedSplit != "" {
			if holdoutGateEnabled(req.HoldoutGate) {
				result := RunResult{
					ID:           id.New("eval"),
					ProjectID:    req.ProjectID,
					DatasetID:    req.DatasetID,
					Profile:      string(req.Profile),
					Split:        requestedSplit,
					SplitSummary: splitSummary,
					MissingSplit: true,
					Metrics: map[string]float64{
						"weighted_sample_count":   0,
						"unweighted_sample_count": 0,
						"missing_split":           1,
					},
					CreatedAt: time.Now().UTC(),
				}
				result.HoldoutGate = EvaluateHoldoutGate(result, *req.HoldoutGate)
				if r.Repository != nil {
					if err := r.Repository.StoreEvaluationRun(ctx, req.TenantID, result); err != nil {
						return RunResult{}, err
					}
				}
				return result, nil
			}
			return RunResult{}, apperrors.New(apperrors.CodeValidation, "dataset split "+string(requestedSplit)+" is empty or missing")
		}
		return RunResult{}, apperrors.New(apperrors.CodeValidation, "dataset is empty")
	}

	runID := id.New("eval")
	latencies := make([]weightedLatency, 0, len(items))
	metricSums := map[string]float64{}
	metricCounts := map[string]float64{}
	type itemResult struct {
		itemID  string
		answer  string
		metrics map[string]float64
	}
	var itemResults []itemResult
	var judgeResults []JudgeResultRecord
	var qagResults []JudgeResultRecord
	var pairwiseResults []PairwiseJudgeResultRecord
	var judgeUsage TokenUsage
	var judgeCost float64
	judgeRun, qagRun, pairwiseRun := JudgeRunRecord{}, JudgeRunRecord{}, JudgeRunRecord{}
	if req.Judge != nil {
		if r.Judge == nil {
			return RunResult{}, apperrors.New(apperrors.CodeValidation, "judge is not configured")
		}
		judgeRun = newJudgeRunRecord(runID, "llm_judge", "absolute", *req.Judge)
	}
	if req.QAG != nil {
		if r.QAG == nil {
			return RunResult{}, apperrors.New(apperrors.CodeValidation, "qag judge is not configured")
		}
		qagRun = newJudgeRunRecord(runID, "qag", "absolute", *req.QAG)
	}
	if req.Pairwise != nil {
		if r.Pairwise == nil {
			return RunResult{}, apperrors.New(apperrors.CodeValidation, "pairwise judge is not configured")
		}
		pairwiseRun = newJudgeRunRecord(runID, "llm_judge", "pairwise", *req.Pairwise)
	}
	for _, item := range items {
		resp, err := r.RAG.Query(ctx, rag.QueryRequest{
			TenantID:           req.TenantID,
			KnowledgeBaseID:    req.KnowledgeBaseID,
			Query:              item.Query,
			Profile:            req.Profile,
			TopK:               req.TopK,
			ScopedShadowItemID: req.ScopedShadowItemID,
		})
		if err != nil {
			return RunResult{}, err
		}
		weight := itemWeight(item)
		latencies = append(latencies, weightedLatency{value: resp.LatencyMS, weight: weight})
		itemMetrics := ScoreItemWithOptions(item, resp, ScoreOptions{TopK: req.TopK})
		if err := ValidateMetricMap(itemMetrics); err != nil {
			return RunResult{}, err
		}
		judgeInput := JudgeInput{
			TenantID:         req.TenantID,
			DatasetID:        req.DatasetID,
			DatasetItemID:    item.ID,
			Query:            item.Query,
			GroundTruth:      item.GroundTruth,
			ExpectedEvidence: item.ExpectedEvidence,
			RelevantDocIDs:   item.RelevantDocIDs,
			Answer:           resp.Answer,
			Citations:        resp.Citations,
			RetrievedChunks:  resp.RetrievedChunks,
		}
		if req.Judge != nil {
			out, err := r.Judge.Judge(ctx, judgeInput)
			if err != nil {
				return RunResult{}, err
			}
			if err := ValidateMetricMap(out.Scores); err != nil {
				return RunResult{}, err
			}
			mergeItemMetrics(itemMetrics, out.Scores)
			addUsageMetrics(itemMetrics, out.TokenUsage, out.CostUSD)
			judgeUsage = addTokenUsage(judgeUsage, out.TokenUsage)
			judgeCost += out.CostUSD
			judgeResults = append(judgeResults, judgeResultRecordFromOutput(judgeRun.ID, item.ID, out))
		}
		if req.QAG != nil {
			out, err := r.QAG.ScoreQAG(ctx, judgeInput)
			if err != nil {
				return RunResult{}, err
			}
			if err := ValidateMetricMap(out.Metrics); err != nil {
				return RunResult{}, err
			}
			mergeItemMetrics(itemMetrics, out.Metrics)
			addUsageMetrics(itemMetrics, out.TokenUsage, out.CostUSD)
			judgeUsage = addTokenUsage(judgeUsage, out.TokenUsage)
			judgeCost += out.CostUSD
			qagResults = append(qagResults, judgeResultRecordFromQAG(qagRun.ID, item.ID, out))
		}
		if req.Pairwise != nil {
			out, err := r.Pairwise.Compare(ctx, PairwiseJudgeInput{
				TenantID:         req.TenantID,
				DatasetID:        req.DatasetID,
				DatasetItemID:    item.ID,
				Query:            item.Query,
				GroundTruth:      item.GroundTruth,
				ExpectedEvidence: item.ExpectedEvidence,
				RelevantDocIDs:   item.RelevantDocIDs,
				AnswerA: CandidateAnswer{
					ID:              "candidate",
					Answer:          resp.Answer,
					Citations:       resp.Citations,
					RetrievedChunks: resp.RetrievedChunks,
				},
				AnswerB: CandidateAnswer{
					ID:     "ground_truth",
					Answer: item.GroundTruth,
				},
				Rubric: req.Pairwise.Rubric,
			})
			if err != nil {
				return RunResult{}, err
			}
			itemMetrics[PrimaryMetricPairwiseAccuracy] = pairwiseWinScore(out.Winner)
			addUsageMetrics(itemMetrics, out.TokenUsage, out.CostUSD)
			judgeUsage = addTokenUsage(judgeUsage, out.TokenUsage)
			judgeCost += out.CostUSD
			pairwiseResults = append(pairwiseResults, pairwiseJudgeResultRecordFromOutput(pairwiseRun.ID, item.ID, out))
		}
		if err := ValidateMetricMap(itemMetrics); err != nil {
			return RunResult{}, err
		}
		for name, value := range itemMetrics {
			if shouldAggregateItemMetric(name) {
				metricSums[name] += value * weight
				metricCounts[name] += weight
			}
		}
		metricSums["prompt_tokens"] += itemMetrics["prompt_tokens"] * weight
		metricSums["completion_tokens"] += itemMetrics["completion_tokens"] * weight
		metricSums["total_tokens"] += itemMetrics["total_tokens"] * weight
		metricSums["cost_usd"] += itemMetrics["cost_usd"] * weight
		itemResults = append(itemResults, itemResult{itemID: item.ID, answer: resp.Answer, metrics: itemMetrics})
	}

	total := len(items)
	weightedSampleCount := splitWeight(items)
	answerScore := weightedAverage(metricSums["answer_accuracy"], metricCounts["answer_accuracy"])
	metrics := map[string]float64{
		"answer_accuracy":                     answerScore,
		"accuracy":                            answerScore,
		"hit_rate":                            answerScore,
		PrimaryMetricDeterministicAnswerMatch: answerScore,
		"latency_p95_ms":                      float64(weightedP95(latencies)),
		"cache_hit_rate":                      weightedAverage(metricSums["cache_hit"], metricCounts["cache_hit"]),
		"weighted_sample_count":               weightedSampleCount,
		"unweighted_sample_count":             float64(total),
		"missing_split":                       0,
	}
	for name, sum := range metricSums {
		if shouldAggregateItemMetric(name) {
			metrics[name] = weightedAverage(sum, metricCounts[name])
		}
	}
	if metricSums["prompt_tokens"] > 0 || metricSums["completion_tokens"] > 0 || metricSums["total_tokens"] > 0 {
		metrics["prompt_tokens"] = metricSums["prompt_tokens"]
		metrics["completion_tokens"] = metricSums["completion_tokens"]
		metrics["total_tokens"] = metricSums["total_tokens"]
	} else if judgeUsage.PromptTokens > 0 || judgeUsage.CompletionTokens > 0 || judgeUsage.TotalTokens > 0 {
		metrics["prompt_tokens"] = float64(judgeUsage.PromptTokens)
		metrics["completion_tokens"] = float64(judgeUsage.CompletionTokens)
		metrics["total_tokens"] = float64(judgeUsage.TotalTokens)
	}
	if metricSums["cost_usd"] > 0 {
		metrics["cost_usd"] = metricSums["cost_usd"]
	} else if judgeCost > 0 {
		metrics["cost_usd"] = judgeCost
	}
	metrics["accuracy"] = answerScore
	metrics["hit_rate"] = answerScore
	metrics[PrimaryMetricDeterministicAnswerMatch] = answerScore
	if _, ok := metricCounts[PrimaryMetricPairwiseAccuracy]; ok {
		metrics[PrimaryMetricPairwiseAccuracy] = weightedAverage(metricSums[PrimaryMetricPairwiseAccuracy], metricCounts[PrimaryMetricPairwiseAccuracy])
	}
	if err := ValidateMetricMap(metrics); err != nil {
		return RunResult{}, err
	}

	result := RunResult{
		ID:                    runID,
		ProjectID:             req.ProjectID,
		DatasetID:             req.DatasetID,
		Profile:               string(req.Profile),
		Total:                 total,
		HitRate:               answerScore,
		Accuracy:              answerScore,
		WeightedSampleCount:   weightedSampleCount,
		UnweightedSampleCount: total,
		Split:                 requestedSplit,
		SplitSummary:          splitSummary,
		MissingSplit:          false,
		Metrics:               metrics,
		CreatedAt:             time.Now().UTC(),
	}
	if holdoutGateEnabled(req.HoldoutGate) {
		result.HoldoutGate = EvaluateHoldoutGate(result, *req.HoldoutGate)
	}
	if r.Repository != nil {
		if err := r.Repository.StoreEvaluationRun(ctx, req.TenantID, result); err != nil {
			return RunResult{}, err
		}
		if req.Judge != nil {
			if err := r.Repository.StoreJudgeRun(ctx, req.TenantID, judgeRun); err != nil {
				return RunResult{}, err
			}
		}
		if req.QAG != nil {
			if err := r.Repository.StoreJudgeRun(ctx, req.TenantID, qagRun); err != nil {
				return RunResult{}, err
			}
		}
		if req.Pairwise != nil {
			if err := r.Repository.StoreJudgeRun(ctx, req.TenantID, pairwiseRun); err != nil {
				return RunResult{}, err
			}
		}
		for _, item := range itemResults {
			if err := r.Repository.StoreEvaluationResult(ctx, runID, item.itemID, item.answer, item.metrics); err != nil {
				return RunResult{}, err
			}
		}
		for _, result := range append(judgeResults, qagResults...) {
			if err := r.Repository.StoreJudgeResult(ctx, result); err != nil {
				return RunResult{}, err
			}
		}
		for _, result := range pairwiseResults {
			if err := r.Repository.StorePairwiseJudgeResult(ctx, result); err != nil {
				return RunResult{}, err
			}
		}
	}
	return result, nil
}

func newJudgeRunRecord(evaluationRunID, mode, comparisonMode string, cfg JudgeConfig) JudgeRunRecord {
	if cfg.PromptVersion == "" {
		cfg.PromptVersion = defaultJudgePromptVersion
	}
	return JudgeRunRecord{
		ID:              id.New("judge"),
		EvaluationRunID: evaluationRunID,
		Provider:        cfg.Provider,
		Model:           cfg.Model,
		PromptVersion:   cfg.PromptVersion,
		RubricHash:      HashJudgeRubric(cfg.Rubric),
		PromptHash:      stableHash(map[string]any{"prompt_version": cfg.PromptVersion, "template": cfg.PromptTemplate}),
		ConfigHash:      HashJudgeConfig(cfg),
		Mode:            mode,
		ComparisonMode:  comparisonMode,
		Rubric:          cfg.Rubric,
		JudgeParams:     cfg,
		Ensemble:        cfg.Ensemble,
		CreatedAt:       time.Now().UTC(),
	}
}

func mergeItemMetrics(dst, src map[string]float64) {
	for key, value := range src {
		dst[key] = value
	}
}

func addUsageMetrics(metrics map[string]float64, usage TokenUsage, cost float64) {
	metrics["prompt_tokens"] += float64(usage.PromptTokens)
	metrics["completion_tokens"] += float64(usage.CompletionTokens)
	metrics["total_tokens"] += float64(usage.TotalTokens)
	metrics["cost_usd"] += cost
}

func judgeResultRecordFromOutput(judgeRunID, itemID string, out JudgeOutput) JudgeResultRecord {
	return JudgeResultRecord{
		ID:            id.New("judger"),
		JudgeRunID:    judgeRunID,
		DatasetItemID: itemID,
		Scores:        out.Scores,
		Labels:        out.Labels,
		Pass:          out.Pass,
		Rationale:     out.Rationale,
		Findings:      out.Findings,
		RawResponse:   out.RawResponse,
		ParsedJSON:    out.ParsedJSON,
		Confidence:    out.Confidence,
		TokenUsage:    out.TokenUsage,
		CostUSD:       out.CostUSD,
		CreatedAt:     firstTime(out.CreatedAt, time.Now().UTC()),
	}
}

func judgeResultRecordFromQAG(judgeRunID, itemID string, out QAGOutput) JudgeResultRecord {
	return JudgeResultRecord{
		ID:            id.New("judger"),
		JudgeRunID:    judgeRunID,
		DatasetItemID: itemID,
		CandidateID:   "qag",
		Scores:        out.Metrics,
		Pass:          out.Score >= 0.5,
		Rationale:     "QAG claim verification",
		Findings:      qagFindings(out.Claims),
		RawResponse:   out.RawResponse,
		ParsedJSON:    out.ParsedJSON,
		TokenUsage:    out.TokenUsage,
		CostUSD:       out.CostUSD,
		CreatedAt:     firstTime(out.CreatedAt, time.Now().UTC()),
	}
}

func pairwiseJudgeResultRecordFromOutput(judgeRunID, itemID string, out PairwiseJudgeOutput) PairwiseJudgeResultRecord {
	return PairwiseJudgeResultRecord{
		ID:            id.New("pairwise"),
		JudgeRunID:    judgeRunID,
		DatasetItemID: itemID,
		CandidateAID:  "candidate",
		CandidateBID:  "ground_truth",
		Winner:        out.Winner,
		Preference:    out.Preference,
		Stable:        out.Stable,
		Reasons:       out.Reasons,
		RawResponse:   out.RawResponse,
		ParsedJSON:    out.ParsedJSON,
		TokenUsage:    out.TokenUsage,
		CostUSD:       out.CostUSD,
		CreatedAt:     firstTime(out.CreatedAt, time.Now().UTC()),
	}
}

func pairwiseWinScore(winner string) float64 {
	switch strings.ToLower(strings.TrimSpace(winner)) {
	case "a", "tie":
		return 1
	default:
		return 0
	}
}

func qagFindings(claims []QAGClaim) []JudgeFinding {
	findings := make([]JudgeFinding, 0, len(claims))
	for _, claim := range claims {
		findings = append(findings, JudgeFinding{
			Metric:   "qag_score",
			Label:    claim.Verdict,
			Message:  claim.Claim,
			Evidence: claim.Evidence,
		})
	}
	return findings
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}

func shouldAggregateItemMetric(name string) bool {
	return DefaultMetricRegistry.ShouldAggregate(name)
}

func holdoutGateEnabled(cfg *HoldoutGateConfig) bool {
	return cfg != nil && cfg.Enabled
}

func (r Runner) Get(ctx context.Context, tenantID, id string) (RunResult, bool, error) {
	if r.Repository == nil {
		return RunResult{}, false, nil
	}
	return r.Repository.GetEvaluationRun(ctx, tenantID, id)
}

func (r Runner) GetInProject(ctx context.Context, tenantID, projectID, id string) (RunResult, bool, error) {
	if r.Repository == nil {
		return RunResult{}, false, nil
	}
	if repository, ok := r.Repository.(ProjectRepository); ok {
		return repository.GetEvaluationRunInProject(ctx, tenantID, projectID, id)
	}
	result, found, err := r.Repository.GetEvaluationRun(ctx, tenantID, id)
	return result, found && result.ProjectID == projectID, err
}

func (r Runner) GetDetail(ctx context.Context, tenantID, id string, options EvaluationDetailOptions) (EvaluationDetail, bool, error) {
	if r.Repository == nil {
		return EvaluationDetail{}, false, nil
	}
	return r.Repository.GetEvaluationDetail(ctx, tenantID, id, options)
}

type MemoryRepository struct {
	mu           sync.RWMutex
	runs         map[string]RunResult
	runTenants   map[string]string
	results      map[string][]map[string]float64
	items        map[string][]EvaluationItemDetail
	judgeRuns    map[string][]JudgeRunRecord
	judgeRes     map[string][]JudgeResultRecord
	pairwise     map[string][]PairwiseJudgeResultRecord
	calibrations map[string][]JudgeCalibrationRunRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		runs:         map[string]RunResult{},
		runTenants:   map[string]string{},
		results:      map[string][]map[string]float64{},
		items:        map[string][]EvaluationItemDetail{},
		judgeRuns:    map[string][]JudgeRunRecord{},
		judgeRes:     map[string][]JudgeResultRecord{},
		pairwise:     map[string][]PairwiseJudgeResultRecord{},
		calibrations: map[string][]JudgeCalibrationRunRecord{},
	}
}

func (r *MemoryRepository) StoreEvaluationRun(_ context.Context, tenantID string, result RunResult) error {
	if err := ValidateMetricMap(result.Metrics); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[result.ID] = result
	r.runTenants[result.ID] = tenantID
	return nil
}

func (r *MemoryRepository) StoreEvaluationResult(_ context.Context, runID, datasetItemID, answer string, metrics map[string]float64) error {
	if err := ValidateMetricMap(metrics); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[runID] = append(r.results[runID], metrics)
	r.items[runID] = append(r.items[runID], EvaluationItemDetail{
		RunID:         runID,
		DatasetItemID: datasetItemID,
		Answer:        answer,
		Metrics:       cloneMetrics(metrics),
	})
	return nil
}

func (r *MemoryRepository) GetEvaluationRun(_ context.Context, tenantID string, id string) (RunResult, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result, ok := r.runs[id]
	if !ok || r.runTenants[id] != tenantID {
		return RunResult{}, false, nil
	}
	return result, ok, nil
}

func (r *MemoryRepository) GetEvaluationRunInProject(_ context.Context, tenantID, projectID, id string) (RunResult, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result, ok := r.runs[id]
	if !ok || r.runTenants[id] != tenantID || result.ProjectID != projectID {
		return RunResult{}, false, nil
	}
	return result, true, nil
}

func (r *MemoryRepository) StoreJudgeRun(_ context.Context, tenantID string, run JudgeRunRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runTenants[run.EvaluationRunID] != tenantID {
		return apperrors.New(apperrors.CodeNotFound, "evaluation run not found")
	}
	r.judgeRuns[run.EvaluationRunID] = append(r.judgeRuns[run.EvaluationRunID], run)
	return nil
}

func (r *MemoryRepository) StoreJudgeResult(_ context.Context, result JudgeResultRecord) error {
	if err := ValidateMetricMap(result.Scores); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	runID := r.evaluationRunIDForJudgeRun(result.JudgeRunID)
	if runID == "" {
		return apperrors.New(apperrors.CodeNotFound, "judge run not found")
	}
	r.judgeRes[runID] = append(r.judgeRes[runID], result)
	return nil
}

func (r *MemoryRepository) StorePairwiseJudgeResult(_ context.Context, result PairwiseJudgeResultRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	runID := r.evaluationRunIDForJudgeRun(result.JudgeRunID)
	if runID == "" {
		return apperrors.New(apperrors.CodeNotFound, "judge run not found")
	}
	r.pairwise[runID] = append(r.pairwise[runID], result)
	return nil
}

func (r *MemoryRepository) StoreJudgeCalibrationRun(_ context.Context, tenantID string, run JudgeCalibrationRunRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calibrations[tenantID] = append(r.calibrations[tenantID], run)
	return nil
}

func (r *MemoryRepository) GetEvaluationDetail(_ context.Context, tenantID, id string, options EvaluationDetailOptions) (EvaluationDetail, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[id]
	if !ok || r.runTenants[id] != tenantID {
		return EvaluationDetail{}, false, nil
	}
	detail := EvaluationDetail{Run: run}
	if options.IncludeItems {
		detail.Items = append([]EvaluationItemDetail(nil), r.items[id]...)
	}
	if options.IncludeJudge {
		detail.JudgeRuns = append([]JudgeRunRecord(nil), r.judgeRuns[id]...)
		detail.JudgeResults = append([]JudgeResultRecord(nil), r.judgeRes[id]...)
		detail.CalibrationRuns = append([]JudgeCalibrationRunRecord(nil), r.calibrations[tenantID]...)
	}
	if options.IncludePairwise {
		detail.PairwiseResults = append([]PairwiseJudgeResultRecord(nil), r.pairwise[id]...)
	}
	return detail, true, nil
}

func (r *MemoryRepository) evaluationRunIDForJudgeRun(judgeRunID string) string {
	for runID, runs := range r.judgeRuns {
		for _, run := range runs {
			if run.ID == judgeRunID {
				return runID
			}
		}
	}
	return ""
}

func cloneMetrics(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func matches(answer, groundTruth string) bool {
	answer = strings.ToLower(answer)
	for _, term := range strings.Fields(strings.ToLower(groundTruth)) {
		if len(term) > 3 && strings.Contains(answer, term) {
			return true
		}
	}
	return false
}
