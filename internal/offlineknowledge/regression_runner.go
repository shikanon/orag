package offlineknowledge

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/rag"
)

var (
	ErrRegressionDatasetRequired      = errors.New("offline knowledge regression dataset is required")
	ErrRegressionKnowledgeBaseMissing = errors.New("offline knowledge regression knowledge base is required")
)

type EvalRunExecutor interface {
	Run(ctx context.Context, req eval.RunRequest) (eval.RunResult, error)
}

type EvalRegressionRunnerOptions struct {
	BaselineRunner          EvalRunExecutor
	WithOptimization        EvalRunExecutor
	Datasets                *dataset.Service
	DatasetID               string
	BaselineProfile         rag.Profile
	WithOptimizationProfile rag.Profile
	TopK                    int
	HoldoutSplit            dataset.DatasetSplit
	HoldoutGate             eval.HoldoutGateConfig
	Judge                   *eval.JudgeConfig
	QAG                     *eval.JudgeConfig
}

type EvalRegressionRunner struct {
	baseline                EvalRunExecutor
	optimized               EvalRunExecutor
	datasets                *dataset.Service
	datasetID               string
	baselineProfile         rag.Profile
	withOptimizationProfile rag.Profile
	topK                    int
	holdoutSplit            dataset.DatasetSplit
	holdoutGate             eval.HoldoutGateConfig
	judge                   *eval.JudgeConfig
	qag                     *eval.JudgeConfig
}

func NewEvalRegressionRunner(opts EvalRegressionRunnerOptions) RegressionRunner {
	return &EvalRegressionRunner{
		baseline:                opts.BaselineRunner,
		optimized:               opts.WithOptimization,
		datasets:                opts.Datasets,
		datasetID:               opts.DatasetID,
		baselineProfile:         opts.BaselineProfile,
		withOptimizationProfile: opts.WithOptimizationProfile,
		topK:                    opts.TopK,
		holdoutSplit:            opts.HoldoutSplit,
		holdoutGate:             opts.HoldoutGate,
		judge:                   cloneJudgeConfig(opts.Judge),
		qag:                     cloneJudgeConfig(opts.QAG),
	}
}

func (r *EvalRegressionRunner) RunRegression(ctx context.Context, request RegressionRequest) (RegressionResult, error) {
	if r == nil {
		return RegressionResult{}, ErrRegressionUnavailable
	}
	datasetID := strings.TrimSpace(r.datasetID)
	if datasetID == "" {
		return RegressionResult{}, ErrRegressionDatasetRequired
	}
	if r.baseline == nil || r.optimized == nil || r.datasets == nil {
		return RegressionResult{}, ErrRegressionUnavailable
	}
	kbID := strings.TrimSpace(request.Item.KBID)
	if kbID == "" {
		return RegressionResult{}, ErrRegressionKnowledgeBaseMissing
	}
	items, err := r.datasets.Items(ctx, request.TenantID, datasetID)
	if err != nil {
		return RegressionResult{}, err
	}
	if len(items) == 0 {
		return RegressionResult{}, ErrRegressionDatasetRequired
	}

	baselineReq := eval.RunRequest{
		TenantID:        request.TenantID,
		DatasetID:       datasetID,
		KnowledgeBaseID: kbID,
		Profile:         defaultProfile(r.baselineProfile, rag.ProfileRealtime),
		TopK:            r.topK,
		Judge:           cloneJudgeConfig(r.judge),
		QAG:             cloneJudgeConfig(r.qag),
	}
	withReq := baselineReq
	withReq.Profile = defaultProfile(r.withOptimizationProfile, baselineReq.Profile)
	scopedItemID := regressionScopedItemID(request)
	withReq.ScopedShadowItemID = scopedItemID

	baseline, err := r.baseline.Run(ctx, baselineReq)
	if err != nil {
		return RegressionResult{}, err
	}
	withOptimization, err := r.optimized.Run(ctx, withReq)
	if err != nil {
		return RegressionResult{}, err
	}
	holdoutGate := eval.HoldoutGateResult{}
	gateConfig := regressionHoldoutGate(request, r.holdoutGate)
	if gateConfig.Enabled {
		holdoutReq := withReq
		holdoutReq.Split = regressionHoldoutSplit(r.holdoutSplit)
		holdoutReq.HoldoutGate = &gateConfig
		holdout, err := r.optimized.Run(ctx, holdoutReq)
		if err != nil {
			return RegressionResult{}, err
		}
		holdoutGate = holdout.HoldoutGate
		if !holdoutGate.Enabled {
			holdoutGate = eval.EvaluateHoldoutGate(holdout, gateConfig)
		}
	}

	fullDatasetUsed := baseline.Total == len(items) && withOptimization.Total == len(items)
	profileNeutrality := profileNeutralityMetadata(baselineReq.Profile, withReq.Profile)
	result := RegressionResult{
		RecallLift:           metric(withOptimization, "recall_at_k", "context_recall") - metric(baseline, "recall_at_k", "context_recall"),
		AnswerQualityLift:    metric(withOptimization, "answer_accuracy", "accuracy", eval.PrimaryMetricPairwiseAccuracy) - metric(baseline, "answer_accuracy", "accuracy", eval.PrimaryMetricPairwiseAccuracy),
		CitationCoverageLift: metric(withOptimization, "citation_precision", "citation_hit_rate") - metric(baseline, "citation_precision", "citation_hit_rate"),
		LatencyDeltaMS:       int64(metric(withOptimization, "latency_p95_ms", "latency_ms") - metric(baseline, "latency_p95_ms", "latency_ms")),
		TokenCostDelta:       metric(withOptimization, "cost_usd", "total_tokens") - metric(baseline, "cost_usd", "total_tokens"),
		HallucinationRisk:    hallucinationRisk(withOptimization),
		FullDatasetUsed:      fullDatasetUsed,
		Passed:               true,
		ScopedItemID:         scopedItemID,
		ProfileNeutrality:    profileNeutrality,
		HoldoutGate:          holdoutGate,
	}
	if !profileNeutrality.SameProfile {
		result.ProfileExperiment = &ProfileExperimentMetadata{
			Enabled:          true,
			BaselineProfile:  string(baselineReq.Profile),
			CandidateProfile: string(withReq.Profile),
		}
	}
	result.LatencyDelta = durationFromMS(result.LatencyDeltaMS)
	if request.FullDatasetRequired && !fullDatasetUsed {
		result.Passed = false
	}
	if gateConfig.Enabled && !holdoutGate.Passed {
		result.Passed = false
	}
	return result, nil
}

func metric(result eval.RunResult, names ...string) float64 {
	if result.Metrics == nil {
		return 0
	}
	for _, name := range names {
		if value, ok := result.Metrics[name]; ok {
			return value
		}
	}
	return 0
}

func hallucinationRisk(result eval.RunResult) float64 {
	if value, ok := result.Metrics["hallucination"]; ok {
		return value
	}
	if value, ok := result.Metrics["groundedness"]; ok {
		return clamp01(1 - value)
	}
	if value, ok := result.Metrics["citation_support"]; ok {
		return clamp01(1 - value)
	}
	return 0
}

func defaultProfile(value, fallback rag.Profile) rag.Profile {
	if value != "" {
		return value
	}
	return fallback
}

func regressionScopedItemID(request RegressionRequest) string {
	if strings.TrimSpace(request.ItemID) != "" {
		return request.ItemID
	}
	return request.Item.ID
}

func profileNeutralityMetadata(baseline, candidate rag.Profile) ProfileNeutralityMetadata {
	same := baseline == candidate
	return ProfileNeutralityMetadata{
		BaselineProfile:      string(baseline),
		CandidateProfile:     string(candidate),
		SameProfile:          same,
		OptimizationLiftOnly: same,
	}
}

func regressionHoldoutGate(request RegressionRequest, fallback eval.HoldoutGateConfig) eval.HoldoutGateConfig {
	if request.HoldoutGate.Enabled {
		return request.HoldoutGate
	}
	return fallback
}

func regressionHoldoutSplit(split dataset.DatasetSplit) dataset.DatasetSplit {
	if split != "" {
		return split
	}
	return dataset.DatasetSplitHoldout
}

func cloneJudgeConfig(in *eval.JudgeConfig) *eval.JudgeConfig {
	if in == nil {
		return nil
	}
	cp := *in
	cp.Metrics = append([]eval.JudgeMetric(nil), in.Metrics...)
	cp.Ensemble = append([]eval.JudgeModelConfig(nil), in.Ensemble...)
	return &cp
}

func durationFromMS(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
