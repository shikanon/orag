package eval

import (
	"testing"

	"github.com/shikanon/orag/internal/dataset"
)

func TestMetricEligibilityExcludesUnannotatedRetrievalMetrics(t *testing.T) {
	unannotated := dataset.Item{GroundTruth: "answer"}
	if metricEligible("ndcg_at_k", unannotated) || metricEligible("citation_precision", unannotated) {
		t.Fatal("retrieval metrics must not treat missing relevance labels as zero-score evidence")
	}
	if !metricEligible("citation_hit_rate", unannotated) || !metricEligible("latency_ms", unannotated) {
		t.Fatal("metrics that do not require relevance labels should remain eligible")
	}
}

func TestCompareRunsRequiresSameSnapshotAndJudgeConfig(t *testing.T) {
	baseline := RunResult{EvaluationFingerprint: "sha256:base", Manifest: EvaluationManifest{Dataset: DatasetSnapshot{ContentHash: "sha256:dataset"}, KnowledgeBaseID: "kb_a", JudgeConfigHash: "judge_a"}}
	candidate := RunResult{EvaluationFingerprint: "sha256:candidate", Manifest: EvaluationManifest{Dataset: DatasetSnapshot{ContentHash: "sha256:dataset"}, KnowledgeBaseID: "kb_a", JudgeConfigHash: "judge_a"}}
	if comparison := CompareRuns(baseline, candidate); !comparison.Comparable {
		t.Fatalf("comparison = %#v, want comparable", comparison)
	}
	candidate.Manifest.Dataset.ContentHash = "sha256:other"
	comparison := CompareRuns(baseline, candidate)
	if comparison.Comparable || len(comparison.HardMismatches) != 1 || comparison.HardMismatches[0] != "dataset.content_hash" {
		t.Fatalf("comparison = %#v", comparison)
	}
}

func TestMetricCatalogProvidesUserFacingExplanation(t *testing.T) {
	definition, ok := DefaultMetricRegistry.Definition("ndcg_at_k")
	if !ok || definition.DisplayName != "NDCG@K" || definition.Formula == "" || len(definition.Requires) == 0 || len(definition.Caveats) == 0 {
		t.Fatalf("definition = %#v", definition)
	}
}

func TestPairwiseWinScoreTreatsTieAsHalfWin(t *testing.T) {
	if got := pairwiseWinScore("tie"); got != 0.5 {
		t.Fatalf("pairwise tie score = %v, want 0.5", got)
	}
}

func TestComparePairedMetricsUsesMatchingEligibleItems(t *testing.T) {
	item := dataset.Item{ID: "case_1", GroundTruth: "answer", Weight: 2}
	baseline := RunResult{ID: "base", DatasetSnapshot: DatasetSnapshot{Items: []dataset.Item{item}}}
	candidate := RunResult{ID: "candidate", DatasetSnapshot: DatasetSnapshot{Items: []dataset.Item{item}}}
	comparisons := ComparePairedMetrics(
		baseline,
		candidate,
		[]EvaluationItemDetail{{DatasetItemID: item.ID, Metrics: map[string]float64{"answer_accuracy": 0}}},
		[]EvaluationItemDetail{{DatasetItemID: item.ID, Metrics: map[string]float64{"answer_accuracy": 1}}},
	)
	if len(comparisons) != 1 || comparisons[0].Metric != "answer_accuracy" || comparisons[0].AbsoluteDelta != 1 || comparisons[0].Decision != "insufficient_sample" {
		t.Fatalf("comparisons = %#v", comparisons)
	}
}
