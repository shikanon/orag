package tutorial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/ingest"
)

func TestLiveRunIndexesPrivatePackAndDelegatesEvaluation(t *testing.T) {
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	content := []byte("tutorial baseline corpus")
	hash := sha256.Sum256(content)
	object := PackObject{Path: "corpus/data.txt", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(content)), ContentType: "text/plain"}
	store := installPrivateObject(t, "tenant_a", "prj_1", "tclj_1", object, content)
	repo := NewMemoryCloneRepository()
	experiment := Experiment{
		ID: "texp_1", TenantID: "tenant_a", ProjectID: "prj_1", CloneJobID: "tclj_1",
		TemplateID: "text-rag", TemplateVersion: "1.0.0", Tier: "quick", PackStatus: PackStatusInstalled,
		RuntimeStatus: "ready", KnowledgeBaseID: "tkb_1", DatasetID: "tds_1", BaselineProfile: "realtime", BaselineTopK: 5,
		PackManifest: Manifest{Objects: []PackObject{object}, Runtime: &RuntimeManifest{
			Baseline: RuntimeBaseline{Profile: "realtime", TopK: 5}, Documents: []RuntimeDocument{{ObjectPath: object.Path, Name: "教程语料"}},
			Dataset: RuntimeDataset{Name: "评测", Items: []RuntimeDatasetItem{{Query: "问题", GroundTruth: "答案"}}},
		}},
	}
	if err := repo.EnsureExperiment(context.Background(), experiment); err != nil {
		t.Fatal(err)
	}
	ingestor := &recordingRuntimeIngestor{}
	evaluator := &recordingRuntimeEvaluator{}
	service := NewLiveRunService(repo, repo, func() time.Time { return now })
	service.Configure(ingestor, evaluator, store)
	subject := Subject{TenantID: "tenant_a", ID: "user_a"}
	run, replayed, err := service.Start(context.Background(), subject, experiment.ProjectID, "run_1")
	if err != nil || replayed || run.Status != ExperimentRunQueued || run.Stage != ExperimentRunStageIndex {
		t.Fatalf("run=%#v replayed=%v err=%v", run, replayed, err)
	}
	duplicate, replayed, err := service.Start(context.Background(), subject, experiment.ProjectID, "run_1")
	if err != nil || !replayed || duplicate.ID != run.ID {
		t.Fatalf("duplicate=%#v replayed=%v err=%v", duplicate, replayed, err)
	}
	if err := service.Execute(context.Background(), subject.TenantID, run.ID); err != nil {
		t.Fatal(err)
	}
	completed, err := service.Get(context.Background(), subject, run.ID)
	if err != nil || completed.Status != ExperimentRunCompleted || completed.Stage != ExperimentRunStageComplete || completed.EvaluationRunID != "eval_1" {
		t.Fatalf("completed=%#v err=%v", completed, err)
	}
	if len(ingestor.requests) != 1 || string(ingestor.requests[0].Content) != string(content) || ingestor.requests[0].KnowledgeBaseID != experiment.KnowledgeBaseID {
		t.Fatalf("ingest requests=%#v", ingestor.requests)
	}
	if evaluator.request.DatasetID != experiment.DatasetID || evaluator.request.KnowledgeBaseID != experiment.KnowledgeBaseID || evaluator.request.TopK != 5 {
		t.Fatalf("evaluation request=%#v", evaluator.request)
	}
}

func TestLiveRunRejectsUnavailableRuntimeAndCancelsQueuedRun(t *testing.T) {
	repo := NewMemoryCloneRepository()
	if err := repo.EnsureExperiment(context.Background(), Experiment{ID: "texp_missing", TenantID: "tenant_a", ProjectID: "prj_missing", TemplateID: "text-rag", Tier: "quick", PackStatus: PackStatusInstalled, RuntimeStatus: "runtime_unavailable"}); err != nil {
		t.Fatal(err)
	}
	service := NewLiveRunService(repo, repo, time.Now)
	subject := Subject{TenantID: "tenant_a", ID: "user_a"}
	if _, _, err := service.Start(context.Background(), subject, "prj_missing", "key"); err != ErrRuntimeUnavailable {
		t.Fatalf("unavailable start error=%v", err)
	}

	experiment := Experiment{ID: "texp_1", TenantID: "tenant_a", ProjectID: "prj_1", CloneJobID: "tclj_1", TemplateID: "text-rag", Tier: "quick", PackStatus: PackStatusInstalled, RuntimeStatus: "ready", KnowledgeBaseID: "kb_1", DatasetID: "ds_1", BaselineProfile: "realtime", BaselineTopK: 1, PackManifest: Manifest{Runtime: &RuntimeManifest{}}}
	if err := repo.EnsureExperiment(context.Background(), experiment); err != nil {
		t.Fatal(err)
	}
	run, _, err := service.Start(context.Background(), subject, "prj_1", "cancel")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.Cancel(context.Background(), subject, run.ID)
	if err != nil || cancelled.Status != ExperimentRunCancelled {
		t.Fatalf("cancelled=%#v err=%v", cancelled, err)
	}
	unsupported := experiment
	unsupported.ID, unsupported.ProjectID, unsupported.Tier = "texp_benchmark", "prj_benchmark", "benchmark"
	if err := repo.EnsureExperiment(context.Background(), unsupported); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Start(context.Background(), subject, unsupported.ProjectID, "benchmark"); err != ErrRuntimeUnavailable {
		t.Fatalf("unsupported tier start error=%v", err)
	}
}

func installPrivateObject(t *testing.T, tenantID, projectID, jobID string, object PackObject, content []byte) *LocalPrivateStore {
	t.Helper()
	root := t.TempDir()
	store, err := NewLocalPrivateStore(filepath.Join(root, "private"), "tutorial-experiments")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "input")
	if err := os.WriteFile(input, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.PutVerified(context.Background(), PrivateObject{TenantID: tenantID, ProjectID: projectID, JobID: jobID, Object: VerifiedObject{PackObject: object, TempPath: input}}); err != nil {
		t.Fatal(err)
	}
	return store
}

type recordingRuntimeIngestor struct{ requests []ingest.Request }

func (r *recordingRuntimeIngestor) Ingest(_ context.Context, request ingest.Request) (ingest.Result, error) {
	r.requests = append(r.requests, request)
	return ingest.Result{}, nil
}

type recordingRuntimeEvaluator struct{ request eval.RunRequest }

func (r *recordingRuntimeEvaluator) Run(_ context.Context, request eval.RunRequest) (eval.RunResult, error) {
	r.request = request
	return eval.RunResult{ID: "eval_1"}, nil
}
