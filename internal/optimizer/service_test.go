package optimizer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/rag"
)

func TestServiceSubmitRunsAsyncAndEvaluatesHoldout(t *testing.T) {
	repo := newMemoryOptimizationRepository()
	started := make(chan struct{})
	release := make(chan struct{})
	runner := &recordingCandidateRunner{
		blockOnFirst: started,
		releaseFirst: release,
	}
	service := &Service{Repository: repo, Runner: runner}

	run, err := service.Submit(context.Background(), basicSubmitRequest())
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if run.Status != RunStatusQueued {
		t.Fatalf("Submit() status = %q, want queued", run.Status)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start asynchronously")
	}
	status, ok, err := service.Get(context.Background(), "tenant_a", run.ID)
	if err != nil || !ok {
		t.Fatalf("Get() = ok %v error %v", ok, err)
	}
	if status.Run.Status != RunStatusRunning {
		t.Fatalf("status while runner blocked = %q, want running", status.Run.Status)
	}
	close(release)

	status = waitForRunStatus(t, service, run.ID, RunStatusCompleted)
	bestID := candidateWithDenseTopK(status.Candidates, 2).ID
	loserID := candidateWithDenseTopK(status.Candidates, 1).ID
	if status.Run.BestCandidateID != bestID || status.Run.HoldoutCandidateID != bestID {
		t.Fatalf("best/holdout = %q/%q, want %s/%s", status.Run.BestCandidateID, status.Run.HoldoutCandidateID, bestID, bestID)
	}
	if status.Run.CompletedCandidateCount != 2 {
		t.Fatalf("completed count = %d, want selection candidates only", status.Run.CompletedCandidateCount)
	}
	best := candidateByID(status.Candidates, bestID)
	if best.HoldoutScore == nil || *best.HoldoutScore != 0.82 {
		t.Fatalf("holdout score = %#v, want 0.82", best.HoldoutScore)
	}
	if !runner.saw(bestID, PhaseHoldout, "holdout") {
		t.Fatalf("runner calls = %#v, want holdout evaluation for selected candidate", runner.calls)
	}
	if runner.saw(loserID, PhaseHoldout, "holdout") {
		t.Fatalf("runner calls = %#v, holdout must not run for losing candidate", runner.calls)
	}
}

func TestServiceBudgetStopAndResumeSkipsCompletedCandidate(t *testing.T) {
	repo := newMemoryOptimizationRepository()
	runner := &recordingCandidateRunner{}
	service := &Service{Repository: repo, Runner: runner, DisableAutoStart: true}
	req := basicSubmitRequest()
	req.HoldoutSplit = ""
	req.Budget = Budget{MaxJudgeCalls: 1}

	run, err := service.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := service.RunPending(context.Background(), "tenant_a", run.ID, req); err != nil {
		t.Fatalf("RunPending() error = %v", err)
	}
	status, _, _ := service.Get(context.Background(), "tenant_a", run.ID)
	if status.Run.Status != RunStatusBudgetStopped {
		t.Fatalf("status = %q, want budget_stopped", status.Run.Status)
	}
	if status.Run.CompletedCandidateCount != 1 || len(status.Run.Checkpoint.CompletedCandidateIDs) != 1 {
		t.Fatalf("checkpoint = %#v, want one completed candidate", status.Run.Checkpoint)
	}
	firstCompletedID := status.Run.Checkpoint.CompletedCandidateIDs[0]

	resumeReq := req
	resumeReq.Budget = Budget{}
	if _, err := service.Resume(context.Background(), "tenant_a", run.ID, resumeReq); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if err := service.RunPending(context.Background(), "tenant_a", run.ID, resumeReq); err != nil {
		t.Fatalf("RunPending(resume) error = %v", err)
	}
	status, _, _ = service.Get(context.Background(), "tenant_a", run.ID)
	if status.Run.Status != RunStatusCompleted {
		t.Fatalf("status after resume = %q, want completed", status.Run.Status)
	}
	if runner.selectionCount(firstCompletedID) != 1 {
		t.Fatalf("runner calls = %#v, completed candidate should not be repeated", runner.calls)
	}
	if len(runner.selectionIDs()) != 2 {
		t.Fatalf("runner calls = %#v, resume should skip completed and run incomplete once", runner.calls)
	}
}

func TestServiceStopsAfterSingleCandidateCostOverrun(t *testing.T) {
	repo := newMemoryOptimizationRepository()
	runner := &recordingCandidateRunner{}
	service := &Service{Repository: repo, Runner: runner, DisableAutoStart: true}
	req := basicSubmitRequest()
	req.SearchSpace.Retrieval.DenseTopK = []int{1}
	req.Search.MaxCandidates = 1
	req.Budget = Budget{MaxCostUSD: 0.05}

	run, err := service.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := service.RunPending(context.Background(), "tenant_a", run.ID, req); err != nil {
		t.Fatalf("RunPending() error = %v", err)
	}
	status, _, _ := service.Get(context.Background(), "tenant_a", run.ID)
	if status.Run.Status != RunStatusBudgetStopped {
		t.Fatalf("status = %q, want budget_stopped", status.Run.Status)
	}
	if status.Run.CostUSD != 0.1 {
		t.Fatalf("cost_usd = %v, want 0.1", status.Run.CostUSD)
	}
	if status.Run.Checkpoint.CostUSD != 0.1 {
		t.Fatalf("checkpoint cost_usd = %v, want 0.1", status.Run.Checkpoint.CostUSD)
	}
	if status.Run.CompletedCandidateCount != 1 || len(status.Run.Checkpoint.CompletedCandidateIDs) != 1 {
		t.Fatalf("checkpoint = %#v, want one completed candidate", status.Run.Checkpoint)
	}
	candidateID := status.Run.Checkpoint.CompletedCandidateIDs[0]
	if runner.saw(candidateID, PhaseHoldout, "holdout") {
		t.Fatalf("runner calls = %#v, holdout should be skipped after cost budget stop", runner.calls)
	}
}

func TestServiceEmptyResumeUsesStoredRunConfig(t *testing.T) {
	repo := newMemoryOptimizationRepository()
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	runner := &recordingCandidateRunner{
		afterCall: func(CandidateRunRequest) {
			now = now.Add(2 * time.Second)
		},
	}
	service := &Service{
		Repository:       repo,
		Runner:           runner,
		DisableAutoStart: true,
		Now: func() time.Time {
			return now
		},
	}
	req := basicSubmitRequest()
	req.Budget = Budget{MaxWallTimeSeconds: 1}
	req.Profile = rag.ProfileHighPrecision
	req.TopK = 7
	req.NamespaceTTL = 45 * time.Second

	run, err := service.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := service.RunPending(context.Background(), "tenant_a", run.ID, req); err != nil {
		t.Fatalf("RunPending() error = %v", err)
	}
	status, _, _ := service.Get(context.Background(), "tenant_a", run.ID)
	if status.Run.Status != RunStatusBudgetStopped {
		t.Fatalf("status = %q, want budget_stopped", status.Run.Status)
	}

	if _, err := service.Resume(context.Background(), "tenant_a", run.ID, SubmitRequest{}); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if err := service.RunPending(context.Background(), "tenant_a", run.ID, SubmitRequest{}); err != nil {
		t.Fatalf("RunPending(resume) error = %v", err)
	}
	status, _, _ = service.Get(context.Background(), "tenant_a", run.ID)
	if status.Run.Status != RunStatusCompleted {
		t.Fatalf("status after resume = %q, want completed", status.Run.Status)
	}
	if status.Run.HoldoutCandidateID == "" {
		t.Fatalf("holdout candidate id was not set after empty resume: %#v", status.Run)
	}
	if !runner.sawConfigured(PhaseHoldout, "holdout", rag.ProfileHighPrecision, 7, 45*time.Second) {
		t.Fatalf("runner calls = %#v, want holdout with stored profile/top_k/namespace ttl", runner.calls)
	}
}

func TestServiceCancelPersistsCheckpointAndStopsScheduling(t *testing.T) {
	repo := newMemoryOptimizationRepository()
	runner := &recordingCandidateRunner{}
	service := &Service{Repository: repo, Runner: runner, DisableAutoStart: true}
	req := basicSubmitRequest()
	req.HoldoutSplit = ""

	run, err := service.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if _, err := service.Cancel(context.Background(), "tenant_a", run.ID, "user requested"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if err := service.RunPending(context.Background(), "tenant_a", run.ID, req); err != nil {
		t.Fatalf("RunPending() error = %v", err)
	}
	status, _, _ := service.Get(context.Background(), "tenant_a", run.ID)
	if status.Run.Status != RunStatusCanceled || status.Run.Checkpoint.CancelRequestedAt == nil {
		t.Fatalf("run = %#v, want canceled with cancel checkpoint", status.Run)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, cancel should stop scheduling new candidates", runner.calls)
	}
}

func TestServiceRecordsRunnerFailure(t *testing.T) {
	repo := newMemoryOptimizationRepository()
	runner := &recordingCandidateRunner{err: errors.New("runner failed")}
	service := &Service{Repository: repo, Runner: runner, DisableAutoStart: true}
	req := basicSubmitRequest()
	req.HoldoutSplit = ""

	run, err := service.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := service.RunPending(context.Background(), "tenant_a", run.ID, req); err == nil {
		t.Fatal("RunPending() error = nil, want runner failure")
	}
	status, _, _ := service.Get(context.Background(), "tenant_a", run.ID)
	if status.Run.Status != RunStatusFailed || status.Run.StatusReason == "" {
		t.Fatalf("run = %#v, want failed with reason", status.Run)
	}
	if got := status.Candidates[0]; got.Status != CandidateStatusFailed || got.Error == "" {
		t.Fatalf("candidate = %#v, want failed with error", got)
	}
}

func basicSubmitRequest() SubmitRequest {
	return SubmitRequest{
		TenantID:        "tenant_a",
		DatasetID:       "ds_1",
		KnowledgeBaseID: "kb_1",
		Objective:       ObjectiveSpec{Maximize: "pairwise_accuracy"},
		SearchSpace: SearchSpace{Retrieval: RetrievalSpace{
			DenseTopK: []int{1, 2},
		}},
		Search:         SearchSpec{Strategy: SearchStrategyGrid, MaxCandidates: 2},
		SelectionSplit: "eval",
		HoldoutSplit:   "holdout",
	}
}

type memoryOptimizationRepository struct {
	mu         sync.Mutex
	runs       map[string]OptimizationRun
	candidates map[string]OptimizationCandidate
	harness    []HarnessRunRecord
}

func newMemoryOptimizationRepository() *memoryOptimizationRepository {
	return &memoryOptimizationRepository{
		runs:       map[string]OptimizationRun{},
		candidates: map[string]OptimizationCandidate{},
	}
}

func (r *memoryOptimizationRepository) CreateOptimizationRun(_ context.Context, run OptimizationRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}

func (r *memoryOptimizationRepository) GetOptimizationRun(_ context.Context, tenantID, runID string) (OptimizationRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID {
		return OptimizationRun{}, false, nil
	}
	return run, true, nil
}

func (r *memoryOptimizationRepository) UpdateOptimizationRun(_ context.Context, run OptimizationRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}

func (r *memoryOptimizationRepository) CreateOptimizationCandidate(_ context.Context, candidate OptimizationCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	candidate.Config.ID = candidate.ID
	r.candidates[candidate.ID] = candidate
	return nil
}

func (r *memoryOptimizationRepository) UpdateOptimizationCandidate(_ context.Context, candidate OptimizationCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.candidates[candidate.ID] = candidate
	return nil
}

func (r *memoryOptimizationRepository) ListOptimizationCandidates(_ context.Context, tenantID, runID string) ([]OptimizationCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID {
		return nil, nil
	}
	out := make([]OptimizationCandidate, 0, len(r.candidates))
	for _, candidate := range r.candidates {
		if candidate.OptimizationRunID == runID {
			out = append(out, candidate)
		}
	}
	sortCandidates(out)
	return out, nil
}

func (r *memoryOptimizationRepository) StoreHarnessRun(_ context.Context, run HarnessRunRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.harness = append(r.harness, run)
	return nil
}

type runnerCall struct {
	CandidateID  string
	Phase        string
	Split        string
	Profile      rag.Profile
	TopK         int
	NamespaceTTL time.Duration
}

type recordingCandidateRunner struct {
	mu           sync.Mutex
	calls        []runnerCall
	err          error
	blockOnFirst chan struct{}
	releaseFirst chan struct{}
	afterCall    func(CandidateRunRequest)
}

func (r *recordingCandidateRunner) RunCandidate(ctx context.Context, req CandidateRunRequest) (CandidateRunResult, error) {
	r.mu.Lock()
	if len(r.calls) == 0 && r.blockOnFirst != nil {
		close(r.blockOnFirst)
	}
	r.calls = append(r.calls, runnerCall{
		CandidateID:  req.Candidate.ID,
		Phase:        req.Phase,
		Split:        req.Split,
		Profile:      req.Profile,
		TopK:         req.TopK,
		NamespaceTTL: req.NamespaceTTL,
	})
	callIndex := len(r.calls)
	r.mu.Unlock()
	if callIndex == 1 && r.releaseFirst != nil {
		select {
		case <-ctx.Done():
			return CandidateRunResult{}, ctx.Err()
		case <-r.releaseFirst:
		}
	}
	if r.err != nil {
		return CandidateRunResult{}, r.err
	}
	if r.afterCall != nil {
		r.afterCall(req)
	}
	metrics := metricsForRequest(req)
	return CandidateRunResult{
		CandidateID:   req.Candidate.ID,
		EvaluationRun: eval.RunResult{ID: "eval_" + req.Candidate.ID + "_" + req.Phase, Metrics: metrics},
		Metrics:       metrics,
	}, nil
}

func metricsForRequest(req CandidateRunRequest) map[string]float64 {
	switch req.Split {
	case "holdout":
		if req.Candidate.Retrieval.DenseTopK == 2 {
			return map[string]float64{"pairwise_accuracy": 0.82, "cost_usd": 0.1}
		}
		return map[string]float64{"pairwise_accuracy": 0.5, "cost_usd": 0.1}
	default:
		if req.Candidate.Retrieval.DenseTopK == 2 {
			return map[string]float64{"pairwise_accuracy": 0.9, "cost_usd": 0.1}
		}
		return map[string]float64{"pairwise_accuracy": 0.7, "cost_usd": 0.1}
	}
}

func (r *recordingCandidateRunner) saw(candidateID, phase, split string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, call := range r.calls {
		if call.CandidateID == candidateID && call.Phase == phase && call.Split == split {
			return true
		}
	}
	return false
}

func (r *recordingCandidateRunner) sawConfigured(phase, split string, profile rag.Profile, topK int, ttl time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, call := range r.calls {
		if call.Phase == phase && call.Split == split && call.Profile == profile && call.TopK == topK && call.NamespaceTTL == ttl {
			return true
		}
	}
	return false
}

func (r *recordingCandidateRunner) selectionCount(candidateID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, call := range r.calls {
		if call.CandidateID == candidateID && call.Phase == PhaseSelection {
			count++
		}
	}
	return count
}

func (r *recordingCandidateRunner) selectionIDs() map[string]struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]struct{}{}
	for _, call := range r.calls {
		if call.Phase == PhaseSelection {
			out[call.CandidateID] = struct{}{}
		}
	}
	return out
}

func waitForRunStatus(t *testing.T, service *Service, runID string, want RunStatus) OptimizationStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, ok, err := service.Get(context.Background(), "tenant_a", runID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if ok && status.Run.Status == want {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, _, _ := service.Get(context.Background(), "tenant_a", runID)
	t.Fatalf("run status = %q, want %q", status.Run.Status, want)
	return OptimizationStatus{}
}

func candidateByID(candidates []OptimizationCandidate, id string) OptimizationCandidate {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate
		}
	}
	return OptimizationCandidate{}
}

func candidateWithDenseTopK(candidates []OptimizationCandidate, topK int) OptimizationCandidate {
	for _, candidate := range candidates {
		if candidate.Config.Retrieval.DenseTopK == topK {
			return candidate
		}
	}
	return OptimizationCandidate{}
}

func sortCandidates(candidates []OptimizationCandidate) {
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].ID < candidates[i].ID {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
}
