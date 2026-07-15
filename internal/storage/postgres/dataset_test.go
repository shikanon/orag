package postgres

import (
	"context"
	"encoding/json"
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
		ID:               "dsi_2",
		DatasetID:        "ds_a",
		Query:            "q",
		GroundTruth:      "a",
		Split:            dataset.DatasetSplitGold,
		Weight:           2.5,
		ExpectedEvidence: []string{"chunk_1"},
		HumanScores:      map[string]float64{"faithfulness": 0.9},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "dsi_2" || got.DatasetID != "ds_a" {
		t.Fatalf("created item = %#v", got)
	}
	for _, want := range []string{"split", "weight", "expected_evidence", "human_scores"} {
		if !strings.Contains(db.execSQL, want) {
			t.Fatalf("AddDatasetItem() SQL missing metadata column %q: %s", want, db.execSQL)
		}
	}
	if got.Split != dataset.DatasetSplitGold || got.Weight != 2.5 {
		t.Fatalf("created metadata = %#v", got)
	}
	if got := db.execArgs[7]; got != dataset.DatasetSplitGold {
		t.Fatalf("split arg = %v, want gold", got)
	}
	if got := db.execArgs[8]; got != 2.5 {
		t.Fatalf("weight arg = %v, want 2.5", got)
	}
	var expected []string
	if err := json.Unmarshal(db.execArgs[9].([]byte), &expected); err != nil {
		t.Fatal(err)
	}
	if len(expected) != 1 || expected[0] != "chunk_1" {
		t.Fatalf("expected evidence JSON = %#v", expected)
	}
	var humanScores map[string]float64
	if err := json.Unmarshal(db.execArgs[10].([]byte), &humanScores); err != nil {
		t.Fatal(err)
	}
	if humanScores["faithfulness"] != 0.9 {
		t.Fatalf("human scores JSON = %#v", humanScores)
	}
}

func TestRepositoryGetDatasetInProjectUsesCompositeScope(t *testing.T) {
	now := time.Now().UTC()
	db := &fakeDatasetQueryer{row: fakeDatasetRow{values: []any{"ds_1", "tenant_1", "prj_1", "Golden", "golden", "v1", now}}}
	repo := &Repository{datasetRunner: db}

	got, found, err := repo.GetDatasetInProject(context.Background(), "tenant_1", "prj_1", "ds_1")
	if err != nil || !found || got.ProjectID != "prj_1" {
		t.Fatalf("GetDatasetInProject() = %#v, %v, %v", got, found, err)
	}
	if !strings.Contains(db.rowSQL, "tenant_id=$1 AND project_id=$2 AND id=$3") {
		t.Fatalf("project-scoped get SQL = %s", db.rowSQL)
	}
	if len(db.rowArgs) != 3 || db.rowArgs[0] != "tenant_1" || db.rowArgs[1] != "prj_1" || db.rowArgs[2] != "ds_1" {
		t.Fatalf("project-scoped get args = %#v", db.rowArgs)
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

	db.row = fakeDatasetRow{values: []any{"ds_a", "tenant_a", "prj_a", "regression", "golden", "v1", time.Now().UTC()}}
	db.rows = &fakePgxRows{rows: [][]any{
		{"dsi_1", "ds_a", "q", "a", []byte(`["doc_1"]`), []byte(`[]`), "gold", 1.5, []byte(`["chunk_1"]`), []byte(`{"faithfulness":0.8}`)},
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
	for _, want := range []string{"i.split", "i.weight", "i.expected_evidence", "i.human_scores"} {
		if !strings.Contains(db.querySQL, want) {
			t.Fatalf("DatasetItems() SQL missing metadata column %q: %s", want, db.querySQL)
		}
	}
	if got := db.queryArgs[0]; got != "tenant_a" {
		t.Fatalf("tenant arg = %v, want tenant_a", got)
	}
	if items[0].Split != dataset.DatasetSplitGold || items[0].Weight != 1.5 {
		t.Fatalf("DatasetItems() metadata = %#v", items[0])
	}
	if items[0].ExpectedEvidence[0] != "chunk_1" || items[0].HumanScores["faithfulness"] != 0.8 {
		t.Fatalf("DatasetItems() metadata JSON = %#v", items[0])
	}
}

func TestRepositoryDatasetItemsBySplitFiltersInSQL(t *testing.T) {
	ctx := context.Background()
	db := &fakeDatasetQueryer{
		row: fakeDatasetRow{values: []any{"ds_a", "tenant_a", "prj_a", "regression", "golden", "v1", time.Now().UTC()}},
		rows: &fakePgxRows{rows: [][]any{
			{"dsi_1", "ds_a", "q", "a", []byte(`["doc_1"]`), []byte(`[]`), "holdout", 2.5, []byte(`[]`), []byte(`{}`)},
		}},
	}
	repo := &Repository{datasetRunner: db}

	items, err := repo.DatasetItemsBySplit(ctx, "tenant_a", "ds_a", dataset.DatasetSplitHoldout)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Split != dataset.DatasetSplitHoldout || items[0].Weight != 2.5 {
		t.Fatalf("DatasetItemsBySplit() = %#v", items)
	}
	for _, want := range []string{"COALESCE(NULLIF(i.split, ''), 'eval')=$3", "d.tenant_id=$1", "i.dataset_id=$2"} {
		if !strings.Contains(db.querySQL, want) {
			t.Fatalf("DatasetItemsBySplit() SQL missing %q: %s", want, db.querySQL)
		}
	}
	if got := db.queryArgs[2]; got != dataset.DatasetSplitHoldout {
		t.Fatalf("split arg = %v, want holdout", got)
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
