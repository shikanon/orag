package evaluationpolicy

import (
	"context"
	"sort"
	"sync"
)

type MemoryRepository struct {
	mu        sync.RWMutex
	policies  map[string]Policy
	evidences map[string]Evidence
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{policies: map[string]Policy{}, evidences: map[string]Evidence{}}
}

func (r *MemoryRepository) Create(_ context.Context, policy Policy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.policies {
		if existing.ProjectID == policy.ProjectID && existing.Name == policy.Name && existing.Version == policy.Version {
			return ErrInvalidPolicy
		}
	}
	r.policies[policy.ID] = clonePolicy(policy)
	return nil
}

func (r *MemoryRepository) Get(_ context.Context, tenantID, projectID, policyID string) (Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	policy, found := r.policies[policyID]
	if !found || policy.TenantID != tenantID || policy.ProjectID != projectID {
		return Policy{}, ErrPolicyNotFound
	}
	return clonePolicy(policy), nil
}

func (r *MemoryRepository) List(_ context.Context, tenantID, projectID string) ([]Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Policy, 0)
	for _, policy := range r.policies {
		if policy.TenantID == tenantID && policy.ProjectID == projectID {
			items = append(items, clonePolicy(policy))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func (r *MemoryRepository) RecordEvidence(_ context.Context, evidence Evidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, found := r.evidences[evidence.ID]; found {
		return ErrInvalidPolicy
	}
	r.evidences[evidence.ID] = cloneEvidence(evidence)
	return nil
}

func clonePolicy(policy Policy) Policy {
	policy.Gates = cloneGates(policy.Gates)
	return policy
}

func cloneEvidence(evidence Evidence) Evidence {
	evidence.FrozenInput.Gates = cloneGates(evidence.FrozenInput.Gates)
	evidence.FrozenInput.Metrics = cloneMetrics(evidence.FrozenInput.Metrics)
	evidence.GateResults = append([]GateResult(nil), evidence.GateResults...)
	return evidence
}
