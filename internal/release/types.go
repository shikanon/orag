// Package release owns the project environment promotion and rollback state machine.
package release

import (
	"context"
	"errors"
	"time"
)

type EnvironmentKind string

const (
	Development EnvironmentKind = "development"
	Staging     EnvironmentKind = "staging"
	Production  EnvironmentKind = "production"
)

var (
	ErrInvalidTransition = errors.New("invalid release transition")
	ErrGateFailed        = errors.New("release gate failed")
	ErrConflict          = errors.New("release environment conflict")
	ErrRollbackTarget    = errors.New("invalid rollback target")
	ErrNotFound          = errors.New("release resource not found")
	ErrBindingMissing    = errors.New("release environment binding missing")
)

type Environment struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"project_id"`
	Kind            EnvironmentKind `json:"kind"`
	ActiveVersionID string          `json:"active_version_id,omitempty"`
	Revision        int64           `json:"revision"`
	Bound           bool            `json:"bound"`
}

type Version struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ContentHash string `json:"content_hash"`
}

type Evidence struct {
	VersionID     string `json:"version_id"`
	EnvironmentID string `json:"environment_id"`
	Passed        bool   `json:"passed"`
	ContentHash   string `json:"content_hash"`
}

type Release struct {
	ID                string          `json:"id"`
	ProjectID         string          `json:"project_id"`
	SourceVersionID   string          `json:"source_version_id"`
	TargetVersionID   string          `json:"target_version_id"`
	SourceEnvironment EnvironmentKind `json:"source_environment"`
	TargetEnvironment EnvironmentKind `json:"target_environment"`
	Action            string          `json:"action"`
	Actor             string          `json:"actor"`
	Reason            string          `json:"reason,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

type PromoteRequest struct {
	ProjectID               string          `json:"project_id,omitempty"`
	SourceEnvironment       EnvironmentKind `json:"source_environment"`
	TargetEnvironment       EnvironmentKind `json:"target_environment"`
	TargetVersionID         string          `json:"target_version_id"`
	ExpectedActiveVersionID string          `json:"expected_active_version_id,omitempty"`
	Actor                   string          `json:"actor,omitempty"`
}

type RollbackRequest struct {
	ProjectID               string          `json:"project_id,omitempty"`
	Environment             EnvironmentKind `json:"environment,omitempty"`
	TargetVersionID         string          `json:"target_version_id"`
	ExpectedActiveVersionID string          `json:"expected_active_version_id,omitempty"`
	Actor                   string          `json:"actor,omitempty"`
	Reason                  string          `json:"reason"`
}

type Repository interface {
	Environments(ctx context.Context, projectID string) ([]Environment, error)
	Environment(ctx context.Context, projectID string, kind EnvironmentKind) (Environment, error)
	Releases(ctx context.Context, projectID string) ([]Release, error)
	Version(ctx context.Context, projectID, versionID string) (Version, error)
	Evidence(ctx context.Context, projectID, versionID string, environment EnvironmentKind) (Evidence, error)
	PreviouslyValidated(ctx context.Context, projectID, versionID string, environment EnvironmentKind) (bool, error)
	Commit(ctx context.Context, environment Environment, release Release) error
}
