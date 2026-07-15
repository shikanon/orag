package orag

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/release"
)

// ReleaseClient is the stable beta release-lifecycle subset of Client.
// Keeping this interface public lets downstream applications inject a mock
// without importing ORAG internals.
type ReleaseClient interface {
	ListEnvironments(context.Context, ListEnvironmentsRequest) ([]Environment, error)
	ListReleases(context.Context, ListReleasesRequest) ([]Release, error)
	Promote(context.Context, PromoteRequest) (Release, error)
	Rollback(context.Context, RollbackRequest) (Release, error)
}

var _ ReleaseClient = (*Client)(nil)

type EnvironmentKind string

const (
	EnvironmentDevelopment EnvironmentKind = "development"
	EnvironmentStaging     EnvironmentKind = "staging"
	EnvironmentProduction  EnvironmentKind = "production"
)

type Environment struct {
	ID              string
	ProjectID       string
	Kind            EnvironmentKind
	ActiveVersionID string
	Revision        int64
	Bound           bool
}

type Release struct {
	ID                string
	ProjectID         string
	SourceVersionID   string
	TargetVersionID   string
	SourceEnvironment EnvironmentKind
	TargetEnvironment EnvironmentKind
	Action            string
	Actor             string
	Reason            string
	CreatedAt         time.Time
}

type ListEnvironmentsRequest struct{ ProjectID string }
type ListReleasesRequest struct{ ProjectID string }

type PromoteRequest struct {
	ProjectID               string
	SourceEnvironment       EnvironmentKind
	TargetEnvironment       EnvironmentKind
	TargetVersionID         string
	ExpectedActiveVersionID string
	Actor                   string
}

type RollbackRequest struct {
	ProjectID               string
	Environment             EnvironmentKind
	TargetVersionID         string
	ExpectedActiveVersionID string
	Actor                   string
	Reason                  string
}

func (c *Client) ListEnvironments(ctx context.Context, req ListEnvironmentsRequest) ([]Environment, error) {
	if err := c.requireOpen("list_environments"); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, newError(CodeInvalidArgument, "list_environments", "project", "", false, errors.New("project_id is required"))
	}
	items, err := c.app.Release.Environments(ctx, projectID)
	if err != nil {
		return nil, releaseError("list_environments", projectID, err)
	}
	result := make([]Environment, len(items))
	for i := range items {
		result[i] = fromEnvironment(items[i])
	}
	return result, nil
}

func (c *Client) ListReleases(ctx context.Context, req ListReleasesRequest) ([]Release, error) {
	if err := c.requireOpen("list_releases"); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return nil, newError(CodeInvalidArgument, "list_releases", "project", "", false, errors.New("project_id is required"))
	}
	items, err := c.app.Release.Releases(ctx, projectID)
	if err != nil {
		return nil, releaseError("list_releases", projectID, err)
	}
	result := make([]Release, len(items))
	for i := range items {
		result[i] = fromRelease(items[i])
	}
	return result, nil
}

func (c *Client) Promote(ctx context.Context, req PromoteRequest) (Release, error) {
	if err := c.requireOpen("promote"); err != nil {
		return Release{}, err
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return Release{}, newError(CodeInvalidArgument, "promote", "project", "", false, errors.New("project_id is required"))
	}
	item, err := c.app.Release.Promote(ctx, release.PromoteRequest{
		ProjectID: projectID, SourceEnvironment: release.EnvironmentKind(req.SourceEnvironment), TargetEnvironment: release.EnvironmentKind(req.TargetEnvironment),
		TargetVersionID: req.TargetVersionID, ExpectedActiveVersionID: req.ExpectedActiveVersionID, Actor: req.Actor,
	})
	if err != nil {
		return Release{}, releaseError("promote", projectID, err)
	}
	return fromRelease(item), nil
}

func (c *Client) Rollback(ctx context.Context, req RollbackRequest) (Release, error) {
	if err := c.requireOpen("rollback"); err != nil {
		return Release{}, err
	}
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return Release{}, newError(CodeInvalidArgument, "rollback", "project", "", false, errors.New("project_id is required"))
	}
	item, err := c.app.Release.Rollback(ctx, release.RollbackRequest{
		ProjectID: projectID, Environment: release.EnvironmentKind(req.Environment), TargetVersionID: req.TargetVersionID,
		ExpectedActiveVersionID: req.ExpectedActiveVersionID, Actor: req.Actor, Reason: req.Reason,
	})
	if err != nil {
		return Release{}, releaseError("rollback", projectID, err)
	}
	return fromRelease(item), nil
}

func fromEnvironment(item release.Environment) Environment {
	return Environment{ID: item.ID, ProjectID: item.ProjectID, Kind: EnvironmentKind(item.Kind), ActiveVersionID: item.ActiveVersionID, Revision: item.Revision, Bound: item.Bound}
}

func fromRelease(item release.Release) Release {
	return Release{ID: item.ID, ProjectID: item.ProjectID, SourceVersionID: item.SourceVersionID, TargetVersionID: item.TargetVersionID, SourceEnvironment: EnvironmentKind(item.SourceEnvironment), TargetEnvironment: EnvironmentKind(item.TargetEnvironment), Action: item.Action, Actor: item.Actor, Reason: item.Reason, CreatedAt: item.CreatedAt}
}

func releaseError(operation, resource string, err error) error {
	switch {
	case errors.Is(err, release.ErrConflict):
		return newError(CodeConflict, operation, resource, "", false, err)
	case errors.Is(err, release.ErrNotFound):
		return newError(CodeNotFound, operation, resource, "", false, err)
	case errors.Is(err, release.ErrGateFailed), errors.Is(err, release.ErrBindingMissing), errors.Is(err, release.ErrInvalidTransition), errors.Is(err, release.ErrRollbackTarget):
		return newError(CodeInvalidArgument, operation, resource, "", false, err)
	default:
		return wrapError(operation, resource, "", err)
	}
}
