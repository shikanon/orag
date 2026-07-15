package evaluationpolicy

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/platform/id"
)

type Service struct {
	repo     Repository
	datasets DatasetResolver
	metrics  MetricRegistry
	now      func() time.Time
}

func NewService(repo Repository, datasets DatasetResolver, metrics MetricRegistry) *Service {
	if metrics == nil {
		metrics = eval.DefaultMetricRegistry
	}
	return &Service{repo: repo, datasets: datasets, metrics: metrics, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Create(ctx context.Context, tenantID, projectID string, input CreateInput) (Policy, error) {
	tenantID = strings.TrimSpace(tenantID)
	projectID = strings.TrimSpace(projectID)
	input.DatasetID = strings.TrimSpace(input.DatasetID)
	input.Name = strings.TrimSpace(input.Name)
	if tenantID == "" || projectID == "" || input.DatasetID == "" || input.Name == "" {
		return Policy{}, fmt.Errorf("%w: tenant, project, dataset, and name are required", ErrInvalidPolicy)
	}
	if err := s.validateGates(input.Gates); err != nil {
		return Policy{}, err
	}
	if s.datasets == nil {
		return Policy{}, fmt.Errorf("%w: dataset resolver is required", ErrInvalidPolicy)
	}
	if _, found, err := s.datasets.GetInProject(ctx, tenantID, projectID, input.DatasetID); err != nil {
		return Policy{}, err
	} else if !found {
		return Policy{}, ErrDatasetNotFound
	}
	if s.repo == nil {
		return Policy{}, fmt.Errorf("%w: repository is required", ErrInvalidPolicy)
	}
	version, err := s.nextVersion(ctx, tenantID, projectID, input.Name)
	if err != nil {
		return Policy{}, err
	}
	policy := Policy{
		ID: id.New("epol"), TenantID: tenantID, ProjectID: projectID, DatasetID: input.DatasetID,
		Name: input.Name, Version: version, Gates: cloneGates(input.Gates), CreatedAt: s.now(),
	}
	if err := s.repo.Create(ctx, policy); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func (s *Service) nextVersion(ctx context.Context, tenantID, projectID, name string) (int, error) {
	policies, err := s.repo.List(ctx, tenantID, projectID)
	if err != nil {
		return 0, err
	}
	version := 1
	for _, existing := range policies {
		if existing.Name == name && existing.Version >= version {
			version = existing.Version + 1
		}
	}
	return version, nil
}

func (s *Service) Get(ctx context.Context, tenantID, projectID, policyID string) (Policy, error) {
	if s.repo == nil {
		return Policy{}, fmt.Errorf("%w: repository is required", ErrInvalidPolicy)
	}
	return s.repo.Get(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(projectID), strings.TrimSpace(policyID))
}

func (s *Service) List(ctx context.Context, tenantID, projectID string) ([]Policy, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidPolicy)
	}
	return s.repo.List(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(projectID))
}

// Freeze converts a completed evaluation into reproducible evidence. The
// caller supplies only identities; the pass/fail decision is derived from the
// recorded server-side metrics and the immutable policy gates.
func Freeze(policy Policy, run eval.RunResult, pipelineVersionID, contentHash string, now time.Time) (Evidence, error) {
	pipelineVersionID = strings.TrimSpace(pipelineVersionID)
	contentHash = strings.TrimSpace(contentHash)
	if policy.ID == "" || policy.ProjectID == "" || policy.DatasetID == "" || policy.Version < 1 {
		return Evidence{}, fmt.Errorf("%w: policy identity is incomplete", ErrInvalidPolicy)
	}
	if run.ID == "" || run.ProjectID != policy.ProjectID || run.DatasetID != policy.DatasetID {
		return Evidence{}, fmt.Errorf("%w: policy and evaluation run must share project and dataset", ErrEvaluationMismatch)
	}
	if pipelineVersionID == "" || contentHash == "" {
		return Evidence{}, fmt.Errorf("%w: pipeline version and content hash are required", ErrEvaluationMismatch)
	}
	metrics := cloneMetrics(run.Metrics)
	results := make([]GateResult, 0, len(policy.Gates))
	passed := true
	for _, gate := range policy.Gates {
		actual, present := metricValue(run, gate.Metric)
		result := GateResult{Metric: gate.Metric, Comparator: gate.Comparator, Threshold: gate.Threshold, Actual: actual, Present: present, Passed: present && compare(actual, gate.Comparator, gate.Threshold)}
		if !result.Passed {
			passed = false
		}
		results = append(results, result)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	frozen := FrozenInput{PolicyID: policy.ID, PolicyVersion: policy.Version, ProjectID: policy.ProjectID, DatasetID: policy.DatasetID, EvaluationRunID: run.ID, PipelineVersion: pipelineVersionID, ContentHash: contentHash, Gates: cloneGates(policy.Gates), Metrics: metrics}
	return Evidence{ID: id.New("epev"), TenantID: policy.TenantID, ProjectID: policy.ProjectID, PolicyID: policy.ID, PolicyVersion: policy.Version, EvaluationRunID: run.ID, PipelineVersionID: pipelineVersionID, ContentHash: contentHash, FrozenInput: frozen, GateResults: results, Passed: passed, CreatedAt: now}, nil
}

func (s *Service) RecordEvidence(ctx context.Context, evidence Evidence) error {
	if s.repo == nil {
		return fmt.Errorf("%w: repository is required", ErrInvalidPolicy)
	}
	return s.repo.RecordEvidence(ctx, evidence)
}

func (s *Service) validateGates(gates []Gate) error {
	if len(gates) == 0 {
		return fmt.Errorf("%w: at least one gate is required", ErrInvalidPolicy)
	}
	seen := make(map[string]struct{}, len(gates))
	for _, gate := range gates {
		metric := strings.TrimSpace(gate.Metric)
		if metric == "" || !s.metrics.IsRegistered(metric) {
			return fmt.Errorf("%w: unknown metric %q", ErrInvalidPolicy, gate.Metric)
		}
		if gate.Comparator != ComparatorGreaterThanOrEqual && gate.Comparator != ComparatorLessThanOrEqual {
			return fmt.Errorf("%w: unsupported comparator %q", ErrInvalidPolicy, gate.Comparator)
		}
		if math.IsNaN(gate.Threshold) || math.IsInf(gate.Threshold, 0) {
			return fmt.Errorf("%w: metric threshold must be finite", ErrInvalidPolicy)
		}
		if _, exists := seen[metric]; exists {
			return fmt.Errorf("%w: duplicate metric gate %q", ErrInvalidPolicy, metric)
		}
		seen[metric] = struct{}{}
	}
	return nil
}

func metricValue(run eval.RunResult, metric string) (float64, bool) {
	if actual, found := run.Metrics[metric]; found {
		return actual, true
	}
	switch metric {
	case "accuracy", "answer_accuracy":
		return run.Accuracy, true
	case "hit_rate":
		return run.HitRate, true
	case "weighted_sample_count":
		return run.WeightedSampleCount, true
	case "unweighted_sample_count":
		return float64(run.UnweightedSampleCount), true
	}
	return 0, false
}

func compare(actual float64, comparator Comparator, threshold float64) bool {
	switch comparator {
	case ComparatorGreaterThanOrEqual:
		return actual >= threshold
	case ComparatorLessThanOrEqual:
		return actual <= threshold
	default:
		return false
	}
}

func cloneGates(in []Gate) []Gate { return append([]Gate(nil), in...) }

func cloneMetrics(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
