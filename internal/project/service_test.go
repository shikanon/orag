package project_test

import (
	"context"
	"errors"
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

func TestServiceCreateWithIDUsesReservedIdentifier(t *testing.T) {
	repo := newMemoryRepository()
	svc := project.NewService(repo, fixedClock)
	got, err := svc.CreateWithID(context.Background(), "tenant_a", "prj_reserved", project.CreateInput{Name: "Tutorial experiment"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "prj_reserved" || len(repo.environmentKinds(got.ID)) != 3 {
		t.Fatalf("CreateWithID() = %#v, environments=%v", got, repo.environmentKinds(got.ID))
	}
	if _, err := svc.CreateWithID(context.Background(), "tenant_a", " ", project.CreateInput{Name: "Tutorial experiment"}); !errors.Is(err, project.ErrConflict) {
		t.Fatalf("blank ID error = %v, want conflict", err)
	}
}

func TestServiceCreateRejectsMissingTenant(t *testing.T) {
	for _, tenantID := range []string{"", " \t\n "} {
		t.Run(tenantID, func(t *testing.T) {
			repo := newMemoryRepository()
			_, err := project.NewService(repo, fixedClock).Create(context.Background(), tenantID, project.CreateInput{Name: "Support"})
			if !errors.Is(err, project.ErrTenantRequired) {
				t.Fatalf("Create() error = %v, want %v", err, project.ErrTenantRequired)
			}
			if repo.createCalls != 0 {
				t.Fatalf("CreateWithEnvironments calls = %d, want 0", repo.createCalls)
			}
		})
	}
}

func TestServiceGetRejectsForeignTenant(t *testing.T) {
	repo := seededMemoryRepository("tenant_a")
	_, err := project.NewService(repo, fixedClock).Get(context.Background(), "tenant_b", "prj_1")
	if err != project.ErrNotFound {
		t.Fatalf("Get() error = %v, want %v", err, project.ErrNotFound)
	}
}

func TestServiceCreateTrimsAndRequiresName(t *testing.T) {
	repo := newMemoryRepository()
	got, err := project.NewService(repo, fixedClock).Create(context.Background(), " tenant_a ", project.CreateInput{
		Name:        " Support ",
		Description: " Customer support ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantID != "tenant_a" || got.Name != "Support" || got.Description != "Customer support" {
		t.Fatalf("Create() = %#v, want trimmed tenant, name, and description", got)
	}

	_, err = project.NewService(repo, fixedClock).Create(context.Background(), "tenant_a", project.CreateInput{Name: " \t "})
	if !errors.Is(err, project.ErrNameRequired) {
		t.Fatalf("Create() error = %v, want %v", err, project.ErrNameRequired)
	}
	if repo.createCalls != 1 {
		t.Fatalf("CreateWithEnvironments calls = %d, want 1", repo.createCalls)
	}
}

func TestServiceGetMapsMissingProjectToNotFound(t *testing.T) {
	_, err := project.NewService(newMemoryRepository(), fixedClock).Get(context.Background(), "tenant_a", "prj_missing")
	if !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, project.ErrNotFound)
	}
}

func TestServiceListAndUpdateRemainTenantScoped(t *testing.T) {
	repo := seededMemoryRepository("tenant_a")
	repo.projects["prj_2"] = project.Project{ID: "prj_2", TenantID: "tenant_b", Name: "Foreign"}
	svc := project.NewService(repo, fixedClock)

	projects, err := svc.List(context.Background(), "tenant_a")
	if err != nil {
		t.Fatal(err)
	}
	if repo.listTenant != "tenant_a" || len(projects) != 1 || projects[0].ID != "prj_1" {
		t.Fatalf("List() tenant = %q, projects = %#v", repo.listTenant, projects)
	}

	_, err = svc.Update(context.Background(), "tenant_b", "prj_1", project.UpdateInput{Name: "Changed"})
	if !errors.Is(err, project.ErrNotFound) || repo.updateCalls != 0 {
		t.Fatalf("foreign Update() error = %v, update calls = %d", err, repo.updateCalls)
	}

	got, err := svc.Update(context.Background(), "tenant_a", "prj_1", project.UpdateInput{Name: " Updated ", Description: " Description "})
	if err != nil {
		t.Fatal(err)
	}
	if repo.getTenant != "tenant_a" || got.Name != "Updated" || got.Description != "Description" || repo.updateCalls != 1 {
		t.Fatalf("Update() = %#v, get tenant = %q, update calls = %d", got, repo.getTenant, repo.updateCalls)
	}

	_, err = svc.Update(context.Background(), "tenant_a", "prj_1", project.UpdateInput{Name: " \t "})
	if !errors.Is(err, project.ErrNameRequired) || repo.updateCalls != 1 {
		t.Fatalf("blank-name Update() error = %v, update calls = %d", err, repo.updateCalls)
	}
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	wantErr := errors.New("repository unavailable")
	tests := []struct {
		name string
		run  func(*memoryRepository) error
	}{
		{name: "create", run: func(repo *memoryRepository) error {
			repo.createErr = wantErr
			_, err := project.NewService(repo, fixedClock).Create(context.Background(), "tenant_a", project.CreateInput{Name: "Support"})
			return err
		}},
		{name: "list", run: func(repo *memoryRepository) error {
			repo.listErr = wantErr
			_, err := project.NewService(repo, fixedClock).List(context.Background(), "tenant_a")
			return err
		}},
		{name: "get", run: func(repo *memoryRepository) error {
			repo.getErr = wantErr
			_, err := project.NewService(repo, fixedClock).Get(context.Background(), "tenant_a", "prj_1")
			return err
		}},
		{name: "update", run: func(repo *memoryRepository) error {
			repo.updateErr = wantErr
			_, err := project.NewService(repo, fixedClock).Update(context.Background(), "tenant_a", "prj_1", project.UpdateInput{Name: "Updated"})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(seededMemoryRepository("tenant_a")); !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want %v", err, wantErr)
			}
		})
	}
}

type memoryRepository struct {
	projects     map[string]project.Project
	environments []project.Environment
	createCalls  int
	updateCalls  int
	listTenant   string
	getTenant    string
	createErr    error
	listErr      error
	getErr       error
	updateErr    error
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
	r.createCalls++
	if r.createErr != nil {
		return r.createErr
	}
	r.projects[p.ID] = p
	r.environments = append(r.environments, environments...)
	return nil
}

func (r *memoryRepository) List(_ context.Context, tenantID string) ([]project.Project, error) {
	r.listTenant = tenantID
	if r.listErr != nil {
		return nil, r.listErr
	}
	var projects []project.Project
	for _, p := range r.projects {
		if p.TenantID == tenantID {
			projects = append(projects, p)
		}
	}
	return projects, nil
}

func (r *memoryRepository) Get(_ context.Context, tenantID, projectID string) (project.Project, bool, error) {
	r.getTenant = tenantID
	if r.getErr != nil {
		return project.Project{}, false, r.getErr
	}
	p, ok := r.projects[projectID]
	return p, ok && p.TenantID == tenantID, nil
}

func (r *memoryRepository) Update(_ context.Context, p project.Project) error {
	r.updateCalls++
	if r.updateErr != nil {
		return r.updateErr
	}
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
