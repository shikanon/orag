package eval

import (
	"strings"
	"testing"

	"github.com/shikanon/orag/internal/platform/apperrors"
)

func TestMetricRegistryAcceptsCurrentAndPlannedMetrics(t *testing.T) {
	metrics := map[string]float64{
		PrimaryMetricDeterministicAnswerMatch: 1,
		"answer_accuracy":                     1,
		"accuracy":                            1,
		"hit_rate":                            1,
		PrimaryMetricPairwiseAccuracy:         1,
		"citation_hit_rate":                   1,
		"context_recall":                      1,
		"citation_precision":                  1,
		"latency_ms":                          42,
		"latency_p95_ms":                      42,
		"cache_hit":                           0,
		"cache_hit_rate":                      0,
		"ndcg_at_k":                           1,
		"recall_at_k":                         1,
		"mrr":                                 1,
		"map":                                 1,
		"coverage":                            1,
		"retrieval_failure_rate":              0,
		"redundancy_rate":                     0,
		"duplicate_count":                     0,
		"deduped_top_k_count":                 3,
		"alpha_ndcg":                          1,
		"aspect_coverage":                     1,
		"faithfulness":                        0.9,
		"groundedness":                        0.9,
		"answer_relevance":                    0.9,
		"hallucination":                       0,
		"completeness":                        0.9,
		"citation_support":                    0.9,
		"qag_score":                           0.9,
		"qag_claim_coverage":                  0.8,
		"qag_question_count":                  4,
		"qag_unverifiable_rate":               0.25,
		"instruction_following":               0.9,
		"safety":                              1,
		"cost_usd":                            0.01,
		"prompt_tokens":                       10,
		"completion_tokens":                   20,
		"total_tokens":                        30,
		"weighted_sample_count":               2,
		"unweighted_sample_count":             1,
		"missing_split":                       0,
	}

	if err := ValidateMetricMap(metrics); err != nil {
		t.Fatalf("ValidateMetricMap() error = %v", err)
	}
}

func TestMetricRegistryRejectsUnknownMetric(t *testing.T) {
	err := ValidateMetricMap(map[string]float64{"unknown_metric": 1})
	if !apperrors.IsCode(err, apperrors.CodeValidation) {
		t.Fatalf("ValidateMetricMap() error = %v, want validation", err)
	}
	if !strings.Contains(err.Error(), "unknown_metric") {
		t.Fatalf("ValidateMetricMap() error = %v, want metric name", err)
	}
}

func TestMetricRegistryDrivesAggregationWhitelist(t *testing.T) {
	if !shouldAggregateItemMetric(PrimaryMetricDeterministicAnswerMatch) {
		t.Fatalf("%s should aggregate with dataset item weights", PrimaryMetricDeterministicAnswerMatch)
	}
	if !shouldAggregateItemMetric("ndcg_at_k") {
		t.Fatal("ndcg_at_k should aggregate")
	}
	if !shouldAggregateItemMetric("latency_ms") {
		t.Fatal("latency_ms should aggregate with dataset item weights")
	}
	if !shouldAggregateItemMetric("faithfulness") {
		t.Fatal("faithfulness should aggregate with dataset item weights")
	}
	for _, metric := range []string{"qag_score", "qag_claim_coverage", "qag_question_count", "qag_unverifiable_rate"} {
		if !shouldAggregateItemMetric(metric) {
			t.Fatalf("%s should aggregate", metric)
		}
	}
}
