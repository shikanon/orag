package tutorial

import (
	"context"
	"errors"
	"sort"

	"github.com/shikanon/orag/internal/eval"
)

var ErrExperimentComparisonUnavailable = errors.New("tutorial experiment comparison is unavailable")

type RuntimeEvaluationReader interface {
	GetInProject(context.Context, string, string, string) (eval.RunResult, bool, error)
}

type ExperimentMetricDelta struct {
	Name          string   `json:"name"`
	Baseline      float64  `json:"baseline"`
	Candidate     float64  `json:"candidate"`
	AbsoluteDelta float64  `json:"absolute_delta"`
	RelativeDelta *float64 `json:"relative_delta,omitempty"`
}

// ExperimentRunComparison contains no inferred quality result. It is a
// projection of the two ordinary persisted evaluation runs plus their durable
// lineage, and Comparable is false whenever the invariant cannot be proven.
type ExperimentRunComparison struct {
	Baseline     ExperimentRun           `json:"baseline"`
	Candidate    ExperimentRun           `json:"candidate"`
	Comparable   bool                    `json:"comparable"`
	Metrics      []ExperimentMetricDelta `json:"metrics,omitempty"`
	IndexMetrics []ExperimentMetricDelta `json:"index_metrics,omitempty"`
}

func (s *LiveRunService) Compare(ctx context.Context, subject Subject, projectID, experimentID, candidateRunID string) (ExperimentRunComparison, error) {
	candidate, err := s.Get(ctx, subject, candidateRunID)
	if err != nil {
		return ExperimentRunComparison{}, err
	}
	if candidate.ProjectID != projectID || candidate.ExperimentID != experimentID || candidate.Variant == "baseline" || candidate.BaselineRunID == "" {
		return ExperimentRunComparison{}, ErrExperimentComparisonUnavailable
	}
	baseline, err := s.Get(ctx, subject, candidate.BaselineRunID)
	if err != nil {
		return ExperimentRunComparison{}, ErrExperimentComparisonUnavailable
	}
	comparison := ExperimentRunComparison{Baseline: baseline, Candidate: candidate}
	if !runsComparable(baseline, candidate) {
		return comparison, nil
	}
	reader, ok := s.evaluator.(RuntimeEvaluationReader)
	if !ok {
		return ExperimentRunComparison{}, ErrExperimentComparisonUnavailable
	}
	baselineResult, found, err := reader.GetInProject(ctx, subject.TenantID, projectID, baseline.EvaluationRunID)
	if err != nil {
		return ExperimentRunComparison{}, err
	}
	if !found {
		return ExperimentRunComparison{}, ErrExperimentComparisonUnavailable
	}
	candidateResult, found, err := reader.GetInProject(ctx, subject.TenantID, projectID, candidate.EvaluationRunID)
	if err != nil {
		return ExperimentRunComparison{}, err
	}
	if !found {
		return ExperimentRunComparison{}, ErrExperimentComparisonUnavailable
	}
	comparison.Comparable = true
	comparison.Metrics = evaluationMetricDeltas(baselineResult, candidateResult)
	comparison.IndexMetrics = indexMetricDeltas(baseline, candidate)
	return comparison, nil
}

func runsComparable(baseline, candidate ExperimentRun) bool {
	return baseline.Status == ExperimentRunCompleted && candidate.Status == ExperimentRunCompleted &&
		baseline.Variant == "baseline" && isComparableTutorialCandidate(candidate) &&
		baseline.ID == candidate.BaselineRunID && baseline.EvaluationRunID != "" && candidate.EvaluationRunID != "" &&
		baseline.ComparisonFingerprint != "" && baseline.ComparisonFingerprint == candidate.ComparisonFingerprint &&
		baseline.DatasetID == candidate.DatasetID && baseline.Profile == candidate.Profile && baseline.TopK == candidate.TopK &&
		baseline.ParserMethod == "basic" && baseline.ChunkSizeTokens == TutorialBaselineChunkSizeTokens && baseline.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens &&
		!baseline.ContextualRetrievalEnabled && baseline.ContextualizedChunkCount == 0 && baseline.AverageContextTokens == 0 &&
		baseline.IndexedChunkCount > 0 && baseline.AverageChunkTokens > 0 && candidate.IndexedChunkCount > 0 && candidate.AverageChunkTokens > 0
}

func isComparableTutorialCandidate(candidate ExperimentRun) bool {
	switch candidate.Variant {
	case TutorialP1StructuredJSONCandidateID:
		return candidate.ParserMethod == TutorialStructuredJSONParserMethod && candidate.ChunkSizeTokens == TutorialBaselineChunkSizeTokens && candidate.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens && !candidate.ContextualRetrievalEnabled
	case TutorialP2RecursiveChunkCandidateID:
		return candidate.ParserMethod == "basic" && candidate.ChunkSizeTokens == TutorialP2ChunkSizeTokens && candidate.ChunkOverlapTokens == TutorialP2ChunkOverlapTokens && !candidate.ContextualRetrievalEnabled
	case TutorialP3ContextualCandidateID:
		return candidate.ParserMethod == "basic" && candidate.ChunkSizeTokens == TutorialBaselineChunkSizeTokens && candidate.ChunkOverlapTokens == TutorialBaselineChunkOverlapTokens && candidate.ContextualRetrievalEnabled && candidate.ContextualizedChunkCount > 0 && candidate.AverageContextTokens > 0
	default:
		return false
	}
}

func indexMetricDeltas(baseline, candidate ExperimentRun) []ExperimentMetricDelta {
	metrics := []ExperimentMetricDelta{
		metricDelta("average_chunk_tokens", baseline.AverageChunkTokens, candidate.AverageChunkTokens),
		metricDelta("chunk_count", float64(baseline.IndexedChunkCount), float64(candidate.IndexedChunkCount)),
	}
	if baseline.ContextualRetrievalEnabled || candidate.ContextualRetrievalEnabled || baseline.ContextualizedChunkCount > 0 || candidate.ContextualizedChunkCount > 0 || baseline.AverageContextTokens > 0 || candidate.AverageContextTokens > 0 {
		metrics = append(metrics,
			metricDelta("contextualized_chunk_count", float64(baseline.ContextualizedChunkCount), float64(candidate.ContextualizedChunkCount)),
			metricDelta("average_context_tokens", baseline.AverageContextTokens, candidate.AverageContextTokens),
		)
	}
	return metrics
}

func metricDelta(name string, baseline, candidate float64) ExperimentMetricDelta {
	delta := candidate - baseline
	item := ExperimentMetricDelta{Name: name, Baseline: baseline, Candidate: candidate, AbsoluteDelta: delta}
	if baseline != 0 {
		relative := delta / baseline
		item.RelativeDelta = &relative
	}
	return item
}

func evaluationMetricDeltas(baseline, candidate eval.RunResult) []ExperimentMetricDelta {
	metrics := make(map[string]struct{}, len(baseline.Metrics)+len(candidate.Metrics))
	for name := range baseline.Metrics {
		metrics[name] = struct{}{}
	}
	for name := range candidate.Metrics {
		metrics[name] = struct{}{}
	}
	names := make([]string, 0, len(metrics))
	for name := range metrics {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]ExperimentMetricDelta, 0, len(names))
	for _, name := range names {
		baselineValue := baseline.Metrics[name]
		candidateValue := candidate.Metrics[name]
		delta := candidateValue - baselineValue
		item := ExperimentMetricDelta{Name: name, Baseline: baselineValue, Candidate: candidateValue, AbsoluteDelta: delta}
		if baselineValue != 0 {
			relative := delta / baselineValue
			item.RelativeDelta = &relative
		}
		items = append(items, item)
	}
	return items
}
