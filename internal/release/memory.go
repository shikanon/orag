package release

import (
	"context"
	"sync"
)

// MemoryRepository is used by the deterministic mock profile and unit tests.
type MemoryRepository struct {
	mu        sync.RWMutex
	envs      map[string]Environment
	versions  map[string]Version
	evidence  map[string]Evidence
	validated map[string]bool
	releases  []Release
}

func NewMemoryRepository(projectID string) *MemoryRepository {
	r := &MemoryRepository{envs: map[string]Environment{}, versions: map[string]Version{}, evidence: map[string]Evidence{}, validated: map[string]bool{}}
	for _, kind := range []EnvironmentKind{Development, Staging, Production} {
		r.envs[string(kind)] = Environment{ID: "env_" + string(kind), ProjectID: projectID, Kind: kind, Bound: true}
	}
	return r
}

func (r *MemoryRepository) Environments(_ context.Context, _ string) ([]Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Environment, 0, len(r.envs))
	for _, kind := range []EnvironmentKind{Development, Staging, Production} {
		items = append(items, r.envs[string(kind)])
	}
	return items, nil
}
func (r *MemoryRepository) Environment(_ context.Context, _ string, kind EnvironmentKind) (Environment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.envs[string(kind)]
	if !ok {
		return Environment{}, ErrNotFound
	}
	return item, nil
}
func (r *MemoryRepository) Releases(_ context.Context, _ string) ([]Release, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Release(nil), r.releases...), nil
}
func (r *MemoryRepository) Version(_ context.Context, _ string, id string) (Version, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.versions[id]
	if !ok {
		return Version{}, ErrNotFound
	}
	return item, nil
}
func (r *MemoryRepository) Evidence(_ context.Context, _ string, id string, env EnvironmentKind) (Evidence, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.evidence[id+"/"+string(env)], nil
}
func (r *MemoryRepository) PreviouslyValidated(_ context.Context, _ string, id string, env EnvironmentKind) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.validated[id+"/"+string(env)], nil
}
func (r *MemoryRepository) Commit(_ context.Context, environment Environment, record Release) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.envs[string(environment.Kind)]
	if current.Revision != environment.Revision-1 {
		return ErrConflict
	}
	r.envs[string(environment.Kind)] = environment
	r.releases = append(r.releases, record)
	return nil
}

func (r *MemoryRepository) PutVersion(version Version) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versions[version.ID] = version
}
func (r *MemoryRepository) PutEvidence(evidence Evidence) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evidence[evidence.VersionID+"/"+evidence.EnvironmentID] = evidence
}
func (r *MemoryRepository) PutValidation(versionID string, environment EnvironmentKind, passed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validated[versionID+"/"+string(environment)] = passed
}
func (r *MemoryRepository) SetEnvironment(environment Environment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.envs[string(environment.Kind)] = environment
}
