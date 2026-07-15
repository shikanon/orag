package optimizer

import (
	"context"
	"sync"
)

type MemoryRepository struct {
	mu         sync.RWMutex
	runs       map[string]OptimizationRun
	candidates map[string]OptimizationCandidate
	harness    []HarnessRunRecord
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		runs:       map[string]OptimizationRun{},
		candidates: map[string]OptimizationCandidate{},
	}
}

func (r *MemoryRepository) CreateOptimizationRun(_ context.Context, run OptimizationRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}

func (r *MemoryRepository) CreateOptimizationRunWithCandidates(_ context.Context, run OptimizationRun, candidates []OptimizationCandidate) error {
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

func (r *MemoryRepository) GetOptimizationRun(_ context.Context, tenantID, runID string) (OptimizationRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[runID]
	if !ok || run.TenantID != tenantID {
		return OptimizationRun{}, false, nil
	}
	return run, true, nil
}

func (r *MemoryRepository) UpdateOptimizationRun(_ context.Context, run OptimizationRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}

func (r *MemoryRepository) CompareAndSwapOptimizationRun(_ context.Context, run OptimizationRun, expectedStatus RunStatus) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.runs[run.ID]
	if !ok || current.TenantID != run.TenantID || current.Status != expectedStatus {
		return false, nil
	}
	r.runs[run.ID] = run
	return true, nil
}

func (r *MemoryRepository) CreateOptimizationCandidate(_ context.Context, candidate OptimizationCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	candidate.Config.ID = candidate.ID
	r.candidates[candidate.ID] = candidate
	return nil
}

func (r *MemoryRepository) UpdateOptimizationCandidate(_ context.Context, candidate OptimizationCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.candidates[candidate.ID] = candidate
	return nil
}

func (r *MemoryRepository) CompareAndSwapOptimizationCandidate(_ context.Context, candidate OptimizationCandidate, expectedStatus CandidateStatus) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.candidates[candidate.ID]
	if !ok || current.OptimizationRunID != candidate.OptimizationRunID || current.Status != expectedStatus {
		return false, nil
	}
	r.candidates[candidate.ID] = candidate
	return true, nil
}

func (r *MemoryRepository) ListOptimizationCandidates(_ context.Context, tenantID, runID string) ([]OptimizationCandidate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	sortOptimizationCandidates(out)
	return out, nil
}

func (r *MemoryRepository) StoreHarnessRun(_ context.Context, run HarnessRunRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.harness = append(r.harness, run)
	return nil
}

func sortOptimizationCandidates(candidates []OptimizationCandidate) {
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].ID < candidates[i].ID {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
}
