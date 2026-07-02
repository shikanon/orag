package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
)

func TestRunReturnsNonZeroOnAppInitFailure(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DATABASE_URL", "postgres://orag:orag@localhost:5432/orag?sslmode=disable")
	t.Setenv("QDRANT_HOST", "localhost")
	t.Setenv("REQUIRE_EXTERNAL_PROVIDERS", "false")
	t.Setenv("INGEST_PARSER_METHOD", "basic")
	t.Setenv("RERANK_PROVIDER", "volcengine")
	t.Setenv("ARK_EMBEDDING_DIMENSIONS", "1024")
	t.Setenv("RAG_RRF_K", "60")
	t.Setenv("RAG_SEMANTIC_CACHE_THRESHOLD", "0.92")

	initErr := errors.New("app init failed")
	serverStarted := false

	code := run(
		context.Background(),
		func(context.Context, config.Config, *slog.Logger) (*core.App, error) {
			return nil, initErr
		},
		func(*core.App) {
			serverStarted = true
		},
	)

	if code != 1 {
		t.Fatalf("run() exit code = %d, want 1", code)
	}
	if serverStarted {
		t.Fatal("server starter was called after app init failed")
	}
}
