package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationStatus describes the repository migrations expected by a binary and
// which of them have been applied to one PostgreSQL database.
type MigrationStatus struct {
	Version   string
	AppliedAt *time.Time
}

// MigrationStatuses reads migration state without changing the database. A
// database that has not been migrated yet reports every local migration as
// pending, which makes the command safe to use before a restore or rollout.
func MigrationStatuses(ctx context.Context, pool *pgxpool.Pool, dir string) ([]MigrationStatus, error) {
	versions, err := migrationVersions(dir)
	if err != nil {
		return nil, err
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return nil, err
	}
	applied := make(map[string]time.Time, len(versions))
	if exists {
		rows, err := pool.Query(ctx, `SELECT version, applied_at FROM schema_migrations`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var version string
			var at time.Time
			if err := rows.Scan(&version, &at); err != nil {
				return nil, err
			}
			applied[version] = at.UTC()
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	status := make([]MigrationStatus, 0, len(versions))
	for _, version := range versions {
		entry := MigrationStatus{Version: version}
		if at, ok := applied[version]; ok {
			entry.AppliedAt = &at
		}
		status = append(status, entry)
	}
	return status, nil
}

func migrationVersions(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	versions := make([]string, 0, len(files))
	for _, file := range files {
		versions = append(versions, strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
	}
	return versions, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL
	)`); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		version := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		up, err := extractGooseUp(string(body))
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, up); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES($1, $2)`, version, time.Now().UTC()); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func extractGooseUp(sql string) (string, error) {
	start := strings.Index(sql, "-- +goose Up")
	if start < 0 {
		return "", fmt.Errorf("missing -- +goose Up marker")
	}
	sql = sql[start+len("-- +goose Up"):]
	if end := strings.Index(sql, "-- +goose Down"); end >= 0 {
		sql = sql[:end]
	}
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "", fmt.Errorf("empty up migration")
	}
	return sql, nil
}

func extractGooseDown(sql string) (string, error) {
	start := strings.Index(sql, "-- +goose Down")
	if start < 0 {
		return "", fmt.Errorf("missing -- +goose Down marker")
	}
	sql = strings.TrimSpace(sql[start+len("-- +goose Down"):])
	if sql == "" {
		return "", fmt.Errorf("empty down migration")
	}
	return sql, nil
}
