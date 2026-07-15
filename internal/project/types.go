package project

import (
	"context"
	"strings"
	"time"
)

func LegacyDefaultID(tenantID string) string {
	return "prj_default_" + strings.TrimSpace(tenantID)
}

func LegacyDefaultEnvironments(tenantID string, now time.Time) []Environment {
	tenantID = strings.TrimSpace(tenantID)
	projectID := LegacyDefaultID(tenantID)
	kinds := [...]EnvironmentKind{EnvironmentDevelopment, EnvironmentStaging, EnvironmentProduction}
	environments := make([]Environment, 0, len(kinds))
	for _, kind := range kinds {
		environments = append(environments, Environment{
			ID:        "env_default_" + string(kind) + "_" + tenantID,
			ProjectID: projectID,
			Kind:      kind,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	return environments
}

type EnvironmentKind string

const (
	EnvironmentDevelopment EnvironmentKind = "development"
	EnvironmentStaging     EnvironmentKind = "staging"
	EnvironmentProduction  EnvironmentKind = "production"
)

type Project struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Environment struct {
	ID        string          `json:"id"`
	ProjectID string          `json:"project_id"`
	Kind      EnvironmentKind `json:"kind"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type CreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Repository interface {
	CreateWithEnvironments(context.Context, Project, []Environment) error
	List(context.Context, string) ([]Project, error)
	Get(context.Context, string, string) (Project, bool, error)
	Update(context.Context, Project) error
}
