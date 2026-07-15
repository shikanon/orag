package tutorial

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
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
	TemplateID      string     `json:"template_id"`
	TemplateVersion string     `json:"template_version"`
	Tier            string     `json:"pack_tier"`
	PackStatus      PackStatus `json:"pack_status"`
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
}

type CloneService struct {
	catalog *Catalog
	repo    CloneRepository
	now     func() time.Time
	newID   func(string) string
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
	case CloneStageVerifyPack:
		return CloneStageDownloadPack
	case CloneStagePackInstalled:
		return CloneStagePackInstalled
	default:
		return stage
	}
}
