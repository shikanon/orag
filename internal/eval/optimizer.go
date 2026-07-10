package eval

import (
	"context"
	"sort"

	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

type OptimizeRequest struct {
	TenantID        string        `json:"-"`
	DatasetID       string        `json:"dataset_id"`
	KnowledgeBaseID string        `json:"knowledge_base_id"`
	Profiles        []rag.Profile `json:"profiles,omitempty"`
	TopKs           []int         `json:"top_ks,omitempty"`
}

type CandidateResult struct {
	Profile              rag.Profile `json:"profile"`
	TopK                 int         `json:"top_k"`
	Score                float64     `json:"score"`
	ScoreMetric          string      `json:"score_metric"`
	FallbackMetric       string      `json:"fallback_metric,omitempty"`
	PairwiseAccuracy     float64     `json:"pairwise_accuracy"`
	NDCGAtK              float64     `json:"ndcg_at_k"`
	RecallAtK            float64     `json:"recall_at_k"`
	MRR                  float64     `json:"mrr"`
	MAP                  float64     `json:"map"`
	RetrievalFailureRate float64     `json:"retrieval_failure_rate"`
	RedundancyRate       float64     `json:"redundancy_rate"`
	DuplicateCount       float64     `json:"duplicate_count"`
	DedupedTopKCount     float64     `json:"deduped_top_k_count"`
	AlphaNDCG            float64     `json:"alpha_ndcg"`
	AspectCoverage       float64     `json:"aspect_coverage"`
	LatencyP95MS         float64     `json:"latency_p95_ms"`
	RunID                string      `json:"run_id"`
}

type OptimizeResult struct {
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	Best       CandidateResult   `json:"best"`
	Candidates []CandidateResult `json:"candidates"`
}

type Optimizer struct {
	Runner EvaluationRunner
}

type EvaluationRunner interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

func (o Optimizer) Optimize(ctx context.Context, req OptimizeRequest) (OptimizeResult, error) {
	profiles := req.Profiles
	if len(profiles) == 0 {
		profiles = []rag.Profile{rag.ProfileRealtime, rag.ProfileHighPrecision}
	}
	topKs := req.TopKs
	if len(topKs) == 0 {
		topKs = []int{8}
	}
	out := OptimizeResult{ID: id.New("opt"), Status: "completed"}
	for _, profile := range profiles {
		for _, topK := range topKs {
			run, err := o.Runner.Run(ctx, RunRequest{
				TenantID:        req.TenantID,
				DatasetID:       req.DatasetID,
				KnowledgeBaseID: req.KnowledgeBaseID,
				Profile:         profile,
				TopK:            topK,
			})
			if err != nil {
				return OptimizeResult{}, err
			}
			if err := ValidateMetricMap(run.Metrics); err != nil {
				return OptimizeResult{}, err
			}
			out.Candidates = append(out.Candidates, candidateFromRun(profile, topK, run))
		}
	}
	sort.SliceStable(out.Candidates, func(i, j int) bool {
		leftPriority := candidateScoreMetricPriority(out.Candidates[i])
		rightPriority := candidateScoreMetricPriority(out.Candidates[j])
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		return out.Candidates[i].Score > out.Candidates[j].Score
	})
	if len(out.Candidates) > 0 {
		out.Best = out.Candidates[0]
	}
	return out, nil
}

func candidateFromRun(profile rag.Profile, topK int, run RunResult) CandidateResult {
	score, scoreMetric, fallbackMetric := optimizerScore(run)
	pairwiseAccuracy := 0.0
	if value, ok := run.Metrics[PrimaryMetricPairwiseAccuracy]; ok {
		pairwiseAccuracy = value
	}
	return CandidateResult{
		Profile:              profile,
		TopK:                 topK,
		Score:                score,
		ScoreMetric:          scoreMetric,
		FallbackMetric:       fallbackMetric,
		PairwiseAccuracy:     pairwiseAccuracy,
		NDCGAtK:              run.Metrics["ndcg_at_k"],
		RecallAtK:            run.Metrics["recall_at_k"],
		MRR:                  run.Metrics["mrr"],
		MAP:                  run.Metrics["map"],
		RetrievalFailureRate: run.Metrics["retrieval_failure_rate"],
		RedundancyRate:       run.Metrics["redundancy_rate"],
		DuplicateCount:       run.Metrics["duplicate_count"],
		DedupedTopKCount:     run.Metrics["deduped_top_k_count"],
		AlphaNDCG:            run.Metrics["alpha_ndcg"],
		AspectCoverage:       run.Metrics["aspect_coverage"],
		LatencyP95MS:         run.Metrics["latency_p95_ms"],
		RunID:                run.ID,
	}
}

func optimizerScore(run RunResult) (float64, string, string) {
	if run.Metrics != nil {
		if score, ok := run.Metrics[PrimaryMetricPairwiseAccuracy]; ok {
			return score, PrimaryMetricPairwiseAccuracy, ""
		}
		if score, ok := run.Metrics[PrimaryMetricDeterministicAnswerMatch]; ok {
			return score, PrimaryMetricDeterministicAnswerMatch, PrimaryMetricDeterministicAnswerMatch
		}
		if score, ok := run.Metrics["answer_accuracy"]; ok {
			return score, "answer_accuracy", "answer_accuracy"
		}
		if score, ok := run.Metrics["accuracy"]; ok {
			return score, "accuracy", "accuracy"
		}
	}
	return run.Accuracy, "accuracy", "accuracy"
}

func candidateScoreMetricPriority(candidate CandidateResult) int {
	if candidate.ScoreMetric == PrimaryMetricPairwiseAccuracy {
		return 2
	}
	if candidate.FallbackMetric != "" {
		return 1
	}
	return 0
}
