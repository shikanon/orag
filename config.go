package orag

import (
	"log/slog"
	"time"

	internalconfig "github.com/shikanon/orag/internal/config"
)

const (
	// StorageMemory keeps all data in process and is intended for examples,
	// tests, and local exploration.
	StorageMemory = "memory"
	// StoragePostgresQdrant uses PostgreSQL for metadata and Qdrant for dense
	// retrieval and semantic cache.
	StoragePostgresQdrant = "qdrant_postgres"
)

// Config configures an embedded ORAG client without reading the environment.
type Config struct {
	TenantID  string
	Storage   StorageConfig
	Models    ModelConfig
	RAG       RAGConfig
	Ingestion IngestionConfig
	Logger    *slog.Logger
}

// StorageConfig configures embedded persistence and vector search.
type StorageConfig struct {
	Backend                     string
	DatabaseURL                 string
	QdrantHost                  string
	QdrantGRPCPort              int
	QdrantAPIKey                string
	QdrantUseTLS                bool
	QdrantCollection            string
	QdrantSemanticCache         string
	QdrantAutoCreateCollections bool
}

// ModelConfig selects providers and model identifiers used by the embedded
// application.
type ModelConfig struct {
	ChatProvider           string
	EmbeddingProvider      string
	RerankProvider         string
	MultimodalProvider     string
	APIKeys                map[string]string
	BaseURLs               map[string]string
	ChatModel              string
	EmbeddingModel         string
	EmbeddingDimensions    int
	RerankModel            string
	MultimodalModel        string
	RerankInstruction      string
	AllowDeterministicMock bool
	Timeout                time.Duration
	RetryTimes             int
}

// RAGConfig controls the default embedded query pipeline.
type RAGConfig struct {
	DefaultProfile         string
	DenseTopK              int
	SparseTopK             int
	RRFK                   int
	RerankTopN             int
	ContextTopN            int
	MaxContextTokens       int
	SemanticCacheThreshold float64
	SemanticCacheMaxItems  int
	NoContextAnswer        string
}

// IngestionConfig controls basic text and file ingestion.
type IngestionConfig struct {
	ChunkSizeTokens   int
	ChunkOverlap      int
	MaxDocumentBytes  int64
	ParserMethod      string
	DoclingServerURL  string
	DoclingTimeout    time.Duration
	MinerUAPIURL      string
	MinerUServerURL   string
	MinerUBackend     string
	MinerUParseMethod string
}

// DefaultConfig returns explicit production-oriented defaults. Callers must
// provide credentials for the selected real model providers.
func DefaultConfig() Config {
	return Config{
		TenantID: defaultTenantID,
		Storage: StorageConfig{
			Backend:                     StoragePostgresQdrant,
			DatabaseURL:                 "postgres://orag:orag@localhost:5432/orag?sslmode=disable",
			QdrantHost:                  "localhost",
			QdrantGRPCPort:              6334,
			QdrantCollection:            "orag_chunks",
			QdrantSemanticCache:         "orag_semantic_cache",
			QdrantAutoCreateCollections: true,
		},
		Models: ModelConfig{
			ChatProvider:        "volcengine",
			EmbeddingProvider:   "volcengine",
			RerankProvider:      "volcengine",
			MultimodalProvider:  "volcengine",
			APIKeys:             map[string]string{},
			BaseURLs:            map[string]string{},
			ChatModel:           "doubao-seed-2-1-pro-260628",
			EmbeddingModel:      "doubao-embedding-vision-251215",
			EmbeddingDimensions: 1024,
			RerankModel:         "m3-v2-rerank",
			MultimodalModel:     "doubao-seed-2-1-pro-260628",
			RerankInstruction:   "Given a web search query, retrieve relevant passages that answer the query.",
			Timeout:             60 * time.Second,
			RetryTimes:          2,
		},
		RAG: RAGConfig{
			DefaultProfile:         "realtime",
			DenseTopK:              50,
			SparseTopK:             50,
			RRFK:                   60,
			RerankTopN:             8,
			ContextTopN:            8,
			MaxContextTokens:       6000,
			SemanticCacheThreshold: 0.92,
			SemanticCacheMaxItems:  10000,
			NoContextAnswer:        "No sufficient evidence was found in the knowledge base.",
		},
		Ingestion: IngestionConfig{
			ChunkSizeTokens:   800,
			ChunkOverlap:      120,
			MaxDocumentBytes:  25 * 1024 * 1024,
			ParserMethod:      "basic",
			DoclingTimeout:    10 * time.Minute,
			MinerUBackend:     "pipeline",
			MinerUParseMethod: "auto",
		},
	}
}

// MockConfig returns explicit dependency-free configuration for examples and
// tests. The application reports model_provider=mock in readiness.
func MockConfig() Config {
	cfg := DefaultConfig()
	cfg.Storage.Backend = StorageMemory
	cfg.Models.ChatProvider = "mock"
	cfg.Models.EmbeddingProvider = "mock"
	cfg.Models.RerankProvider = "mock"
	cfg.Models.MultimodalProvider = "mock"
	cfg.Models.ChatModel = "orag-deterministic-mock"
	cfg.Models.EmbeddingModel = "orag-deterministic-embedding"
	cfg.Models.EmbeddingDimensions = 4
	cfg.Models.RerankModel = "orag-deterministic-rerank"
	cfg.Models.MultimodalModel = "orag-deterministic-multimodal"
	cfg.Models.AllowDeterministicMock = true
	cfg.RAG.DenseTopK = 8
	cfg.RAG.SparseTopK = 8
	cfg.RAG.RerankTopN = 4
	cfg.RAG.ContextTopN = 4
	return cfg
}

func (c Config) internal() (internalconfig.Config, error) {
	providerKeys := cloneStrings(c.Models.APIKeys)
	providerURLs := cloneStrings(c.Models.BaseURLs)
	cfg := internalconfig.Config{
		Server:  internalconfig.ServerConfig{Host: "127.0.0.1", Port: 8080, PublicBaseURL: "http://localhost:8080"},
		Storage: internalconfig.StorageConfig{Backend: c.Storage.Backend},
		Auth: internalconfig.AuthConfig{
			JWTSecret:            "orag-embedded-sdk",
			TokenTTL:             24 * time.Hour,
			AdminDefaultUsername: "admin",
			AdminDefaultPassword: "admin",
		},
		Database: internalconfig.DatabaseConfig{URL: c.Storage.DatabaseURL},
		Qdrant: internalconfig.QdrantConfig{
			Host:                    c.Storage.QdrantHost,
			GRPCPort:                c.Storage.QdrantGRPCPort,
			APIKey:                  c.Storage.QdrantAPIKey,
			UseTLS:                  c.Storage.QdrantUseTLS,
			Collection:              c.Storage.QdrantCollection,
			SemanticCacheCollection: c.Storage.QdrantSemanticCache,
			AutoCreateCollections:   c.Storage.QdrantAutoCreateCollections,
		},
		Ark: internalconfig.ArkConfig{
			APIKey:              providerKeys[c.Models.ChatProvider],
			BaseURL:             providerURLs[c.Models.ChatProvider],
			ChatModel:           c.Models.ChatModel,
			EmbeddingModel:      c.Models.EmbeddingModel,
			EmbeddingDimensions: c.Models.EmbeddingDimensions,
			RerankProvider:      c.Models.RerankProvider,
			RerankBaseURL:       providerURLs[c.Models.RerankProvider],
			RerankModel:         c.Models.RerankModel,
			RerankAPIKey:        providerKeys[c.Models.RerankProvider],
			RerankInstruct:      c.Models.RerankInstruction,
			MultimodalModel:     c.Models.MultimodalModel,
			Timeout:             c.Models.Timeout,
			RetryTimes:          c.Models.RetryTimes,
		},
		Models: internalconfig.ModelProviderConfig{
			ChatProvider:           c.Models.ChatProvider,
			EmbeddingProvider:      c.Models.EmbeddingProvider,
			RerankProvider:         c.Models.RerankProvider,
			MultimodalProvider:     c.Models.MultimodalProvider,
			AllowDeterministicMock: c.Models.AllowDeterministicMock,
			ProviderAPIKeys:        providerKeys,
			ProviderBaseURLs:       providerURLs,
		},
		RAG: internalconfig.RAGConfig{
			DefaultProfile:          c.RAG.DefaultProfile,
			DenseTopK:               c.RAG.DenseTopK,
			SparseTopK:              c.RAG.SparseTopK,
			RRFK:                    c.RAG.RRFK,
			RerankTopN:              c.RAG.RerankTopN,
			ContextTopN:             c.RAG.ContextTopN,
			MaxContextTokens:        c.RAG.MaxContextTokens,
			SemanticCacheThreshold:  c.RAG.SemanticCacheThreshold,
			SemanticCacheMaxEntries: c.RAG.SemanticCacheMaxItems,
			NoContextAnswer:         c.RAG.NoContextAnswer,
			PromptCacheMode:         "auto",
			QueryRewriteEnabled:     false,
			MultiQueryCount:         3,
			HyDEEnabled:             false,
			QueryRouter: internalconfig.QueryRouterConfig{
				Strategy:          "heuristic",
				DirectMaxRunes:    16,
				ComplexMinSignals: 2,
			},
			GraphRetrieval: internalconfig.GraphRetrievalConfig{TopK: 8, MaxEntitiesPerChunk: 6},
		},
		Ingestion: internalconfig.IngestionConfig{
			ChunkSizeTokens:    c.Ingestion.ChunkSizeTokens,
			ChunkOverlapTokens: c.Ingestion.ChunkOverlap,
			MaxDocumentBytes:   c.Ingestion.MaxDocumentBytes,
			ParserMethod:       c.Ingestion.ParserMethod,
			ContextualRetrieval: internalconfig.ContextualRetrievalConfig{
				MaxDocumentChars: 12000,
				MaxChunkChars:    2000,
				MaxContextChars:  500,
				FailureMode:      "fallback",
			},
			RAPTOR: internalconfig.RAPTORConfig{BranchFactor: 4, MaxLevels: 2, MaxSummaryChars: 1000},
			MinerU: internalconfig.MinerUConfig{
				APIURL:      c.Ingestion.MinerUAPIURL,
				ServerURL:   c.Ingestion.MinerUServerURL,
				Backend:     c.Ingestion.MinerUBackend,
				ParseMethod: c.Ingestion.MinerUParseMethod,
				Lang:        "English",
				Formula:     true,
				Table:       true,
			},
			Docling: internalconfig.DoclingConfig{ServerURL: c.Ingestion.DoclingServerURL, Timeout: c.Ingestion.DoclingTimeout},
		},
		ObjectStorage: internalconfig.ObjectStorageConfig{Provider: "local", MockUpload: true},
		Tutorial:      internalconfig.TutorialConfig{CatalogBaseURL: "https://orag.oss-cn-guangzhou.aliyuncs.com/tutorial-packs"},
		Observability: internalconfig.ObservabilityConfig{Trace: internalconfig.TracePrivacyConfig{StoreQuery: true, QueryMaxBytes: 2048, RetentionDays: 30}},
		Maintenance: internalconfig.MaintenanceConfig{OfflineKnowledgeOrganizer: internalconfig.OfflineKnowledgeOrganizerConfig{
			Schedule:                      "0 2 * * *",
			LookbackDays:                  7,
			MaxQuestionsPerRun:            500,
			MaxClustersPerRun:             200,
			MaxCodexConcurrency:           4,
			MaxCodexDeepSearchSteps:       12,
			MaxCodexTokensPerQuestion:     20000,
			MaxToolQPSPerTenant:           20,
			MaxToolRowsPerCall:            50,
			MaxReplayConcurrency:          8,
			MaxEvalConcurrency:            4,
			MinQuestionOccurrence:         2,
			LongTailSamplingRate:          0.05,
			ExplicitNegativeFeedbackBoost: 10,
			MinVerifyConfidence:           0.8,
			MinPublishConfidence:          0.9,
			ShadowEventTTLDays:            14,
			ShadowEventSamplingRate:       1,
			MinRecallLift:                 0.05,
			MinAnswerQualityLift:          0.03,
			MaxLatencyDeltaMS:             300,
		}},
	}
	return cfg, cfg.Validate()
}

func cloneStrings(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
