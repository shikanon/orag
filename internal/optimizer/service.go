package optimizer

import (
	"context"
	"errors"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

const (
	PhaseSelection = "selection"
	PhaseHoldout   = "holdout"
)

var ErrOptimizationNotFound = errors.New("optimization run not found")

type Repository interface {
	CreateOptimizationRun(ctx context.Context, run OptimizationRun) error
	GetOptimizationRun(ctx context.Context, tenantID, runID string) (OptimizationRun, bool, error)
	UpdateOptimizationRun(ctx context.Context, run OptimizationRun) error
	CreateOptimizationCandidate(ctx context.Context, candidate OptimizationCandidate) error
	UpdateOptimizationCandidate(ctx context.Context, candidate OptimizationCandidate) error
	ListOptimizationCandidates(ctx context.Context, tenantID, runID string) ([]OptimizationCandidate, error)
	StoreHarnessRun(ctx context.Context, run HarnessRunRecord) error
}

type OptimizationRun struct {
	ID                      string
	TenantID                string
	DatasetID               string
	KnowledgeBaseID         string
	Objective               ObjectiveSpec
	SearchSpace             SearchSpace
	Runner                  map[string]any
	Status                  RunStatus
	StatusReason            string
	BestCandidateID         string
	HoldoutCandidateID      string
	SamplingStrategy        SearchStrategy
	SearchSpaceSize         int64
	SampledCandidateCount   int
	CompletedCandidateCount int
	Checkpoint              Checkpoint
	TokenUsage              TokenUsage
	CostUSD                 float64
	CostBudgetUSD           *float64
	CancelRequestedAt       *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type OptimizationCandidate struct {
	ID                string
	OptimizationRunID string
	Config            CandidateConfig
	Status            CandidateStatus
	EvaluationRunID   string
	JudgeRunID        string
	ObjectiveScore    float64
	HoldoutScore      *float64
	Confidence        map[string]float64
	Metrics           map[string]float64
	TokenUsage        TokenUsage
	CostUSD           float64
	Artifacts         map[string]any
	TempNamespaces    []TempNamespace
	CleanupStatus     CleanupStatus
	ExpiresAt         *time.Time
	Error             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type HarnessRunRecord struct {
	ID             string
	TenantID       string
	CandidateID    string
	HarnessType    string
	Argv           []string
	WorkingDir     string
	EnvRedacted    map[string]string
	StdoutRedacted string
	StderrRedacted string
	ParsedMetrics  map[string]float64
	ExitCode       int
	Metrics        map[string]float64
	Artifacts      map[string]any
	StartedAt      time.Time
	EndedAt        *time.Time
}

type SubmitRequest struct {
	TenantID        string
	DatasetID       string
	KnowledgeBaseID string
	Objective       ObjectiveSpec
	SearchSpace     SearchSpace
	Search          SearchSpec
	Budget          Budget
	Profile         rag.Profile
	TopK            int
	NamespaceTTL    time.Duration
	SelectionSplit  string
	HoldoutSplit    string
	Runner          map[string]any
}

type OptimizationStatus struct {
	Run        OptimizationRun
	Candidates []OptimizationCandidate
}

type Service struct {
	Repository       Repository
	Runner           CandidateRunner
	RateLimiter      RateLimiter
	Now              func() time.Time
	DisableAutoStart bool
}

func (s *Service) Submit(ctx context.Context, req SubmitRequest) (OptimizationRun, error) {
	now := s.clock()
	runID := id.New("opt")
	searchSpec := req.Search
	searchSpec.RunID = runID
	search, err := GenerateCandidates(req.SearchSpace, searchSpec)
	if err != nil {
		return OptimizationRun{}, err
	}
	costBudget := costBudget(req)
	run := OptimizationRun{
		ID:                      runID,
		TenantID:                req.TenantID,
		DatasetID:               req.DatasetID,
		KnowledgeBaseID:         req.KnowledgeBaseID,
		Objective:               req.Objective,
		SearchSpace:             req.SearchSpace,
		Runner:                  req.Runner,
		Status:                  RunStatusQueued,
		SamplingStrategy:        search.Strategy,
		SearchSpaceSize:         search.SearchSpaceSize,
		SampledCandidateCount:   len(search.Candidates),
		CompletedCandidateCount: 0,
		Checkpoint:              Checkpoint{Stage: "submitted"},
		CostBudgetUSD:           costBudget,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := s.repo().CreateOptimizationRun(ctx, run); err != nil {
		return OptimizationRun{}, err
	}
	for _, config := range search.Candidates {
		candidate := OptimizationCandidate{
			ID:                config.ID,
			OptimizationRunID: run.ID,
			Config:            config,
			Status:            CandidateStatusQueued,
			Metrics:           map[string]float64{},
			Confidence:        map[string]float64{},
			Artifacts:         map[string]any{},
			CleanupStatus:     CleanupNotRequired,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := s.repo().CreateOptimizationCandidate(ctx, candidate); err != nil {
			return OptimizationRun{}, err
		}
	}
	if !s.DisableAutoStart {
		go s.run(context.Background(), run.ID, req)
	}
	return run, nil
}

func (s *Service) Get(ctx context.Context, tenantID, runID string) (OptimizationStatus, bool, error) {
	run, ok, err := s.repo().GetOptimizationRun(ctx, tenantID, runID)
	if err != nil || !ok {
		return OptimizationStatus{}, ok, err
	}
	candidates, err := s.repo().ListOptimizationCandidates(ctx, tenantID, runID)
	if err != nil {
		return OptimizationStatus{}, false, err
	}
	return OptimizationStatus{Run: run, Candidates: candidates}, true, nil
}

func (s *Service) Cancel(ctx context.Context, tenantID, runID, reason string) (OptimizationRun, error) {
	run, ok, err := s.repo().GetOptimizationRun(ctx, tenantID, runID)
	if err != nil {
		return OptimizationRun{}, err
	}
	if !ok {
		return OptimizationRun{}, ErrOptimizationNotFound
	}
	now := s.clock()
	run.Status = RunStatusCanceling
	run.StatusReason = reason
	run.CancelRequestedAt = &now
	run.Checkpoint.CancelRequestedAt = &now
	run.Checkpoint.StatusReason = reason
	run.UpdatedAt = now
	return run, s.repo().UpdateOptimizationRun(ctx, run)
}

func (s *Service) Resume(ctx context.Context, tenantID, runID string, req SubmitRequest) (OptimizationRun, error) {
	run, ok, err := s.repo().GetOptimizationRun(ctx, tenantID, runID)
	if err != nil {
		return OptimizationRun{}, err
	}
	if !ok {
		return OptimizationRun{}, ErrOptimizationNotFound
	}
	now := s.clock()
	run.Status = RunStatusQueued
	run.StatusReason = ""
	run.CancelRequestedAt = nil
	run.Checkpoint.CancelRequestedAt = nil
	run.Checkpoint.StatusReason = ""
	run.UpdatedAt = now
	if err := s.repo().UpdateOptimizationRun(ctx, run); err != nil {
		return OptimizationRun{}, err
	}
	if !s.DisableAutoStart {
		go s.run(context.Background(), run.ID, req)
	}
	return run, nil
}

func (s *Service) RunPending(ctx context.Context, tenantID, runID string, req SubmitRequest) error {
	return s.run(ctx, runID, req)
}

func (s *Service) run(ctx context.Context, runID string, req SubmitRequest) error {
	run, ok, err := s.repo().GetOptimizationRun(ctx, req.TenantID, runID)
	if err != nil || !ok {
		return err
	}
	started := s.clock()
	if err := s.transitionRun(ctx, &run, RunStatusRunning, "running", ""); err != nil {
		return err
	}
	candidates, err := s.repo().ListOptimizationCandidates(ctx, req.TenantID, run.ID)
	if err != nil {
		return err
	}
	completed := run.Checkpoint.completedSet()
	for i := range candidates {
		candidate := candidates[i]
		if _, ok := completed[candidate.ID]; ok || isCandidateComplete(candidate.Status) {
			continue
		}
		if stop, err := s.shouldStop(ctx, &run, req, started); err != nil || stop {
			return err
		}
		if err := s.runCandidate(ctx, &run, &candidate, req, PhaseSelection, selectionSplit(req)); err != nil {
			run.Checkpoint.markFailed(candidate.ID)
			return s.failRun(ctx, &run, err)
		}
	}
	if run.Status == RunStatusCanceled || run.Status == RunStatusBudgetStopped {
		return nil
	}
	if err := s.scoreAndPromote(ctx, &run, req); err != nil {
		return s.failRun(ctx, &run, err)
	}
	if req.HoldoutSplit != "" && run.BestCandidateID != "" {
		if err := s.runHoldout(ctx, &run, req); err != nil {
			return s.failRun(ctx, &run, err)
		}
	}
	if err := s.cleanupPendingCandidates(ctx, &run); err != nil {
		return s.failRun(ctx, &run, err)
	}
	if run.Status != RunStatusBudgetStopped && run.Status != RunStatusCanceled {
		return s.transitionRun(ctx, &run, RunStatusCompleted, "completed", "")
	}
	return nil
}

func (s *Service) runCandidate(ctx context.Context, run *OptimizationRun, candidate *OptimizationCandidate, req SubmitRequest, phase, split string) error {
	if err := s.transitionCandidate(ctx, candidate, CandidateStatusRunning, nil); err != nil {
		return err
	}
	if err := s.limiter().Wait(ctx); err != nil {
		return err
	}
	result, err := s.Runner.RunCandidate(ctx, CandidateRunRequest{
		TenantID:        run.TenantID,
		DatasetID:       run.DatasetID,
		KnowledgeBaseID: run.KnowledgeBaseID,
		Candidate:       candidate.Config,
		Profile:         req.Profile,
		TopK:            req.TopK,
		NamespaceTTL:    req.NamespaceTTL,
		Phase:           phase,
		Split:           split,
	})
	if err != nil {
		candidate.Error = err.Error()
		_ = s.transitionCandidate(ctx, candidate, CandidateStatusFailed, nil)
		return err
	}
	candidate.EvaluationRunID = result.EvaluationRun.ID
	candidate.Metrics = cloneMetrics(result.Metrics)
	if candidate.Metrics == nil {
		candidate.Metrics = map[string]float64{}
	}
	candidate.CostUSD = candidate.Metrics["cost_usd"]
	candidate.TempNamespaces = result.TempNamespaces
	candidate.CleanupStatus = result.CleanupStatus
	if len(candidate.TempNamespaces) > 0 {
		expiresAt := candidate.TempNamespaces[0].ExpiresAt
		candidate.ExpiresAt = &expiresAt
	}
	if err := s.transitionCandidate(ctx, candidate, CandidateStatusEvaluated, nil); err != nil {
		return err
	}
	if err := s.transitionCandidate(ctx, candidate, CandidateStatusJudged, nil); err != nil {
		return err
	}
	run.CostUSD += candidate.CostUSD
	run.Checkpoint.CostUSD = run.CostUSD
	run.Checkpoint.JudgeCalls++
	run.Checkpoint.TempNamespaces = append(run.Checkpoint.TempNamespaces, candidate.TempNamespaces...)
	if phase == PhaseSelection {
		run.CompletedCandidateCount++
		run.Checkpoint.markCompleted(candidate.ID)
	}
	run.UpdatedAt = s.clock()
	if err := s.repo().UpdateOptimizationRun(ctx, *run); err != nil {
		return err
	}
	return nil
}

func (s *Service) scoreAndPromote(ctx context.Context, run *OptimizationRun, req SubmitRequest) error {
	candidates, err := s.repo().ListOptimizationCandidates(ctx, run.TenantID, run.ID)
	if err != nil {
		return err
	}
	inputs := make([]CandidateInput, 0, len(candidates))
	byID := map[string]OptimizationCandidate{}
	for _, candidate := range candidates {
		if candidate.Status == CandidateStatusFailed || candidate.Status == CandidateStatusQueued || candidate.Status == CandidateStatusRunning {
			continue
		}
		inputs = append(inputs, CandidateInput{ID: candidate.ID, Metrics: candidate.Metrics, CreatedAt: candidate.CreatedAt})
		byID[candidate.ID] = candidate
	}
	result, err := EvaluateObjective(req.Objective, inputs)
	if err != nil {
		return err
	}
	for _, score := range result.Candidates {
		candidate := byID[score.ID]
		candidate.ObjectiveScore = score.Score
		candidate.Confidence = map[string]float64{"pairwise_win_rate": score.PairwiseWinRate}
		status := CandidateStatusScored
		if score.ID == result.Best.ID {
			status = CandidateStatusPromoted
			run.BestCandidateID = score.ID
			run.Checkpoint.BestCandidateID = score.ID
		}
		if err := s.transitionCandidate(ctx, &candidate, status, score.Metrics); err != nil {
			return err
		}
	}
	run.UpdatedAt = s.clock()
	return s.repo().UpdateOptimizationRun(ctx, *run)
}

func (s *Service) runHoldout(ctx context.Context, run *OptimizationRun, req SubmitRequest) error {
	candidates, err := s.repo().ListOptimizationCandidates(ctx, run.TenantID, run.ID)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if candidate.ID != run.BestCandidateID {
			continue
		}
		if err := s.runCandidate(ctx, run, &candidate, req, PhaseHoldout, req.HoldoutSplit); err != nil {
			return err
		}
		score := candidate.Metrics[req.Objective.Maximize]
		if req.Objective.Maximize == "" {
			score = candidate.Metrics["pairwise_accuracy"]
		}
		candidate.HoldoutScore = &score
		run.HoldoutCandidateID = candidate.ID
		run.Checkpoint.HoldoutCandidateID = candidate.ID
		if err := s.transitionCandidate(ctx, &candidate, CandidateStatusHoldoutEvaluated, nil); err != nil {
			return err
		}
		if err := s.cleanupCandidate(ctx, &candidate); err != nil {
			return err
		}
		run.UpdatedAt = s.clock()
		return s.repo().UpdateOptimizationRun(ctx, *run)
	}
	return nil
}

func (s *Service) cleanupCandidate(ctx context.Context, candidate *OptimizationCandidate) error {
	if candidate.CleanupStatus != CleanupPending {
		if candidate.CleanupStatus == "" {
			candidate.CleanupStatus = CleanupNotRequired
		}
		return nil
	}
	cleaner, ok := s.Runner.(interface {
		CleanupCandidateNamespaces(context.Context, string) ([]TempNamespace, error)
	})
	if ok {
		namespaces, err := cleaner.CleanupCandidateNamespaces(ctx, candidate.ID)
		if err != nil {
			candidate.CleanupStatus = CleanupFailed
			candidate.TempNamespaces = namespaces
			_ = s.repo().UpdateOptimizationCandidate(ctx, *candidate)
			return err
		}
		candidate.TempNamespaces = namespaces
	}
	candidate.CleanupStatus = CleanupDone
	return s.transitionCandidate(ctx, candidate, CandidateStatusCleanupDone, nil)
}

func (s *Service) cleanupPendingCandidates(ctx context.Context, run *OptimizationRun) error {
	candidates, err := s.repo().ListOptimizationCandidates(ctx, run.TenantID, run.ID)
	if err != nil {
		return err
	}
	for i := range candidates {
		if candidates[i].CleanupStatus != CleanupPending {
			continue
		}
		if err := s.cleanupCandidate(ctx, &candidates[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) shouldStop(ctx context.Context, run *OptimizationRun, req SubmitRequest, started time.Time) (bool, error) {
	current, ok, err := s.repo().GetOptimizationRun(ctx, run.TenantID, run.ID)
	if err != nil || !ok {
		return false, err
	}
	*run = current
	if run.CancelRequestedAt != nil || run.Status == RunStatusCanceling {
		return true, s.transitionRun(ctx, run, RunStatusCanceled, "canceled", run.StatusReason)
	}
	if req.Budget.MaxJudgeCalls > 0 && run.Checkpoint.JudgeCalls >= req.Budget.MaxJudgeCalls {
		return true, s.transitionRun(ctx, run, RunStatusBudgetStopped, "budget_stopped", "max judge calls reached")
	}
	if req.Budget.MaxCostUSD > 0 && run.CostUSD >= req.Budget.MaxCostUSD {
		return true, s.transitionRun(ctx, run, RunStatusBudgetStopped, "budget_stopped", "max cost reached")
	}
	if wall := req.Budget.wallTime(); wall > 0 && s.clock().Sub(started) >= wall {
		return true, s.transitionRun(ctx, run, RunStatusBudgetStopped, "budget_stopped", "max wall time reached")
	}
	return false, nil
}

func (s *Service) transitionRun(ctx context.Context, run *OptimizationRun, status RunStatus, stage, reason string) error {
	run.Status = status
	run.StatusReason = reason
	run.Checkpoint.Stage = stage
	run.Checkpoint.StatusReason = reason
	run.UpdatedAt = s.clock()
	return s.repo().UpdateOptimizationRun(ctx, *run)
}

func (s *Service) transitionCandidate(ctx context.Context, candidate *OptimizationCandidate, status CandidateStatus, metrics map[string]float64) error {
	candidate.Status = status
	if metrics != nil {
		candidate.Metrics = cloneMetrics(metrics)
	}
	candidate.UpdatedAt = s.clock()
	return s.repo().UpdateOptimizationCandidate(ctx, *candidate)
}

func (s *Service) failRun(ctx context.Context, run *OptimizationRun, err error) error {
	if err == nil {
		return nil
	}
	_ = s.transitionRun(ctx, run, RunStatusFailed, "failed", err.Error())
	return err
}

func (s *Service) repo() Repository {
	if s.Repository == nil {
		panic("optimizer repository is required")
	}
	return s.Repository
}

func (s *Service) limiter() RateLimiter {
	if s.RateLimiter == nil {
		return NoopRateLimiter{}
	}
	return s.RateLimiter
}

func (s *Service) clock() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func isCandidateComplete(status CandidateStatus) bool {
	return status == CandidateStatusScored ||
		status == CandidateStatusPromoted ||
		status == CandidateStatusHoldoutEvaluated ||
		status == CandidateStatusCleanupDone
}

func selectionSplit(req SubmitRequest) string {
	if req.SelectionSplit != "" {
		return req.SelectionSplit
	}
	return "eval"
}

func costBudget(req SubmitRequest) *float64 {
	if req.Budget.MaxCostUSD > 0 {
		v := req.Budget.MaxCostUSD
		return &v
	}
	if req.Objective.Budget.CostLimitUSD > 0 {
		v := req.Objective.Budget.CostLimitUSD
		return &v
	}
	return nil
}
