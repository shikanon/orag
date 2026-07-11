package project

import (
	"context"
	"time"
)

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
