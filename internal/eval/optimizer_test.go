package eval

import (
	"context"
	"testing"

	"github.com/shikanon/orag/internal/rag"
)

func TestOptimizerRanksCandidatesByPairwiseAccuracy(t *testing.T) {
	runner := &fakeOptimizationRunner{
		runs: []RunResult{
			{
				ID:       "run_pairwise_high",
				Accuracy: 0.20,
				Metrics: map[string]float64{
					"pairwise_accuracy":      0.90,
					"recall_at_k":            0.40,
					"ndcg_at_k":              0.50,
					"mrr":                    0.60,
					"map":                    0.70,
					"retrieval_failure_rate": 0.10,
					"redundancy_rate":        0.20,
					"duplicate_count":        1,
					"deduped_top_k_count":    3,
					"alpha_ndcg":             0.80,
					"aspect_coverage":        0.75,
					"latency_p95_ms":         120,
				},
			},
			{
				ID:       "run_recall_high",
				Accuracy: 0.99,
				Metrics: map[string]float64{
					"pairwise_accuracy":      0.70,
					"recall_at_k":            0.95,
					"ndcg_at_k":              0.90,
					"mrr":                    0.80,
					"map":                    0.85,
					"retrieval_failure_rate": 0,
					"redundancy_rate":        0.30,
					"duplicate_count":        2,
					"deduped_top_k_count":    6,
					"alpha_ndcg":             0.65,
					"aspect_coverage":        1,
					"latency_p95_ms":         240,
				},
			},
		},
	}

	result, err := (Optimizer{Runner: runner}).Optimize(context.Background(), OptimizeRequest{
		TenantID:        "tenant_default",
		DatasetID:       "dataset_default",
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime},
		TopKs:           []int{4, 8},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Best.RunID != "run_pairwise_high" || result.Best.Score != 0.90 || result.Best.PairwiseAccuracy != 0.90 {
		t.Fatalf("best = %#v, want pairwise accuracy winner", result.Best)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates len = %d, want 2", len(result.Candidates))
	}
	if result.Candidates[0].RunID != "run_pairwise_high" || result.Candidates[1].RunID != "run_recall_high" {
		t.Fatalf("candidates sorted by pairwise accuracy = %#v", result.Candidates)
	}
	if result.Candidates[1].RecallAtK != 0.95 || result.Candidates[1].PairwiseAccuracy != 0.70 {
		t.Fatalf("diagnostics missing from recall-heavy candidate = %#v", result.Candidates[1])
	}
	if result.Candidates[0].LatencyP95MS != 120 || result.Candidates[0].NDCGAtK != 0.50 ||
		result.Candidates[0].MRR != 0.60 || result.Candidates[0].MAP != 0.70 ||
		result.Candidates[0].RetrievalFailureRate != 0.10 || result.Candidates[0].RedundancyRate != 0.20 ||
		result.Candidates[0].DuplicateCount != 1 || result.Candidates[0].DedupedTopKCount != 3 ||
		result.Candidates[0].AlphaNDCG != 0.80 || result.Candidates[0].AspectCoverage != 0.75 {
		t.Fatalf("diagnostics missing from best candidate = %#v", result.Candidates[0])
	}
}

type fakeOptimizationRunner struct {
	runs []RunResult
	next int
}

func (r *fakeOptimizationRunner) Run(_ context.Context, _ RunRequest) (RunResult, error) {
	run := r.runs[r.next]
	r.next++
	return run, nil
}
