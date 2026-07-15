package tutorial

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/project"
)

var (
	ErrCloneSubjectRequired  = errors.New("tutorial clone tenant and subject are required")
	ErrCloneProjectName      = errors.New("tutorial clone project name is required")
	ErrCloneIdempotencyKey   = errors.New("tutorial clone idempotency key is required")
	ErrCloneLicenseRequired  = errors.New("tutorial clone pack license must be accepted")
	ErrCloneJobNotFound      = errors.New("tutorial clone job not found")
	ErrCloneExperimentAbsent = errors.New("tutorial experiment not found")
	ErrCloneNotRetryable     = errors.New("tutorial clone job is not retryable")
)

type CloneStage string

const (
	CloneStageCreateProject    CloneStage = "create_project"
	CloneStageValidateManifest CloneStage = "validate_manifest"
	CloneStageDownloadPack     CloneStage = "download_pack"
	CloneStageVerifyPack       CloneStage = "verify_pack"
	CloneStageWritePrivate     CloneStage = "write_private_store"
	CloneStageCreateResources  CloneStage = "create_runtime_resources"
	CloneStagePackInstalled    CloneStage = "pack_installed"
)

type CloneStatus string

const (
	CloneStatusQueued    CloneStatus = "queued"
	CloneStatusRunning   CloneStatus = "running"
	CloneStatusFailed    CloneStatus = "failed"
	CloneStatusCompleted CloneStatus = "completed"
)

type PackStatus string

const (
	PackStatusPending    PackStatus = "pending"
	PackStatusInstalling PackStatus = "installing"
	PackStatusInstalled  PackStatus = "pack_installed"
	PackStatusFailed     PackStatus = "failed"
)

type Subject struct {
	TenantID string
	ID       string
}

type TemplateRef struct {
	TemplateID string
	Version    string
	Tier       string
}

type CloneRequest struct {
	TemplateID         string
	Version            string
	Tier               string
	ProjectName        string
	ProjectDescription string
	IdempotencyKey     string
	LicenseAccepted    bool
}

type CloneJob struct {
	ID                 string       `json:"id"`
	TenantID           string       `json:"tenant_id"`
	SubjectID          string       `json:"-"`
	ProjectID          string       `json:"project_id"`
	ProjectName        string       `json:"project_name"`
	ProjectDescription string       `json:"project_description"`
	TemplateID         string       `json:"template_id"`
	TemplateVersion    string       `json:"template_version"`
	Tier               string       `json:"pack_tier"`
	IdempotencyKey     string       `json:"-"`
	Stage              CloneStage   `json:"stage"`
	Status             CloneStatus  `json:"status"`
	Attempt            int          `json:"attempt"`
	LastErrorCode      string       `json:"failure_code,omitempty"`
	Events             []StageEvent `json:"events"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

type StageEvent struct {
	Stage      CloneStage `json:"stage"`
	Outcome    string     `json:"outcome"`
	DetailCode string     `json:"detail_code,omitempty"`
	OccurredAt time.Time  `json:"occurred_at"`
}

type Experiment struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	ProjectID       string     `json:"project_id"`
	CloneJobID      string     `json:"-"`
	TemplateID      string     `json:"template_id"`
	TemplateVersion string     `json:"template_version"`
	Tier            string     `json:"pack_tier"`
	PackStatus      PackStatus `json:"pack_status"`
	RuntimeStatus   string     `json:"runtime_status"`
	KnowledgeBaseID string     `json:"knowledge_base_id,omitempty"`
	DatasetID       string     `json:"dataset_id,omitempty"`
	BaselineProfile string     `json:"baseline_profile,omitempty"`
	BaselineTopK    int        `json:"baseline_top_k,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// CloneRepository persists requests before any project or remote Pack action
// starts. Its compare-and-swap methods are used by the worker added in the
// installation slice.
type CloneRepository interface {
	CreateOrGet(context.Context, CloneJob) (CloneJob, bool, error)
	GetJob(context.Context, string, string) (CloneJob, bool, error)
	Retry(context.Context, string, string, CloneStage, time.Time) (CloneJob, bool, error)
	GetExperiment(context.Context, string, string) (Experiment, bool, error)
	Acquire(context.Context, string, string, time.Time) (CloneJob, bool, error)
	Advance(context.Context, string, string, CloneStage, CloneStage, CloneStatus, time.Time) (CloneJob, bool, error)
	Fail(context.Context, string, string, CloneStage, string, time.Time) (CloneJob, bool, error)
	EnsureExperiment(context.Context, Experiment) error
	SetExperimentStatus(context.Context, string, string, PackStatus, time.Time) error
	SetExperimentRuntime(context.Context, string, string, RuntimeResources, time.Time) error
	RecoverPending(context.Context, time.Time) ([]CloneJob, error)
}

type CloneProjectService interface {
	CreateWithID(context.Context, string, string, project.CreateInput) (project.Project, error)
	Get(context.Context, string, string) (project.Project, error)
}

// RuntimeResources are derived by the server from a verified Pack declaration.
// They are stable project identifiers, never browser inputs.
type RuntimeResources struct {
	Status          string
	KnowledgeBaseID string
	DatasetID       string
	BaselineProfile string
	BaselineTopK    int
}

type RuntimeInitializer interface {
	Ensure(context.Context, CloneJob, Manifest) (RuntimeResources, error)
}

type CloneService struct {
	catalog  *Catalog
	repo     CloneRepository
	projects CloneProjectService
	reader   *PublicPackReader
	private  PrivateStore
	runtime  RuntimeInitializer
	now      func() time.Time
	newID    func(string) string
}

func (s *CloneService) ConfigureRuntime(initializer RuntimeInitializer) {
	if s != nil {
		s.runtime = initializer
	}
}

func (s *CloneService) ConfigureInstaller(projects CloneProjectService, reader *PublicPackReader, private PrivateStore) {
	if s == nil {
		return
	}
	s.projects = projects
	s.reader = reader
	s.private = private
}

func NewCloneService(catalog *Catalog, repo CloneRepository, now func() time.Time) *CloneService {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &CloneService{catalog: catalog, repo: repo, now: now, newID: id.New}
}

func (s *CloneService) Start(ctx context.Context, subject Subject, input CloneRequest) (CloneJob, bool, error) {
	if s == nil || s.catalog == nil || s.repo == nil {
		return CloneJob{}, false, errors.New("tutorial clone service is unavailable")
	}
	subject = normalizeSubject(subject)
	if subject.TenantID == "" || subject.ID == "" {
		return CloneJob{}, false, ErrCloneSubjectRequired
	}
	input = normalizeCloneRequest(input)
	if input.ProjectName == "" {
		return CloneJob{}, false, ErrCloneProjectName
	}
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > 200 {
		return CloneJob{}, false, ErrCloneIdempotencyKey
	}
	template, err := s.catalog.Get(input.TemplateID, input.Version)
	if err != nil {
		return CloneJob{}, false, err
	}
	pack, ok := templatePack(template, input.Tier)
	if !ok {
		return CloneJob{}, false, fmt.Errorf("%w: pack tier is unavailable", ErrManifestInvalid)
	}
	if pack.RequiresLicenseCheck && !input.LicenseAccepted {
		return CloneJob{}, false, ErrCloneLicenseRequired
	}
	now := s.now().UTC()
	job := CloneJob{
		ID:                 s.newID("tclj"),
		TenantID:           subject.TenantID,
		SubjectID:          subject.ID,
		ProjectID:          s.newID("prj"),
		ProjectName:        input.ProjectName,
		ProjectDescription: input.ProjectDescription,
		TemplateID:         template.ID,
		TemplateVersion:    template.Version,
		Tier:               pack.Tier,
		IdempotencyKey:     input.IdempotencyKey,
		Stage:              CloneStageCreateProject,
		Status:             CloneStatusQueued,
		Attempt:            1,
		Events: []StageEvent{{
			Stage: CloneStageCreateProject, Outcome: "queued", OccurredAt: now,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	return s.repo.CreateOrGet(ctx, job)
}

func (s *CloneService) GetJob(ctx context.Context, subject Subject, jobID string) (CloneJob, error) {
	subject = normalizeSubject(subject)
	job, found, err := s.repo.GetJob(ctx, subject.TenantID, strings.TrimSpace(jobID))
	if err != nil {
		return CloneJob{}, err
	}
	if !found {
		return CloneJob{}, ErrCloneJobNotFound
	}
	return job, nil
}

func (s *CloneService) Retry(ctx context.Context, subject Subject, jobID string) (CloneJob, error) {
	subject = normalizeSubject(subject)
	job, err := s.GetJob(ctx, subject, jobID)
	if err != nil {
		return CloneJob{}, err
	}
	if job.Status != CloneStatusFailed {
		return CloneJob{}, ErrCloneNotRetryable
	}
	retryStage := resumeStage(job.Stage)
	updated, changed, err := s.repo.Retry(ctx, subject.TenantID, job.ID, retryStage, s.now().UTC())
	if err != nil {
		return CloneJob{}, err
	}
	if !changed {
		return CloneJob{}, ErrCloneNotRetryable
	}
	return updated, nil
}

func (s *CloneService) GetExperiment(ctx context.Context, subject Subject, projectID string) (Experiment, error) {
	subject = normalizeSubject(subject)
	experiment, found, err := s.repo.GetExperiment(ctx, subject.TenantID, strings.TrimSpace(projectID))
	if err != nil {
		return Experiment{}, err
	}
	if !found {
		return Experiment{}, ErrCloneExperimentAbsent
	}
	return experiment, nil
}

// RecoverPending returns work that was queued before process startup and
// safely requeues work that the previous process left in an in-flight state.
// The runner owns scheduling; this method never performs a transfer itself.
func (s *CloneService) RecoverPending(ctx context.Context) ([]CloneJob, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("tutorial clone service is unavailable")
	}
	return s.repo.RecoverPending(ctx, s.now().UTC())
}

// Run advances a single durable job until it reaches a terminal state or loses
// its compare-and-swap claim. Duplicate schedules from the same worker
// lifecycle are therefore benign.
func (s *CloneService) Run(ctx context.Context, subject Subject, jobID string) error {
	if s == nil || s.repo == nil || s.projects == nil || s.reader == nil || s.private == nil {
		return ErrPrivateStoreConfiguration
	}
	subject = normalizeSubject(subject)
	for {
		job, claimed, err := s.repo.Acquire(ctx, subject.TenantID, strings.TrimSpace(jobID), s.now().UTC())
		if err != nil || !claimed {
			return err
		}
		switch job.Stage {
		case CloneStageCreateProject:
			if err := s.ensureProjectAndExperiment(ctx, job); err != nil {
				return s.fail(ctx, job, err)
			}
			if _, advanced, err := s.repo.Advance(ctx, job.TenantID, job.ID, CloneStageCreateProject, CloneStageValidateManifest, CloneStatusQueued, s.now().UTC()); err != nil || !advanced {
				return err
			}
		case CloneStageValidateManifest:
			if _, err := s.fetchManifest(ctx, job); err != nil {
				return s.fail(ctx, job, err)
			}
			if _, advanced, err := s.repo.Advance(ctx, job.TenantID, job.ID, CloneStageValidateManifest, CloneStageDownloadPack, CloneStatusQueued, s.now().UTC()); err != nil || !advanced {
				return err
			}
		case CloneStageDownloadPack:
			manifest, err := s.fetchManifest(ctx, job)
			if err == nil {
				err = s.downloadAndVerifyPack(ctx, job, manifest)
			}
			if err != nil {
				return s.fail(ctx, job, err)
			}
			if _, advanced, err := s.repo.Advance(ctx, job.TenantID, job.ID, CloneStageDownloadPack, CloneStageVerifyPack, CloneStatusQueued, s.now().UTC()); err != nil || !advanced {
				return err
			}
		case CloneStageVerifyPack:
			// FetchObject verifies the content type, declared byte count and SHA-256
			// before its temporary file is returned. Keep this durable stage explicit
			// so a caller can distinguish acquisition from a trusted install attempt.
			if _, advanced, err := s.repo.Advance(ctx, job.TenantID, job.ID, CloneStageVerifyPack, CloneStageWritePrivate, CloneStatusQueued, s.now().UTC()); err != nil || !advanced {
				return err
			}
		case CloneStageWritePrivate:
			manifest, err := s.fetchManifest(ctx, job)
			if err == nil {
				err = s.installVerifiedPack(ctx, job, manifest)
			}
			if err != nil {
				return s.fail(ctx, job, err)
			}
			if _, advanced, err := s.repo.Advance(ctx, job.TenantID, job.ID, CloneStageWritePrivate, CloneStageCreateResources, CloneStatusQueued, s.now().UTC()); err != nil || !advanced {
				return err
			}
		case CloneStageCreateResources:
			manifest, err := s.fetchManifest(ctx, job)
			if err != nil {
				return s.fail(ctx, job, err)
			}
			resources := RuntimeResources{Status: "runtime_unavailable"}
			if manifest.Runtime != nil && s.runtime != nil {
				resources, err = s.runtime.Ensure(ctx, job, manifest)
				if err != nil {
					return s.fail(ctx, job, err)
				}
			}
			if err := s.repo.SetExperimentRuntime(ctx, job.TenantID, job.ProjectID, resources, s.now().UTC()); err != nil {
				return s.fail(ctx, job, err)
			}
			if err := s.repo.SetExperimentStatus(ctx, job.TenantID, job.ProjectID, PackStatusInstalled, s.now().UTC()); err != nil {
				return s.fail(ctx, job, err)
			}
			if _, advanced, err := s.repo.Advance(ctx, job.TenantID, job.ID, CloneStageCreateResources, CloneStagePackInstalled, CloneStatusCompleted, s.now().UTC()); err != nil || !advanced {
				return err
			}
			return nil
		default:
			return s.fail(ctx, job, fmt.Errorf("unsupported clone stage %q", job.Stage))
		}
	}
}

func (s *CloneService) ensureProjectAndExperiment(ctx context.Context, job CloneJob) error {
	if _, err := s.projects.Get(ctx, job.TenantID, job.ProjectID); err != nil {
		if !errors.Is(err, project.ErrNotFound) {
			return err
		}
		if _, err := s.projects.CreateWithID(ctx, job.TenantID, job.ProjectID, project.CreateInput{Name: job.ProjectName, Description: job.ProjectDescription}); err != nil {
			return err
		}
	}
	now := s.now().UTC()
	return s.repo.EnsureExperiment(ctx, Experiment{
		ID: s.newID("texp"), TenantID: job.TenantID, ProjectID: job.ProjectID,
		TemplateID: job.TemplateID, TemplateVersion: job.TemplateVersion, Tier: job.Tier,
		CloneJobID: job.ID, PackStatus: PackStatusInstalling, RuntimeStatus: "pending", CreatedAt: now, UpdatedAt: now,
	})
}

func (s *CloneService) fetchManifest(ctx context.Context, job CloneJob) (Manifest, error) {
	template, err := s.catalog.Get(job.TemplateID, job.TemplateVersion)
	if err != nil {
		return Manifest{}, err
	}
	pack, ok := templatePack(template, job.Tier)
	if !ok {
		return Manifest{}, ErrManifestInvalid
	}
	raw, err := s.reader.FetchManifest(ctx, pack.ManifestPath)
	if err != nil {
		return Manifest{}, err
	}
	return ParseManifest(raw, template, pack)
}

func (s *CloneService) downloadAndVerifyPack(ctx context.Context, job CloneJob, manifest Manifest) error {
	template, err := s.catalog.Get(job.TemplateID, job.TemplateVersion)
	if err != nil {
		return err
	}
	pack, ok := templatePack(template, job.Tier)
	if !ok {
		return ErrManifestInvalid
	}
	for _, item := range manifest.Objects {
		object, err := s.reader.FetchObject(ctx, pack.ManifestPath, item)
		if err != nil {
			return err
		}
		if err := object.Remove(); err != nil {
			return err
		}
	}
	return nil
}

func (s *CloneService) installVerifiedPack(ctx context.Context, job CloneJob, manifest Manifest) error {
	template, err := s.catalog.Get(job.TemplateID, job.TemplateVersion)
	if err != nil {
		return err
	}
	pack, ok := templatePack(template, job.Tier)
	if !ok {
		return ErrManifestInvalid
	}
	for _, item := range manifest.Objects {
		object, err := s.reader.FetchObject(ctx, pack.ManifestPath, item)
		if err != nil {
			return err
		}
		err = s.private.PutVerified(ctx, PrivateObject{TenantID: job.TenantID, ProjectID: job.ProjectID, JobID: job.ID, Object: object})
		removeErr := object.Remove()
		if err != nil {
			return err
		}
		if removeErr != nil {
			return removeErr
		}
	}
	return nil
}

func (s *CloneService) fail(ctx context.Context, job CloneJob, cause error) error {
	code := cloneFailureCode(cause)
	_, _, transitionErr := s.repo.Fail(ctx, job.TenantID, job.ID, job.Stage, code, s.now().UTC())
	if transitionErr != nil {
		return transitionErr
	}
	return cause
}

func cloneFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrPublicPackResponse):
		return "public_pack_unavailable"
	case errors.Is(err, ErrPublicPackChecksum):
		return "object_checksum_mismatch"
	case errors.Is(err, ErrPublicPackSize):
		return "object_size_invalid"
	case errors.Is(err, ErrPublicPackContentType):
		return "object_content_type_invalid"
	case errors.Is(err, ErrPrivateStoreConfiguration):
		return "storage_not_configured"
	case errors.Is(err, ErrPrivateStoreWrite):
		return "private_storage_write_failed"
	case errors.Is(err, ErrManifestInvalid):
		return "manifest_invalid"
	default:
		return "clone_failed"
	}
}

func normalizeSubject(subject Subject) Subject {
	subject.TenantID = strings.TrimSpace(subject.TenantID)
	subject.ID = strings.TrimSpace(subject.ID)
	return subject
}

func normalizeCloneRequest(input CloneRequest) CloneRequest {
	input.TemplateID = strings.TrimSpace(input.TemplateID)
	input.Version = strings.TrimSpace(input.Version)
	input.Tier = strings.TrimSpace(input.Tier)
	input.ProjectName = strings.TrimSpace(input.ProjectName)
	input.ProjectDescription = strings.TrimSpace(input.ProjectDescription)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	return input
}

func templatePack(template Template, tier string) (PackRef, bool) {
	for _, pack := range template.Packs {
		if pack.Tier == tier {
			return pack, true
		}
	}
	return PackRef{}, false
}

func resumeStage(stage CloneStage) CloneStage {
	switch stage {
	case CloneStageDownloadPack, CloneStageVerifyPack, CloneStageWritePrivate:
		return CloneStageDownloadPack
	case CloneStagePackInstalled:
		return CloneStagePackInstalled
	default:
		return stage
	}
}
