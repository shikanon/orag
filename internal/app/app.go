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
	modelprovider "github.com/shikanon/orag/internal/llm/provider"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/optimizer"
	"github.com/shikanon/orag/internal/platform/httpclient"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
	qdrantstore "github.com/shikanon/orag/internal/storage/qdrant"
)

type App struct {
	Config    config.Config
	Logger    *slog.Logger
	Auth      *auth.Service
	KBStore   kb.KnowledgeBaseRepository
	Ingest    *ingest.Service
	RAG       *rag.Service
	Datasets  *dataset.Service
	Eval      eval.Runner
	Optimizer *optimizer.Service
	Metrics   *observability.Metrics
	Traces    TraceRepository

	Postgres *pgxpool.Pool
	Qdrant   *qdrantstore.Client
	closers  []func() error
}

type TraceRepository interface {
	GetTrace(ctx context.Context, traceID string) (postgres.TraceRecord, bool, error)
	GetTraceForTenant(ctx context.Context, tenantID, traceID string) (postgres.TraceRecord, bool, error)
	ListTraces(ctx context.Context, filter postgres.TraceListFilter) ([]postgres.TraceRecord, error)
	TraceNodeStats(ctx context.Context, filter postgres.TraceListFilter) ([]postgres.TraceNodeStat, error)
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*App, error) {
	_ = ctx
	model, err := buildModelClient(cfg)
	if err != nil {
		return nil, err
	}

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
	retriever := buildRAGRetriever(cfg, hybrid, backend.store)
	ragSvc := &rag.Service{
		Retriever:              retriever,
		Model:                  model,
		Cache:                  backend.cache,
		Packer:                 rag.ContextPacker{MaxTokens: cfg.RAG.MaxContextTokens, TopN: cfg.RAG.ContextTopN},
		PromptStrategy:         prompt.NewStrategy(cfg.RAG.PromptCacheMode),
		DefaultProfile:         rag.Profile(cfg.RAG.DefaultProfile),
		NoContextAnswer:        cfg.RAG.NoContextAnswer,
		TopK:                   cfg.RAG.DenseTopK,
		SemanticCacheThreshold: cfg.RAG.SemanticCacheThreshold,
		RRFK:                   cfg.RAG.RRFK,

		QueryRewriteEnabled: cfg.RAG.QueryRewriteEnabled,
		MultiQueryCount:     cfg.RAG.MultiQueryCount,
		HyDEEnabled:         cfg.RAG.HyDEEnabled,
		QueryRouter:         buildQueryRouter(cfg),
		Logger:              logger,
	}
	metrics := observability.NewMetrics()
	graphRunner, err := raggraph.NewRAGGraph(ctx, ragSvc)
	if err != nil {
		return nil, err
	}
	graphRunner.TraceStore = backend.traceStore
	graphRunner.Metrics = metrics
	ragSvc.Pipeline = graphRunner
	ingestSvc := &ingest.Service{
		Parser:           buildDocumentParser(cfg, model),
		Splitter:         chunker.Recursive{SizeTokens: cfg.Ingestion.ChunkSizeTokens, OverlapTokens: cfg.Ingestion.ChunkOverlapTokens},
		Embedder:         model,
		Contextualizer:   buildContextualizer(cfg, model),
		RAPTORBuilder:    buildRAPTORBuilder(cfg, model),
		GraphBuilder:     buildGraphBuilder(cfg),
		KnowledgeBases:   backend.store,
		Indexer:          backend.indexer,
		Jobs:             backend.jobs,
		Uploads:          ingest.NewMemoryUploadStore(),
		MaxDocumentBytes: cfg.Ingestion.MaxDocumentBytes,
	}
	datasets := dataset.NewService(backend.datasetRepo)
	evalRunner := eval.Runner{RAG: ragSvc, Datasets: datasets, Repository: backend.evalRepo}
	optimizerRunner := optimizer.InternalRAGRunner{
		BaseRAG:    ragSvc,
		Datasets:   datasets,
		Repository: backend.evalRepo,
		Namespaces: optimizer.NewTempNamespaceManager(nil),
	}
	return &App{
		Config:   cfg,
		Logger:   logger,
		Auth:     authSvc,
		KBStore:  backend.store,
		Ingest:   ingestSvc,
		RAG:      ragSvc,
		Datasets: datasets,
		Eval:     evalRunner,
		Optimizer: &optimizer.Service{
			Repository: backend.optimizerRepo,
			Runner:     optimizerRunner,
		},
		Metrics:  metrics,
		Traces:   backend.traceRepo,
		Postgres: backend.pool,
		Qdrant:   backend.qdrant,
		closers:  backend.closers,
	}, nil
}

func buildModelClient(cfg config.Config) (*modelprovider.Client, error) {
	apiKeys := make(map[modelprovider.Name]string, len(cfg.Models.ProviderAPIKeys))
	for name, value := range cfg.Models.ProviderAPIKeys {
		apiKeys[modelprovider.NormalizeName(name)] = value
	}
	baseURLs := modelProviderBaseURLs(cfg)
	return modelprovider.NewClient(modelprovider.Config{
		ChatProvider:           modelprovider.NormalizeName(cfg.Models.ChatProvider),
		EmbeddingProvider:      modelprovider.NormalizeName(cfg.Models.EmbeddingProvider),
		RerankProvider:         modelprovider.NormalizeName(cfg.Models.RerankProvider),
		MultimodalProvider:     modelprovider.NormalizeName(cfg.Models.MultimodalProvider),
		APIKeys:                apiKeys,
		BaseURLs:               baseURLs,
		ChatModel:              cfg.Ark.ChatModel,
		EmbeddingModel:         cfg.Ark.EmbeddingModel,
		EmbeddingDimensions:    cfg.Ark.EmbeddingDimensions,
		RerankModel:            cfg.Ark.RerankModel,
		RerankInstruct:         cfg.Ark.RerankInstruct,
		MultimodalModel:        cfg.Ark.MultimodalModel,
		AllowDeterministicMock: cfg.Models.AllowDeterministicMock,
		Timeout:                cfg.Ark.Timeout,
		RetryTimes:             cfg.Ark.RetryTimes,
	}, httpclient.New(cfg.Ark.Timeout))
}

func modelProviderBaseURLs(cfg config.Config) map[modelprovider.Name]string {
	urls := map[modelprovider.Name]string{}
	for name, value := range cfg.Models.ProviderBaseURLs {
		urls[modelprovider.NormalizeName(name)] = value
	}
	if urls[modelprovider.VolcEngine] == "" {
		urls[modelprovider.VolcEngine] = cfg.Ark.BaseURL
	}
	registry := modelprovider.BuiltinRegistry()
	for _, name := range registry.Names() {
		if urls[name] == "" {
			urls[name] = modelprovider.DefaultBaseURL(name)
		}
	}
	return urls
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

func buildContextualizer(cfg config.Config, model ingest.ChatModel) ingest.Contextualizer {
	if !cfg.Ingestion.ContextualRetrieval.Enabled {
		return nil
	}
	return ingest.LLMContextualizer{
		Model:            model,
		MaxDocumentChars: cfg.Ingestion.ContextualRetrieval.MaxDocumentChars,
		MaxChunkChars:    cfg.Ingestion.ContextualRetrieval.MaxChunkChars,
		MaxContextChars:  cfg.Ingestion.ContextualRetrieval.MaxContextChars,
		FailureMode:      ingest.ContextualFailureMode(cfg.Ingestion.ContextualRetrieval.FailureMode),
	}
}

func buildRAPTORBuilder(cfg config.Config, model ingest.ChatModel) ingest.RAPTORBuilder {
	if !cfg.Ingestion.RAPTOR.Enabled {
		return nil
	}
	return ingest.LLMRAPTORBuilder{
		Model:           model,
		BranchFactor:    cfg.Ingestion.RAPTOR.BranchFactor,
		MaxLevels:       cfg.Ingestion.RAPTOR.MaxLevels,
		MaxSummaryChars: cfg.Ingestion.RAPTOR.MaxSummaryChars,
	}
}

func buildGraphBuilder(cfg config.Config) ingest.GraphBuilder {
	if !cfg.RAG.GraphRetrieval.Enabled {
		return nil
	}
	return ingest.LightweightGraphBuilder{MaxEntitiesPerChunk: cfg.RAG.GraphRetrieval.MaxEntitiesPerChunk}
}

func buildRAGRetriever(cfg config.Config, base kb.Retriever, store kb.KnowledgeBaseRepository) kb.Retriever {
	if !cfg.RAG.GraphRetrieval.Enabled {
		return base
	}
	graphStore, ok := store.(kb.GraphStore)
	if !ok {
		return base
	}
	return kb.GraphRetriever{Base: base, Store: graphStore, TopK: cfg.RAG.GraphRetrieval.TopK}
}

func buildQueryRouter(cfg config.Config) rag.QueryRouter {
	if !cfg.RAG.QueryRouter.Enabled {
		return nil
	}
	return rag.HeuristicQueryRouter{
		DirectMaxRunes:    cfg.RAG.QueryRouter.DirectMaxRunes,
		ComplexMinSignals: cfg.RAG.QueryRouter.ComplexMinSignals,
	}
}

type knowledgeBackend struct {
	store         kb.KnowledgeBaseRepository
	indexer       kb.Indexer
	dense         kb.Retriever
	sparse        kb.Retriever
	cache         rag.SemanticCacheStore
	jobs          ingest.JobStore
	traceStore    raggraph.TraceStore
	traceRepo     TraceRepository
	datasetRepo   dataset.Repository
	evalRepo      eval.Repository
	optimizerRepo optimizer.Repository
	pool          *pgxpool.Pool
	qdrant        *qdrantstore.Client
	closers       []func() error
}

type knowledgeBaseVectorDeleter interface {
	DeleteKnowledgeBaseVectors(ctx context.Context, tenantID, kbID string) error
}

type knowledgeBaseSemanticCacheDeleter interface {
	DeleteKnowledgeBaseSemanticCache(ctx context.Context, tenantID, kbID string) error
}

type knowledgeBaseStore struct {
	primary              kb.KnowledgeBaseRepository
	vectorDeleter        knowledgeBaseVectorDeleter
	semanticCacheDeleter knowledgeBaseSemanticCacheDeleter
}

func (s knowledgeBaseStore) PutKnowledgeBase(ctx context.Context, item kb.KnowledgeBase) error {
	return s.primary.PutKnowledgeBase(ctx, item)
}

func (s knowledgeBaseStore) ListKnowledgeBases(ctx context.Context, tenantID string) ([]kb.KnowledgeBase, error) {
	return s.primary.ListKnowledgeBases(ctx, tenantID)
}

func (s knowledgeBaseStore) GetKnowledgeBase(ctx context.Context, tenantID, id string) (kb.KnowledgeBase, bool, error) {
	return s.primary.GetKnowledgeBase(ctx, tenantID, id)
}

func (s knowledgeBaseStore) DeleteKnowledgeBase(ctx context.Context, tenantID, id string) (bool, error) {
	if _, ok, err := s.primary.GetKnowledgeBase(ctx, tenantID, id); err != nil {
		return false, err
	} else if !ok {
		return false, nil
	}
	if s.vectorDeleter != nil {
		if err := s.vectorDeleter.DeleteKnowledgeBaseVectors(ctx, tenantID, id); err != nil {
			return false, err
		}
	}
	if s.semanticCacheDeleter != nil {
		if err := s.semanticCacheDeleter.DeleteKnowledgeBaseSemanticCache(ctx, tenantID, id); err != nil {
			return false, err
		}
	}
	return s.primary.DeleteKnowledgeBase(ctx, tenantID, id)
}

func (s knowledgeBaseStore) StoreGraphRelations(ctx context.Context, relations []kb.GraphRelation) error {
	store, ok := s.primary.(kb.GraphStore)
	if !ok {
		return nil
	}
	return store.StoreGraphRelations(ctx, relations)
}

func (s knowledgeBaseStore) ExpandGraph(ctx context.Context, req kb.GraphExpansionRequest) ([]kb.SearchResult, error) {
	store, ok := s.primary.(kb.GraphStore)
	if !ok {
		return nil, nil
	}
	return store.ExpandGraph(ctx, req)
}

func buildKnowledgeBackend(ctx context.Context, cfg config.Config, defaultTenant string) (knowledgeBackend, error) {
	if cfg.Storage.Backend == "memory" {
		store := kb.NewMemoryStore()
		traceRepo := newMemoryTraceRepository()
		if err := bootstrapMemory(ctx, store, defaultTenant); err != nil {
			return knowledgeBackend{}, err
		}
		return knowledgeBackend{
			store:         store,
			indexer:       store,
			dense:         kb.DenseRetriever{Store: store},
			sparse:        kb.SparseRetriever{Store: store},
			cache:         rag.NewSemanticCache(cfg.RAG.SemanticCacheMaxEntries),
			jobs:          ingest.NewMemoryJobStore(),
			traceStore:    traceRepo,
			traceRepo:     traceRepo,
			datasetRepo:   dataset.NewMemoryRepository(),
			evalRepo:      eval.NewMemoryRepository(),
			optimizerRepo: optimizer.NewMemoryRepository(),
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
	repo.StageChunks = true

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
		store:         knowledgeBaseStore{primary: repo, vectorDeleter: vectors, semanticCacheDeleter: cache},
		indexer:       indexer,
		dense:         vectors,
		sparse:        postgres.NewFTSRetriever(repo),
		cache:         cache,
		jobs:          repo,
		traceStore:    repo,
		traceRepo:     repo,
		datasetRepo:   repo,
		evalRepo:      repo,
		optimizerRepo: repo,
		pool:          pool,
		qdrant:        qdrantClient,
		closers: []func() error{
			qdrantClient.Conn.Close,
			func() error {
				pool.Close()
				return nil
			},
		},
	}, nil
}

func bootstrapMemory(ctx context.Context, store *kb.MemoryStore, tenantID string) error {
	now := time.Now().UTC()
	return store.PutKnowledgeBase(ctx, kb.KnowledgeBase{
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
		a.observeDependencyCheck("postgres", "ready", 0)
		a.observeDependencyCheck("qdrant", "ready", 0)
	} else {
		if a.Postgres == nil {
			checks["postgres"] = ReadinessCheck{Status: "error", Error: "not configured"}
			a.observeDependencyCheck("postgres", "error", 0)
			ready = false
		} else {
			start := time.Now()
			checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := a.Postgres.Ping(checkCtx)
			cancel()
			status := "ready"
			if err != nil {
				status = "error"
				if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
					status = "timeout"
				}
				checks["postgres"] = ReadinessCheck{Status: "error", Error: err.Error()}
				ready = false
			} else {
				checks["postgres"] = ReadinessCheck{Status: "ready"}
			}
			a.observeDependencyCheck("postgres", status, time.Since(start).Milliseconds())
		}
		if a.Qdrant == nil {
			checks["qdrant"] = ReadinessCheck{Status: "error", Error: "not configured"}
			a.observeDependencyCheck("qdrant", "error", 0)
			ready = false
		} else {
			qdrantReady := true
			start := time.Now()
			checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := a.Qdrant.ValidateCollection(checkCtx, a.Config.Qdrant.Collection, a.Config.Ark.EmbeddingDimensions)
			cancel()
			status := "ready"
			if err != nil {
				status = "error"
				if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
					status = "timeout"
				}
				checks["qdrant.main_collection"] = ReadinessCheck{Status: "error", Error: err.Error()}
				checks["qdrant"] = ReadinessCheck{Status: "error", Error: err.Error()}
				ready = false
				qdrantReady = false
			} else {
				checks["qdrant.main_collection"] = ReadinessCheck{Status: "ready"}
			}
			a.observeDependencyCheck("qdrant", status, time.Since(start).Milliseconds())

			start = time.Now()
			checkCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			err = a.Qdrant.ValidateCollection(checkCtx, a.Config.Qdrant.SemanticCacheCollection, a.Config.Ark.EmbeddingDimensions)
			cancel()
			status = "ready"
			if err != nil {
				status = "error"
				if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
					status = "timeout"
				}
				checks["qdrant.semantic_cache_collection"] = ReadinessCheck{Status: "error", Error: err.Error()}
				checks["qdrant"] = ReadinessCheck{Status: "error", Error: err.Error()}
				ready = false
				qdrantReady = false
			} else {
				checks["qdrant.semantic_cache_collection"] = ReadinessCheck{Status: "ready"}
			}
			a.observeDependencyCheck("qdrant", status, time.Since(start).Milliseconds())
			if qdrantReady {
				checks["qdrant"] = ReadinessCheck{Status: "ready"}
			}
		}
	}
	checks["model_provider"] = ReadinessCheck{Status: a.modelProviderStatus()}
	a.observeDependencyCheck("model_provider", checks["model_provider"].Status, 0)
	return checks, ready
}

func (a *App) observeDependencyCheck(dependency, status string, latencyMS int64) {
	if a == nil || a.Metrics == nil {
		return
	}
	a.Metrics.ObserveDependencyCheck(dependency, status, latencyMS)
}

func (a *App) modelProviderStatus() string {
	if a.Config.Models.AllowDeterministicMock {
		for _, provider := range []string{
			a.Config.Models.ChatProvider,
			a.Config.Models.EmbeddingProvider,
			a.Config.Models.RerankProvider,
			a.Config.Models.MultimodalProvider,
		} {
			if modelprovider.NormalizeName(provider) == modelprovider.Mock {
				return "mock"
			}
		}
	}
	return "configured"
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
