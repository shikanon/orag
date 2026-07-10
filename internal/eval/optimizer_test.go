package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/rag"
)

func TestOptimizerUsesAnswerAccuracy(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	dsSvc := dataset.NewService(dsRepo)
	ds, err := dsSvc.Create(ctx, "tenant_default", "optimizer-answer", "golden")
	if err != nil {
		t.Fatal(err)
	}
	_, err = dsSvc.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:       "qdrant vector",
		GroundTruth: "qdrant",
	})
	if err != nil {
		t.Fatal(err)
	}

	ragSvc := &rag.Service{
		Pipeline: pipelineFunc(func(_ context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
			if req.Profile == rag.ProfileHighPrecision {
				return rag.QueryResponse{Answer: "qdrant answer", CacheStatus: "miss"}, nil
			}
			return rag.QueryResponse{
				Answer: "postgres answer",
				Citations: []rag.Citation{{
					ChunkID:    "chk_1",
					DocumentID: "doc_1",
				}},
				CacheStatus: "miss",
			}, nil
		}),
	}
	runner := Runner{RAG: ragSvc, Datasets: dsSvc, Repository: NewMemoryRepository()}
	result, err := (Optimizer{Runner: runner}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_default",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime, rag.ProfileHighPrecision},
		TopKs:           []int{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(result.Candidates))
	}
	if result.Best.Profile != rag.ProfileHighPrecision || result.Best.Score != 1 {
		t.Fatalf("best = %#v, want high_precision score 1", result.Best)
	}
	var realtime, highPrecision CandidateResult
	for _, candidate := range result.Candidates {
		switch candidate.Profile {
		case rag.ProfileRealtime:
			realtime = candidate
		case rag.ProfileHighPrecision:
			highPrecision = candidate
		}
	}
	if realtime.Score != 0 || realtime.PairwiseAccuracy != 0 ||
		realtime.ScoreMetric != PrimaryMetricDeterministicAnswerMatch ||
		realtime.FallbackMetric != PrimaryMetricDeterministicAnswerMatch {
		t.Fatalf("citation-only candidate = %#v, want deterministic fallback score 0", realtime)
	}
	if highPrecision.Score != 1 || highPrecision.PairwiseAccuracy != 0 ||
		highPrecision.ScoreMetric != PrimaryMetricDeterministicAnswerMatch ||
		highPrecision.FallbackMetric != PrimaryMetricDeterministicAnswerMatch {
		t.Fatalf("answer-correct candidate = %#v, want deterministic fallback score 1", highPrecision)
	}
}

func TestOptimizerRequiresDatasetTenant(t *testing.T) {
	ctx := context.Background()
	dsSvc := dataset.NewService(dataset.NewMemoryRepository())
	ds, err := dsSvc.Create(ctx, "tenant_a", "optimizer-tenant", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.Create(ctx, "tenant_b", "tenant-b-regression", "golden"); err != nil {
		t.Fatal(err)
	}
	if _, err := dsSvc.AddItem(ctx, "tenant_a", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}

	runner := Runner{Datasets: dsSvc}
	result, err := (Optimizer{Runner: runner}).Optimize(ctx, OptimizeRequest{
		TenantID:        "tenant_b",
		DatasetID:       ds.ID,
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime},
		TopKs:           []int{1},
	})
	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("Optimize() error = %v, want dataset not found", err)
	}
	if !apperrors.IsCode(err, apperrors.CodeNotFound) {
		t.Fatalf("Optimize() error = %v, want not-found app error", err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %d, want 0", len(result.Candidates))
	}
}

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

func TestOptimizerPrefersRealPairwiseOverFallbackMetric(t *testing.T) {
	runner := &fakeOptimizationRunner{
		runs: []RunResult{
			{
				ID:       "run_fallback_high",
				Accuracy: 1,
				Metrics: map[string]float64{
					PrimaryMetricDeterministicAnswerMatch: 1,
				},
			},
			{
				ID:       "run_pairwise_real",
				Accuracy: 0.2,
				Metrics: map[string]float64{
					PrimaryMetricPairwiseAccuracy: 0.6,
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

	if result.Best.RunID != "run_pairwise_real" || result.Best.ScoreMetric != PrimaryMetricPairwiseAccuracy {
		t.Fatalf("best = %#v, want real pairwise prioritized", result.Best)
	}
	if result.Candidates[1].RunID != "run_fallback_high" ||
		result.Candidates[1].ScoreMetric != PrimaryMetricDeterministicAnswerMatch ||
		result.Candidates[1].FallbackMetric != PrimaryMetricDeterministicAnswerMatch ||
		result.Candidates[1].PairwiseAccuracy != 0 {
		t.Fatalf("fallback candidate = %#v, want explicit deterministic fallback without pairwise", result.Candidates[1])
	}
}

func TestOptimizerRejectsUnknownMetricBeforeCandidateScoring(t *testing.T) {
	runner := &fakeOptimizationRunner{
		runs: []RunResult{{
			ID:       "run_unknown_metric",
			Accuracy: 1,
			Metrics: map[string]float64{
				PrimaryMetricPairwiseAccuracy: 1,
				"harness_custom":              0.5,
			},
		}},
	}

	result, err := (Optimizer{Runner: runner}).Optimize(context.Background(), OptimizeRequest{
		TenantID:        "tenant_default",
		DatasetID:       "dataset_default",
		KnowledgeBaseID: "kb_default",
		Profiles:        []rag.Profile{rag.ProfileRealtime},
		TopKs:           []int{4},
	})
	if !apperrors.IsCode(err, apperrors.CodeValidation) {
		t.Fatalf("Optimize() error = %v, want validation", err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %d, want 0", len(result.Candidates))
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
