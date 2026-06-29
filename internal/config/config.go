package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server        ServerConfig
	Storage       StorageConfig
	Auth          AuthConfig
	Database      DatabaseConfig
	Qdrant        QdrantConfig
	Ark           ArkConfig
	RAG           RAGConfig
	Ingestion     IngestionConfig
	ObjectStorage ObjectStorageConfig
	Observability ObservabilityConfig
}

type StorageConfig struct {
	Backend string
}

type ServerConfig struct {
	Host          string
	Port          int
	PublicBaseURL string
	Debug         bool
}

func (c ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type AuthConfig struct {
	JWTSecret            string
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
}

type IngestionConfig struct {
	ChunkSizeTokens    int
	ChunkOverlapTokens int
	MaxDocumentBytes   int64
	ParserMethod       string
	MinerU             MinerUConfig
	Docling            DoclingConfig
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

type ObservabilityConfig struct {
	OTLPEndpoint      string
	LangFuseHost      string
	LangFusePublicKey string
	LangFuseSecretKey string
	RecordPrompts     bool
}

func Load() (Config, error) {
	cfg := Config{
		Server: ServerConfig{
			Host:          getenv("HOST", "0.0.0.0"),
			Port:          getenvInt("PORT", 8080),
			PublicBaseURL: getenv("PUBLIC_BASE_URL", "http://localhost:8080"),
			Debug:         getenvBool("DEBUG", false),
		},
		Storage: StorageConfig{
			Backend: getenv("STORAGE_BACKEND", "qdrant_postgres"),
		},
		Auth: AuthConfig{
			JWTSecret:            getenv("JWT_SECRET", getenv("SECRET_KEY", "orag-dev-secret-change-me")),
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
			RequireExternalProviders: getenvBool("REQUIRE_EXTERNAL_PROVIDERS", false),
		},
		Ingestion: IngestionConfig{
			ChunkSizeTokens:    getenvInt("INGEST_CHUNK_SIZE_TOKENS", 800),
			ChunkOverlapTokens: getenvInt("INGEST_CHUNK_OVERLAP_TOKENS", 120),
			MaxDocumentBytes:   int64(getenvInt("INGEST_MAX_DOCUMENT_BYTES", 25*1024*1024)),
			ParserMethod:       strings.ToLower(strings.TrimSpace(getenv("INGEST_PARSER_METHOD", "basic"))),
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
		Observability: ObservabilityConfig{
			OTLPEndpoint:      getenv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			LangFuseHost:      getenv("LANGFUSE_HOST", ""),
			LangFusePublicKey: getenv("LANGFUSE_PUBLIC_KEY", ""),
			LangFuseSecretKey: getenv("LANGFUSE_SECRET_KEY", ""),
			RecordPrompts:     getenvBool("OBSERVABILITY_RECORD_PROMPTS", false),
		},
	}

	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	var missing []string
	if c.Auth.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if c.RAG.RequireExternalProviders && c.Ark.APIKey == "" {
		missing = append(missing, "ARK_API_KEY")
	}
	if c.Qdrant.Host == "" {
		missing = append(missing, "QDRANT_HOST")
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
	if c.Ingestion.ParserMethod == "mineru" && strings.TrimSpace(c.Ingestion.MinerU.APIURL) == "" {
		return errors.New("MINERU_APISERVER or MINERU_API_URL is required when INGEST_PARSER_METHOD=mineru")
	}
	if c.Ingestion.ParserMethod == "docling" && strings.TrimSpace(c.Ingestion.Docling.ServerURL) == "" {
		return errors.New("DOCLING_SERVER_URL is required when INGEST_PARSER_METHOD=docling")
	}
	if c.Ark.RerankProvider != "volcengine" && c.Ark.RerankProvider != "aliyun" {
		return errors.New("RERANK_PROVIDER must be volcengine or aliyun")
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
	if len(missing) > 0 {
		return fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c Config) RedactedEnv() map[string]string {
	return map[string]string{
		"HOST":                             c.Server.Host,
		"PORT":                             strconv.Itoa(c.Server.Port),
		"DATABASE_URL":                     redact(c.Database.URL),
		"STORAGE_BACKEND":                  c.Storage.Backend,
		"QDRANT_HOST":                      c.Qdrant.Host,
		"QDRANT_GRPC_PORT":                 strconv.Itoa(c.Qdrant.GRPCPort),
		"QDRANT_COLLECTION":                c.Qdrant.Collection,
		"QDRANT_SEMANTIC_CACHE_COLLECTION": c.Qdrant.SemanticCacheCollection,
		"ARK_BASE_URL":                     c.Ark.BaseURL,
		"ARK_API_KEY":                      redact(c.Ark.APIKey),
		"ARK_CHAT_MODEL":                   c.Ark.ChatModel,
		"ARK_EMBEDDING_MODEL":              c.Ark.EmbeddingModel,
		"INGEST_PARSER_METHOD":             c.Ingestion.ParserMethod,
		"MINERU_APISERVER":                 c.Ingestion.MinerU.APIURL,
		"MINERU_SERVER_URL":                c.Ingestion.MinerU.ServerURL,
		"MINERU_BACKEND":                   c.Ingestion.MinerU.Backend,
		"MINERU_PARSE_METHOD":              c.Ingestion.MinerU.ParseMethod,
		"DOCLING_SERVER_URL":               c.Ingestion.Docling.ServerURL,
		"RERANK_PROVIDER":                  c.Ark.RerankProvider,
		"ARK_RERANK_MODEL":                 c.Ark.RerankModel,
		"ALIYUN_RERANK_API_KEY":            redact(c.Ark.RerankAPIKey),
		"JWT_SECRET":                       redact(c.Auth.JWTSecret),
		"RAG_QUERY_REWRITE_ENABLED":        strconv.FormatBool(c.RAG.QueryRewriteEnabled),
		"RAG_MULTI_QUERY_COUNT":            strconv.Itoa(c.RAG.MultiQueryCount),
		"RAG_HYDE_ENABLED":                 strconv.FormatBool(c.RAG.HyDEEnabled),
		"PROMPT_CACHE_MODE":                c.RAG.PromptCacheMode,
	}
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
