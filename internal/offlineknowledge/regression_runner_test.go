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
