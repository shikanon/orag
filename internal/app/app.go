package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/ingest/parser"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/platform/httpclient"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
	qdrantstore "github.com/shikanon/orag/internal/storage/qdrant"
)

type App struct {
	Config   config.Config
	Logger   *slog.Logger
	Auth     *auth.Service
	KBStore  kb.KnowledgeBaseRepository
	Ingest   *ingest.Service
	RAG      *rag.Service
	Datasets *dataset.Service
	Eval     eval.Runner
	Metrics  *observability.Metrics

	Postgres *pgxpool.Pool
	Qdrant   *qdrantstore.Client
	closers  []func() error
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	_ = ctx
	model := ark.NewClient(ark.Config{
		APIKey:              cfg.Ark.APIKey,
		BaseURL:             cfg.Ark.BaseURL,
		ChatModel:           cfg.Ark.ChatModel,
		EmbeddingModel:      cfg.Ark.EmbeddingModel,
		EmbeddingDimensions: cfg.Ark.EmbeddingDimensions,
		RerankProvider:      cfg.Ark.RerankProvider,
		RerankBaseURL:       cfg.Ark.RerankBaseURL,
		RerankModel:         cfg.Ark.RerankModel,
		RerankAPIKey:        cfg.Ark.RerankAPIKey,
		RerankInstruct:      cfg.Ark.RerankInstruct,
		MultimodalModel:     cfg.Ark.MultimodalModel,
		Timeout:             cfg.Ark.Timeout,
		RetryTimes:          cfg.Ark.RetryTimes,
	}, httpclient.New(cfg.Ark.Timeout))

	defaultTenant := "tenant_default"
	authSvc := auth.NewService(cfg.Auth.JWTSecret, cfg.Auth.TokenTTL)
	backend, err := buildKnowledgeBackend(ctx, cfg, defaultTenant)
	if err != nil {
		return nil, err
	}
	hybrid := kb.HybridRetriever{
		Dense:      backend.dense,
		Sparse:     backend.sparse,
		RRFK:       cfg.RAG.RRFK,
		TopN:       cfg.RAG.RerankTopN,
		DenseTopK:  cfg.RAG.DenseTopK,
		SparseTopK: cfg.RAG.SparseTopK,
	}
	ragSvc := &rag.Service{
		Retriever:              hybrid,
		Model:                  model,
		Cache:                  backend.cache,
		Packer:                 rag.ContextPacker{MaxTokens: cfg.RAG.MaxContextTokens, TopN: cfg.RAG.ContextTopN},
		PromptStrategy:         prompt.NewStrategy(cfg.RAG.PromptCacheMode),
		DefaultProfile:         rag.Profile(cfg.RAG.DefaultProfile),
		NoContextAnswer:        cfg.RAG.NoContextAnswer,
		TopK:                   cfg.RAG.DenseTopK,
		SemanticCacheThreshold: cfg.RAG.SemanticCacheThreshold,

		QueryRewriteEnabled: cfg.RAG.QueryRewriteEnabled,
		MultiQueryCount:     cfg.RAG.MultiQueryCount,
		HyDEEnabled:         cfg.RAG.HyDEEnabled,
		Logger:              logger,
	}
	graphRunner, err := raggraph.NewRAGGraph(ctx, ragSvc)
	if err != nil {
		return nil, err
	}
	graphRunner.TraceStore = backend.traceStore
	ragSvc.Pipeline = graphRunner
	ingestSvc := &ingest.Service{
		Parser:           buildDocumentParser(cfg, model),
		Splitter:         chunker.Recursive{SizeTokens: cfg.Ingestion.ChunkSizeTokens, OverlapTokens: cfg.Ingestion.ChunkOverlapTokens},
		Embedder:         model,
		Indexer:          backend.indexer,
		Jobs:             backend.jobs,
		MaxDocumentBytes: cfg.Ingestion.MaxDocumentBytes,
	}
	datasets := dataset.NewService(backend.datasetRepo)
	return &App{
		Config:   cfg,
		Logger:   logger,
		Auth:     authSvc,
		KBStore:  backend.store,
		Ingest:   ingestSvc,
		RAG:      ragSvc,
		Datasets: datasets,
		Eval:     eval.Runner{RAG: ragSvc, Datasets: datasets, Repository: backend.evalRepo},
		Metrics:  observability.NewMetrics(),
		Postgres: backend.pool,
		Qdrant:   backend.qdrant,
		closers:  backend.closers,
	}, nil
}

func buildDocumentParser(cfg config.Config, model ark.MultimodalParser) parser.Parser {
	return parser.New(parser.Config{
		Method:     cfg.Ingestion.ParserMethod,
		Multimodal: model,
		HTTPClient: httpclient.New(cfg.Ingestion.Docling.Timeout),
		MinerU: parser.MinerUConfig{
			APIURL:        cfg.Ingestion.MinerU.APIURL,
			ServerURL:     cfg.Ingestion.MinerU.ServerURL,
			Backend:       cfg.Ingestion.MinerU.Backend,
			ParseMethod:   cfg.Ingestion.MinerU.ParseMethod,
			Lang:          cfg.Ingestion.MinerU.Lang,
			Formula:       cfg.Ingestion.MinerU.Formula,
			Table:         cfg.Ingestion.MinerU.Table,
			RequestZipOut: true,
		},
		Docling: parser.DoclingConfig{
			ServerURL: cfg.Ingestion.Docling.ServerURL,
			Timeout:   cfg.Ingestion.Docling.Timeout,
		},
	})
}

type knowledgeBackend struct {
	store       kb.KnowledgeBaseRepository
	indexer     kb.Indexer
	dense       kb.Retriever
	sparse      kb.Retriever
	cache       rag.SemanticCacheStore
	jobs        ingest.JobStore
	traceStore  raggraph.TraceStore
	datasetRepo dataset.Repository
	evalRepo    eval.Repository
	pool        *pgxpool.Pool
	qdrant      *qdrantstore.Client
	closers     []func() error
}

func buildKnowledgeBackend(ctx context.Context, cfg config.Config, defaultTenant string) (knowledgeBackend, error) {
	if cfg.Storage.Backend == "memory" {
		store := kb.NewMemoryStore()
		bootstrapMemory(store, defaultTenant)
		return knowledgeBackend{
			store:       store,
			indexer:     store,
			dense:       kb.DenseRetriever{Store: store},
			sparse:      kb.SparseRetriever{Store: store},
			cache:       rag.NewSemanticCache(cfg.RAG.SemanticCacheMaxEntries),
			jobs:        ingest.NewMemoryJobStore(),
			datasetRepo: dataset.NewMemoryRepository(),
			evalRepo:    eval.NewMemoryRepository(),
		}, nil
	}

	pool, err := postgres.Open(ctx, cfg.Database.URL)
	if err != nil {
		return knowledgeBackend{}, err
	}
	repo := postgres.NewRepository(pool)
	if err := repo.BootstrapDefaults(ctx, defaultTenant, "kb_default"); err != nil {
		pool.Close()
		return knowledgeBackend{}, err
	}

	qdrantCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	qdrantClient, err := qdrantstore.Open(qdrantCtx, qdrantstore.Config{
		Host:   cfg.Qdrant.Host,
		Port:   cfg.Qdrant.GRPCPort,
		APIKey: cfg.Qdrant.APIKey,
		UseTLS: cfg.Qdrant.UseTLS,
	})
	if err != nil {
		pool.Close()
		return knowledgeBackend{}, err
	}
	if cfg.Qdrant.AutoCreateCollections {
		if err := qdrantClient.EnsureCollection(ctx, cfg.Qdrant.Collection, cfg.Ark.EmbeddingDimensions); err != nil {
			_ = qdrantClient.Conn.Close()
			pool.Close()
			return knowledgeBackend{}, err
		}
		if err := qdrantClient.EnsureCollection(ctx, cfg.Qdrant.SemanticCacheCollection, cfg.Ark.EmbeddingDimensions); err != nil {
			_ = qdrantClient.Conn.Close()
			pool.Close()
			return knowledgeBackend{}, err
		}
	}
	vectors := qdrantstore.VectorStore{Client: qdrantClient, Collection: cfg.Qdrant.Collection}
	indexer := kb.CompositeIndexer{Indexers: []kb.Indexer{repo, vectors}}
	cache := qdrantstore.SemanticCache{Client: qdrantClient, Collection: cfg.Qdrant.SemanticCacheCollection, Threshold: cfg.RAG.SemanticCacheThreshold}
	return knowledgeBackend{
		store:       repo,
		indexer:     indexer,
		dense:       vectors,
		sparse:      postgres.NewFTSRetriever(repo),
		cache:       cache,
		jobs:        repo,
		traceStore:  repo,
		datasetRepo: repo,
		evalRepo:    repo,
		pool:        pool,
		qdrant:      qdrantClient,
		closers: []func() error{
			qdrantClient.Conn.Close,
			func() error {
				pool.Close()
				return nil
			},
		},
	}, nil
}

func bootstrapMemory(store *kb.MemoryStore, tenantID string) {
	now := time.Now().UTC()
	_ = store.PutKnowledgeBase(kb.KnowledgeBase{
		ID:          "kb_default",
		TenantID:    tenantID,
		Name:        "Default Knowledge Base",
		Description: "默认知识库",
		Metadata:    map[string]string{"created_by": "bootstrap"},
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

func (a *App) BootstrapToken() string {
	token, _ := a.Auth.IssueToken("tenant_default", id.New("user"))
	return token
}

type ReadinessCheck struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (a *App) Readiness(ctx context.Context) (map[string]ReadinessCheck, bool) {
	checks := map[string]ReadinessCheck{}
	ready := true
	if a.Config.Storage.Backend == "memory" {
		checks["storage"] = ReadinessCheck{Status: "ready"}
	} else {
		if a.Postgres == nil {
			checks["postgres"] = ReadinessCheck{Status: "error", Error: "not configured"}
			ready = false
		} else if err := a.Postgres.Ping(ctx); err != nil {
			checks["postgres"] = ReadinessCheck{Status: "error", Error: err.Error()}
			ready = false
		} else {
			checks["postgres"] = ReadinessCheck{Status: "ready"}
		}
		if a.Qdrant == nil {
			checks["qdrant"] = ReadinessCheck{Status: "error", Error: "not configured"}
			ready = false
		} else {
			mainOK, mainErr := a.Qdrant.CollectionExists(ctx, a.Config.Qdrant.Collection)
			cacheOK, cacheErr := a.Qdrant.CollectionExists(ctx, a.Config.Qdrant.SemanticCacheCollection)
			switch {
			case mainErr != nil:
				checks["qdrant"] = ReadinessCheck{Status: "error", Error: mainErr.Error()}
				ready = false
			case cacheErr != nil:
				checks["qdrant"] = ReadinessCheck{Status: "error", Error: cacheErr.Error()}
				ready = false
			case !mainOK || !cacheOK:
				checks["qdrant"] = ReadinessCheck{Status: "error", Error: "required collection is missing"}
				ready = false
			default:
				checks["qdrant"] = ReadinessCheck{Status: "ready"}
			}
		}
	}
	if a.Config.Ark.APIKey == "" {
		checks["ark"] = ReadinessCheck{Status: "mock"}
	} else {
		checks["ark"] = ReadinessCheck{Status: "configured"}
	}
	return checks, ready
}

func (a *App) Close() error {
	var errs []error
	for i := len(a.closers) - 1; i >= 0; i-- {
		if a.closers[i] == nil {
			continue
		}
		if err := a.closers[i](); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
