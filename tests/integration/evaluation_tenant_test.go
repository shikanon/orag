package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/rag"
)

func TestEvaluationDatasetTenantIsolationWithPostgres(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := app.Postgres.Exec(ctx, `
		INSERT INTO tenants(id, name)
		VALUES($1, $2), ($3, $4)
		ON CONFLICT (id) DO NOTHING`,
		"tenant_a", "Tenant A", "tenant_b", "Tenant B"); err != nil {
		t.Fatal(err)
	}
	ds, err := app.Datasets.Create(ctx, "tenant_a", "tenant a regression", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.Create(ctx, "tenant_b", "tenant b regression", "golden"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_a", ds.ID, dataset.Item{
		Query:       "tenant a query",
		GroundTruth: "tenant a answer",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.Datasets.AddItem(ctx, "tenant_b", ds.ID, dataset.Item{
		Query:       "tenant b query",
		GroundTruth: "tenant b answer",
	}); !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("AddItem() error = %v, want ErrDatasetNotFound", err)
	}
	if got := countRows(t, app.Postgres.QueryRow(ctx, `SELECT count(*) FROM dataset_items WHERE dataset_id=$1`, ds.ID)); got != 1 {
		t.Fatalf("dataset_items count = %d, want 1", got)
	}

	_, err = app.Eval.Run(ctx, eval.RunRequest{
		TenantID:        "tenant_b",
		DatasetID:       ds.ID,
		KnowledgeBaseID: testKBID,
		Profile:         rag.ProfileRealtime,
	})
	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("Run() error = %v, want ErrDatasetNotFound", err)
	}
	if got := countRows(t, app.Postgres.QueryRow(ctx, `SELECT count(*) FROM evaluation_runs WHERE dataset_id=$1`, ds.ID)); got != 0 {
		t.Fatalf("evaluation_runs count = %d, want 0", got)
	}
	if got := countRows(t, app.Postgres.QueryRow(ctx, `
		SELECT count(*)
		FROM evaluation_results er
		JOIN evaluation_runs r ON r.id = er.run_id
		WHERE r.dataset_id=$1`, ds.ID)); got != 0 {
		t.Fatalf("evaluation_results count = %d, want 0", got)
	}

	result, err := (eval.Optimizer{Runner: app.Eval}).Optimize(ctx, eval.OptimizeRequest{
		TenantID:        "tenant_b",
		DatasetID:       ds.ID,
		KnowledgeBaseID: testKBID,
		Profiles:        []rag.Profile{rag.ProfileRealtime},
		TopKs:           []int{1},
	})
	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("Optimize() error = %v, want ErrDatasetNotFound", err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("optimization candidates = %d, want 0", len(result.Candidates))
	}
	if got := countRows(t, app.Postgres.QueryRow(ctx, `SELECT count(*) FROM evaluation_runs WHERE dataset_id=$1`, ds.ID)); got != 0 {
		t.Fatalf("evaluation_runs count after optimization = %d, want 0", got)
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func countRows(t *testing.T, row rowScanner) int {
	t.Helper()
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
