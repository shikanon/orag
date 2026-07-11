package project_test

import (
	"context"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/project"
)

var fixedTime = time.Date(2026, time.July, 11, 9, 30, 0, 0, time.UTC)

func fixedClock() time.Time { return fixedTime }

func TestServiceCreateInitializesThreeEnvironments(t *testing.T) {
	repo := newMemoryRepository()
	svc := project.NewService(repo, fixedClock)
	got, err := svc.Create(context.Background(), "tenant_a", project.CreateInput{Name: "Support"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Support" {
		t.Fatalf("Name = %q, want Support", got.Name)
	}
	gotKinds := repo.environmentKinds(got.ID)
	wantKinds := []project.EnvironmentKind{"development", "staging", "production"}
	if len(gotKinds) != len(wantKinds) {
		t.Fatalf("environment kinds = %v, want %v", gotKinds, wantKinds)
	}
	for i := range wantKinds {
		if gotKinds[i] != wantKinds[i] {
			t.Fatalf("environment kinds = %v, want %v", gotKinds, wantKinds)
		}
	}
}

func TestServiceGetRejectsForeignTenant(t *testing.T) {
	repo := seededMemoryRepository("tenant_a")
	_, err := project.NewService(repo, fixedClock).Get(context.Background(), "tenant_b", "prj_1")
	if err != project.ErrNotFound {
		t.Fatalf("Get() error = %v, want %v", err, project.ErrNotFound)
	}
}

type memoryRepository struct {
	projects     map[string]project.Project
	environments []project.Environment
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{projects: make(map[string]project.Project)}
}

func seededMemoryRepository(tenantID string) *memoryRepository {
	repo := newMemoryRepository()
	repo.projects["prj_1"] = project.Project{ID: "prj_1", TenantID: tenantID, Name: "Support"}
	return repo
}

func (r *memoryRepository) CreateWithEnvironments(_ context.Context, p project.Project, environments []project.Environment) error {
	r.projects[p.ID] = p
	r.environments = append(r.environments, environments...)
	return nil
}

func (r *memoryRepository) List(_ context.Context, tenantID string) ([]project.Project, error) {
	var projects []project.Project
	for _, p := range r.projects {
		if p.TenantID == tenantID {
			projects = append(projects, p)
		}
	}
	return projects, nil
}

func (r *memoryRepository) Get(_ context.Context, tenantID, projectID string) (project.Project, bool, error) {
	p, ok := r.projects[projectID]
	return p, ok && p.TenantID == tenantID, nil
}

func (r *memoryRepository) Update(_ context.Context, p project.Project) error {
	r.projects[p.ID] = p
	return nil
}

func (r *memoryRepository) environmentKinds(projectID string) []project.EnvironmentKind {
	var kinds []project.EnvironmentKind
	for _, environment := range r.environments {
		if environment.ProjectID == projectID {
			kinds = append(kinds, environment.Kind)
		}
	}
	return kinds
}
