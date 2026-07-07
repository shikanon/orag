package optimizer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/platform/apperrors"
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

func TestServiceRejectsConfigChangingResume(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SubmitRequest)
	}{
		{
			name: "dataset_id",
			mutate: func(req *SubmitRequest) {
				req.DatasetID = "ds_2"
			},
		},
		{
			name: "knowledge_base_id",
			mutate: func(req *SubmitRequest) {
				req.KnowledgeBaseID = "kb_2"
			},
		},
		{
			name: "objective",
			mutate: func(req *SubmitRequest) {
				req.Objective = ObjectiveSpec{Maximize: "faithfulness"}
			},
		},
		{
			name: "search_space",
			mutate: func(req *SubmitRequest) {
				req.SearchSpace.Retrieval.DenseTopK = []int{1, 3}
			},
		},
		{
			name: "search",
			mutate: func(req *SubmitRequest) {
				req.Search.MaxCandidates = 1
			},
		},
		{
			name: "profile",
			mutate: func(req *SubmitRequest) {
				req.Profile = rag.ProfileHighPrecision
			},
		},
		{
			name: "top_k",
			mutate: func(req *SubmitRequest) {
				req.TopK = 5
			},
		},
		{
			name: "namespace_ttl",
			mutate: func(req *SubmitRequest) {
				req.NamespaceTTL = time.Minute
			},
		},
		{
			name: "selection_split",
			mutate: func(req *SubmitRequest) {
				req.SelectionSplit = "dev"
			},
		},
		{
			name: "holdout_split",
			mutate: func(req *SubmitRequest) {
				req.HoldoutSplit = "test"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMemoryOptimizationRepository()
			service := &Service{Repository: repo, Runner: &recordingCandidateRunner{}, DisableAutoStart: true}
			req := basicSubmitRequest()
			req.Profile = rag.ProfileRealtime
			req.TopK = 3
			req.NamespaceTTL = 30 * time.Second

			run, err := service.Submit(context.Background(), req)
			if err != nil {
				t.Fatalf("Submit() error = %v", err)
			}
			resumeReq := req
			tt.mutate(&resumeReq)

			_, err = service.Resume(context.Background(), "tenant_a", run.ID, resumeReq)
			if !apperrors.IsCode(err, apperrors.CodeValidation) {
				t.Fatalf("Resume() error = %v, want validation error", err)
			}
		})
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

func TestServiceCancelDuringLastCandidateWinsOverCompleted(t *testing.T) {
	repo := newMemoryOptimizationRepository()
	var service *Service
	var runID string
	runner := &recordingCandidateRunner{
		afterCall: func(CandidateRunRequest) {
			if _, err := service.Cancel(context.Background(), "tenant_a", runID, "stop after current candidate"); err != nil {
				t.Fatalf("Cancel() error = %v", err)
			}
		},
	}
	service = &Service{Repository: repo, Runner: runner, DisableAutoStart: true}
	req := basicSubmitRequest()
	req.HoldoutSplit = ""
	req.SearchSpace = SearchSpace{Retrieval: RetrievalSpace{DenseTopK: []int{1}}}
	req.Search = SearchSpec{Strategy: SearchStrategyGrid, MaxCandidates: 1}

	run, err := service.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	runID = run.ID
	if err := service.RunPending(context.Background(), "tenant_a", run.ID, req); err != nil {
		t.Fatalf("RunPending() error = %v", err)
	}

	status, _, _ := service.Get(context.Background(), "tenant_a", run.ID)
	if status.Run.Status != RunStatusCanceled || status.Run.Checkpoint.CancelRequestedAt == nil {
		t.Fatalf("run = %#v, want canceled after in-flight candidate finishes", status.Run)
	}
	if status.Run.BestCandidateID != "" {
		t.Fatalf("best candidate = %q, want no scoring after cancellation", status.Run.BestCandidateID)
	}
}

func TestServiceCostBudgetExceededAfterCandidateStopsRun(t *testing.T) {
	repo := newMemoryOptimizationRepository()
	runner := &recordingCandidateRunner{}
	service := &Service{Repository: repo, Runner: runner, DisableAutoStart: true}
	req := basicSubmitRequest()
	req.HoldoutSplit = ""
	req.SearchSpace = SearchSpace{Retrieval: RetrievalSpace{DenseTopK: []int{1}}}
	req.Search = SearchSpec{Strategy: SearchStrategyGrid, MaxCandidates: 1}
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
		t.Fatalf("status = %q, want budget_stopped after final cost check", status.Run.Status)
	}
	if status.Run.BestCandidateID != "" {
		t.Fatalf("best candidate = %q, want no scoring after budget stop", status.Run.BestCandidateID)
	}
	if status.Run.CostUSD <= req.Budget.MaxCostUSD {
		t.Fatalf("cost = %v, want over budget %v", status.Run.CostUSD, req.Budget.MaxCostUSD)
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

func TestServiceSubmitDoesNotPersistPartialRunWhenAtomicCreateFails(t *testing.T) {
	want := errors.New("atomic create failed")
	repo := &failingAtomicCreateRepository{
		memoryOptimizationRepository: newMemoryOptimizationRepository(),
		err:                          want,
	}
	runner := &recordingCandidateRunner{}
	service := &Service{Repository: repo, Runner: runner}

	_, err := service.Submit(context.Background(), basicSubmitRequest())
	if !errors.Is(err, want) {
		t.Fatalf("Submit() error = %v, want %v", err, want)
	}
	if repo.atomicCalls != 1 {
		t.Fatalf("atomic create calls = %d, want 1", repo.atomicCalls)
	}
	if repo.createRunCalls != 0 || repo.createCandidateCalls != 0 {
		t.Fatalf("legacy create calls = run %d candidate %d, want 0/0", repo.createRunCalls, repo.createCandidateCalls)
	}
	if repo.atomicRun.ID == "" {
		t.Fatal("atomic create did not receive a run")
	}
	if repo.atomicRun.Status != RunStatusQueued || repo.atomicRun.Checkpoint.Stage != "submitted" {
		t.Fatalf("atomic run = %#v, want queued submitted run", repo.atomicRun)
	}
	if len(repo.atomicCandidates) != 2 {
		t.Fatalf("atomic candidates = %d, want 2: %#v", len(repo.atomicCandidates), repo.atomicCandidates)
	}
	for _, candidate := range repo.atomicCandidates {
		if candidate.OptimizationRunID != repo.atomicRun.ID {
			t.Fatalf("candidate run ID = %q, want %q", candidate.OptimizationRunID, repo.atomicRun.ID)
		}
		if candidate.Status != CandidateStatusQueued {
			t.Fatalf("candidate status = %q, want queued", candidate.Status)
		}
		if candidate.Config.ID != candidate.ID {
			t.Fatalf("candidate config ID = %q, want %q", candidate.Config.ID, candidate.ID)
		}
	}
	if _, ok, err := repo.GetOptimizationRun(context.Background(), "tenant_a", repo.atomicRun.ID); err != nil || ok {
		t.Fatalf("GetOptimizationRun() = ok %v error %v, want not found", ok, err)
	}
	candidates, err := repo.ListOptimizationCandidates(context.Background(), "tenant_a", repo.atomicRun.ID)
	if err != nil {
		t.Fatalf("ListOptimizationCandidates() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("stored candidates = %#v, want none", candidates)
	}
	if got := len(runner.selectionIDs()); got != 0 {
		t.Fatalf("runner selection calls = %d, want 0", got)
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

func TestMemoryRepositoryCreateOptimizationRunWithCandidates(t *testing.T) {
	repo := NewMemoryRepository()
	now := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	run := OptimizationRun{
		ID:        "opt_1",
		TenantID:  "tenant_a",
		Status:    RunStatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	candidates := []OptimizationCandidate{
		{
			ID:                "cand_b",
			OptimizationRunID: "opt_1",
			Config:            CandidateConfig{ID: "stale", Retrieval: RetrievalCandidate{DenseTopK: 2}},
			Status:            CandidateStatusQueued,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
		{
			ID:                "cand_a",
			OptimizationRunID: "opt_1",
			Config:            CandidateConfig{Retrieval: RetrievalCandidate{DenseTopK: 1}},
			Status:            CandidateStatusQueued,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}

	if err := repo.CreateOptimizationRunWithCandidates(context.Background(), run, candidates); err != nil {
		t.Fatalf("CreateOptimizationRunWithCandidates() error = %v", err)
	}
	gotRun, ok, err := repo.GetOptimizationRun(context.Background(), "tenant_a", "opt_1")
	if err != nil || !ok {
		t.Fatalf("GetOptimizationRun() = ok %v error %v", ok, err)
	}
	if gotRun.ID != run.ID || gotRun.Status != run.Status {
		t.Fatalf("run = %#v, want %#v", gotRun, run)
	}
	gotCandidates, err := repo.ListOptimizationCandidates(context.Background(), "tenant_a", "opt_1")
	if err != nil {
		t.Fatalf("ListOptimizationCandidates() error = %v", err)
	}
	if len(gotCandidates) != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", len(gotCandidates), gotCandidates)
	}
	if gotCandidates[0].ID != "cand_a" || gotCandidates[1].ID != "cand_b" {
		t.Fatalf("candidate order = %#v, want cand_a then cand_b", gotCandidates)
	}
	for _, candidate := range gotCandidates {
		if candidate.Config.ID != candidate.ID {
			t.Fatalf("candidate config ID = %q, want %q for %#v", candidate.Config.ID, candidate.ID, candidate)
		}
		if candidate.OptimizationRunID != run.ID {
			t.Fatalf("candidate run ID = %q, want %q", candidate.OptimizationRunID, run.ID)
		}
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

func (r *memoryOptimizationRepository) CreateOptimizationRunWithCandidates(_ context.Context, run OptimizationRun, candidates []OptimizationCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copies := make([]OptimizationCandidate, len(candidates))
	for i, candidate := range candidates {
		candidate.Config.ID = candidate.ID
		copies[i] = candidate
	}
	r.runs[run.ID] = run
	for _, candidate := range copies {
		r.candidates[candidate.ID] = candidate
	}
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

type failingAtomicCreateRepository struct {
	*memoryOptimizationRepository
	err                  error
	atomicCalls          int
	createRunCalls       int
	createCandidateCalls int
	atomicRun            OptimizationRun
	atomicCandidates     []OptimizationCandidate
}

func (r *failingAtomicCreateRepository) CreateOptimizationRun(ctx context.Context, run OptimizationRun) error {
	r.createRunCalls++
	return r.memoryOptimizationRepository.CreateOptimizationRun(ctx, run)
}

func (r *failingAtomicCreateRepository) CreateOptimizationRunWithCandidates(_ context.Context, run OptimizationRun, candidates []OptimizationCandidate) error {
	r.atomicCalls++
	r.atomicRun = run
	r.atomicCandidates = append([]OptimizationCandidate(nil), candidates...)
	return r.err
}

func (r *failingAtomicCreateRepository) CreateOptimizationCandidate(ctx context.Context, candidate OptimizationCandidate) error {
	r.createCandidateCalls++
	return r.memoryOptimizationRepository.CreateOptimizationCandidate(ctx, candidate)
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
