package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	modelprovider "github.com/shikanon/orag/internal/llm/provider"
)

type Config struct {
	Server        ServerConfig
	Storage       StorageConfig
	Auth          AuthConfig
	Database      DatabaseConfig
	Qdrant        QdrantConfig
	Ark           ArkConfig
	Models        ModelProviderConfig
	RAG           RAGConfig
	Ingestion     IngestionConfig
	ObjectStorage ObjectStorageConfig
	Observability ObservabilityConfig
	Maintenance   MaintenanceConfig
	Tutorial      TutorialConfig
}

type StorageConfig struct {
	Backend string
}

type ServerConfig struct {
	Host          string
	Port          int
	PublicBaseURL string
	Debug         bool
	BuildRevision string
}

func (c ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type AuthConfig struct {
	JWTSecret            string
	APIKeyPepper         string
	TokenTTL             time.Duration
	AdminDefaultUsername string
	AdminDefaultPassword string
}

type DatabaseConfig struct {
	URL string
}

type QdrantConfig struct {
	Host                    string
	GRPCPort                int
	APIKey                  string
	UseTLS                  bool
	Collection              string
	SemanticCacheCollection string
	AutoCreateCollections   bool
}

type ArkConfig struct {
	APIKey              string
	BaseURL             string
	ChatModel           string
	EmbeddingModel      string
	EmbeddingDimensions int
	RerankProvider      string
	RerankBaseURL       string
	RerankModel         string
	RerankAPIKey        string
	RerankInstruct      string
	MultimodalModel     string
	Timeout             time.Duration
	RetryTimes          int
	LiveTests           bool
}

type ModelProviderConfig struct {
	ChatProvider           string
	EmbeddingProvider      string
	RerankProvider         string
	MultimodalProvider     string
	AllowDeterministicMock bool
	ProviderAPIKeys        map[string]string
	ProviderBaseURLs       map[string]string
}

type RAGConfig struct {
	DefaultProfile           string
	DenseTopK                int
	SparseTopK               int
	RRFK                     int
	RerankTopN               int
	ContextTopN              int
	MaxContextTokens         int
	SemanticCacheThreshold   float64
	SemanticCacheMaxEntries  int
	QueryRewriteEnabled      bool
	MultiQueryCount          int
	HyDEEnabled              bool
	NoContextAnswer          string
	PromptCacheMode          string
	RequireExternalProviders bool
	QueryRouter              QueryRouterConfig
	GraphRetrieval           GraphRetrievalConfig
}

type QueryRouterConfig struct {
	Enabled           bool
	Strategy          string
	DirectMaxRunes    int
	ComplexMinSignals int
}

type GraphRetrievalConfig struct {
	Enabled             bool
	TopK                int
	MaxEntitiesPerChunk int
}

type IngestionConfig struct {
	ChunkSizeTokens     int
	ChunkOverlapTokens  int
	MaxDocumentBytes    int64
	ParserMethod        string
	ContextualRetrieval ContextualRetrievalConfig
	RAPTOR              RAPTORConfig
	MinerU              MinerUConfig
	Docling             DoclingConfig
}

type RAPTORConfig struct {
	Enabled         bool
	BranchFactor    int
	MaxLevels       int
	MaxSummaryChars int
}

type ContextualRetrievalConfig struct {
	Enabled          bool
	MaxDocumentChars int
	MaxChunkChars    int
	MaxContextChars  int
	FailureMode      string
}

type MinerUConfig struct {
	APIURL      string
	ServerURL   string
	Backend     string
	ParseMethod string
	Lang        string
	Formula     bool
	Table       bool
}

type DoclingConfig struct {
	ServerURL string
	Timeout   time.Duration
}

type ObjectStorageConfig struct {
	Provider        string
	Region          string
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
	CDNDomain       string
	MockUpload      bool
}

type TutorialConfig struct {
	CatalogBaseURL           string
	MaxManifestBytes         int64
	MaxObjectBytes           int64
	HTTPTimeout              time.Duration
	PrivateOutputDirectory   string
	PrivateOutputPrefix      string
	AllowInsecureCatalogHTTP bool
}

type ObservabilityConfig struct {
	OTLPEndpoint        string
	OTLPMetricsEndpoint string
	LangFuseHost        string
	LangFusePublicKey   string
	LangFuseSecretKey   string
	RecordPrompts       bool
	Trace               TracePrivacyConfig
}

type TracePrivacyConfig struct {
	StoreQuery    bool
	QueryMaxBytes int
	RetentionDays int
}

type MaintenanceConfig struct {
	OfflineKnowledgeOrganizer OfflineKnowledgeOrganizerConfig
}

type OfflineKnowledgeOrganizerConfig struct {
	Enabled                         bool
	Schedule                        string
	Targets                         []OfflineKnowledgeOrganizerTargetConfig
	LookbackDays                    int
	MaxQuestionsPerRun              int
	MaxClustersPerRun               int
	CodexEnabled                    bool
	CodexCommand                    string
	CodexEndpoint                   string
	MaxCodexConcurrency             int
	MaxCodexDeepSearchSteps         int
	MaxCodexTokensPerQuestion       int
	MaxToolQPSPerTenant             int
	MaxToolRowsPerCall              int
	MaxReplayConcurrency            int
	MaxEvalConcurrency              int
	MinQuestionOccurrence           int
	LongTailSamplingRate            float64
	ExplicitNegativeFeedbackBoost   int
	MinVerifyConfidence             float64
	MinPublishConfidence            float64
	EvidenceValidationEnabled       bool
	ConclusionJudgeEnabled          bool
	ShadowRetrievalEnabled          bool
	ShadowInjectEnabled             bool
	ShadowEventTTLDays              int
	ShadowEventSamplingRate         float64
	AutoPublishEnabled              bool
	RegressionEvalEnabled           bool
	RegressionDatasetID             string
	FullRegressionForRewriteEnabled bool
	MinRecallLift                   float64
	MinAnswerQualityLift            float64
	MaxLatencyDeltaMS               int
}

type OfflineKnowledgeOrganizerTargetConfig struct {
	TenantID string
	KBID     string
}

func Load() (Config, error) {
	jwtSecret := getenv("JWT_SECRET", getenv("SECRET_KEY", "orag-dev-secret-change-me"))
	cfg := Config{
		Server: ServerConfig{
			Host:          getenv("HOST", "0.0.0.0"),
			Port:          getenvInt("PORT", 8080),
			PublicBaseURL: getenv("PUBLIC_BASE_URL", "http://localhost:8080"),
			Debug:         getenvBool("DEBUG", false),
			BuildRevision: strings.TrimSpace(getenv("ORAG_BUILD_REVISION", "dev")),
		},
		Storage: StorageConfig{
			Backend: getenv("STORAGE_BACKEND", "qdrant_postgres"),
		},
		Auth: AuthConfig{
			JWTSecret:            jwtSecret,
			APIKeyPepper:         getenv("API_KEY_PEPPER", jwtSecret),
			TokenTTL:             getenvDuration("AUTH_TOKEN_TTL", 24*time.Hour),
			AdminDefaultUsername: getenv("ADMIN_DEFAULT_USERNAME", "admin"),
			AdminDefaultPassword: getenv("ADMIN_DEFAULT_PASSWORD", "admin"),
		},
		Database: DatabaseConfig{
			URL: getenv("DATABASE_URL", "postgres://orag:orag@localhost:5432/orag?sslmode=disable"),
		},
		Qdrant: QdrantConfig{
			Host:                    getenv("QDRANT_HOST", "localhost"),
			GRPCPort:                getenvInt("QDRANT_GRPC_PORT", 6334),
			APIKey:                  getenv("QDRANT_API_KEY", ""),
			UseTLS:                  getenvBool("QDRANT_USE_TLS", false),
			Collection:              getenv("QDRANT_COLLECTION", "orag_chunks"),
			SemanticCacheCollection: getenv("QDRANT_SEMANTIC_CACHE_COLLECTION", "orag_semantic_cache"),
			AutoCreateCollections:   getenvBool("QDRANT_AUTO_CREATE_COLLECTIONS", true),
		},
		Ark: ArkConfig{
			APIKey:              getenv("ARK_API_KEY", getenv("LLM_API_KEY", "")),
			BaseURL:             getenv("ARK_BASE_URL", getenv("LLM_API_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3")),
			ChatModel:           getenv("ARK_CHAT_MODEL", "doubao-seed-2-1-pro-260628"),
			EmbeddingModel:      getenv("ARK_EMBEDDING_MODEL", "doubao-embedding-vision-251215"),
			EmbeddingDimensions: getenvInt("ARK_EMBEDDING_DIMENSIONS", 1024),
			RerankProvider:      getenv("RERANK_PROVIDER", "volcengine"),
			RerankBaseURL:       defaultRerankBaseURL(),
			RerankModel:         defaultRerankModel(),
			RerankAPIKey:        getenv("ALIYUN_RERANK_API_KEY", getenv("DASHSCOPE_API_KEY", "")),
			RerankInstruct:      getenv("RERANK_INSTRUCT", "Given a web search query, retrieve relevant passages that answer the query."),
			MultimodalModel:     getenv("ARK_MULTIMODAL_MODEL", "doubao-seed-2-1-pro-260628"),
			Timeout:             getenvDuration("ARK_TIMEOUT", 60*time.Second),
			RetryTimes:          getenvInt("ARK_RETRY_TIMES", 2),
			LiveTests:           getenvBool("LIVE_ARK_TESTS", false),
		},
		RAG: RAGConfig{
			DefaultProfile:           getenv("RAG_DEFAULT_PROFILE", "realtime"),
			DenseTopK:                getenvInt("RAG_DENSE_TOP_K", 50),
			SparseTopK:               getenvInt("RAG_SPARSE_TOP_K", 50),
			RRFK:                     getenvInt("RAG_RRF_K", 60),
			RerankTopN:               getenvInt("RAG_RERANK_TOP_N", 8),
			ContextTopN:              getenvInt("RAG_CONTEXT_TOP_N", 8),
			MaxContextTokens:         getenvInt("RAG_MAX_CONTEXT_TOKENS", 6000),
			SemanticCacheThreshold:   getenvFloat("RAG_SEMANTIC_CACHE_THRESHOLD", 0.92),
			SemanticCacheMaxEntries:  getenvInt("RAG_SEMANTIC_CACHE_MAX_ENTRIES", 10000),
			QueryRewriteEnabled:      getenvBool("RAG_QUERY_REWRITE_ENABLED", true),
			MultiQueryCount:          getenvInt("RAG_MULTI_QUERY_COUNT", 3),
			HyDEEnabled:              getenvBool("RAG_HYDE_ENABLED", true),
			NoContextAnswer:          getenv("RAG_NO_CONTEXT_ANSWER", "未在知识库中检索到足够依据，无法基于上下文回答。"),
			PromptCacheMode:          getenv("PROMPT_CACHE_MODE", "auto"),
			RequireExternalProviders: getenvBool("REQUIRE_EXTERNAL_PROVIDERS", true),
			QueryRouter: QueryRouterConfig{
				Enabled:           getenvBool("RAG_QUERY_ROUTER_ENABLED", false),
				Strategy:          strings.ToLower(strings.TrimSpace(getenv("RAG_QUERY_ROUTER_STRATEGY", "heuristic"))),
				DirectMaxRunes:    getenvInt("RAG_QUERY_ROUTER_DIRECT_MAX_RUNES", 16),
				ComplexMinSignals: getenvInt("RAG_QUERY_ROUTER_COMPLEX_MIN_SIGNALS", 2),
			},
			GraphRetrieval: GraphRetrievalConfig{
				Enabled:             getenvBool("RAG_GRAPH_RETRIEVAL_ENABLED", false),
				TopK:                getenvInt("RAG_GRAPH_RETRIEVAL_TOP_K", 8),
				MaxEntitiesPerChunk: getenvInt("INGEST_GRAPH_MAX_ENTITIES_PER_CHUNK", 6),
			},
		},
		Ingestion: IngestionConfig{
			ChunkSizeTokens:    getenvInt("INGEST_CHUNK_SIZE_TOKENS", 800),
			ChunkOverlapTokens: getenvInt("INGEST_CHUNK_OVERLAP_TOKENS", 120),
			MaxDocumentBytes:   int64(getenvInt("INGEST_MAX_DOCUMENT_BYTES", 25*1024*1024)),
			ParserMethod:       strings.ToLower(strings.TrimSpace(getenv("INGEST_PARSER_METHOD", "basic"))),
			ContextualRetrieval: ContextualRetrievalConfig{
				Enabled:          getenvBool("INGEST_CONTEXTUAL_RETRIEVAL_ENABLED", false),
				MaxDocumentChars: getenvInt("INGEST_CONTEXTUAL_MAX_DOCUMENT_CHARS", 12000),
				MaxChunkChars:    getenvInt("INGEST_CONTEXTUAL_MAX_CHUNK_CHARS", 2000),
				MaxContextChars:  getenvInt("INGEST_CONTEXTUAL_MAX_CONTEXT_CHARS", 500),
				FailureMode:      strings.ToLower(strings.TrimSpace(getenv("INGEST_CONTEXTUAL_FAILURE_MODE", "fallback"))),
			},
			RAPTOR: RAPTORConfig{
				Enabled:         getenvBool("INGEST_RAPTOR_ENABLED", false),
				BranchFactor:    getenvInt("INGEST_RAPTOR_BRANCH_FACTOR", 4),
				MaxLevels:       getenvInt("INGEST_RAPTOR_MAX_LEVELS", 2),
				MaxSummaryChars: getenvInt("INGEST_RAPTOR_MAX_SUMMARY_CHARS", 1000),
			},
			MinerU: MinerUConfig{
				APIURL:      getenv("MINERU_API_URL", getenv("MINERU_APISERVER", "")),
				ServerURL:   getenv("MINERU_SERVER_URL", ""),
				Backend:     getenv("MINERU_BACKEND", "pipeline"),
				ParseMethod: getenv("MINERU_PARSE_METHOD", "auto"),
				Lang:        getenv("MINERU_LANG", "English"),
				Formula:     getenvBool("MINERU_FORMULA_ENABLE", true),
				Table:       getenvBool("MINERU_TABLE_ENABLE", true),
			},
			Docling: DoclingConfig{
				ServerURL: getenv("DOCLING_SERVER_URL", ""),
				Timeout:   getenvDuration("DOCLING_TIMEOUT", 10*time.Minute),
			},
		},
		ObjectStorage: ObjectStorageConfig{
			Provider:        getenv("OBJECT_STORAGE_PROVIDER", "local"),
			Region:          getenv("OBJECT_STORAGE_REGION", ""),
			Endpoint:        getenv("OBJECT_STORAGE_ENDPOINT", ""),
			Bucket:          getenv("OBJECT_STORAGE_BUCKET_NAME", ""),
			AccessKeyID:     getenv("OBJECT_STORAGE_ACCESS_KEY_ID", ""),
			AccessKeySecret: getenv("OBJECT_STORAGE_ACCESS_KEY_SECRET", ""),
			CDNDomain:       getenv("OBJECT_STORAGE_CDN_DOMAIN", ""),
			MockUpload:      getenvBool("OBJECT_STORAGE_MOCK_UPLOAD", true),
		},
		Tutorial: TutorialConfig{
			CatalogBaseURL:           getenv("TUTORIAL_CATALOG_BASE_URL", "https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs"),
			MaxManifestBytes:         int64(getenvInt("TUTORIAL_MAX_MANIFEST_BYTES", 4*1024*1024)),
			MaxObjectBytes:           int64(getenvInt("TUTORIAL_MAX_OBJECT_BYTES", 32*1024*1024*1024)),
			HTTPTimeout:              getenvDuration("TUTORIAL_HTTP_TIMEOUT", 2*time.Minute),
			PrivateOutputDirectory:   getenv("TUTORIAL_PRIVATE_OUTPUT_DIR", "./.orag/tutorial-packs"),
			PrivateOutputPrefix:      getenv("TUTORIAL_PRIVATE_OUTPUT_PREFIX", "tutorial-experiments"),
			AllowInsecureCatalogHTTP: getenvBool("ORAG_TEST_MODE", false),
		},
		Observability: ObservabilityConfig{
			OTLPEndpoint:        getenv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			OTLPMetricsEndpoint: getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", ""),
			LangFuseHost:        getenv("LANGFUSE_HOST", ""),
			LangFusePublicKey:   getenv("LANGFUSE_PUBLIC_KEY", ""),
			LangFuseSecretKey:   getenv("LANGFUSE_SECRET_KEY", ""),
			RecordPrompts:       getenvBool("OBSERVABILITY_RECORD_PROMPTS", false),
			Trace: TracePrivacyConfig{
				StoreQuery:    getenvBool("TRACE_STORE_QUERY", true),
				QueryMaxBytes: getenvInt("TRACE_QUERY_MAX_BYTES", 2048),
				RetentionDays: getenvInt("TRACE_RETENTION_DAYS", 30),
			},
		},
		Maintenance: MaintenanceConfig{
			OfflineKnowledgeOrganizer: OfflineKnowledgeOrganizerConfig{
				Enabled:                         getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_ENABLED", false),
				Schedule:                        getenv("OFFLINE_KNOWLEDGE_ORGANIZER_SCHEDULE", "0 2 * * *"),
				Targets:                         nil,
				LookbackDays:                    getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_LOOKBACK_DAYS", 7),
				MaxQuestionsPerRun:              getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_QUESTIONS_PER_RUN", 500),
				MaxClustersPerRun:               getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CLUSTERS_PER_RUN", 200),
				CodexEnabled:                    getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENABLED", false),
				CodexCommand:                    getenv("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_COMMAND", ""),
				CodexEndpoint:                   getenv("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENDPOINT", ""),
				MaxCodexConcurrency:             getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_CONCURRENCY", 4),
				MaxCodexDeepSearchSteps:         getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_DEEP_SEARCH_STEPS", 12),
				MaxCodexTokensPerQuestion:       getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_TOKENS_PER_QUESTION", 20000),
				MaxToolQPSPerTenant:             getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_TOOL_QPS_PER_TENANT", 20),
				MaxToolRowsPerCall:              getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_TOOL_ROWS_PER_CALL", 50),
				MaxReplayConcurrency:            getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_REPLAY_CONCURRENCY", 8),
				MaxEvalConcurrency:              getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_EVAL_CONCURRENCY", 4),
				MinQuestionOccurrence:           getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_QUESTION_OCCURRENCE", 2),
				LongTailSamplingRate:            getenvFloat("OFFLINE_KNOWLEDGE_ORGANIZER_LONG_TAIL_SAMPLING_RATE", 0.05),
				ExplicitNegativeFeedbackBoost:   getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_EXPLICIT_NEGATIVE_FEEDBACK_BOOST", 10),
				MinVerifyConfidence:             getenvFloat("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_VERIFY_CONFIDENCE", 0.8),
				MinPublishConfidence:            getenvFloat("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_PUBLISH_CONFIDENCE", 0.9),
				EvidenceValidationEnabled:       getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_EVIDENCE_VALIDATION_ENABLED", true),
				ConclusionJudgeEnabled:          getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_CONCLUSION_JUDGE_ENABLED", true),
				ShadowRetrievalEnabled:          getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_RETRIEVAL_ENABLED", true),
				ShadowInjectEnabled:             getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_INJECT_ENABLED", false),
				ShadowEventTTLDays:              getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_EVENT_TTL_DAYS", 14),
				ShadowEventSamplingRate:         getenvFloat("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_EVENT_SAMPLING_RATE", 1.0),
				AutoPublishEnabled:              getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_AUTO_PUBLISH_ENABLED", false),
				RegressionEvalEnabled:           getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_REGRESSION_EVAL_ENABLED", true),
				RegressionDatasetID:             getenv("OFFLINE_KNOWLEDGE_ORGANIZER_REGRESSION_DATASET_ID", ""),
				FullRegressionForRewriteEnabled: getenvBool("OFFLINE_KNOWLEDGE_ORGANIZER_FULL_REGRESSION_FOR_REWRITE_ENABLED", true),
				MinRecallLift:                   getenvFloat("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_RECALL_LIFT", 0.05),
				MinAnswerQualityLift:            getenvFloat("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_ANSWER_QUALITY_LIFT", 0.03),
				MaxLatencyDeltaMS:               getenvInt("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_LATENCY_DELTA_MS", 300),
			},
		},
	}
	targets, err := parseOfflineKnowledgeOrganizerTargets(getenv("OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS", ""))
	if err != nil {
		return Config{}, err
	}
	cfg.Maintenance.OfflineKnowledgeOrganizer.Targets = targets
	cfg.Models = loadModelProviders()
	cfg.applyModelProviderDefaults()
	if cfg.Ark.APIKey == "" {
		cfg.Ark.APIKey = cfg.ModelProviderAPIKey(modelprovider.VolcEngine)
	}
	cfg.Ark.RerankProvider = cfg.Models.RerankProvider

	return cfg, cfg.Validate()
}

func (c *Config) applyModelProviderDefaults() {
	registry := modelprovider.BuiltinRegistry()
	c.Ark.ChatModel = selectedProviderModel(registry, c.Models.ChatProvider, modelprovider.CapabilityChat, "ARK_CHAT_MODEL", c.Ark.ChatModel)
	c.Ark.EmbeddingModel = selectedProviderModel(registry, c.Models.EmbeddingProvider, modelprovider.CapabilityEmbedding, "ARK_EMBEDDING_MODEL", c.Ark.EmbeddingModel)
	c.Ark.RerankModel = selectedProviderModel(registry, c.Models.RerankProvider, modelprovider.CapabilityRerank, "ARK_RERANK_MODEL", c.Ark.RerankModel)
	c.Ark.MultimodalModel = selectedProviderModel(registry, c.Models.MultimodalProvider, modelprovider.CapabilityImage2Text, "ARK_MULTIMODAL_MODEL", c.Ark.MultimodalModel)
}

func selectedProviderModel(registry modelprovider.Registry, provider string, capability modelprovider.Capability, envKey string, current string) string {
	if value, ok := os.LookupEnv(envKey); ok && strings.TrimSpace(value) != "" {
		return current
	}
	info, ok := registry.Get(modelprovider.NormalizeName(provider))
	if !ok {
		return current
	}
	if model := strings.TrimSpace(info.DefaultModels[capability]); model != "" {
		return model
	}
	if capability == modelprovider.CapabilityImage2Text {
		if model := strings.TrimSpace(info.DefaultModels[modelprovider.CapabilityChat]); model != "" {
			return model
		}
	}
	return current
}

func (c Config) Validate() error {
	var missing []string
	if c.Auth.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if c.Auth.APIKeyPepper == "" {
		missing = append(missing, "API_KEY_PEPPER")
	}
	if err := c.validateModelProviders(&missing); err != nil {
		return err
	}
	if c.Qdrant.Host == "" {
		missing = append(missing, "QDRANT_HOST")
	}
	if c.Server.BuildRevision != "" && (len(c.Server.BuildRevision) > 200 || strings.IndexFunc(c.Server.BuildRevision, func(r rune) bool { return r <= ' ' || r == 0x7f }) >= 0) {
		return errors.New("ORAG_BUILD_REVISION must be an identifier without whitespace or control characters")
	}
	if c.Database.URL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if c.Storage.Backend != "qdrant_postgres" && c.Storage.Backend != "memory" {
		return errors.New("STORAGE_BACKEND must be qdrant_postgres or memory")
	}
	if c.Ingestion.ParserMethod != "basic" && c.Ingestion.ParserMethod != "mineru" && c.Ingestion.ParserMethod != "docling" {
		return errors.New("INGEST_PARSER_METHOD must be basic, mineru, or docling")
	}
	if c.Ingestion.ContextualRetrieval.FailureMode != "fallback" && c.Ingestion.ContextualRetrieval.FailureMode != "fail" {
		return errors.New("INGEST_CONTEXTUAL_FAILURE_MODE must be fallback or fail")
	}
	if c.Ingestion.RAPTOR.BranchFactor < 2 {
		return errors.New("INGEST_RAPTOR_BRANCH_FACTOR must be at least 2")
	}
	if c.Ingestion.RAPTOR.MaxLevels <= 0 {
		return errors.New("INGEST_RAPTOR_MAX_LEVELS must be positive")
	}
	if c.Ingestion.RAPTOR.MaxSummaryChars <= 0 {
		return errors.New("INGEST_RAPTOR_MAX_SUMMARY_CHARS must be positive")
	}
	if c.Ingestion.ParserMethod == "mineru" && strings.TrimSpace(c.Ingestion.MinerU.APIURL) == "" {
		return errors.New("MINERU_APISERVER or MINERU_API_URL is required when INGEST_PARSER_METHOD=mineru")
	}
	if c.Ingestion.ParserMethod == "docling" && strings.TrimSpace(c.Ingestion.Docling.ServerURL) == "" {
		return errors.New("DOCLING_SERVER_URL is required when INGEST_PARSER_METHOD=docling")
	}
	if c.Ark.EmbeddingDimensions <= 0 {
		return errors.New("ARK_EMBEDDING_DIMENSIONS must be positive")
	}
	if c.RAG.RRFK <= 0 {
		return errors.New("RAG_RRF_K must be positive")
	}
	if c.RAG.SemanticCacheThreshold <= 0 || c.RAG.SemanticCacheThreshold > 1 {
		return errors.New("RAG_SEMANTIC_CACHE_THRESHOLD must be in (0, 1]")
	}
	if c.RAG.QueryRouter.Enabled && c.RAG.QueryRouter.Strategy != "heuristic" {
		return errors.New("RAG_QUERY_ROUTER_STRATEGY must be heuristic")
	}
	if c.RAG.QueryRouter.DirectMaxRunes <= 0 {
		return errors.New("RAG_QUERY_ROUTER_DIRECT_MAX_RUNES must be positive")
	}
	if c.RAG.QueryRouter.ComplexMinSignals <= 0 {
		return errors.New("RAG_QUERY_ROUTER_COMPLEX_MIN_SIGNALS must be positive")
	}
	if c.RAG.GraphRetrieval.TopK <= 0 {
		return errors.New("RAG_GRAPH_RETRIEVAL_TOP_K must be positive")
	}
	if c.RAG.GraphRetrieval.MaxEntitiesPerChunk <= 0 {
		return errors.New("INGEST_GRAPH_MAX_ENTITIES_PER_CHUNK must be positive")
	}
	if err := c.Tutorial.Validate(); err != nil {
		return err
	}
	if err := c.validateTutorialPrivateStore(); err != nil {
		return err
	}
	if c.Observability.Trace.QueryMaxBytes < 0 {
		return errors.New("TRACE_QUERY_MAX_BYTES must be non-negative")
	}
	if c.Observability.Trace.RetentionDays <= 0 {
		return errors.New("TRACE_RETENTION_DAYS must be positive")
	}
	if err := c.Maintenance.OfflineKnowledgeOrganizer.Validate(); err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c TutorialConfig) Validate() error {
	catalog, err := url.Parse(strings.TrimSpace(c.CatalogBaseURL))
	if err != nil || catalog.Host == "" || catalog.User != nil || (catalog.Scheme != "https" && !(c.AllowInsecureCatalogHTTP && catalog.Scheme == "http")) {
		return errors.New("TUTORIAL_CATALOG_BASE_URL must be an absolute HTTPS URL")
	}
	if c.MaxManifestBytes <= 0 {
		return errors.New("TUTORIAL_MAX_MANIFEST_BYTES must be positive")
	}
	if c.MaxObjectBytes <= 0 || c.MaxObjectBytes < c.MaxManifestBytes {
		return errors.New("TUTORIAL_MAX_OBJECT_BYTES must be positive and at least TUTORIAL_MAX_MANIFEST_BYTES")
	}
	if c.HTTPTimeout <= 0 {
		return errors.New("TUTORIAL_HTTP_TIMEOUT must be positive")
	}
	if strings.TrimSpace(c.PrivateOutputDirectory) == "" {
		return errors.New("TUTORIAL_PRIVATE_OUTPUT_DIR must not be empty")
	}
	prefix := strings.Trim(strings.TrimSpace(c.PrivateOutputPrefix), "/")
	if prefix == "" || strings.Contains(prefix, "\\") || strings.Contains(prefix, "..") || strings.Contains(prefix, "/") {
		return errors.New("TUTORIAL_PRIVATE_OUTPUT_PREFIX must be a single relative path component")
	}
	return nil
}

func (c Config) validateTutorialPrivateStore() error {
	switch strings.ToLower(strings.TrimSpace(c.ObjectStorage.Provider)) {
	case "local":
		return nil
	case "aliyun_oss":
		if strings.TrimSpace(c.ObjectStorage.Endpoint) == "" || strings.TrimSpace(c.ObjectStorage.Bucket) == "" || strings.TrimSpace(c.ObjectStorage.AccessKeyID) == "" || strings.TrimSpace(c.ObjectStorage.AccessKeySecret) == "" {
			return errors.New("OBJECT_STORAGE_ENDPOINT, OBJECT_STORAGE_BUCKET_NAME, OBJECT_STORAGE_ACCESS_KEY_ID, and OBJECT_STORAGE_ACCESS_KEY_SECRET are required when OBJECT_STORAGE_PROVIDER=aliyun_oss")
		}
		catalog, _ := url.Parse(c.Tutorial.CatalogBaseURL)
		if strings.HasPrefix(catalog.Host, strings.TrimSpace(c.ObjectStorage.Bucket)+".") {
			return errors.New("OBJECT_STORAGE_BUCKET_NAME must not be the public tutorial catalog bucket")
		}
		return nil
	default:
		return errors.New("OBJECT_STORAGE_PROVIDER must be local or aliyun_oss for tutorial Pack installation")
	}
}

func (c OfflineKnowledgeOrganizerConfig) Validate() error {
	if strings.TrimSpace(c.Schedule) == "" || len(strings.Fields(c.Schedule)) != 5 {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_SCHEDULE must be a 5-field cron expression")
	}
	positive := map[string]int{
		"OFFLINE_KNOWLEDGE_ORGANIZER_LOOKBACK_DAYS":                    c.LookbackDays,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_QUESTIONS_PER_RUN":            c.MaxQuestionsPerRun,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CLUSTERS_PER_RUN":             c.MaxClustersPerRun,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_CONCURRENCY":            c.MaxCodexConcurrency,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_DEEP_SEARCH_STEPS":      c.MaxCodexDeepSearchSteps,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CODEX_TOKENS_PER_QUESTION":    c.MaxCodexTokensPerQuestion,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_TOOL_QPS_PER_TENANT":          c.MaxToolQPSPerTenant,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_TOOL_ROWS_PER_CALL":           c.MaxToolRowsPerCall,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_REPLAY_CONCURRENCY":           c.MaxReplayConcurrency,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_EVAL_CONCURRENCY":             c.MaxEvalConcurrency,
		"OFFLINE_KNOWLEDGE_ORGANIZER_MIN_QUESTION_OCCURRENCE":          c.MinQuestionOccurrence,
		"OFFLINE_KNOWLEDGE_ORGANIZER_EXPLICIT_NEGATIVE_FEEDBACK_BOOST": c.ExplicitNegativeFeedbackBoost,
		"OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_EVENT_TTL_DAYS":            c.ShadowEventTTLDays,
	}
	for key, value := range positive {
		if value <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
	}
	if c.MaxLatencyDeltaMS < 0 {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_MAX_LATENCY_DELTA_MS must be non-negative")
	}
	if !c.CodexEnabled && (strings.TrimSpace(c.CodexCommand) != "" || strings.TrimSpace(c.CodexEndpoint) != "") {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENABLED must be true when Codex command or endpoint is configured")
	}
	if !isRate(c.LongTailSamplingRate) {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_LONG_TAIL_SAMPLING_RATE must be in [0, 1]")
	}
	if !isRate(c.ShadowEventSamplingRate) {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_EVENT_SAMPLING_RATE must be in [0, 1]")
	}
	if !isRate(c.MinVerifyConfidence) || c.MinVerifyConfidence <= 0 {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_VERIFY_CONFIDENCE must be in (0, 1]")
	}
	if !isRate(c.MinPublishConfidence) || c.MinPublishConfidence <= 0 {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_PUBLISH_CONFIDENCE must be in (0, 1]")
	}
	if c.MinPublishConfidence < c.MinVerifyConfidence {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_PUBLISH_CONFIDENCE must be greater than or equal to OFFLINE_KNOWLEDGE_ORGANIZER_MIN_VERIFY_CONFIDENCE")
	}
	if c.MinRecallLift < 0 {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_RECALL_LIFT must be non-negative")
	}
	if c.MinAnswerQualityLift < 0 {
		return errors.New("OFFLINE_KNOWLEDGE_ORGANIZER_MIN_ANSWER_QUALITY_LIFT must be non-negative")
	}
	return nil
}

func isRate(value float64) bool {
	return value >= 0 && value <= 1
}

func (c Config) RedactedEnv() map[string]string {
	return map[string]string{
		"HOST":                                              c.Server.Host,
		"PORT":                                              strconv.Itoa(c.Server.Port),
		"DATABASE_URL":                                      redact(c.Database.URL),
		"STORAGE_BACKEND":                                   c.Storage.Backend,
		"QDRANT_HOST":                                       c.Qdrant.Host,
		"QDRANT_GRPC_PORT":                                  strconv.Itoa(c.Qdrant.GRPCPort),
		"QDRANT_COLLECTION":                                 c.Qdrant.Collection,
		"QDRANT_SEMANTIC_CACHE_COLLECTION":                  c.Qdrant.SemanticCacheCollection,
		"ARK_BASE_URL":                                      c.Ark.BaseURL,
		"ARK_API_KEY":                                       redact(c.Ark.APIKey),
		"ARK_CHAT_MODEL":                                    c.Ark.ChatModel,
		"ARK_EMBEDDING_MODEL":                               c.Ark.EmbeddingModel,
		"LLM_CHAT_PROVIDER":                                 c.Models.ChatProvider,
		"LLM_EMBEDDING_PROVIDER":                            c.Models.EmbeddingProvider,
		"LLM_RERANK_PROVIDER":                               c.Models.RerankProvider,
		"LLM_MULTIMODAL_PROVIDER":                           c.Models.MultimodalProvider,
		"ALLOW_DETERMINISTIC_MOCK":                          strconv.FormatBool(c.Models.AllowDeterministicMock),
		"INGEST_PARSER_METHOD":                              c.Ingestion.ParserMethod,
		"INGEST_CONTEXTUAL_RETRIEVAL_ENABLED":               strconv.FormatBool(c.Ingestion.ContextualRetrieval.Enabled),
		"INGEST_CONTEXTUAL_FAILURE_MODE":                    c.Ingestion.ContextualRetrieval.FailureMode,
		"INGEST_RAPTOR_ENABLED":                             strconv.FormatBool(c.Ingestion.RAPTOR.Enabled),
		"INGEST_RAPTOR_MAX_LEVELS":                          strconv.Itoa(c.Ingestion.RAPTOR.MaxLevels),
		"MINERU_APISERVER":                                  c.Ingestion.MinerU.APIURL,
		"MINERU_SERVER_URL":                                 c.Ingestion.MinerU.ServerURL,
		"MINERU_BACKEND":                                    c.Ingestion.MinerU.Backend,
		"MINERU_PARSE_METHOD":                               c.Ingestion.MinerU.ParseMethod,
		"DOCLING_SERVER_URL":                                c.Ingestion.Docling.ServerURL,
		"RERANK_PROVIDER":                                   c.Ark.RerankProvider,
		"ARK_RERANK_MODEL":                                  c.Ark.RerankModel,
		"ALIYUN_RERANK_API_KEY":                             redact(c.Ark.RerankAPIKey),
		"JWT_SECRET":                                        redact(c.Auth.JWTSecret),
		"API_KEY_PEPPER":                                    redact(c.Auth.APIKeyPepper),
		"RAG_QUERY_REWRITE_ENABLED":                         strconv.FormatBool(c.RAG.QueryRewriteEnabled),
		"RAG_MULTI_QUERY_COUNT":                             strconv.Itoa(c.RAG.MultiQueryCount),
		"RAG_HYDE_ENABLED":                                  strconv.FormatBool(c.RAG.HyDEEnabled),
		"RAG_QUERY_ROUTER_ENABLED":                          strconv.FormatBool(c.RAG.QueryRouter.Enabled),
		"RAG_QUERY_ROUTER_STRATEGY":                         c.RAG.QueryRouter.Strategy,
		"RAG_GRAPH_RETRIEVAL_ENABLED":                       strconv.FormatBool(c.RAG.GraphRetrieval.Enabled),
		"RAG_GRAPH_RETRIEVAL_TOP_K":                         strconv.Itoa(c.RAG.GraphRetrieval.TopK),
		"PROMPT_CACHE_MODE":                                 c.RAG.PromptCacheMode,
		"OFFLINE_KNOWLEDGE_ORGANIZER_ENABLED":               strconv.FormatBool(c.Maintenance.OfflineKnowledgeOrganizer.Enabled),
		"OFFLINE_KNOWLEDGE_ORGANIZER_SCHEDULE":              c.Maintenance.OfflineKnowledgeOrganizer.Schedule,
		"OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS":               formatOfflineKnowledgeOrganizerTargets(c.Maintenance.OfflineKnowledgeOrganizer.Targets),
		"OFFLINE_KNOWLEDGE_ORGANIZER_LOOKBACK_DAYS":         strconv.Itoa(c.Maintenance.OfflineKnowledgeOrganizer.LookbackDays),
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_QUESTIONS_PER_RUN": strconv.Itoa(c.Maintenance.OfflineKnowledgeOrganizer.MaxQuestionsPerRun),
		"OFFLINE_KNOWLEDGE_ORGANIZER_MAX_CLUSTERS_PER_RUN":  strconv.Itoa(c.Maintenance.OfflineKnowledgeOrganizer.MaxClustersPerRun),
		"OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENABLED":         strconv.FormatBool(c.Maintenance.OfflineKnowledgeOrganizer.CodexEnabled),
		"OFFLINE_KNOWLEDGE_ORGANIZER_CODEX_ENDPOINT":        redact(c.Maintenance.OfflineKnowledgeOrganizer.CodexEndpoint),
		"OFFLINE_KNOWLEDGE_ORGANIZER_SHADOW_EVENT_TTL_DAYS": strconv.Itoa(c.Maintenance.OfflineKnowledgeOrganizer.ShadowEventTTLDays),
		"OFFLINE_KNOWLEDGE_ORGANIZER_REGRESSION_DATASET_ID": c.Maintenance.OfflineKnowledgeOrganizer.RegressionDatasetID,
	}
}

func parseOfflineKnowledgeOrganizerTargets(raw string) ([]OfflineKnowledgeOrganizerTargetConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	targets := make([]OfflineKnowledgeOrganizerTargetConfig, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		pieces := strings.Split(part, ":")
		if len(pieces) != 2 {
			return nil, fmt.Errorf("OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS entry %q must use tenant:kb format", part)
		}
		target := OfflineKnowledgeOrganizerTargetConfig{
			TenantID: strings.TrimSpace(pieces[0]),
			KBID:     strings.TrimSpace(pieces[1]),
		}
		if target.TenantID == "" || target.KBID == "" {
			return nil, fmt.Errorf("OFFLINE_KNOWLEDGE_ORGANIZER_TARGETS entry %q must include tenant and kb", part)
		}
		key := target.TenantID + "\x00" + target.KBID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}
	return targets, nil
}

func formatOfflineKnowledgeOrganizerTargets(targets []OfflineKnowledgeOrganizerTargetConfig) string {
	if len(targets) == 0 {
		return ""
	}
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.TenantID == "" && target.KBID == "" {
			continue
		}
		parts = append(parts, target.TenantID+":"+target.KBID)
	}
	return strings.Join(parts, ",")
}

func loadModelProviders() ModelProviderConfig {
	registry := modelprovider.BuiltinRegistry()
	cfg := ModelProviderConfig{
		ChatProvider:           string(modelprovider.NormalizeName(getenv("LLM_CHAT_PROVIDER", getenv("LLM_PROVIDER", string(modelprovider.VolcEngine))))),
		EmbeddingProvider:      string(modelprovider.NormalizeName(getenv("LLM_EMBEDDING_PROVIDER", string(modelprovider.VolcEngine)))),
		RerankProvider:         string(modelprovider.NormalizeName(getenv("LLM_RERANK_PROVIDER", getenv("RERANK_PROVIDER", string(modelprovider.VolcEngine))))),
		MultimodalProvider:     string(modelprovider.NormalizeName(getenv("LLM_MULTIMODAL_PROVIDER", string(modelprovider.VolcEngine)))),
		AllowDeterministicMock: getenvBool("ALLOW_DETERMINISTIC_MOCK", false) || getenvBool("ORAG_TEST_MODE", false),
		ProviderAPIKeys:        map[string]string{},
		ProviderBaseURLs:       map[string]string{},
	}
	for _, name := range registry.Names() {
		info := registry.MustGet(name)
		for _, key := range info.RequiredEnv {
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				cfg.ProviderAPIKeys[string(info.Name)] = value
				break
			}
		}
		if value := providerBaseURLFromEnv(info.Name); value != "" {
			cfg.ProviderBaseURLs[string(info.Name)] = value
		}
	}
	return cfg
}

func (c Config) validateModelProviders(missing *[]string) error {
	registry := modelprovider.BuiltinRegistry()
	missingKeys := map[modelprovider.Name]bool{}
	missingBaseURLs := map[modelprovider.Name]bool{}
	checks := []struct {
		provider   string
		capability modelprovider.Capability
		env        string
	}{
		{provider: c.Models.ChatProvider, capability: modelprovider.CapabilityChat, env: "LLM_CHAT_PROVIDER"},
		{provider: c.Models.EmbeddingProvider, capability: modelprovider.CapabilityEmbedding, env: "LLM_EMBEDDING_PROVIDER"},
		{provider: c.Models.RerankProvider, capability: modelprovider.CapabilityRerank, env: "LLM_RERANK_PROVIDER"},
		{provider: c.Models.MultimodalProvider, capability: modelprovider.CapabilityImage2Text, env: "LLM_MULTIMODAL_PROVIDER"},
	}
	for _, check := range checks {
		name := modelprovider.NormalizeName(check.provider)
		if name == "" {
			return fmt.Errorf("%s must not be empty", check.env)
		}
		info, ok := registry.Get(name)
		if !ok {
			return fmt.Errorf("%s %q is not supported", check.env, check.provider)
		}
		if !info.Supports(check.capability) {
			return fmt.Errorf("%s provider %q does not support %s", check.env, info.Name, check.capability)
		}
		if info.Name == modelprovider.Mock && !c.Models.AllowDeterministicMock {
			return fmt.Errorf("ALLOW_DETERMINISTIC_MOCK=true is required when %s=mock", check.env)
		}
		if info.Name != modelprovider.Mock && c.ModelProviderAPIKey(info.Name) == "" {
			if !missingKeys[info.Name] {
				*missing = append(*missing, fmt.Sprintf("%s (%s)", strings.Join(info.RequiredEnv, " or "), info.Name))
				missingKeys[info.Name] = true
			}
		}
		if info.Name != modelprovider.Mock && c.ModelProviderBaseURL(info.Name) == "" && modelprovider.DefaultBaseURL(info.Name) == "" {
			if !missingBaseURLs[info.Name] {
				*missing = append(*missing, fmt.Sprintf("%s_BASE_URL (%s)", providerEnvPrefix(info.Name), info.Name))
				missingBaseURLs[info.Name] = true
			}
		}
	}
	return nil
}

func (c Config) ModelProviderAPIKey(name modelprovider.Name) string {
	if c.Models.ProviderAPIKeys == nil {
		return ""
	}
	if value := strings.TrimSpace(c.Models.ProviderAPIKeys[string(modelprovider.NormalizeName(string(name)))]); value != "" {
		return value
	}
	if info, ok := modelprovider.BuiltinRegistry().Get(name); ok {
		return strings.TrimSpace(c.Models.ProviderAPIKeys[string(info.Name)])
	}
	return ""
}

func (c Config) ModelProviderBaseURL(name modelprovider.Name) string {
	if c.Models.ProviderBaseURLs == nil {
		return ""
	}
	if value := strings.TrimSpace(c.Models.ProviderBaseURLs[string(modelprovider.NormalizeName(string(name)))]); value != "" {
		return value
	}
	if info, ok := modelprovider.BuiltinRegistry().Get(name); ok {
		return strings.TrimSpace(c.Models.ProviderBaseURLs[string(info.Name)])
	}
	return ""
}

func providerBaseURLFromEnv(name modelprovider.Name) string {
	if name == modelprovider.VolcEngine {
		return getenv("ARK_BASE_URL", getenv("LLM_API_BASE_URL", ""))
	}
	return strings.TrimSpace(os.Getenv(providerEnvPrefix(name) + "_BASE_URL"))
}

func providerEnvPrefix(name modelprovider.Name) string {
	return strings.ToUpper(strings.ReplaceAll(string(name), "-", "_"))
}

func redact(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 6 {
		return "***"
	}
	return v[:3] + "***" + v[len(v)-3:]
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		return v
	}
	return fallback
}

func defaultRerankBaseURL() string {
	if strings.EqualFold(getenv("RERANK_PROVIDER", "volcengine"), "aliyun") {
		return getenv("ALIYUN_RERANK_BASE_URL", "https://dashscope.aliyuncs.com/compatible-api/v1")
	}
	return getenv("ARK_RERANK_BASE_URL", getenv("ARK_BASE_URL", getenv("LLM_API_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3")))
}

func defaultRerankModel() string {
	if strings.EqualFold(getenv("RERANK_PROVIDER", "volcengine"), "aliyun") {
		return getenv("ALIYUN_RERANK_MODEL", "qwen3-rerank")
	}
	return getenv("ARK_RERANK_MODEL", "m3-v2-rerank")
}

func getenvInt(key string, fallback int) int {
	v := getenv(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getenvFloat(key string, fallback float64) float64 {
	v := getenv(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getenvBool(key string, fallback bool) bool {
	v := strings.ToLower(getenv(key, ""))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	v := getenv(key, "")
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
