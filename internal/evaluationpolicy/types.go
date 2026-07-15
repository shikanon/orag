// Package evaluationpolicy owns immutable, project-scoped evaluation policies
// and the server-derived evidence produced from completed evaluation runs.
package evaluationpolicy

import (
	"context"
	"errors"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
)

type Comparator string

const (
	ComparatorGreaterThanOrEqual Comparator = "gte"
	ComparatorLessThanOrEqual    Comparator = "lte"
)

var (
	ErrInvalidPolicy      = errors.New("invalid evaluation policy")
	ErrDatasetNotFound    = errors.New("evaluation policy dataset not found")
	ErrPolicyNotFound     = errors.New("evaluation policy not found")
	ErrEvaluationMismatch = errors.New("evaluation run does not match policy")
)

// Gate describes one non-overridable metric condition. Gates are copied into
// evidence at evaluation time so later policy edits cannot alter old results.
type Gate struct {
	Metric     string     `json:"metric"`
	Comparator Comparator `json:"comparator"`
	Threshold  float64    `json:"threshold"`
}

// Policy is immutable. Revisions are created as new records rather than by
// mutating a policy that may already have release evidence.
type Policy struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	ProjectID string    `json:"project_id"`
	DatasetID string    `json:"dataset_id"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	Gates     []Gate    `json:"gates"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateInput struct {
	DatasetID string `json:"dataset_id"`
	Name      string `json:"name"`
	Gates     []Gate `json:"gates"`
}

// GateResult includes the value observed by the evaluation runner. Passed is
// calculated by the server and never accepted as a client input.
type GateResult struct {
	Metric     string     `json:"metric"`
	Comparator Comparator `json:"comparator"`
	Threshold  float64    `json:"threshold"`
	Actual     float64    `json:"actual,omitempty"`
	Present    bool       `json:"present"`
	Passed     bool       `json:"passed"`
}

// FrozenInput is the reproducibility envelope used for release evidence.
// It snapshots both the immutable policy and the completed evaluation result.
type FrozenInput struct {
	PolicyID        string             `json:"policy_id"`
	PolicyVersion   int                `json:"policy_version"`
	ProjectID       string             `json:"project_id"`
	DatasetID       string             `json:"dataset_id"`
	EvaluationRunID string             `json:"evaluation_run_id"`
	PipelineVersion string             `json:"pipeline_version_id"`
	ContentHash     string             `json:"content_hash"`
	Environment     string             `json:"environment,omitempty"`
	Gates           []Gate             `json:"gates"`
	Metrics         map[string]float64 `json:"metrics"`
}

// Evidence is append-only server-derived gate evidence. It is intentionally
// separate from the legacy manual release validation record.
type Evidence struct {
	ID                string       `json:"id"`
	TenantID          string       `json:"tenant_id"`
	ProjectID         string       `json:"project_id"`
	PolicyID          string       `json:"policy_id"`
	PolicyVersion     int          `json:"policy_version"`
	EvaluationRunID   string       `json:"evaluation_run_id"`
	PipelineVersionID string       `json:"pipeline_version_id"`
	ContentHash       string       `json:"content_hash"`
	Environment       string       `json:"environment,omitempty"`
	FrozenInput       FrozenInput  `json:"frozen_input"`
	GateResults       []GateResult `json:"gate_results"`
	Passed            bool         `json:"passed"`
	CreatedAt         time.Time    `json:"created_at"`
}

type DatasetResolver interface {
	GetInProject(ctx context.Context, tenantID, projectID, datasetID string) (dataset.Dataset, bool, error)
}

type Repository interface {
	Create(ctx context.Context, policy Policy) error
	Get(ctx context.Context, tenantID, projectID, policyID string) (Policy, error)
	List(ctx context.Context, tenantID, projectID string) ([]Policy, error)
	RecordEvidence(ctx context.Context, evidence Evidence) error
}

// MetricRegistry is deliberately narrow to retain a stable contract while
// allowing tests and future metric registries to remain independent.
type MetricRegistry interface {
	IsRegistered(name string) bool
}

var _ MetricRegistry = eval.DefaultMetricRegistry
