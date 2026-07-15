package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/evaluationpolicy"
)

func TestEvaluationPolicyMigrationHasImmutablePolicyAndEvidenceTables(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000022_project_evaluation_policies.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS project_evaluation_policies",
		"UNIQUE (project_id, name, version)",
		"CREATE TABLE IF NOT EXISTS project_evaluation_evidence",
		"frozen_input JSONB NOT NULL",
		"gate_results JSONB NOT NULL",
		"REFERENCES pipeline_versions(id)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}

func TestRepositoryStoresPolicyAndFrozenEvidence(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{}
	repository := &Repository{evalQueryer: queryer}
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	policy := evaluationpolicy.Policy{ID: "epol_1", TenantID: "tenant_1", ProjectID: "prj_1", DatasetID: "ds_1", Name: "Quality", Version: 1, Gates: []evaluationpolicy.Gate{{Metric: "answer_accuracy", Comparator: evaluationpolicy.ComparatorGreaterThanOrEqual, Threshold: 0.8}}, CreatedAt: now}
	if err := repository.Create(context.Background(), policy); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "project_evaluation_policies") || !strings.Contains(string(queryer.execArgs[6].([]byte)), "answer_accuracy") {
		t.Fatalf("policy insert sql=%s args=%#v", queryer.execSQL, queryer.execArgs)
	}
	evidence := evaluationpolicy.Evidence{ID: "epev_1", TenantID: "tenant_1", ProjectID: "prj_1", PolicyID: "epol_1", PolicyVersion: 1, EvaluationRunID: "eval_1", PipelineVersionID: "pv_1", ContentHash: "sha256:definition", FrozenInput: evaluationpolicy.FrozenInput{PolicyID: "epol_1", PolicyVersion: 1, ProjectID: "prj_1", DatasetID: "ds_1", EvaluationRunID: "eval_1", PipelineVersion: "pv_1", ContentHash: "sha256:definition", Gates: policy.Gates, Metrics: map[string]float64{"answer_accuracy": 0.9}}, GateResults: []evaluationpolicy.GateResult{{Metric: "answer_accuracy", Comparator: evaluationpolicy.ComparatorGreaterThanOrEqual, Threshold: 0.8, Actual: 0.9, Present: true, Passed: true}}, Passed: true, CreatedAt: now}
	if err := repository.RecordEvidence(context.Background(), evidence); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "project_evaluation_evidence") || !strings.Contains(string(queryer.execArgs[8].([]byte)), "evaluation_run_id") || !strings.Contains(string(queryer.execArgs[9].([]byte)), "answer_accuracy") {
		t.Fatalf("evidence insert sql=%s args=%#v", queryer.execSQL, queryer.execArgs)
	}
}
