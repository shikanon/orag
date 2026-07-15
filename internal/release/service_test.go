package release

import (
	"context"
	"errors"
	"testing"
)

func TestServicePromoteRequiresEvidenceAndCAS(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo)
	_, err := svc.Promote(context.Background(), PromoteRequest{ProjectID: "p1", SourceEnvironment: Development, TargetEnvironment: Staging, TargetVersionID: "v1", ExpectedActiveVersionID: "", Actor: "alice"})
	if !errors.Is(err, ErrGateFailed) {
		t.Fatalf("Promote() error = %v, want gate failed", err)
	}
	repo.evidence["v1/staging"] = Evidence{VersionID: "v1", EnvironmentID: "staging", Passed: true, ContentHash: "hash-v1"}
	release, err := svc.Promote(context.Background(), PromoteRequest{ProjectID: "p1", SourceEnvironment: Development, TargetEnvironment: Staging, TargetVersionID: "v1", Actor: "alice"})
	if err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	if release.Action != "promote" || repo.env["staging"].ActiveVersionID != "v1" {
		t.Fatalf("unexpected release or state: %#v", release)
	}
	_, err = svc.Promote(context.Background(), PromoteRequest{ProjectID: "p1", SourceEnvironment: Development, TargetEnvironment: Staging, TargetVersionID: "v1", ExpectedActiveVersionID: "stale", Actor: "alice"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale Promote() error = %v, want conflict", err)
	}
}

func TestServicePromoteRejectsLegacyHashOnlyVersion(t *testing.T) {
	repo := newMemoryRepository()
	repo.versions["v1"] = Version{ID: "v1", ProjectID: "p1", ContentHash: "hash-v1"}
	repo.evidence["v1/staging"] = Evidence{VersionID: "v1", EnvironmentID: "staging", Passed: true, ContentHash: "hash-v1"}
	_, err := NewService(repo).Promote(context.Background(), PromoteRequest{ProjectID: "p1", SourceEnvironment: Development, TargetEnvironment: Staging, TargetVersionID: "v1", Actor: "alice"})
	if !errors.Is(err, ErrGateFailed) {
		t.Fatalf("Promote() error = %v, want frozen-definition gate failure", err)
	}
}

func TestServiceRejectsSkippedPromotionAndRollsBackValidatedVersion(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo)
	_, err := svc.Promote(context.Background(), PromoteRequest{ProjectID: "p1", SourceEnvironment: Development, TargetEnvironment: Production, TargetVersionID: "v1", Actor: "alice"})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("skipped Promote() error = %v, want invalid transition", err)
	}
	staging := repo.env["staging"]
	staging.ActiveVersionID = "v2"
	staging.Revision = 2
	repo.env["staging"] = staging
	repo.validated["v1/staging"] = true
	release, err := svc.Rollback(context.Background(), RollbackRequest{ProjectID: "p1", Environment: Staging, TargetVersionID: "v1", ExpectedActiveVersionID: "v2", Actor: "alice", Reason: "restore known-good version"})
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if release.Action != "rollback" || repo.env["staging"].ActiveVersionID != "v1" {
		t.Fatalf("unexpected rollback state: %#v", repo.env["staging"])
	}
	_, err = svc.Rollback(context.Background(), RollbackRequest{ProjectID: "p1", Environment: Staging, TargetVersionID: "v2", ExpectedActiveVersionID: "v1", Actor: "alice", Reason: "same"})
	if !errors.Is(err, ErrRollbackTarget) {
		t.Fatalf("unvalidated Rollback() error = %v, want rollback target", err)
	}
}

type memoryRepository struct {
	env       map[string]Environment
	versions  map[string]Version
	evidence  map[string]Evidence
	validated map[string]bool
	releases  []Release
}

func (r *memoryRepository) Environments(_ context.Context, _ string) ([]Environment, error) {
	return nil, nil
}
func (r *memoryRepository) Releases(_ context.Context, _ string) ([]Release, error) {
	return append([]Release(nil), r.releases...), nil
}
func (r *memoryRepository) Versions(_ context.Context, _ string) ([]Version, error) {
	items := make([]Version, 0, len(r.versions))
	for _, v := range r.versions {
		items = append(items, v)
	}
	return items, nil
}
func (r *memoryRepository) CreateVersion(_ context.Context, v Version) error {
	if _, ok := r.versions[v.ID]; ok {
		return ErrConflict
	}
	r.versions[v.ID] = v
	return nil
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{env: map[string]Environment{"development": {ID: "dev", ProjectID: "p1", Kind: Development, ActiveVersionID: "v1", Bound: true}, "staging": {ID: "stg", ProjectID: "p1", Kind: Staging, Bound: true}, "production": {ID: "prd", ProjectID: "p1", Kind: Production, Bound: true}}, versions: map[string]Version{"v1": {ID: "v1", ProjectID: "p1", PipelineID: "pipe_1", Definition: []byte(`{"nodes":[]}`), ContentHash: "hash-v1"}, "v2": {ID: "v2", ProjectID: "p1", PipelineID: "pipe_1", Definition: []byte(`{"nodes":[]}`), ContentHash: "hash-v2"}}, evidence: map[string]Evidence{}, validated: map[string]bool{}}
}
func (r *memoryRepository) Environment(_ context.Context, _ string, kind EnvironmentKind) (Environment, error) {
	item, ok := r.env[string(kind)]
	if !ok {
		return Environment{}, ErrNotFound
	}
	return item, nil
}
func (r *memoryRepository) Version(_ context.Context, _ string, id string) (Version, error) {
	item, ok := r.versions[id]
	if !ok {
		return Version{}, ErrNotFound
	}
	return item, nil
}
func (r *memoryRepository) Evidence(_ context.Context, _ string, id string, env EnvironmentKind) (Evidence, error) {
	item, ok := r.evidence[id+"/"+string(env)]
	if !ok {
		return Evidence{}, nil
	}
	return item, nil
}
func (r *memoryRepository) SaveEvidence(_ context.Context, evidence Evidence) error {
	r.evidence[evidence.VersionID+"/"+evidence.EnvironmentID] = evidence
	r.validated[evidence.VersionID+"/"+evidence.EnvironmentID] = evidence.Passed
	return nil
}
func (r *memoryRepository) PreviouslyValidated(_ context.Context, _ string, id string, env EnvironmentKind) (bool, error) {
	return r.validated[id+"/"+string(env)], nil
}
func (r *memoryRepository) Commit(_ context.Context, env Environment, rel Release) error {
	r.env[string(env.Kind)] = env
	r.releases = append(r.releases, rel)
	return nil
}
