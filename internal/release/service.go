package release

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Environments(ctx context.Context, projectID string) ([]Environment, error) {
	return s.repo.Environments(ctx, projectID)
}
func (s *Service) Releases(ctx context.Context, projectID string) ([]Release, error) {
	return s.repo.Releases(ctx, projectID)
}
func (s *Service) Versions(ctx context.Context, projectID string) ([]Version, error) {
	return s.repo.Versions(ctx, projectID)
}
func (s *Service) Version(ctx context.Context, projectID, versionID string) (Version, error) {
	return s.repo.Version(ctx, projectID, versionID)
}
func (s *Service) CreateVersion(ctx context.Context, version Version) error {
	if version.ProjectID == "" || version.ID == "" || version.ContentHash == "" {
		return fmt.Errorf("%w: project, id, and content hash are required", ErrInvalidTransition)
	}
	if version.CreatedAt.IsZero() {
		version.CreatedAt = s.now()
	}
	return s.repo.CreateVersion(ctx, version)
}
func (s *Service) Validate(ctx context.Context, projectID, versionID string, evidence Evidence) error {
	if projectID == "" || versionID == "" || evidence.EnvironmentID == "" {
		return fmt.Errorf("%w: project, version, and environment are required", ErrGateFailed)
	}
	version, err := s.repo.Version(ctx, projectID, versionID)
	if err != nil {
		return err
	}
	if evidence.VersionID == "" {
		evidence.VersionID = versionID
	}
	evidence.ProjectID = projectID
	if evidence.VersionID != versionID || evidence.ContentHash != version.ContentHash {
		return fmt.Errorf("%w: evidence content hash does not match version", ErrGateFailed)
	}
	return s.repo.SaveEvidence(ctx, evidence)
}

// ActivateDevelopment makes an evaluated immutable version active in
// development. It deliberately has no environment input so callers cannot use
// it to skip the ordered development-to-staging-to-production lifecycle.
func (s *Service) ActivateDevelopment(ctx context.Context, req ActivateRequest) (Release, error) {
	if req.ProjectID == "" || req.TargetVersionID == "" || req.Actor == "" {
		return Release{}, fmt.Errorf("%w: project, version, and actor are required", ErrInvalidTransition)
	}
	environment, err := s.repo.Environment(ctx, req.ProjectID, Development)
	if err != nil {
		return Release{}, err
	}
	if environment.ActiveVersionID != req.ExpectedActiveVersionID {
		return Release{}, fmt.Errorf("%w: expected active version %q, current %q", ErrConflict, req.ExpectedActiveVersionID, environment.ActiveVersionID)
	}
	version, err := s.repo.Version(ctx, req.ProjectID, req.TargetVersionID)
	if err != nil {
		return Release{}, err
	}
	if version.ProjectID != req.ProjectID {
		return Release{}, ErrNotFound
	}
	if version.PipelineID == "" || len(version.Definition) == 0 {
		return Release{}, fmt.Errorf("%w: a frozen pipeline definition is required", ErrGateFailed)
	}
	if !environment.Bound {
		return Release{}, ErrBindingMissing
	}
	evidence, err := s.repo.Evidence(ctx, req.ProjectID, version.ID, Development)
	if err != nil {
		return Release{}, err
	}
	if !evidence.Passed || evidence.ContentHash != version.ContentHash {
		return Release{}, fmt.Errorf("%w: successful %s evidence is required", ErrGateFailed, Development)
	}
	record := Release{ID: id.New("rel"), ProjectID: req.ProjectID, SourceVersionID: environment.ActiveVersionID, TargetVersionID: version.ID, SourceEnvironment: Development, TargetEnvironment: Development, Action: ActionActivate, Actor: req.Actor, CreatedAt: s.now()}
	environment.ActiveVersionID = version.ID
	environment.Revision++
	if err := s.repo.Commit(ctx, environment, record); err != nil {
		return Release{}, err
	}
	return record, nil
}

func (s *Service) Promote(ctx context.Context, req PromoteRequest) (Release, error) {
	if req.ProjectID == "" || req.TargetVersionID == "" || req.Actor == "" {
		return Release{}, fmt.Errorf("%w: project, version, and actor are required", ErrInvalidTransition)
	}
	if !validPromotion(req.SourceEnvironment, req.TargetEnvironment) {
		return Release{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, req.SourceEnvironment, req.TargetEnvironment)
	}
	source, err := s.repo.Environment(ctx, req.ProjectID, req.SourceEnvironment)
	if err != nil {
		return Release{}, err
	}
	target, err := s.repo.Environment(ctx, req.ProjectID, req.TargetEnvironment)
	if err != nil {
		return Release{}, err
	}
	if source.ActiveVersionID == "" || source.ActiveVersionID != req.TargetVersionID {
		return Release{}, fmt.Errorf("%w: source environment is not active on target version", ErrInvalidTransition)
	}
	if target.ActiveVersionID != req.ExpectedActiveVersionID {
		return Release{}, fmt.Errorf("%w: expected active version %q, current %q", ErrConflict, req.ExpectedActiveVersionID, target.ActiveVersionID)
	}
	version, err := s.repo.Version(ctx, req.ProjectID, req.TargetVersionID)
	if err != nil {
		return Release{}, err
	}
	if version.ProjectID != req.ProjectID {
		return Release{}, ErrNotFound
	}
	if version.PipelineID == "" || len(version.Definition) == 0 {
		return Release{}, fmt.Errorf("%w: a frozen pipeline definition is required", ErrGateFailed)
	}
	if !target.Bound {
		return Release{}, ErrBindingMissing
	}
	evidence, err := s.repo.Evidence(ctx, req.ProjectID, version.ID, req.TargetEnvironment)
	if err != nil {
		return Release{}, err
	}
	if !evidence.Passed || evidence.ContentHash != version.ContentHash {
		return Release{}, fmt.Errorf("%w: successful %s evidence is required", ErrGateFailed, req.TargetEnvironment)
	}
	record := Release{ID: id.New("rel"), ProjectID: req.ProjectID, SourceVersionID: source.ActiveVersionID, TargetVersionID: version.ID, SourceEnvironment: req.SourceEnvironment, TargetEnvironment: req.TargetEnvironment, Action: ActionPromote, Actor: req.Actor, CreatedAt: s.now()}
	target.ActiveVersionID = version.ID
	target.Revision++
	if err := s.repo.Commit(ctx, target, record); err != nil {
		return Release{}, err
	}
	return record, nil
}

func (s *Service) Rollback(ctx context.Context, req RollbackRequest) (Release, error) {
	if req.ProjectID == "" || req.TargetVersionID == "" || req.Actor == "" || strings.TrimSpace(req.Reason) == "" {
		return Release{}, fmt.Errorf("%w: project, version, actor, and reason are required", ErrRollbackTarget)
	}
	if req.Environment != Development && req.Environment != Staging && req.Environment != Production {
		return Release{}, fmt.Errorf("%w: unknown environment", ErrRollbackTarget)
	}
	env, err := s.repo.Environment(ctx, req.ProjectID, req.Environment)
	if err != nil {
		return Release{}, err
	}
	if env.ActiveVersionID != req.ExpectedActiveVersionID {
		return Release{}, fmt.Errorf("%w: expected active version %q, current %q", ErrConflict, req.ExpectedActiveVersionID, env.ActiveVersionID)
	}
	if env.ActiveVersionID == req.TargetVersionID {
		return Release{}, fmt.Errorf("%w: target is already active", ErrRollbackTarget)
	}
	version, err := s.repo.Version(ctx, req.ProjectID, req.TargetVersionID)
	if err != nil {
		return Release{}, err
	}
	valid, err := s.repo.PreviouslyValidated(ctx, req.ProjectID, version.ID, req.Environment)
	if err != nil {
		return Release{}, err
	}
	if !valid {
		return Release{}, fmt.Errorf("%w: target version was not validated in %s", ErrRollbackTarget, req.Environment)
	}
	if !env.Bound {
		return Release{}, ErrBindingMissing
	}
	record := Release{ID: id.New("rel"), ProjectID: req.ProjectID, SourceVersionID: env.ActiveVersionID, TargetVersionID: version.ID, SourceEnvironment: req.Environment, TargetEnvironment: req.Environment, Action: ActionRollback, Actor: req.Actor, Reason: strings.TrimSpace(req.Reason), CreatedAt: s.now()}
	env.ActiveVersionID = version.ID
	env.Revision++
	if err := s.repo.Commit(ctx, env, record); err != nil {
		return Release{}, err
	}
	return record, nil
}

func validPromotion(source, target EnvironmentKind) bool {
	return (source == Development && target == Staging) || (source == Staging && target == Production)
}
