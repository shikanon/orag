package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTutorialCatalogBaseURL(t *testing.T) {
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("TUTORIAL_CATALOG_BASE_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Tutorial.CatalogBaseURL; got != "https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs" {
		t.Fatalf("tutorial catalog base URL = %q", got)
	}
}

func TestLoadTutorialCatalogBaseURLOverride(t *testing.T) {
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("TUTORIAL_CATALOG_BASE_URL", "https://example.test/packs")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tutorial.CatalogBaseURL != "https://example.test/packs" {
		t.Fatalf("tutorial catalog base URL = %q", cfg.Tutorial.CatalogBaseURL)
	}
}

func TestTutorialConfigRejectsInsecureCatalogOutsideTestMode(t *testing.T) {
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("TUTORIAL_CATALOG_BASE_URL", "http://example.test/packs")
	t.Setenv("ORAG_TEST_MODE", "false")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TUTORIAL_CATALOG_BASE_URL") {
		t.Fatalf("Load() error = %v, want secure catalog URL error", err)
	}
}

func TestTutorialConfigAllowsFixtureCatalogOnlyInTestMode(t *testing.T) {
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("TUTORIAL_CATALOG_BASE_URL", "http://127.0.0.1:9999/packs")
	t.Setenv("ORAG_TEST_MODE", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Tutorial.AllowInsecureCatalogHTTP || cfg.Tutorial.MaxManifestBytes != 4*1024*1024 || cfg.Tutorial.MaxObjectBytes != 32*1024*1024*1024 {
		t.Fatalf("tutorial config = %#v", cfg.Tutorial)
	}
}

func TestTutorialPrivateStoreRequiresSeparateAliyunOutputBucket(t *testing.T) {
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("OBJECT_STORAGE_PROVIDER", "aliyun_oss")
	t.Setenv("OBJECT_STORAGE_ENDPOINT", "https://oss-cn-guangzhou.aliyuncs.com")
	t.Setenv("OBJECT_STORAGE_BUCKET_NAME", "lensrhyme")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY_ID", "id")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY_SECRET", "secret")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "must not be the public tutorial catalog bucket") {
		t.Fatalf("Load() error = %v, want public/private bucket separation error", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("ARK_BASE_URL", "")
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("RAG_QUERY_REWRITE_ENABLED", "")
	t.Setenv("RAG_MULTI_QUERY_COUNT", "")
	t.Setenv("RAG_HYDE_ENABLED", "")
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
	if cfg.Ingestion.ContextualRetrieval.Enabled {
		t.Fatal("contextual retrieval should default to disabled")
	}
	if cfg.Ingestion.RAPTOR.Enabled {
		t.Fatal("RAPTOR should default to disabled")
	}
	if !cfg.RAG.QueryRewriteEnabled {
		t.Fatal("expected query rewrite default to be enabled")
	}
	if cfg.RAG.MultiQueryCount != 3 {
		t.Fatalf("default multi-query count = %d, want 3", cfg.RAG.MultiQueryCount)
	}
	if !cfg.RAG.HyDEEnabled {
		t.Fatal("expected HyDE default to be enabled")
	}
	if cfg.RAG.QueryRouter.Enabled {
		t.Fatal("query router should default to disabled")
	}
	if cfg.RAG.GraphRetrieval.Enabled {
		t.Fatal("graph retrieval should default to disabled")
	}
	offline := cfg.Maintenance.OfflineKnowledgeOrganizer
	if offline.Enabled {
		t.Fatal("offline knowledge organizer should default to disabled")
	}
	if offline.Schedule != "0 2 * * *" {
		t.Fatalf("offline knowledge schedule = %q", offline.Schedule)
	}
	if offline.LookbackDays != 7 || offline.MaxQuestionsPerRun != 500 || offline.MaxClustersPerRun != 200 {
		t.Fatalf("offline knowledge defaults = %#v", offline)
	}
	if len(offline.Targets) != 0 {
		t.Fatalf("offline knowledge default targets = %#v, want empty", offline.Targets)
	}
	if offline.MaxCodexConcurrency != 4 || offline.MaxCodexDeepSearchSteps != 12 {
		t.Fatalf("offline knowledge Codex defaults = %#v", offline)
	}
	if offline.CodexEnabled || offline.CodexCommand != "" || offline.CodexEndpoint != "" {
		t.Fatalf("offline knowledge Codex runtime defaults = %#v", offline)
	}
	if offline.ShadowEventTTLDays != 14 {
		t.Fatalf("offline knowledge shadow ttl = %d", offline.ShadowEventTTLDays)
	}
	if offline.MinVerifyConfidence != 0.8 || offline.MinPublishConfidence != 0.9 {
		t.Fatalf("offline knowledge confidence defaults = %#v", offline)
	}
	if !offline.EvidenceValidationEnabled || !offline.ConclusionJudgeEnabled || !offline.ShadowRetrievalEnabled {
		t.Fatalf("offline knowledge feature toggles = %#v", offline)
	}
}

func TestLoadOfflineKnowledgeOrganizerConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_ENABLED", "true")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SCHEDULE", "15 3 * * *")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS", "tenant_1:kb_1, tenant_2:kb_2,tenant_1:kb_1")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_LOOKBACK_DAYS", "14")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_QUESTIONS_PER_RUN", "600")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CLUSTERS_PER_RUN", "300")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENABLED", "true")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_COMMAND", "codex analyze")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENDPOINT", "https://codex.example.test/analyze")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_CONCURRENCY", "6")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_DEEP_SEARCH_STEPS", "16")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_TOKENS_PER_QUESTION", "24000")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_TOOL_QPS_PER_TENANT", "25")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_TOOL_ROWS_PER_CALL", "80")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_REPLAY_CONCURRENCY", "10")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_EVAL_CONCURRENCY", "5")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_QUESTION_OCCURRENCE", "3")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_LONG_TAIL_SAMPLING_RATE", "0.15")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_EXPLICIT_NEGATIVE_FEEDBACK_BOOST", "12")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_VERIFY_CONFIDENCE", "0.81")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_PUBLISH_CONFIDENCE", "0.93")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_EVIDENCE_VALIDATION_ENABLED", "false")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_CONCLUSION_JUDGE_ENABLED", "false")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_RETRIEVAL_ENABLED", "false")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_INJECT_ENABLED", "true")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_EVENT_TTL_DAYS", "21")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_EVENT_SAMPLING_RATE", "0.5")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_AUTO_PUBLISH_ENABLED", "true")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_REGRESSION_EVAL_ENABLED", "false")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_REGRESSION_DATASET_ID", "ds_regression")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_FULL_REGRESSION_FOR_REWRITE_ENABLED", "false")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_RECALL_LIFT", "0.07")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_ANSWER_QUALITY_LIFT", "0.04")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_LATENCY_DELTA_MS", "450")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := cfg.Maintenance.OfflineKnowledgeOrganizer
	if !got.Enabled || got.Schedule != "15 3 * * *" || got.LookbackDays != 14 || got.MaxQuestionsPerRun != 600 || got.MaxClustersPerRun != 300 {
		t.Fatalf("offline knowledge organizer config = %#v", got)
	}
	if len(got.Targets) != 2 || got.Targets[0].TenantID != "tenant_1" || got.Targets[0].KBID != "kb_1" || got.Targets[1].TenantID != "tenant_2" || got.Targets[1].KBID != "kb_2" {
		t.Fatalf("offline knowledge organizer targets = %#v", got.Targets)
	}
	if got.MaxCodexConcurrency != 6 || got.MaxCodexDeepSearchSteps != 16 || got.MaxCodexTokensPerQuestion != 24000 {
		t.Fatalf("offline knowledge Codex config = %#v", got)
	}
	if !got.CodexEnabled || got.CodexCommand != "codex analyze" || got.CodexEndpoint != "https://codex.example.test/analyze" {
		t.Fatalf("offline knowledge Codex runtime config = %#v", got)
	}
	if got.MaxToolQPSPerTenant != 25 || got.MaxToolRowsPerCall != 80 || got.MaxReplayConcurrency != 10 || got.MaxEvalConcurrency != 5 {
		t.Fatalf("offline knowledge quota config = %#v", got)
	}
	if got.MinQuestionOccurrence != 3 || got.LongTailSamplingRate != 0.15 || got.ExplicitNegativeFeedbackBoost != 12 {
		t.Fatalf("offline knowledge mining config = %#v", got)
	}
	if got.MinVerifyConfidence != 0.81 || got.MinPublishConfidence != 0.93 {
		t.Fatalf("offline knowledge confidence config = %#v", got)
	}
	if got.EvidenceValidationEnabled || got.ConclusionJudgeEnabled || got.ShadowRetrievalEnabled || !got.ShadowInjectEnabled {
		t.Fatalf("offline knowledge toggles = %#v", got)
	}
	if got.ShadowEventTTLDays != 21 || got.ShadowEventSamplingRate != 0.5 || !got.AutoPublishEnabled {
		t.Fatalf("offline knowledge shadow config = %#v", got)
	}
	if got.RegressionEvalEnabled || got.RegressionDatasetID != "ds_regression" || got.FullRegressionForRewriteEnabled || got.MinRecallLift != 0.07 || got.MinAnswerQualityLift != 0.04 || got.MaxLatencyDeltaMS != 450 {
		t.Fatalf("offline knowledge regression config = %#v", got)
	}
	env := cfg.RedactedEnv()
	if env["OFFLINE_KNOWLEDGE_ORGANIZER_ENABLED"] != "true" || env["OFFLINE_KNOWLEDGE_ORGANIZER_SCHEDULE"] != "15 3 * * *" || env["OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS"] != "tenant_1:kb_1,tenant_2:kb_2" {
		t.Fatalf("redacted offline knowledge env = %#v", env)
	}
	if env["OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENABLED"] != "true" || env["OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENDPOINT"] == "" {
		t.Fatalf("redacted offline knowledge Codex env = %#v", env)
	}
	if env["OFFLINE_KNOWLEDGE_ORGANIZER_REGRESSION_DATASET_ID"] != "ds_regression" {
		t.Fatalf("redacted offline knowledge regression env = %#v", env)
	}
}

func TestInvalidOfflineKnowledgeOrganizerTargets(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS", "tenant_only")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid offline knowledge target format error")
	}

	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS", "tenant_1:")
	if _, err := Load(); err == nil {
		t.Fatal("expected empty offline knowledge target kb error")
	}
}

func TestInvalidOfflineKnowledgeOrganizerConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SCHEDULE", "not-cron")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid offline knowledge schedule error")
	}

	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SCHEDULE", "0 2 * * *")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENDPOINT", "https://codex.example.test/analyze")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENABLED", "false")
	if _, err := Load(); err == nil {
		t.Fatal("expected Codex endpoint without enablement error")
	}

	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SCHEDULE", "0 2 * * *")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_VERIFY_CONFIDENCE", "0.95")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_PUBLISH_CONFIDENCE", "0.90")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid offline knowledge confidence order error")
	}

	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_VERIFY_CONFIDENCE", "0.80")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_PUBLISH_CONFIDENCE", "0.90")
	t.Setenv("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_EVENT_SAMPLING_RATE", "1.5")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid offline knowledge sampling rate error")
	}
}

func TestLoadQueryRouterConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("RAG_QUERY_ROUTER_ENABLED", "true")
	t.Setenv("RAG_QUERY_ROUTER_STRATEGY", "heuristic")
	t.Setenv("RAG_QUERY_ROUTER_DIRECT_MAX_RUNES", "24")
	t.Setenv("RAG_QUERY_ROUTER_COMPLEX_MIN_SIGNALS", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := cfg.RAG.QueryRouter
	if !got.Enabled || got.Strategy != "heuristic" || got.DirectMaxRunes != 24 || got.ComplexMinSignals != 3 {
		t.Fatalf("query router config = %#v", got)
	}
	env := cfg.RedactedEnv()
	if env["RAG_QUERY_ROUTER_ENABLED"] != "true" || env["RAG_QUERY_ROUTER_STRATEGY"] != "heuristic" {
		t.Fatalf("redacted query router env = %#v", env)
	}
}

func TestLoadRAPTORConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("INGEST_RAPTOR_ENABLED", "true")
	t.Setenv("INGEST_RAPTOR_BRANCH_FACTOR", "3")
	t.Setenv("INGEST_RAPTOR_MAX_LEVELS", "4")
	t.Setenv("INGEST_RAPTOR_MAX_SUMMARY_CHARS", "640")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := cfg.Ingestion.RAPTOR
	if !got.Enabled || got.BranchFactor != 3 || got.MaxLevels != 4 || got.MaxSummaryChars != 640 {
		t.Fatalf("RAPTOR config = %#v", got)
	}
	env := cfg.RedactedEnv()
	if env["INGEST_RAPTOR_ENABLED"] != "true" || env["INGEST_RAPTOR_MAX_LEVELS"] != "4" {
		t.Fatalf("redacted RAPTOR env = %#v", env)
	}
}

func TestLoadGraphRetrievalConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("RAG_GRAPH_RETRIEVAL_ENABLED", "true")
	t.Setenv("RAG_GRAPH_RETRIEVAL_TOP_K", "12")
	t.Setenv("INGEST_GRAPH_MAX_ENTITIES_PER_CHUNK", "5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := cfg.RAG.GraphRetrieval
	if !got.Enabled || got.TopK != 12 || got.MaxEntitiesPerChunk != 5 {
		t.Fatalf("graph retrieval config = %#v", got)
	}
	env := cfg.RedactedEnv()
	if env["RAG_GRAPH_RETRIEVAL_ENABLED"] != "true" || env["RAG_GRAPH_RETRIEVAL_TOP_K"] != "12" {
		t.Fatalf("redacted graph retrieval env = %#v", env)
	}
}

func TestLoadContextualRetrievalConfig(t *testing.T) {
	t.Setenv("ARK_API_KEY", "ark-test-key")
	t.Setenv("INGEST_CONTEXTUAL_RETRIEVAL_ENABLED", "true")
	t.Setenv("INGEST_CONTEXTUAL_MAX_DOCUMENT_CHARS", "9000")
	t.Setenv("INGEST_CONTEXTUAL_MAX_CHUNK_CHARS", "1500")
	t.Setenv("INGEST_CONTEXTUAL_MAX_CONTEXT_CHARS", "320")
	t.Setenv("INGEST_CONTEXTUAL_FAILURE_MODE", "fail")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := cfg.Ingestion.ContextualRetrieval
	if !got.Enabled || got.MaxDocumentChars != 9000 || got.MaxChunkChars != 1500 || got.MaxContextChars != 320 || got.FailureMode != "fail" {
		t.Fatalf("contextual retrieval config = %#v", got)
	}
}

func TestEnvExampleContainsDocumentedOnboardingKeys(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", ".env.example"))
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}

	values := parseEnvExample(t, string(content))
	for _, key := range []string{
		"ADMIN_DEFAULT_USERNAME",
		"ADMIN_DEFAULT_PASSWORD",
		"AUTH_TOKEN_TTL",
		"JWT_SECRET",
		"API_KEY_PEPPER",
		"ARK_API_KEY",
		"REQUIRE_EXTERNAL_PROVIDERS",
		"DATABASE_URL",
		"QDRANT_HOST",
		"QDRANT_GRPC_PORT",
		"QDRANT_COLLECTION",
		"QDRANT_SEMANTIC_CACHE_COLLECTION",
	} {
		if _, ok := values[key]; !ok {
			t.Fatalf(".env.example missing %s", key)
		}
	}
}

func TestRedactedEnv(t *testing.T) {
	t.Setenv("ARK_API_KEY", "abcdefghi")
	t.Setenv("ALIYUN_RERANK_API_KEY", "sk-test-secret")
	t.Setenv("JWT_SECRET", "secret-jwt-value")
	t.Setenv("API_KEY_PEPPER", "secret-api-key-pepper")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/orag?sslmode=disable")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY_ID", "tutorial-access-key")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY_SECRET", "tutorial-access-secret")
	t.Setenv("OBJECT_STORAGE_BUCKET_NAME", "tenant-private-tutorial-bucket")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	env := cfg.RedactedEnv()
	if env["STORAGE_BACKEND"] != "qdrant_postgres" {
		t.Fatalf("redacted env storage backend = %q", env["STORAGE_BACKEND"])
	}
	for _, key := range []string{"ARK_API_KEY", "ALIYUN_RERANK_API_KEY", "JWT_SECRET", "API_KEY_PEPPER", "DATABASE_URL"} {
		if got := env[key]; got == "" || got == "abcdefghi" || got == "sk-test-secret" || got == "secret-jwt-value" || got == "secret-api-key-pepper" || got == "postgres://user:pass@localhost:5432/orag?sslmode=disable" {
			t.Fatalf("expected %s to be redacted, got %q", key, got)
		}
	}
	for _, value := range []string{"tutorial-access-key", "tutorial-access-secret", "tenant-private-tutorial-bucket"} {
		for key, got := range env {
			if got == value {
				t.Fatalf("redacted env leaked tutorial storage value %q in %s", value, key)
			}
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
	t.Setenv("LLM_RERANK_PROVIDER", "")
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

func parseEnvExample(t *testing.T, content string) map[string]string {
	t.Helper()

	values := map[string]string{}
	for lineNo, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			t.Fatalf(".env.example line %d is not KEY=value", lineNo+1)
		}
		values[strings.TrimSpace(key)] = value
	}
	return values
}
