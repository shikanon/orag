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
