package optimizer

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/platform/apperrors"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

const (
	PhaseSelection = "selection"
	PhaseHoldout   = "holdout"
)

var (
	ErrOptimizationNotFound      = errors.New("optimization run not found")
	ErrOptimizationStateConflict = errors.New("optimization state conflict")
)

type Repository interface {
	CreateOptimizationRun(ctx context.Context, run OptimizationRun) error
	CreateOptimizationRunWithCandidates(ctx context.Context, run OptimizationRun, candidates []OptimizationCandidate) error
	GetOptimizationRun(ctx context.Context, tenantID, runID string) (OptimizationRun, bool, error)
	UpdateOptimizationRun(ctx context.Context, run OptimizationRun) error
	CompareAndSwapOptimizationRun(ctx context.Context, run OptimizationRun, expectedStatus RunStatus) (bool, error)
	CreateOptimizationCandidate(ctx context.Context, candidate OptimizationCandidate) error
	UpdateOptimizationCandidate(ctx context.Context, candidate OptimizationCandidate) error
	CompareAndSwapOptimizationCandidate(ctx context.Context, candidate OptimizationCandidate, expectedStatus CandidateStatus) (bool, error)
	ListOptimizationCandidates(ctx context.Context, tenantID, runID string) ([]OptimizationCandidate, error)
	StoreHarnessRun(ctx context.Context, run HarnessRunRecord) error
}

type ProjectRepository interface {
	GetOptimizationRunInProject(ctx context.Context, tenantID, projectID, runID string) (OptimizationRun, bool, error)
}

type OptimizationRun struct {
	ID                      string
	TenantID                string
	ProjectID               string
	DatasetID               string
	KnowledgeBaseID         string
	Objective               ObjectiveSpec
	SearchSpace             SearchSpace
	Config                  RunConfig
	Runner                  map[string]any
	Status                  RunStatus
	StatusReason            string
	BestCandidateID         string
	HoldoutCandidateID      string
	HoldoutGate             eval.HoldoutGateResult
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
	HoldoutGate       eval.HoldoutGateResult
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
	ProjectID       string
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
	HoldoutGate     eval.HoldoutGateConfig
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
		ProjectID:               req.ProjectID,
		DatasetID:               req.DatasetID,
		KnowledgeBaseID:         req.KnowledgeBaseID,
		Objective:               req.Objective,
		SearchSpace:             req.SearchSpace,
		Config:                  RunConfigFromSubmitRequest(req),
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
	candidates := make([]OptimizationCandidate, 0, len(search.Candidates))
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
		candidates = append(candidates, candidate)
	}
	if err := s.repo().CreateOptimizationRunWithCandidates(ctx, run, candidates); err != nil {
		return OptimizationRun{}, err
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

func (s *Service) GetInProject(ctx context.Context, tenantID, projectID, runID string) (OptimizationStatus, bool, error) {
	repository, ok := s.repo().(ProjectRepository)
	if !ok {
		status, found, err := s.Get(ctx, tenantID, runID)
		return status, found && status.Run.ProjectID == projectID, err
	}
	run, found, err := repository.GetOptimizationRunInProject(ctx, tenantID, projectID, runID)
	if err != nil || !found {
		return OptimizationStatus{}, found, err
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
	resumeReq := req
	if isEmptySubmitRequest(resumeReq) {
		resumeReq = run.StoredSubmitRequest()
	}
	if resumeReq.TenantID == "" {
		resumeReq.TenantID = tenantID
	}
	if resumeReq.ProjectID == "" {
		resumeReq.ProjectID = run.ProjectID
	}
	if err := validateResumeConfigInvariant(run, resumeReq); err != nil {
		return OptimizationRun{}, err
	}
	expectedStatus := run.Status
	if !isResumableRunStatus(expectedStatus) {
		return OptimizationRun{}, stateConflict("optimization run " + run.ID + " cannot resume from status " + string(expectedStatus))
	}
	now := s.clock()
	run.DatasetID = resumeReq.DatasetID
	run.ProjectID = resumeReq.ProjectID
	run.KnowledgeBaseID = resumeReq.KnowledgeBaseID
	run.Objective = resumeReq.Objective
	run.SearchSpace = resumeReq.SearchSpace
	run.Config = RunConfigFromSubmitRequest(resumeReq)
	run.Runner = resumeReq.Runner
	run.Status = RunStatusQueued
	run.StatusReason = ""
	run.CancelRequestedAt = nil
	run.Checkpoint.CancelRequestedAt = nil
	run.Checkpoint.StatusReason = ""
	run.UpdatedAt = now
	swapped, err := s.repo().CompareAndSwapOptimizationRun(ctx, run, expectedStatus)
	if err != nil {
		return OptimizationRun{}, err
	}
	if !swapped {
		return OptimizationRun{}, stateConflict("optimization run " + run.ID + " changed while resuming")
	}
	if !s.DisableAutoStart {
		go s.run(context.Background(), run.ID, resumeReq)
	}
	return run, nil
}

func (s *Service) RunPending(ctx context.Context, tenantID, runID string, req SubmitRequest) error {
	if isEmptySubmitRequest(req) {
		run, ok, err := s.repo().GetOptimizationRun(ctx, tenantID, runID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrOptimizationNotFound
		}
		req = run.StoredSubmitRequest()
	}
	if req.TenantID == "" {
		req.TenantID = tenantID
	}
	return s.run(ctx, runID, req)
}

func (s *Service) run(ctx context.Context, runID string, req SubmitRequest) error {
	run, ok, err := s.repo().GetOptimizationRun(ctx, req.TenantID, runID)
	if err != nil || !ok {
		return err
	}
	if run.Status == RunStatusCanceling || run.CancelRequestedAt != nil {
		return s.claimRun(ctx, &run, run.Status, RunStatusCanceled, "canceled", run.StatusReason)
	}
	if run.Status != RunStatusQueued {
		return stateConflict("optimization run " + run.ID + " cannot start from status " + string(run.Status))
	}
	started := s.clock()
	if err := s.claimRun(ctx, &run, RunStatusQueued, RunStatusRunning, "running", ""); err != nil {
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
		if stop, err := s.shouldStop(ctx, &run, req, started, true); err != nil || stop {
			return err
		}
		if err := s.runCandidate(ctx, &run, &candidate, req, PhaseSelection, selectionSplit(req)); err != nil {
			run.Checkpoint.markFailed(candidate.ID)
			return s.failRun(ctx, &run, err)
		}
		if stop, err := s.shouldStop(ctx, &run, req, started, false); err != nil || stop {
			return err
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
	if err := s.claimCandidate(ctx, candidate, phase); err != nil {
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
		HoldoutGate:     holdoutGateForPhase(req, phase),
	})
	if err != nil {
		candidate.Error = err.Error()
		_ = s.transitionCandidate(ctx, candidate, CandidateStatusFailed, nil)
		return err
	}
	candidate.EvaluationRunID = result.EvaluationRun.ID
	candidate.Metrics = cloneMetrics(result.Metrics)
	candidate.HoldoutGate = result.HoldoutGate
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
	if err := s.mergeCurrentRunControlState(ctx, run); err != nil {
		return err
	}
	if err := s.repo().UpdateOptimizationRun(ctx, *run); err != nil {
		return err
	}
	return nil
}

func (s *Service) scoreAndPromote(ctx context.Context, run *OptimizationRun, req SubmitRequest) error {
	if req.HoldoutGate.Enabled && strings.TrimSpace(req.HoldoutSplit) == "" {
		return apperrors.New(apperrors.CodeValidation, "holdout gate failed: missing_split")
	}
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
		candidate.Confidence = map[string]float64{}
		if score.HasPairwiseOutcomes {
			candidate.Confidence["pairwise_win_rate"] = score.PairwiseWinRate
		}
		status := CandidateStatusScored
		if score.ID == result.Best.ID {
			run.BestCandidateID = score.ID
			run.Checkpoint.BestCandidateID = score.ID
			if req.HoldoutSplit == "" {
				status = CandidateStatusPromoted
			}
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
		scoreMetric := strings.TrimSpace(req.Objective.Maximize)
		if scoreMetric == "" {
			scoreMetric = defaultObjectiveMetric([]CandidateInput{{ID: candidate.ID, Metrics: candidate.Metrics}})
		}
		score := candidate.Metrics[scoreMetric]
		candidate.HoldoutScore = &score
		candidate.HoldoutGate = holdoutGateFromCandidate(candidate, req)
		if !candidate.HoldoutGate.Passed {
			run.HoldoutGate = candidate.HoldoutGate
			run.Checkpoint.StatusReason = holdoutGateFailureMessage(candidate.HoldoutGate)
			_ = s.repo().UpdateOptimizationRun(ctx, *run)
			return apperrors.New(apperrors.CodeValidation, run.Checkpoint.StatusReason)
		}
		run.HoldoutCandidateID = candidate.ID
		run.Checkpoint.HoldoutCandidateID = candidate.ID
		run.HoldoutGate = candidate.HoldoutGate
		if err := s.transitionCandidate(ctx, &candidate, CandidateStatusHoldoutEvaluated, nil); err != nil {
			return err
		}
		if err := s.transitionCandidate(ctx, &candidate, CandidateStatusPromoted, nil); err != nil {
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

func (s *Service) shouldStop(ctx context.Context, run *OptimizationRun, req SubmitRequest, started time.Time, checkWallTime bool) (bool, error) {
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
	if wall := req.Budget.wallTime(); checkWallTime && wall > 0 && s.clock().Sub(started) >= wall {
		return true, s.transitionRun(ctx, run, RunStatusBudgetStopped, "budget_stopped", "max wall time reached")
	}
	return false, nil
}

func (s *Service) mergeCurrentRunControlState(ctx context.Context, run *OptimizationRun) error {
	current, ok, err := s.repo().GetOptimizationRun(ctx, run.TenantID, run.ID)
	if err != nil || !ok {
		return err
	}
	if current.CancelRequestedAt != nil || current.Status == RunStatusCanceling || current.Status == RunStatusCanceled {
		run.Status = current.Status
		run.StatusReason = current.StatusReason
		run.CancelRequestedAt = current.CancelRequestedAt
		run.Checkpoint.CancelRequestedAt = current.Checkpoint.CancelRequestedAt
		run.Checkpoint.StatusReason = current.Checkpoint.StatusReason
	}
	return nil
}

func (s *Service) transitionRun(ctx context.Context, run *OptimizationRun, status RunStatus, stage, reason string) error {
	run.Status = status
	run.StatusReason = reason
	run.Checkpoint.Stage = stage
	run.Checkpoint.StatusReason = reason
	run.UpdatedAt = s.clock()
	return s.repo().UpdateOptimizationRun(ctx, *run)
}

func (s *Service) claimRun(ctx context.Context, run *OptimizationRun, expectedStatus, status RunStatus, stage, reason string) error {
	run.Status = status
	run.StatusReason = reason
	run.Checkpoint.Stage = stage
	run.Checkpoint.StatusReason = reason
	run.UpdatedAt = s.clock()
	swapped, err := s.repo().CompareAndSwapOptimizationRun(ctx, *run, expectedStatus)
	if err != nil {
		return err
	}
	if !swapped {
		return stateConflict("optimization run " + run.ID + " was already acquired")
	}
	return nil
}

func (s *Service) claimCandidate(ctx context.Context, candidate *OptimizationCandidate, phase string) error {
	expectedStatus := candidate.Status
	allowed := false
	switch phase {
	case PhaseSelection:
		allowed = expectedStatus == CandidateStatusQueued || expectedStatus == CandidateStatusFailed
	case PhaseHoldout:
		allowed = expectedStatus == CandidateStatusScored
	}
	if !allowed {
		return stateConflict("optimization candidate " + candidate.ID + " cannot start " + phase + " from status " + string(expectedStatus))
	}
	candidate.Status = CandidateStatusRunning
	candidate.Error = ""
	candidate.UpdatedAt = s.clock()
	swapped, err := s.repo().CompareAndSwapOptimizationCandidate(ctx, *candidate, expectedStatus)
	if err != nil {
		return err
	}
	if !swapped {
		return stateConflict("optimization candidate " + candidate.ID + " was already acquired")
	}
	return nil
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

func isResumableRunStatus(status RunStatus) bool {
	return status == RunStatusFailed || status == RunStatusCanceled || status == RunStatusBudgetStopped
}

func stateConflict(message string) error {
	return apperrors.Wrap(apperrors.CodeConflict, message, ErrOptimizationStateConflict)
}

func selectionSplit(req SubmitRequest) string {
	if req.SelectionSplit != "" {
		return req.SelectionSplit
	}
	return "eval"
}

func holdoutGateForPhase(req SubmitRequest, phase string) *eval.HoldoutGateConfig {
	if phase != PhaseHoldout || !req.HoldoutGate.Enabled {
		return nil
	}
	cfg := req.HoldoutGate
	return &cfg
}

func holdoutGateFromCandidate(candidate OptimizationCandidate, req SubmitRequest) eval.HoldoutGateResult {
	if candidate.HoldoutGate.Enabled {
		return candidate.HoldoutGate
	}
	if !req.HoldoutGate.Enabled {
		return eval.HoldoutGateResult{Passed: true}
	}
	return eval.EvaluateHoldoutGate(eval.RunResult{
		Total:                 int(candidate.Metrics["unweighted_sample_count"]),
		Metrics:               candidate.Metrics,
		Split:                 dataset.DatasetSplit(req.HoldoutSplit),
		UnweightedSampleCount: int(candidate.Metrics["unweighted_sample_count"]),
		WeightedSampleCount:   candidate.Metrics["weighted_sample_count"],
		MissingSplit:          candidate.Metrics["missing_split"] > 0,
	}, req.HoldoutGate)
}

func holdoutGateFailureMessage(gate eval.HoldoutGateResult) string {
	if len(gate.Reasons) == 0 {
		return "holdout gate failed"
	}
	return "holdout gate failed: " + strings.Join(gate.Reasons, ",")
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

func isEmptySubmitRequest(req SubmitRequest) bool {
	return len(req.Runner) == 0 && RunConfigFromSubmitRequest(req).IsZero()
}

func validateResumeConfigInvariant(run OptimizationRun, req SubmitRequest) error {
	if run.ProjectID != "" && req.ProjectID != "" && run.ProjectID != req.ProjectID {
		return apperrors.New(apperrors.CodeValidation, "resume request changes project_id; submit a new optimization run")
	}
	stored := run.storedConfig()
	requested := RunConfigFromSubmitRequest(req)
	if field := changedResumeConfigField(stored, requested); field != "" {
		return apperrors.New(apperrors.CodeValidation, "resume request changes "+field+"; submit a new optimization run")
	}
	return nil
}

func changedResumeConfigField(stored, requested RunConfig) string {
	if stored.DatasetID != requested.DatasetID {
		return "dataset_id"
	}
	if stored.KnowledgeBaseID != requested.KnowledgeBaseID {
		return "knowledge_base_id"
	}
	if !reflect.DeepEqual(stored.Objective, requested.Objective) {
		return "objective"
	}
	if !reflect.DeepEqual(stored.SearchSpace, requested.SearchSpace) {
		return "search_space"
	}
	if !reflect.DeepEqual(stored.Search, requested.Search) {
		return "search"
	}
	if stored.Profile != requested.Profile {
		return "profile"
	}
	if stored.TopK != requested.TopK {
		return "top_k"
	}
	if stored.NamespaceTTL != requested.NamespaceTTL {
		return "namespace_ttl"
	}
	if stored.SelectionSplit != requested.SelectionSplit {
		return "selection_split"
	}
	if stored.HoldoutSplit != requested.HoldoutSplit {
		return "holdout_split"
	}
	if !reflect.DeepEqual(stored.HoldoutGate, requested.HoldoutGate) {
		return "holdout_gate"
	}
	return ""
}
