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
}

func NewMemoryCloneRepository() *MemoryCloneRepository {
	return &MemoryCloneRepository{
		jobs:        make(map[string]CloneJob),
		idempotency: make(map[string]string),
		experiments: make(map[string]Experiment),
	}
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
