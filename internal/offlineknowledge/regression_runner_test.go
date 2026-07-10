package offlineknowledge

import (
	"context"
	"testing"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

func TestEvalRegressionRunnerComparesBaselineAndOptimizationWithRealEvalRunner(t *testing.T) {
	ctx := context.Background()
	dsSvc := regressionDatasetService(t, []dataset.Item{{
		ID:             "item_1",
		DatasetID:      "ds_regression",
		Query:          "What is ORAG?",
		GroundTruth:    "ORAG framework",
		RelevantDocIDs: []string{"doc_1"},
	}}, "ds_regression")
	runner := NewEvalRegressionRunner(EvalRegressionRunnerOptions{
		BaselineRunner: eval.Runner{
			RAG: &rag.Service{Pipeline: regressionPipeline{resp: rag.QueryResponse{
				Answer:          "unrelated answer",
				RetrievedChunks: nil,
				LatencyMS:       100,
			}}},
			Datasets: dsSvc,
		},
		WithOptimization: eval.Runner{
			RAG: &rag.Service{Pipeline: regressionPipeline{resp: rag.QueryResponse{
				Answer: "ORAG framework answer",
				Citations: []rag.Citation{{
					ChunkID:    "chunk_1",
					DocumentID: "doc_1",
				}},
				RetrievedChunks: []kb.SearchResult{{
					Chunk: kb.Chunk{ID: "chunk_1", DocumentID: "doc_1"},
				}},
				LatencyMS: 125,
			}}},
			Datasets: dsSvc,
		},
		Datasets:  dsSvc,
		DatasetID: "ds_regression",
		TopK:      3,
	})

	result, err := runner.RunRegression(ctx, RegressionRequest{
		TenantID: "tenant_1",
		Item: OptimizationItem{
			ID:                "opt_1",
			TenantID:          "tenant_1",
			KBID:              "kb_1",
			QuestionClusterID: "cluster_must_not_be_used_as_dataset",
			ItemType:          ItemTypeAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunRegression() error = %v", err)
	}
	if !result.Passed || !result.FullDatasetUsed {
		t.Fatalf("result pass/full_dataset = %v/%v, want true/true", result.Passed, result.FullDatasetUsed)
	}
	if result.RecallLift != 1 || result.AnswerQualityLift != 1 || result.CitationCoverageLift != 1 {
		t.Fatalf("lifts = recall %v answer %v citation %v, want all 1", result.RecallLift, result.AnswerQualityLift, result.CitationCoverageLift)
	}
	if result.LatencyDeltaMS != 25 || result.LatencyDelta.Milliseconds() != 25 {
		t.Fatalf("latency delta = %d/%s, want 25ms", result.LatencyDeltaMS, result.LatencyDelta)
	}
}

func TestEvalRegressionRunnerFailsWhenFullDatasetRequiredButEvalIsPartial(t *testing.T) {
	dsSvc := regressionDatasetService(t, []dataset.Item{
		{ID: "item_1", DatasetID: "ds_regression", Query: "q1", GroundTruth: "a1"},
		{ID: "item_2", DatasetID: "ds_regression", Query: "q2", GroundTruth: "a2"},
	}, "ds_regression")
	runner := NewEvalRegressionRunner(EvalRegressionRunnerOptions{
		BaselineRunner:   regressionEvalRunnerFunc(func(context.Context, eval.RunRequest) (eval.RunResult, error) { return eval.RunResult{Total: 1}, nil }),
		WithOptimization: regressionEvalRunnerFunc(func(context.Context, eval.RunRequest) (eval.RunResult, error) { return eval.RunResult{Total: 1}, nil }),
		Datasets:         dsSvc,
		DatasetID:        "ds_regression",
	})

	result, err := runner.RunRegression(context.Background(), RegressionRequest{
		TenantID:            "tenant_1",
		FullDatasetRequired: true,
		Item: OptimizationItem{
			ID:                "rewrite_1",
			TenantID:          "tenant_1",
			KBID:              "kb_1",
			QuestionClusterID: "cluster_must_not_be_used_as_dataset",
			ItemType:          ItemTypeQueryRewrite,
		},
	})
	if err != nil {
		t.Fatalf("RunRegression() error = %v", err)
	}
	if result.Passed || result.FullDatasetUsed {
		t.Fatalf("result pass/full_dataset = %v/%v, want false/false", result.Passed, result.FullDatasetUsed)
	}
}

func TestEvalRegressionRunnerRequiresConfiguredDatasetID(t *testing.T) {
	runner := NewEvalRegressionRunner(EvalRegressionRunnerOptions{
		BaselineRunner:   regressionEvalRunnerFunc(func(context.Context, eval.RunRequest) (eval.RunResult, error) { return eval.RunResult{}, nil }),
		WithOptimization: regressionEvalRunnerFunc(func(context.Context, eval.RunRequest) (eval.RunResult, error) { return eval.RunResult{}, nil }),
		Datasets:         dataset.NewService(),
	})

	_, err := runner.RunRegression(context.Background(), RegressionRequest{
		TenantID: "tenant_1",
		Item: OptimizationItem{
			ID:                "opt_missing_dataset",
			TenantID:          "tenant_1",
			KBID:              "kb_1",
			QuestionClusterID: "cluster_legacy_should_not_be_used",
			ItemType:          ItemTypeAnswer,
		},
	})
	if err != ErrRegressionDatasetRequired {
		t.Fatalf("RunRegression() error = %v, want %v", err, ErrRegressionDatasetRequired)
	}
}

func TestEvalRegressionRunnerDefaultsToProfileNeutralScopedCandidate(t *testing.T) {
	dsSvc := regressionDatasetService(t, []dataset.Item{{
		ID:          "item_1",
		DatasetID:   "ds_regression",
		Query:       "What is ORAG?",
		GroundTruth: "ORAG framework",
	}}, "ds_regression")
	baseline := &recordingRegressionEvalRunner{result: eval.RunResult{
		Total:   1,
		Metrics: map[string]float64{"answer_accuracy": 0.4},
	}}
	candidate := &recordingRegressionEvalRunner{result: eval.RunResult{
		Total:   1,
		Metrics: map[string]float64{"answer_accuracy": 0.9},
	}}
	runner := NewEvalRegressionRunner(EvalRegressionRunnerOptions{
		BaselineRunner:   baseline,
		WithOptimization: candidate,
		Datasets:         dsSvc,
		DatasetID:        "ds_regression",
		BaselineProfile:  rag.ProfileRealtime,
	})

	result, err := runner.RunRegression(context.Background(), RegressionRequest{
		TenantID: "tenant_1",
		ItemID:   "opt_scoped",
		Item: OptimizationItem{
			ID:       "opt_scoped",
			TenantID: "tenant_1",
			KBID:     "kb_1",
			ItemType: ItemTypeAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunRegression() error = %v", err)
	}
	if len(baseline.requests) != 1 || len(candidate.requests) != 1 {
		t.Fatalf("runner calls baseline=%d candidate=%d, want 1/1", len(baseline.requests), len(candidate.requests))
	}
	if baseline.requests[0].Profile != rag.ProfileRealtime || candidate.requests[0].Profile != rag.ProfileRealtime {
		t.Fatalf("profiles baseline=%q candidate=%q, want same realtime", baseline.requests[0].Profile, candidate.requests[0].Profile)
	}
	if baseline.requests[0].ScopedShadowItemID != "" {
		t.Fatalf("baseline scoped item = %q, want empty", baseline.requests[0].ScopedShadowItemID)
	}
	if candidate.requests[0].ScopedShadowItemID != "opt_scoped" {
		t.Fatalf("candidate scoped item = %q, want opt_scoped", candidate.requests[0].ScopedShadowItemID)
	}
	if result.ScopedItemID != "opt_scoped" {
		t.Fatalf("result scoped item = %q, want opt_scoped", result.ScopedItemID)
	}
	if !result.ProfileNeutrality.SameProfile || !result.ProfileNeutrality.OptimizationLiftOnly || result.ProfileExperiment != nil {
		t.Fatalf("profile metadata = %#v experiment=%#v, want neutral optimization lift only", result.ProfileNeutrality, result.ProfileExperiment)
	}
}

func TestEvalRegressionRunnerReportsExplicitProfileExperimentSeparately(t *testing.T) {
	dsSvc := regressionDatasetService(t, []dataset.Item{{
		ID:          "item_1",
		DatasetID:   "ds_regression",
		Query:       "What is ORAG?",
		GroundTruth: "ORAG framework",
	}}, "ds_regression")
	runner := NewEvalRegressionRunner(EvalRegressionRunnerOptions{
		BaselineRunner: &recordingRegressionEvalRunner{result: eval.RunResult{
			Total:   1,
			Metrics: map[string]float64{"answer_accuracy": 0.4},
		}},
		WithOptimization: &recordingRegressionEvalRunner{result: eval.RunResult{
			Total:   1,
			Metrics: map[string]float64{"answer_accuracy": 0.9},
		}},
		Datasets:                dsSvc,
		DatasetID:               "ds_regression",
		BaselineProfile:         rag.ProfileRealtime,
		WithOptimizationProfile: rag.ProfileHighPrecision,
	})

	result, err := runner.RunRegression(context.Background(), RegressionRequest{
		TenantID: "tenant_1",
		ItemID:   "opt_profile_experiment",
		Item: OptimizationItem{
			ID:       "opt_profile_experiment",
			TenantID: "tenant_1",
			KBID:     "kb_1",
			ItemType: ItemTypeAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunRegression() error = %v", err)
	}
	if result.ProfileNeutrality.SameProfile || result.ProfileNeutrality.OptimizationLiftOnly {
		t.Fatalf("profile neutrality = %#v, want explicit non-neutral experiment", result.ProfileNeutrality)
	}
	if result.ProfileExperiment == nil || !result.ProfileExperiment.Enabled {
		t.Fatalf("profile experiment = %#v, want enabled metadata", result.ProfileExperiment)
	}
	if result.ProfileExperiment.BaselineProfile != string(rag.ProfileRealtime) ||
		result.ProfileExperiment.CandidateProfile != string(rag.ProfileHighPrecision) {
		t.Fatalf("profile experiment = %#v, want realtime/high_precision", result.ProfileExperiment)
	}
}

func TestEvalRegressionRunnerRecordsHoldoutGateAndBlocksPass(t *testing.T) {
	dsSvc := regressionDatasetService(t, []dataset.Item{
		{ID: "item_eval", DatasetID: "ds_regression", Query: "eval", GroundTruth: "a", Split: dataset.DatasetSplitEval},
		{ID: "item_holdout", DatasetID: "ds_regression", Query: "holdout", GroundTruth: "a", Split: dataset.DatasetSplitHoldout},
	}, "ds_regression")
	baseline := &recordingRegressionEvalRunner{result: eval.RunResult{
		Total:   2,
		Metrics: map[string]float64{"answer_accuracy": 0.4},
	}}
	candidate := &recordingRegressionEvalRunner{}
	candidate.result = eval.RunResult{
		Total:   2,
		Metrics: map[string]float64{"answer_accuracy": 0.9},
	}
	candidateWithHoldout := regressionEvalRunnerFunc(func(_ context.Context, req eval.RunRequest) (eval.RunResult, error) {
		candidate.requests = append(candidate.requests, req)
		if req.Split == dataset.DatasetSplitHoldout {
			result := eval.RunResult{
				Total:                 1,
				Split:                 dataset.DatasetSplitHoldout,
				UnweightedSampleCount: 1,
				WeightedSampleCount:   1,
				Metrics: map[string]float64{
					eval.PrimaryMetricDeterministicAnswerMatch: 0.5,
				},
			}
			result.HoldoutGate = eval.EvaluateHoldoutGate(result, *req.HoldoutGate)
			return result, nil
		}
		return candidate.result, nil
	})
	runner := NewEvalRegressionRunner(EvalRegressionRunnerOptions{
		BaselineRunner:   baseline,
		WithOptimization: candidateWithHoldout,
		Datasets:         dsSvc,
		DatasetID:        "ds_regression",
		HoldoutGate: eval.HoldoutGateConfig{
			Enabled:        true,
			MinSampleCount: 1,
			MinQuality:     0.8,
		},
	})

	result, err := runner.RunRegression(context.Background(), RegressionRequest{
		TenantID: "tenant_1",
		ItemID:   "opt_holdout",
		Item: OptimizationItem{
			ID:       "opt_holdout",
			TenantID: "tenant_1",
			KBID:     "kb_1",
			ItemType: ItemTypeAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunRegression() error = %v", err)
	}
	if result.Passed || !result.HoldoutGate.Enabled || result.HoldoutGate.Passed {
		t.Fatalf("result = %#v, want holdout gate failure to block pass", result)
	}
	if len(candidate.requests) != 2 || candidate.requests[1].Split != dataset.DatasetSplitHoldout {
		t.Fatalf("candidate requests = %#v, want main and holdout split runs", candidate.requests)
	}
}

func regressionDatasetService(t *testing.T, items []dataset.Item, datasetID string) *dataset.Service {
	t.Helper()
	repo := dataset.NewMemoryRepository()
	if _, err := repo.CreateDataset(context.Background(), dataset.Dataset{
		ID:       datasetID,
		TenantID: "tenant_1",
		Name:     "offline regression",
		Kind:     "golden",
	}); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if _, err := repo.AddDatasetItem(context.Background(), "tenant_1", item); err != nil {
			t.Fatal(err)
		}
	}
	return dataset.NewService(repo)
}

type regressionPipeline struct {
	resp rag.QueryResponse
}

func (p regressionPipeline) Invoke(_ context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
	resp := p.resp
	resp.TraceID = req.TraceID
	return resp, nil
}

type regressionEvalRunnerFunc func(context.Context, eval.RunRequest) (eval.RunResult, error)

func (f regressionEvalRunnerFunc) Run(ctx context.Context, req eval.RunRequest) (eval.RunResult, error) {
	return f(ctx, req)
}

type recordingRegressionEvalRunner struct {
	requests []eval.RunRequest
	result   eval.RunResult
	err      error
}

func (r *recordingRegressionEvalRunner) Run(_ context.Context, req eval.RunRequest) (eval.RunResult, error) {
	r.requests = append(r.requests, req)
	if r.err != nil {
		return eval.RunResult{}, r.err
	}
	return r.result, nil
}
