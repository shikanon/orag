package evaluationpolicy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
)

func TestCreateRejectsInvalidPolicyInputs(t *testing.T) {
	resolver := datasetResolver{datasets: map[string]dataset.Dataset{"ds_1": {ID: "ds_1", TenantID: "tenant_1", ProjectID: "prj_1"}}}
	service := NewService(&memoryRepository{}, resolver, eval.DefaultMetricRegistry)
	for _, test := range []struct {
		name  string
		input CreateInput
	}{
		{name: "empty dataset", input: CreateInput{Name: "Quality", Gates: []Gate{{Metric: "answer_accuracy", Comparator: ComparatorGreaterThanOrEqual, Threshold: 0.8}}}},
		{name: "unknown metric", input: CreateInput{DatasetID: "ds_1", Name: "Quality", Gates: []Gate{{Metric: "made_up", Comparator: ComparatorGreaterThanOrEqual, Threshold: 0.8}}}},
		{name: "duplicate gate", input: CreateInput{DatasetID: "ds_1", Name: "Quality", Gates: []Gate{{Metric: "answer_accuracy", Comparator: ComparatorGreaterThanOrEqual, Threshold: 0.8}, {Metric: "answer_accuracy", Comparator: ComparatorGreaterThanOrEqual, Threshold: 0.9}}}},
		{name: "invalid comparator", input: CreateInput{DatasetID: "ds_1", Name: "Quality", Gates: []Gate{{Metric: "answer_accuracy", Comparator: "equals", Threshold: 0.8}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Create(t.Context(), "tenant_1", "prj_1", test.input)
			if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("Create() error = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

func TestCreateRejectsForeignProjectDataset(t *testing.T) {
	resolver := datasetResolver{datasets: map[string]dataset.Dataset{"ds_1": {ID: "ds_1", TenantID: "tenant_1", ProjectID: "prj_foreign"}}}
	service := NewService(&memoryRepository{}, resolver, eval.DefaultMetricRegistry)
	_, err := service.Create(t.Context(), "tenant_1", "prj_1", CreateInput{DatasetID: "ds_1", Name: "Quality", Gates: []Gate{{Metric: "answer_accuracy", Comparator: ComparatorGreaterThanOrEqual, Threshold: 0.8}}})
	if !errors.Is(err, ErrDatasetNotFound) {
		t.Fatalf("Create() error = %v, want ErrDatasetNotFound", err)
	}
}

func TestCreateAddsAnImmutablePolicyRevision(t *testing.T) {
	resolver := datasetResolver{datasets: map[string]dataset.Dataset{"ds_1": {ID: "ds_1", TenantID: "tenant_1", ProjectID: "prj_1"}}}
	repository := newMemoryRepository()
	service := NewService(repository, resolver, eval.DefaultMetricRegistry)
	input := CreateInput{DatasetID: "ds_1", Name: "Quality", Gates: []Gate{{Metric: "answer_accuracy", Comparator: ComparatorGreaterThanOrEqual, Threshold: 0.8}}}
	first, err := service.Create(t.Context(), "tenant_1", "prj_1", input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(t.Context(), "tenant_1", "prj_1", input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Version != 1 || second.Version != 2 {
		t.Fatalf("policy revisions = %#v, %#v", first, second)
	}
}

func TestFreezeSnapshotsPolicyAndDerivesGateResult(t *testing.T) {
	policy := Policy{ID: "epol_1", TenantID: "tenant_1", ProjectID: "prj_1", DatasetID: "ds_1", Version: 1, Gates: []Gate{{Metric: "answer_accuracy", Comparator: ComparatorGreaterThanOrEqual, Threshold: 0.8}, {Metric: "latency_p95_ms", Comparator: ComparatorLessThanOrEqual, Threshold: 300}}}
	run := eval.RunResult{ID: "eval_1", ProjectID: "prj_1", DatasetID: "ds_1", Metrics: map[string]float64{"answer_accuracy": 0.9, "latency_p95_ms": 320}}
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	evidence, err := Freeze(policy, run, "pv_1", "sha256:definition", now)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Passed || len(evidence.GateResults) != 2 || !evidence.GateResults[0].Passed || evidence.GateResults[1].Passed {
		t.Fatalf("Freeze() evidence = %#v", evidence)
	}
	policy.Gates[0].Threshold = 1
	run.Metrics["answer_accuracy"] = 0
	if evidence.FrozenInput.Gates[0].Threshold != 0.8 || evidence.FrozenInput.Metrics["answer_accuracy"] != 0.9 {
		t.Fatalf("Freeze() did not snapshot evidence: %#v", evidence.FrozenInput)
	}
}

func TestFreezeRejectsMismatchedEvaluation(t *testing.T) {
	policy := Policy{ID: "epol_1", ProjectID: "prj_1", DatasetID: "ds_1", Version: 1, Gates: []Gate{{Metric: "answer_accuracy", Comparator: ComparatorGreaterThanOrEqual, Threshold: 0.8}}}
	_, err := Freeze(policy, eval.RunResult{ID: "eval_1", ProjectID: "prj_2", DatasetID: "ds_1"}, "pv_1", "sha256:definition", time.Time{})
	if !errors.Is(err, ErrEvaluationMismatch) {
		t.Fatalf("Freeze() error = %v, want ErrEvaluationMismatch", err)
	}
}

type datasetResolver struct{ datasets map[string]dataset.Dataset }

func (r datasetResolver) GetInProject(_ context.Context, tenantID, projectID, datasetID string) (dataset.Dataset, bool, error) {
	item, found := r.datasets[datasetID]
	return item, found && item.TenantID == tenantID && item.ProjectID == projectID, nil
}

type memoryRepository struct{ policies []Policy }

func newMemoryRepository() *memoryRepository { return &memoryRepository{} }
func (r *memoryRepository) Create(_ context.Context, policy Policy) error {
	r.policies = append(r.policies, policy)
	return nil
}
func (r *memoryRepository) Get(_ context.Context, tenantID, projectID, policyID string) (Policy, error) {
	for _, policy := range r.policies {
		if policy.TenantID == tenantID && policy.ProjectID == projectID && policy.ID == policyID {
			return policy, nil
		}
	}
	return Policy{}, ErrPolicyNotFound
}
func (r *memoryRepository) List(_ context.Context, tenantID, projectID string) ([]Policy, error) {
	items := make([]Policy, 0, len(r.policies))
	for _, policy := range r.policies {
		if policy.TenantID == tenantID && policy.ProjectID == projectID {
			items = append(items, policy)
		}
	}
	return items, nil
}
func (*memoryRepository) RecordEvidence(context.Context, Evidence) error { return nil }
