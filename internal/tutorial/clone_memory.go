package tutorial

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryCloneRepository struct {
	mu          sync.RWMutex
	jobs        map[string]CloneJob
	idempotency map[string]string
	experiments map[string]Experiment
	runs        map[string]ExperimentRun
	runKeys     map[string]string
}

func NewMemoryCloneRepository() *MemoryCloneRepository {
	return &MemoryCloneRepository{
		jobs:        make(map[string]CloneJob),
		idempotency: make(map[string]string),
		experiments: make(map[string]Experiment),
		runs:        make(map[string]ExperimentRun),
		runKeys:     make(map[string]string),
	}
}

func (r *MemoryCloneRepository) CreateOrGetRun(_ context.Context, run ExperimentRun, idempotencyKey string) (ExperimentRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := run.TenantID + "\x00" + run.ProjectID + "\x00" + run.Variant + "\x00" + idempotencyKey
	if existingID, ok := r.runKeys[key]; ok {
		return cloneExperimentRun(r.runs[existingID]), true, nil
	}
	r.runs[run.ID] = cloneExperimentRun(run)
	r.runKeys[key] = run.ID
	return cloneExperimentRun(run), false, nil
}

func (r *MemoryCloneRepository) GetExperimentRun(_ context.Context, tenantID, runID string) (ExperimentRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[runID]
	return cloneExperimentRun(run), ok && run.TenantID == tenantID, nil
}

func (r *MemoryCloneRepository) FindCompletedBaseline(_ context.Context, tenantID, projectID, experimentID, comparisonFingerprint string) (ExperimentRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var latest ExperimentRun
	found := false
	for _, run := range r.runs {
		if run.TenantID != tenantID || run.ProjectID != projectID || run.ExperimentID != experimentID || run.Variant != "baseline" || run.Status != ExperimentRunCompleted || run.ComparisonFingerprint != comparisonFingerprint {
			continue
		}
		if !found || run.UpdatedAt.After(latest.UpdatedAt) || (run.UpdatedAt.Equal(latest.UpdatedAt) && run.ID > latest.ID) {
			latest, found = run, true
		}
	}
	return cloneExperimentRun(latest), found, nil
}

func (r *MemoryCloneRepository) AcquireExperimentRun(_ context.Context, tenantID, runID string, now time.Time) (ExperimentRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID || run.Status != ExperimentRunQueued {
		return ExperimentRun{}, false, nil
	}
	run.Status = ExperimentRunRunning
	run.UpdatedAt = now
	run.Events = append(run.Events, ExperimentRunEvent{Stage: run.Stage, Outcome: "started", OccurredAt: now})
	r.runs[run.ID] = run
	return cloneExperimentRun(run), true, nil
}

func (r *MemoryCloneRepository) AdvanceExperimentRun(_ context.Context, tenantID, runID string, expected, next ExperimentRunStage, now time.Time) (ExperimentRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID || run.Status != ExperimentRunRunning || run.Stage != expected {
		return ExperimentRun{}, false, nil
	}
	run.Stage = next
	run.Status = ExperimentRunQueued
	run.UpdatedAt = now
	run.Events = append(run.Events, ExperimentRunEvent{Stage: next, Outcome: "completed", OccurredAt: now})
	r.runs[run.ID] = run
	return cloneExperimentRun(run), true, nil
}

func (r *MemoryCloneRepository) RecordExperimentRunIndexStats(_ context.Context, tenantID, runID string, chunkCount int, averageChunkTokens float64, now time.Time) (ExperimentRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID || run.Status != ExperimentRunRunning || run.Stage != ExperimentRunStageIndex {
		return ExperimentRun{}, false, nil
	}
	run.IndexedChunkCount = chunkCount
	run.AverageChunkTokens = averageChunkTokens
	run.UpdatedAt = now
	r.runs[run.ID] = run
	return cloneExperimentRun(run), true, nil
}

func (r *MemoryCloneRepository) CompleteExperimentRun(_ context.Context, tenantID, runID, evaluationID string, now time.Time) (ExperimentRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID || run.Status != ExperimentRunRunning || run.Stage != ExperimentRunStageEvaluate {
		return ExperimentRun{}, false, nil
	}
	run.Stage = ExperimentRunStageComplete
	run.Status = ExperimentRunCompleted
	run.EvaluationRunID = evaluationID
	run.UpdatedAt = now
	run.Events = append(run.Events, ExperimentRunEvent{Stage: ExperimentRunStageComplete, Outcome: "completed", OccurredAt: now})
	r.runs[run.ID] = run
	return cloneExperimentRun(run), true, nil
}

func (r *MemoryCloneRepository) FailExperimentRun(_ context.Context, tenantID, runID string, stage ExperimentRunStage, code string, now time.Time) (ExperimentRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID || run.Status != ExperimentRunRunning || run.Stage != stage {
		return ExperimentRun{}, false, nil
	}
	run.Status = ExperimentRunFailed
	run.FailureCode = code
	run.UpdatedAt = now
	run.Events = append(run.Events, ExperimentRunEvent{Stage: stage, Outcome: "failed", DetailCode: code, OccurredAt: now})
	r.runs[run.ID] = run
	return cloneExperimentRun(run), true, nil
}

func (r *MemoryCloneRepository) CancelExperimentRun(_ context.Context, tenantID, runID string, now time.Time) (ExperimentRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID {
		return ExperimentRun{}, false, nil
	}
	switch run.Status {
	case ExperimentRunQueued:
		run.Status = ExperimentRunCancelled
		run.Events = append(run.Events, ExperimentRunEvent{Stage: run.Stage, Outcome: "cancelled", OccurredAt: now})
	case ExperimentRunRunning:
		run.Status = ExperimentRunCancelRequested
		run.Events = append(run.Events, ExperimentRunEvent{Stage: run.Stage, Outcome: "cancel_requested", OccurredAt: now})
	default:
		return ExperimentRun{}, false, nil
	}
	run.UpdatedAt = now
	r.runs[run.ID] = run
	return cloneExperimentRun(run), true, nil
}

func (r *MemoryCloneRepository) MarkExperimentRunCancelled(_ context.Context, tenantID, runID string, now time.Time) (ExperimentRun, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID || run.Status != ExperimentRunCancelRequested {
		return ExperimentRun{}, false, nil
	}
	run.Status = ExperimentRunCancelled
	run.UpdatedAt = now
	run.Events = append(run.Events, ExperimentRunEvent{Stage: run.Stage, Outcome: "cancelled", OccurredAt: now})
	r.runs[run.ID] = run
	return cloneExperimentRun(run), true, nil
}

func (r *MemoryCloneRepository) RecoverExperimentRuns(_ context.Context, now time.Time) ([]ExperimentRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]ExperimentRun, 0)
	for id, run := range r.runs {
		switch run.Status {
		case ExperimentRunRunning:
			run.Status = ExperimentRunQueued
			run.UpdatedAt = now
			run.Events = append(run.Events, ExperimentRunEvent{Stage: run.Stage, Outcome: "recovered", OccurredAt: now})
			r.runs[id] = run
			items = append(items, cloneExperimentRun(run))
		case ExperimentRunQueued:
			items = append(items, cloneExperimentRun(run))
		case ExperimentRunCancelRequested:
			run.Status = ExperimentRunCancelled
			run.UpdatedAt = now
			run.Events = append(run.Events, ExperimentRunEvent{Stage: run.Stage, Outcome: "cancelled", OccurredAt: now})
			r.runs[id] = run
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (r *MemoryCloneRepository) CreateOrGet(_ context.Context, job CloneJob) (CloneJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := cloneIdempotencyKey(job)
	if existingID, ok := r.idempotency[key]; ok {
		return cloneJob(r.jobs[existingID]), true, nil
	}
	r.jobs[job.ID] = cloneJob(job)
	r.idempotency[key] = job.ID
	return cloneJob(job), false, nil
}

func (r *MemoryCloneRepository) GetJob(_ context.Context, tenantID, jobID string) (CloneJob, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.jobs[jobID]
	return cloneJob(job), ok && job.TenantID == tenantID, nil
}

func (r *MemoryCloneRepository) Retry(_ context.Context, tenantID, jobID string, stage CloneStage, now time.Time) (CloneJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok || job.TenantID != tenantID || job.Status != CloneStatusFailed {
		return CloneJob{}, false, nil
	}
	job.Stage = stage
	job.Status = CloneStatusQueued
	job.Attempt++
	job.LastErrorCode = ""
	job.UpdatedAt = now
	job.Events = append(job.Events, StageEvent{Stage: stage, Outcome: "retry_queued", OccurredAt: now})
	r.jobs[job.ID] = job
	return cloneJob(job), true, nil
}

func (r *MemoryCloneRepository) GetExperiment(_ context.Context, tenantID, projectID string) (Experiment, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	experiment, ok := r.experiments[projectID]
	return experiment, ok && experiment.TenantID == tenantID, nil
}

func (r *MemoryCloneRepository) Acquire(_ context.Context, tenantID, jobID string, now time.Time) (CloneJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok || job.TenantID != tenantID || job.Status != CloneStatusQueued {
		return CloneJob{}, false, nil
	}
	job.Status = CloneStatusRunning
	job.UpdatedAt = now
	job.Events = append(job.Events, StageEvent{Stage: job.Stage, Outcome: "started", OccurredAt: now})
	r.jobs[job.ID] = job
	return cloneJob(job), true, nil
}

func (r *MemoryCloneRepository) Advance(_ context.Context, tenantID, jobID string, expected, next CloneStage, status CloneStatus, now time.Time) (CloneJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok || job.TenantID != tenantID || job.Status != CloneStatusRunning || job.Stage != expected {
		return CloneJob{}, false, nil
	}
	job.Stage = next
	job.Status = status
	job.UpdatedAt = now
	job.Events = append(job.Events, StageEvent{Stage: next, Outcome: "completed", OccurredAt: now})
	r.jobs[job.ID] = job
	return cloneJob(job), true, nil
}

func (r *MemoryCloneRepository) Fail(_ context.Context, tenantID, jobID string, stage CloneStage, code string, now time.Time) (CloneJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	if !ok || job.TenantID != tenantID || job.Status != CloneStatusRunning || job.Stage != stage {
		return CloneJob{}, false, nil
	}
	job.Status = CloneStatusFailed
	job.LastErrorCode = code
	job.UpdatedAt = now
	job.Events = append(job.Events, StageEvent{Stage: stage, Outcome: "failed", DetailCode: code, OccurredAt: now})
	r.jobs[job.ID] = job
	return cloneJob(job), true, nil
}

func (r *MemoryCloneRepository) EnsureExperiment(_ context.Context, experiment Experiment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.experiments[experiment.ProjectID]; ok {
		if existing.TenantID != experiment.TenantID || existing.TemplateID != experiment.TemplateID || existing.TemplateVersion != experiment.TemplateVersion || existing.Tier != experiment.Tier {
			return ErrCloneExperimentAbsent
		}
		return nil
	}
	r.experiments[experiment.ProjectID] = experiment
	return nil
}

func (r *MemoryCloneRepository) SetExperimentStatus(_ context.Context, tenantID, projectID string, status PackStatus, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	experiment, ok := r.experiments[projectID]
	if !ok || experiment.TenantID != tenantID {
		return ErrCloneExperimentAbsent
	}
	experiment.PackStatus = status
	experiment.UpdatedAt = now
	r.experiments[projectID] = experiment
	return nil
}

func (r *MemoryCloneRepository) SetExperimentRuntime(_ context.Context, tenantID, projectID string, resources RuntimeResources, manifest Manifest, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	experiment, ok := r.experiments[projectID]
	if !ok || experiment.TenantID != tenantID {
		return ErrCloneExperimentAbsent
	}
	experiment.RuntimeStatus = resources.Status
	experiment.KnowledgeBaseID = resources.KnowledgeBaseID
	experiment.DatasetID = resources.DatasetID
	experiment.BaselineProfile = resources.BaselineProfile
	experiment.BaselineTopK = resources.BaselineTopK
	experiment.PackManifest = cloneManifest(manifest)
	experiment.UpdatedAt = now
	r.experiments[projectID] = experiment
	return nil
}

func (r *MemoryCloneRepository) RecoverPending(_ context.Context, now time.Time) ([]CloneJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]CloneJob, 0)
	for id, job := range r.jobs {
		if job.Status == CloneStatusRunning {
			job.Status = CloneStatusQueued
			job.UpdatedAt = now
			job.Events = append(job.Events, StageEvent{Stage: job.Stage, Outcome: "recovered", OccurredAt: now})
			r.jobs[id] = job
		}
		if job.Status == CloneStatusQueued {
			items = append(items, cloneJob(job))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (r *MemoryCloneRepository) Jobs() []CloneJob {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]CloneJob, 0, len(r.jobs))
	for _, job := range r.jobs {
		items = append(items, cloneJob(job))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items
}

func cloneIdempotencyKey(job CloneJob) string {
	return job.TenantID + "\x00" + job.SubjectID + "\x00" + job.TemplateID + "\x00" + job.TemplateVersion + "\x00" + job.IdempotencyKey
}

func cloneJob(job CloneJob) CloneJob {
	job.Events = append([]StageEvent(nil), job.Events...)
	return job
}

func cloneExperimentRun(run ExperimentRun) ExperimentRun {
	run.Events = append([]ExperimentRunEvent(nil), run.Events...)
	return run
}
