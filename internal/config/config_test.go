package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("ARK_BASE_URL", "")
	t.Setenv("ARK_API_KEY", "ark-test-key")
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
	if cfg.Models.ChatProvider != "volcengine" {
		t.Fatalf("default chat provider = %q", cfg.Models.ChatProvider)
	}
	if cfg.Models.EmbeddingProvider != "volcengine" {
		t.Fatalf("default embedding provider = %q", cfg.Models.EmbeddingProvider)
	}
	if cfg.Models.RerankProvider != "volcengine" {
		t.Fatalf("default rerank provider = %q", cfg.Models.RerankProvider)
	}
	if cfg.Models.MultimodalProvider != "volcengine" {
		t.Fatalf("default multimodal provider = %q", cfg.Models.MultimodalProvider)
	}
	if cfg.Ingestion.ParserMethod != "basic" {
		t.Fatalf("default parser method = %q", cfg.Ingestion.ParserMethod)
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

func TestDefaultRequiresModelProviderAPIKey(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("LLM_API_KEY", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected missing ARK_API_KEY error")
	}
	if strings.Count(err.Error(), "ARK_API_KEY") != 1 {
		t.Fatalf("expected ARK_API_KEY to be reported once, got %q", err.Error())
	}
}

func TestExplicitMockModeAllowsMissingProviderAPIKey(t *testing.T) {
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("LLM_CHAT_PROVIDER", "mock")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Models.AllowDeterministicMock {
		t.Fatal("expected deterministic mock to be explicit")
	}
}

func TestProviderSpecificAPIKeyValidation(t *testing.T) {
	t.Setenv("LLM_CHAT_PROVIDER", "openai")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "openai")
	t.Setenv("LLM_RERANK_PROVIDER", "jina")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "openai")
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "openai-test-key")
	t.Setenv("JINA_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected missing JINA_API_KEY error")
	}
}

func TestProviderSpecificBaseURLValidation(t *testing.T) {
	t.Setenv("LLM_CHAT_PROVIDER", "azure-openai")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("AZURE_OPENAI_API_KEY", "azure-test-key")
	t.Setenv("AZURE_OPENAI_BASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected missing AZURE_OPENAI_BASE_URL error")
	}
}

func TestSelectedProviderUsesProviderDefaultModels(t *testing.T) {
	t.Setenv("LLM_CHAT_PROVIDER", "cohere")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "cohere")
	t.Setenv("LLM_RERANK_PROVIDER", "cohere")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("COHERE_API_KEY", "cohere-test-key")
	t.Setenv("ARK_API_KEY", "")
	t.Setenv("ARK_CHAT_MODEL", "")
	t.Setenv("ARK_EMBEDDING_MODEL", "")
	t.Setenv("ARK_RERANK_MODEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Ark.ChatModel != "command-r-plus" {
		t.Fatalf("chat model = %q", cfg.Ark.ChatModel)
	}
	if cfg.Ark.EmbeddingModel != "embed-v4.0" {
		t.Fatalf("embedding model = %q", cfg.Ark.EmbeddingModel)
	}
	if cfg.Ark.RerankModel != "rerank-v3.5" {
		t.Fatalf("rerank model = %q", cfg.Ark.RerankModel)
	}
}

func TestUnsupportedProviderCapability(t *testing.T) {
	t.Setenv("LLM_CHAT_PROVIDER", "jina")
	t.Setenv("JINA_API_KEY", "jina-test-key")
	_, err := Load()
	if err == nil {
		t.Fatal("expected unsupported chat capability error")
	}
}

func TestInvalidStorageBackend(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("STORAGE_BACKEND", "unknown")
	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid storage backend error")
	}
}

func TestInvalidRerankProvider(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("RERANK_PROVIDER", "unknown")
	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid rerank provider error")
	}
}

func TestLoadMinerUParserConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("INGEST_PARSER_METHOD", "mineru")
	t.Setenv("MINERU_APISERVER", "http://mineru:8000")
	t.Setenv("MINERU_SERVER_URL", "http://mineru-vlm:30000")
	t.Setenv("MINERU_BACKEND", "vlm-http-client")
	t.Setenv("MINERU_PARSE_METHOD", "ocr")
	t.Setenv("MINERU_LANG", "English")
	t.Setenv("MINERU_FORMULA_ENABLE", "false")
	t.Setenv("MINERU_TABLE_ENABLE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Ingestion.ParserMethod != "mineru" {
		t.Fatalf("parser method = %q", cfg.Ingestion.ParserMethod)
	}
	if cfg.Ingestion.MinerU.APIURL != "http://mineru:8000" {
		t.Fatalf("mineru api url = %q", cfg.Ingestion.MinerU.APIURL)
	}
	if cfg.Ingestion.MinerU.ServerURL != "http://mineru-vlm:30000" {
		t.Fatalf("mineru server url = %q", cfg.Ingestion.MinerU.ServerURL)
	}
	if cfg.Ingestion.MinerU.Backend != "vlm-http-client" || cfg.Ingestion.MinerU.ParseMethod != "ocr" {
		t.Fatalf("mineru config = %#v", cfg.Ingestion.MinerU)
	}
	if cfg.Ingestion.MinerU.Formula || !cfg.Ingestion.MinerU.Table {
		t.Fatalf("mineru toggles = %#v", cfg.Ingestion.MinerU)
	}
}

func TestLoadDoclingParserConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("INGEST_PARSER_METHOD", "docling")
	t.Setenv("DOCLING_SERVER_URL", "http://docling:5001")
	t.Setenv("DOCLING_TIMEOUT", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Ingestion.ParserMethod != "docling" {
		t.Fatalf("parser method = %q", cfg.Ingestion.ParserMethod)
	}
	if cfg.Ingestion.Docling.ServerURL != "http://docling:5001" {
		t.Fatalf("docling server url = %q", cfg.Ingestion.Docling.ServerURL)
	}
	if cfg.Ingestion.Docling.Timeout.String() != "45s" {
		t.Fatalf("docling timeout = %s", cfg.Ingestion.Docling.Timeout)
	}
}

func TestInvalidParserMethod(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("INGEST_PARSER_METHOD", "unknown")
	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid parser method error")
	}
}

func TestRemoteParserRequiresEndpoint(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("INGEST_PARSER_METHOD", "mineru")
	t.Setenv("MINERU_APISERVER", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing MinerU endpoint error")
	}

	t.Setenv("INGEST_PARSER_METHOD", "docling")
	t.Setenv("DOCLING_SERVER_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing Docling endpoint error")
	}
}
