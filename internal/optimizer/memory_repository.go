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
