package postgres

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	evalpkg "github.com/shikanon/orag/internal/eval"
)

func TestProjectOwnedEvaluationOptimizationMigration(t *testing.T) {
	body, err := os.ReadFile("../../../migrations/000019_project_owned_evaluation_optimization.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, required := range []string{
		"ALTER TABLE evaluation_runs ADD COLUMN project_id TEXT",
		"ALTER TABLE optimization_runs ADD COLUMN project_id TEXT",
		"SET project_id = dataset.project_id",
		"evaluation_runs_tenant_project_fk",
		"optimization_runs_tenant_project_fk",
		"evaluation_runs_tenant_project_created_idx",
		"optimization_runs_tenant_project_status_idx",
		"ALTER TABLE optimization_runs DROP COLUMN IF EXISTS project_id",
		"ALTER TABLE evaluation_runs DROP COLUMN IF EXISTS project_id",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}

func TestRepositoryStoresEvaluationProjectScope(t *testing.T) {
	queryer := &fakeKnowledgeBaseQueryer{}
	repository := &Repository{evalQueryer: queryer}
	result := evalpkg.RunResult{
		ID:        "eval_1",
		ProjectID: "prj_1",
		DatasetID: "ds_1",
		Profile:   "realtime",
		CreatedAt: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC),
	}
	if err := repository.StoreEvaluationRun(context.Background(), "tenant_1", result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(queryer.execSQL, "tenant_id, project_id, dataset_id") || !strings.Contains(queryer.execSQL, "NULLIF($3,'')") {
		t.Fatalf("evaluation insert SQL=%s", queryer.execSQL)
	}
	if got := queryer.execArgs[2]; got != "prj_1" {
		t.Fatalf("project arg=%#v want=prj_1", got)
	}
}

func TestRepositoryProjectScopedControlRootLookups(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*Repository) error
	}{
		{
			name: "evaluation",
			call: func(repository *Repository) error {
				_, _, err := repository.GetEvaluationRunInProject(context.Background(), "tenant_1", "prj_1", "eval_1")
				return err
			},
		},
		{
			name: "optimization",
			call: func(repository *Repository) error {
				_, _, err := repository.GetOptimizationRunInProject(context.Background(), "tenant_1", "prj_1", "opt_1")
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			queryer := &fakeKnowledgeBaseQueryer{row: fakeTraceRow{err: pgx.ErrNoRows}}
			repository := &Repository{evalQueryer: queryer}
			if err := test.call(repository); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(queryer.rowSQL, "tenant_id=$1 AND project_id=$2 AND id=$3") {
				t.Fatalf("project-scoped SQL=%s", queryer.rowSQL)
			}
			if !reflect.DeepEqual(queryer.rowArgs, []any{"tenant_1", "prj_1", map[string]string{"evaluation": "eval_1", "optimization": "opt_1"}[test.name]}) {
				t.Fatalf("project-scoped args=%#v", queryer.rowArgs)
			}
		})
	}
}
