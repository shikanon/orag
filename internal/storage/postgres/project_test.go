package postgres

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shikanon/orag/internal/project"
)

var require projectTestRequire

func TestProjectMigrationDefinesTenantScopedTables(t *testing.T) {
	sql := readProjectMigration(t, "../../../migrations/000016_projects.sql")
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS projects", "tenant_id TEXT NOT NULL REFERENCES tenants(id)", "CREATE TABLE IF NOT EXISTS project_environments", "UNIQUE (project_id, kind)", "ON DELETE CASCADE", "(tenant_id, updated_at DESC)"} {
		require.Contains(t, sql, fragment)
	}
}

func TestProjectCreateWithEnvironmentsCommitsAllRows(t *testing.T) {
	tx := &fakeProjectTx{}
	repo := &ProjectRepository{db: &fakeProjectDB{tx: tx}}
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	p := project.Project{ID: "project-1", TenantID: "tenant-1", Name: "Console", Description: "Control plane", CreatedAt: now, UpdatedAt: now}
	envs := []project.Environment{
		{ID: "env-dev", ProjectID: p.ID, Kind: project.EnvironmentDevelopment, CreatedAt: now, UpdatedAt: now},
		{ID: "env-stage", ProjectID: p.ID, Kind: project.EnvironmentStaging, CreatedAt: now, UpdatedAt: now},
		{ID: "env-prod", ProjectID: p.ID, Kind: project.EnvironmentProduction, CreatedAt: now, UpdatedAt: now},
	}

	require.NoError(t, repo.CreateWithEnvironments(context.Background(), p, envs))
	require.Len(t, tx.execs, 4)
	require.Contains(t, tx.execs[0].sql, "INSERT INTO projects")
	for _, call := range tx.execs[1:] {
		require.Contains(t, call.sql, "INSERT INTO project_environments")
	}
	require.Equal(t, 1, tx.commits)
	require.Equal(t, 1, tx.rollbacks)
}

func TestProjectCreateWithEnvironmentsRollsBackOnEnvironmentFailure(t *testing.T) {
	tx := &fakeProjectTx{failAt: 3, execErr: errors.New("insert environment")}
	repo := &ProjectRepository{db: &fakeProjectDB{tx: tx}}
	p := project.Project{ID: "project-1", TenantID: "tenant-1"}
	envs := []project.Environment{{ID: "dev", ProjectID: p.ID}, {ID: "stage", ProjectID: p.ID}, {ID: "prod", ProjectID: p.ID}}

	err := repo.CreateWithEnvironments(context.Background(), p, envs)
	require.ErrorContains(t, err, "insert environment")
	require.Zero(t, tx.commits)
	require.Equal(t, 1, tx.rollbacks)
}

func TestProjectQueriesAreTenantScoped(t *testing.T) {
	now := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	db := &fakeProjectDB{
		rows: &fakeProjectRows{values: [][]any{{"project-1", "tenant-1", "One", "Desc", now, now}}},
		row:  &fakeProjectRow{values: []any{"project-1", "tenant-1", "One", "Desc", now, now}},
	}
	repo := &ProjectRepository{db: db}

	items, err := repo.List(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Contains(t, db.querySQL, "WHERE tenant_id=$1")
	require.Equal(t, []any{"tenant-1"}, db.queryArgs)

	got, ok, err := repo.Get(context.Background(), "tenant-1", "project-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "project-1", got.ID)
	require.Contains(t, db.rowSQL, "WHERE tenant_id=$1 AND id=$2")
	require.Equal(t, []any{"tenant-1", "project-1"}, db.rowArgs)

	require.NoError(t, repo.Update(context.Background(), project.Project{ID: "project-1", TenantID: "tenant-1", Name: "Updated", Description: "New", UpdatedAt: now}))
	require.Contains(t, db.execSQL, "WHERE tenant_id=$1 AND id=$2")
	require.Equal(t, "tenant-1", db.execArgs[0])
	require.Equal(t, "project-1", db.execArgs[1])
}

func TestProjectGetReturnsNotFound(t *testing.T) {
	repo := &ProjectRepository{db: &fakeProjectDB{row: &fakeProjectRow{err: pgx.ErrNoRows}}}
	_, ok, err := repo.Get(context.Background(), "tenant-1", "missing")
	require.NoError(t, err)
	require.False(t, ok)
}

func readProjectMigration(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

type projectExecCall struct {
	sql  string
	args []any
}
type fakeProjectTx struct {
	execs              []projectExecCall
	failAt             int
	execErr            error
	commits, rollbacks int
}

func (t *fakeProjectTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.execs = append(t.execs, projectExecCall{sql: strings.TrimSpace(sql), args: args})
	if t.failAt == len(t.execs) {
		return pgconn.CommandTag{}, t.execErr
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (t *fakeProjectTx) Commit(context.Context) error   { t.commits++; return nil }
func (t *fakeProjectTx) Rollback(context.Context) error { t.rollbacks++; return nil }

type fakeProjectDB struct {
	tx        projectTx
	beginErr  error
	execSQL   string
	execArgs  []any
	execErr   error
	querySQL  string
	queryArgs []any
	rows      projectRows
	queryErr  error
	rowSQL    string
	rowArgs   []any
	row       projectRow
}

func (d *fakeProjectDB) BeginProjectTx(context.Context) (projectTx, error) { return d.tx, d.beginErr }
func (d *fakeProjectDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	d.execSQL, d.execArgs = sql, args
	return pgconn.NewCommandTag("UPDATE 1"), d.execErr
}
func (d *fakeProjectDB) Query(_ context.Context, sql string, args ...any) (projectRows, error) {
	d.querySQL, d.queryArgs = sql, args
	return d.rows, d.queryErr
}
func (d *fakeProjectDB) QueryRow(_ context.Context, sql string, args ...any) projectRow {
	d.rowSQL, d.rowArgs = sql, args
	return d.row
}

type fakeProjectRow struct {
	values []any
	err    error
}

func (r *fakeProjectRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return assignProjectValues(r.values, dest)
}

type fakeProjectRows struct {
	values [][]any
	index  int
	err    error
}

func (r *fakeProjectRows) Next() bool { return r.index < len(r.values) }
func (r *fakeProjectRows) Scan(dest ...any) error {
	err := assignProjectValues(r.values[r.index], dest)
	r.index++
	return err
}
func (r *fakeProjectRows) Close()     {}
func (r *fakeProjectRows) Err() error { return r.err }
func assignProjectValues(values []any, dest []any) error {
	for i, value := range values {
		switch out := dest[i].(type) {
		case *string:
			*out = value.(string)
		case *time.Time:
			*out = value.(time.Time)
		}
	}
	return nil
}

type projectTestRequire struct{}

func (projectTestRequire) NoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
func (projectTestRequire) ErrorContains(t *testing.T, err error, text string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), text) {
		t.Fatalf("error %v does not contain %q", err, text)
	}
}
func (projectTestRequire) Contains(t *testing.T, value, fragment string) {
	t.Helper()
	if !strings.Contains(value, fragment) {
		t.Fatalf("%q does not contain %q", value, fragment)
	}
}
func (projectTestRequire) Len(t *testing.T, value any, want int) {
	t.Helper()
	var got int
	switch v := value.(type) {
	case []project.Project:
		got = len(v)
	case []projectExecCall:
		got = len(v)
	default:
		t.Fatalf("unsupported Len type %T", value)
	}
	if got != want {
		t.Fatalf("length = %d, want %d", got, want)
	}
}
func (projectTestRequire) Equal(t *testing.T, want, got any) {
	t.Helper()
	if !projectValuesEqual(want, got) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
func (projectTestRequire) True(t *testing.T, value bool) {
	t.Helper()
	if !value {
		t.Fatal("value is false")
	}
}
func (projectTestRequire) False(t *testing.T, value bool) {
	t.Helper()
	if value {
		t.Fatal("value is true")
	}
}
func (projectTestRequire) Zero(t *testing.T, value int) {
	t.Helper()
	if value != 0 {
		t.Fatalf("value = %d, want zero", value)
	}
}
func projectValuesEqual(a, b any) bool { return reflect.DeepEqual(a, b) }
