package project

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
)

var (
	ErrNotFound       = errors.New("project not found")
	ErrTenantRequired = errors.New("project tenant is required")
	ErrNameRequired   = errors.New("project name is required")
	ErrConflict       = errors.New("project conflict")
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

func (s *Service) Create(ctx context.Context, tenantID string, input CreateInput) (Project, error) {
	return s.CreateWithID(ctx, tenantID, id.New("prj"), input)
}

// CreateWithID creates a project with a caller-provided identifier. It is
// intended for internal, idempotent workflows that reserve an identifier
// before a durable background task creates the project. HTTP handlers must use
// Create so callers cannot choose project identifiers.
func (s *Service) CreateWithID(ctx context.Context, tenantID, projectID string, input CreateInput) (Project, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return Project{}, ErrTenantRequired
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Project{}, ErrConflict
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Project{}, ErrNameRequired
	}

	now := s.now()
	created := Project{
		ID:          projectID,
		TenantID:    tenantID,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	kinds := [...]EnvironmentKind{EnvironmentDevelopment, EnvironmentStaging, EnvironmentProduction}
	environments := make([]Environment, 0, len(kinds))
	for _, kind := range kinds {
		environments = append(environments, Environment{
			ID:        id.New("env"),
			ProjectID: created.ID,
			Kind:      kind,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	if err := s.repo.CreateWithEnvironments(ctx, created, environments); err != nil {
		return Project{}, err
	}
	return created, nil
}

func (s *Service) List(ctx context.Context, tenantID string) ([]Project, error) {
	return s.repo.List(ctx, tenantID)
}

func (s *Service) Get(ctx context.Context, tenantID, projectID string) (Project, error) {
	got, ok, err := s.repo.Get(ctx, tenantID, projectID)
	if err != nil {
		return Project{}, err
	}
	if !ok || got.TenantID != tenantID {
		return Project{}, ErrNotFound
	}
	return got, nil
}

func (s *Service) Update(ctx context.Context, tenantID, projectID string, input UpdateInput) (Project, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Project{}, ErrNameRequired
	}
	updated, err := s.Get(ctx, tenantID, projectID)
	if err != nil {
		return Project{}, err
	}
	updated.Name = name
	updated.Description = strings.TrimSpace(input.Description)
	updated.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, updated); err != nil {
		return Project{}, err
	}
	return updated, nil
}
