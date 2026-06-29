package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("ARK_BASE_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("default port = %d", cfg.Server.Port)
	}
	if cfg.Qdrant.Collection == "" {
		t.Fatal("expected qdrant collection default")
	}
	if cfg.Storage.Backend != "qdrant_postgres" {
		t.Fatalf("default storage backend = %q", cfg.Storage.Backend)
	}
	if cfg.Ark.BaseURL == "" {
		t.Fatal("expected empty ARK_BASE_URL env to use default")
	}
	if cfg.Ark.EmbeddingModel != "doubao-embedding-vision-251215" {
		t.Fatalf("default embedding model = %q", cfg.Ark.EmbeddingModel)
	}
}

func TestRedactedEnv(t *testing.T) {
	t.Setenv("ARK_API_KEY", "abcdefghi")
	t.Setenv("ALIYUN_RERANK_API_KEY", "sk-test-secret")
	t.Setenv("JWT_SECRET", "secret-jwt-value")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/orag?sslmode=disable")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	env := cfg.RedactedEnv()
	if env["STORAGE_BACKEND"] != "qdrant_postgres" {
		t.Fatalf("redacted env storage backend = %q", env["STORAGE_BACKEND"])
	}
	for _, key := range []string{"ARK_API_KEY", "ALIYUN_RERANK_API_KEY", "JWT_SECRET", "DATABASE_URL"} {
		if got := env[key]; got == "" || got == "abcdefghi" || got == "sk-test-secret" || got == "secret-jwt-value" || got == "postgres://user:pass@localhost:5432/orag?sslmode=disable" {
			t.Fatalf("expected %s to be redacted, got %q", key, got)
		}
	}
}

func TestRequireExternalProviders(t *testing.T) {
	t.Setenv("REQUIRE_EXTERNAL_PROVIDERS", "true")
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("LLM_API_KEY", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected missing ARK_API_KEY error")
	}
}

func TestInvalidStorageBackend(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "unknown")
	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid storage backend error")
	}
}

func TestInvalidRerankProvider(t *testing.T) {
	t.Setenv("RERANK_PROVIDER", "unknown")
	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid rerank provider error")
	}
}
