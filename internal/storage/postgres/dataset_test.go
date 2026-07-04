package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shikanon/orag/internal/dataset"
)

func TestRepositoryAddDatasetItemEnforcesTenant(t *testing.T) {
	ctx := context.Background()
	db := &fakeDatasetQueryer{execTag: pgconn.NewCommandTag("INSERT 0 0")}
	repo := &Repository{datasetRunner: db}

	_, err := repo.AddDatasetItem(ctx, "tenant_b", dataset.Item{
		ID:          "dsi_1",
		DatasetID:   "ds_a",
		Query:       "q",
		GroundTruth: "a",
	})
	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("AddDatasetItem() err = %v, want not_found", err)
	}
	if !strings.Contains(db.execSQL, "WHERE EXISTS") || !strings.Contains(db.execSQL, "tenant_id=$6 AND id=$2") {
		t.Fatalf("AddDatasetItem() SQL missing tenant dataset guard: %s", db.execSQL)
	}
	if got := db.execArgs[5]; got != "tenant_b" {
		t.Fatalf("tenant arg = %v, want tenant_b", got)
	}

	db.execTag = pgconn.NewCommandTag("INSERT 0 1")
	got, err := repo.AddDatasetItem(ctx, "tenant_a", dataset.Item{
		ID:          "dsi_2",
		DatasetID:   "ds_a",
		Query:       "q",
		GroundTruth: "a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "dsi_2" || got.DatasetID != "ds_a" {
		t.Fatalf("created item = %#v", got)
	}
}

func TestRepositoryDatasetItemsEnforcesTenant(t *testing.T) {
	ctx := context.Background()
	db := &fakeDatasetQueryer{row: fakeDatasetRow{err: pgx.ErrNoRows}}
	repo := &Repository{datasetRunner: db}

	_, err := repo.DatasetItems(ctx, "tenant_b", "ds_a")
	if !errors.Is(err, dataset.ErrDatasetNotFound) {
		t.Fatalf("DatasetItems() err = %v, want not_found", err)
	}
	if db.queryCalled {
		t.Fatal("DatasetItems() queried items after dataset tenant check failed")
	}

	db.row = fakeDatasetRow{values: []any{"ds_a", "tenant_a", "regression", "golden", "v1", time.Now().UTC()}}
	db.rows = &fakePgxRows{rows: [][]any{
		{"dsi_1", "ds_a", "q", "a", []byte(`["doc_1"]`), []byte(`[]`)},
	}}
	items, err := repo.DatasetItems(ctx, "tenant_a", "ds_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "dsi_1" || items[0].RelevantDocIDs[0] != "doc_1" {
		t.Fatalf("DatasetItems() = %#v", items)
	}
	if !strings.Contains(db.querySQL, "JOIN datasets d ON d.id=i.dataset_id") || !strings.Contains(db.querySQL, "d.tenant_id=$1 AND i.dataset_id=$2") {
		t.Fatalf("DatasetItems() SQL missing tenant join: %s", db.querySQL)
	}
	if got := db.queryArgs[0]; got != "tenant_a" {
		t.Fatalf("tenant arg = %v, want tenant_a", got)
	}
}

type fakeDatasetQueryer struct {
	execSQL  string
	execArgs []any
	execTag  pgconn.CommandTag
	execErr  error

	queryCalled bool
	querySQL    string
	queryArgs   []any
	queryErr    error
	rows        *fakePgxRows

	rowSQL  string
	rowArgs []any
	row     fakeDatasetRow
}

func (f *fakeDatasetQueryer) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execSQL = sql
	f.execArgs = args
	return f.execTag, f.execErr
}

func (f *fakeDatasetQueryer) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.queryCalled = true
	f.querySQL = sql
	f.queryArgs = args
	return f.rows, f.queryErr
}

func (f *fakeDatasetQueryer) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.rowSQL = sql
	f.rowArgs = args
	return f.row
}

type fakeDatasetRow struct {
	values []any
	err    error
}

func (r fakeDatasetRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	assignScanValues(dest, r.values)
	return nil
}

type fakePgxRows struct {
	rows [][]any
	idx  int
	err  error
}

func (r *fakePgxRows) Close() {}

func (r *fakePgxRows) Err() error {
	return r.err
}

func (r *fakePgxRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("SELECT 1")
}

func (r *fakePgxRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *fakePgxRows) Next() bool {
	return r.idx < len(r.rows)
}

func (r *fakePgxRows) Scan(dest ...any) error {
	assignScanValues(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func (r *fakePgxRows) Values() ([]any, error) {
	return r.rows[r.idx], nil
}

func (r *fakePgxRows) RawValues() [][]byte {
	return nil
}

func (r *fakePgxRows) Conn() *pgx.Conn {
	return nil
}
