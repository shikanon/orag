package app

import (
	"context"
	"log/slog"
	"testing"

	"github.com/shikanon/orag/internal/config"
)

func TestReadinessReportsModelProviderConfigured(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ARK_API_KEY", "ark-test-key")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	app, err := New(context.Background(), cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer app.Close()

	checks, ready := app.Readiness(context.Background())
	if !ready {
		t.Fatalf("ready = false checks=%#v", checks)
	}
	if _, ok := checks["ark"]; ok {
		t.Fatalf("legacy ark readiness should not be reported: %#v", checks)
	}
	if checks["model_provider"].Status != "configured" {
		t.Fatalf("model provider readiness = %#v", checks["model_provider"])
	}
}

func TestReadinessReportsExplicitMockModelProvider(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("LLM_CHAT_PROVIDER", "mock")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	app, err := New(context.Background(), cfg, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer app.Close()

	checks, ready := app.Readiness(context.Background())
	if !ready {
		t.Fatalf("ready = false checks=%#v", checks)
	}
	if checks["model_provider"].Status != "mock" {
		t.Fatalf("model provider readiness = %#v", checks["model_provider"])
	}
}
