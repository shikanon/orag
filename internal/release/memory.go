package release

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
)

// MemoryRepository is used by the deterministic mock profile and unit tests.
type MemoryRepository struct {
	mu               sync.RWMutex
	defaultProjectID string
	envs             map[string]Environment
	versions         map[string]Version
	evidence         map[string]Evidence
	validated        map[string]bool
	releases         []Release
}

func NewMemoryRepository(projectID string) *MemoryRepository {
	r := &MemoryRepository{defaultProjectID: strings.TrimSpace(projectID), envs: map[string]Environment{}, versions: map[string]Version{}, evidence: map[string]Evidence{}, validated: map[string]bool{}}
	r.ensureProjectLocked(r.defaultProjectID)
	return r
}

func (r *MemoryRepository) Environments(_ context.Context, projectID string) ([]Environment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	projectID = r.ensureProjectLocked(projectID)
	items := make([]Environment, 0, len(r.envs))
	for _, kind := range []EnvironmentKind{Development, Staging, Production} {
		items = append(items, r.envs[environmentKey(projectID, kind)])
	}
	return items, nil
}
func (r *MemoryRepository) Environment(_ context.Context, projectID string, kind EnvironmentKind) (Environment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	projectID = r.ensureProjectLocked(projectID)
	item, ok := r.envs[environmentKey(projectID, kind)]
	if !ok {
		return Environment{}, ErrNotFound
	}
	return item, nil
}
func (r *MemoryRepository) Releases(_ context.Context, projectID string) ([]Release, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Release, 0, len(r.releases))
	for _, item := range r.releases {
		if item.ProjectID == r.projectID(projectID) {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *MemoryRepository) Versions(_ context.Context, projectID string) ([]Version, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	projectID = r.projectID(projectID)
	items := make([]Version, 0, len(r.versions))
	for _, item := range r.versions {
		if item.ProjectID == projectID {
			items = append(items, cloneVersion(item))
		}
	}
	return items, nil
}
func (r *MemoryRepository) CreateVersion(_ context.Context, version Version) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	version.ProjectID = r.projectID(version.ProjectID)
	if _, exists := r.versions[versionKey(version.ProjectID, version.ID)]; exists {
		return ErrConflict
	}
	r.versions[versionKey(version.ProjectID, version.ID)] = cloneVersion(version)
	return nil
}
func (r *MemoryRepository) Version(_ context.Context, projectID, id string) (Version, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	projectID = r.projectID(projectID)
	item, ok := r.versions[versionKey(projectID, id)]
	if !ok {
		return Version{}, ErrNotFound
	}
	return cloneVersion(item), nil
}
func (r *MemoryRepository) Evidence(_ context.Context, projectID, id string, env EnvironmentKind) (Evidence, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.evidence[evidenceKey(r.projectID(projectID), id, env)], nil
}
func (r *MemoryRepository) SaveEvidence(_ context.Context, evidence Evidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	evidence.ProjectID = r.projectID(evidence.ProjectID)
	key := evidenceKey(evidence.ProjectID, evidence.VersionID, EnvironmentKind(evidence.EnvironmentID))
	r.evidence[key] = evidence
	r.validated[key] = evidence.Passed
	return nil
}
func (r *MemoryRepository) PreviouslyValidated(_ context.Context, projectID, id string, env EnvironmentKind) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.validated[evidenceKey(r.projectID(projectID), id, env)], nil
}
func (r *MemoryRepository) Commit(_ context.Context, environment Environment, record Release) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	environment.ProjectID = r.projectID(environment.ProjectID)
	current := r.envs[environmentKey(environment.ProjectID, environment.Kind)]
	if current.Revision != environment.Revision-1 {
		return ErrConflict
	}
	r.envs[environmentKey(environment.ProjectID, environment.Kind)] = environment
	r.releases = append(r.releases, record)
	return nil
}

func (r *MemoryRepository) PutVersion(version Version) {
	r.mu.Lock()
	defer r.mu.Unlock()
	version.ProjectID = r.projectID(version.ProjectID)
	r.versions[versionKey(version.ProjectID, version.ID)] = cloneVersion(version)
}
func (r *MemoryRepository) PutEvidence(evidence Evidence) {
	r.mu.Lock()
	defer r.mu.Unlock()
	evidence.ProjectID = r.projectID(evidence.ProjectID)
	key := evidenceKey(evidence.ProjectID, evidence.VersionID, EnvironmentKind(evidence.EnvironmentID))
	r.evidence[key] = evidence
	r.validated[key] = evidence.Passed
}
func (r *MemoryRepository) PutValidation(versionID string, environment EnvironmentKind, passed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validated[evidenceKey(r.defaultProjectID, versionID, environment)] = passed
}
func (r *MemoryRepository) SetEnvironment(environment Environment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	environment.ProjectID = r.projectID(environment.ProjectID)
	r.envs[environmentKey(environment.ProjectID, environment.Kind)] = environment
}

func (r *MemoryRepository) ensureProjectLocked(projectID string) string {
	projectID = r.projectID(projectID)
	for _, kind := range []EnvironmentKind{Development, Staging, Production} {
		key := environmentKey(projectID, kind)
		if _, ok := r.envs[key]; !ok {
			r.envs[key] = Environment{ID: "env_" + projectID + "_" + string(kind), ProjectID: projectID, Kind: kind, Bound: true}
		}
	}
	return projectID
}

func (r *MemoryRepository) projectID(projectID string) string {
	if projectID = strings.TrimSpace(projectID); projectID != "" {
		return projectID
	}
	return r.defaultProjectID
}

func environmentKey(projectID string, kind EnvironmentKind) string {
	return projectID + "\x00" + string(kind)
}
func versionKey(projectID, versionID string) string { return projectID + "\x00" + versionID }
func evidenceKey(projectID, versionID string, kind EnvironmentKind) string {
	return projectID + "\x00" + versionID + "\x00" + string(kind)
}

func cloneVersion(version Version) Version {
	version.Definition = append(json.RawMessage(nil), version.Definition...)
	return version
}
