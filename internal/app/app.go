package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shikanon/orag/internal/auth"
	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/evaluationpolicy"
	raggraph "github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/ingest/parser"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	modelprovider "github.com/shikanon/orag/internal/llm/provider"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/offlineknowledge"
	"github.com/shikanon/orag/internal/optimizer"
	"github.com/shikanon/orag/internal/pipeline"
	"github.com/shikanon/orag/internal/platform/httpclient"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/project"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/release"
	"github.com/shikanon/orag/internal/storage/postgres"
	qdrantstore "github.com/shikanon/orag/internal/storage/qdrant"
	"github.com/shikanon/orag/internal/tutorial"
)

type App struct {
	Config              config.Config
	Logger              *slog.Logger
	Auth                *auth.Service
	APIKeys             *auth.APIKeyService
	KBStore             kb.KnowledgeBaseRepository
	Ingest              *ingest.Service
	RAG                 *rag.Service
	Datasets            *dataset.Service
	Projects            *project.Service
	Tutorials           *tutorial.Catalog
	TutorialClones      *tutorial.CloneService
	TutorialCloneRunner *tutorial.CloneRunner
	TutorialRuns        *tutorial.LiveRunService
	TutorialRunRunner   *tutorial.ExperimentRunRunner
	Eval                eval.Runner
	EvaluationPolicy    *evaluationpolicy.Service
	Optimizer           *optimizer.Service
	OfflineKnowledge    *offlineknowledge.Service
	OfflineScheduler    *offlineknowledge.Scheduler
	Release             *release.Service
	Pipeline            *pipeline.Service
	PipelineCompiler    *pipeline.Compiler
	PipelineDebug       *pipeline.DebugRunner
	ProductionQuery     rag.QueryRunner
	Metrics             *observability.Metrics
	Traces              TraceRepository

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
	tutorials, err := tutorial.NewCatalog()
	if err != nil {
		return nil, err
	}
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
	projects := project.NewService(backend.projectRepo, func() time.Time { return time.Now().UTC() })
	tutorialClones := tutorial.NewCloneService(tutorials, backend.tutorialCloneRepo, func() time.Time { return time.Now().UTC() })
	publicPacks, err := tutorial.NewPublicPackReader(
		cfg.Tutorial.CatalogBaseURL,
		cfg.Tutorial.MaxManifestBytes,
		cfg.Tutorial.MaxObjectBytes,
		cfg.Tutorial.HTTPTimeout,
		"",
		httpclient.New(cfg.Tutorial.HTTPTimeout),
	)
	if err != nil {
		return nil, err
	}
	privatePacks, err := tutorial.NewPrivateStore(tutorial.PrivateStoreConfig{
		Provider: cfg.ObjectStorage.Provider, Endpoint: cfg.ObjectStorage.Endpoint, Bucket: cfg.ObjectStorage.Bucket,
		AccessKeyID: cfg.ObjectStorage.AccessKeyID, AccessKeySecret: cfg.ObjectStorage.AccessKeySecret,
		LocalDirectory: cfg.Tutorial.PrivateOutputDirectory, Prefix: cfg.Tutorial.PrivateOutputPrefix,
	})
	if err != nil {
		return nil, err
	}
	tutorialClones.ConfigureInstaller(projects, publicPacks, privatePacks)
	tutorialClones.ConfigureRuntime(tutorial.ResourceInitializer{KnowledgeBases: backend.store, Datasets: datasets})
	releaseSvc := release.NewService(backend.releaseRepo)
	pipelineSvc := pipeline.NewService(backend.pipelineRepo, pipeline.BuiltinRegistry())
	pipelineCompiler := pipeline.NewCompiler(ragSvc, pipeline.BuiltinRegistry())
	pipelineDebug := &pipeline.DebugRunner{Drafts: pipelineSvc, Compiler: pipelineCompiler}
	productionQuery := &pipeline.ProductionRunner{Release: releaseSvc, Compiler: pipelineCompiler, Executor: graphRunner}
	evaluationPolicySvc := evaluationpolicy.NewService(backend.evaluationPolicyRepo, datasets, eval.DefaultMetricRegistry)
	apiKeys := auth.NewAPIKeyService(backend.apiKeyRepo, cfg.Auth.APIKeyPepper)
	evalRunner := eval.Runner{RAG: ragSvc, Datasets: datasets, Repository: backend.evalRepo}
	tutorialRunRepo, ok := backend.tutorialCloneRepo.(tutorial.ExperimentRunRepository)
	if !ok {
		return nil, errors.New("tutorial run repository is unavailable")
	}
	tutorialRuns := tutorial.NewLiveRunService(tutorialRunRepo, backend.tutorialCloneRepo, func() time.Time { return time.Now().UTC() })
	tutorialRuns.Configure(ingest.NewVariantService(ingestSvc, parser.New(parser.Config{Method: parser.MethodBasic, Multimodal: model}), chunker.Recursive{
		SizeTokens: tutorial.TutorialBaselineChunkSizeTokens, OverlapTokens: tutorial.TutorialBaselineChunkOverlapTokens,
	}), evalRunner, privatePacks)
	tutorialRuns.ConfigureCandidateIngestors(tutorial.RuntimeEnvironment{
		ChatProvider: cfg.Models.ChatProvider, ChatModel: cfg.Ark.ChatModel,
		EmbeddingProvider: cfg.Models.EmbeddingProvider, EmbeddingModel: cfg.Ark.EmbeddingModel,
		RerankProvider: cfg.Models.RerankProvider, RerankModel: cfg.Ark.RerankModel,
		MultimodalProvider: cfg.Models.MultimodalProvider, MultimodalModel: cfg.Ark.MultimodalModel,
		PromptCacheMode: cfg.RAG.PromptCacheMode, EvaluatorVersion: "standard_eval_v1",
	}, map[string]tutorial.RuntimeIngestor{
		tutorial.TutorialP1StructuredJSONCandidateID: ingest.NewVariantService(ingestSvc, parser.New(parser.Config{
			Method: parser.MethodStructuredJSON, Multimodal: model,
		}), chunker.Recursive{SizeTokens: tutorial.TutorialBaselineChunkSizeTokens, OverlapTokens: tutorial.TutorialBaselineChunkOverlapTokens}),
		tutorial.TutorialP2RecursiveChunkCandidateID: ingest.NewVariantService(ingestSvc, parser.New(parser.Config{
			Method: parser.MethodBasic, Multimodal: model,
		}), chunker.Recursive{SizeTokens: tutorial.TutorialP2ChunkSizeTokens, OverlapTokens: tutorial.TutorialP2ChunkOverlapTokens}),
	})
	optimizerRunner := optimizer.InternalRAGRunner{
		BaseRAG:    ragSvc,
		Datasets:   datasets,
		Repository: backend.evalRepo,
		Namespaces: optimizer.NewTempNamespaceManager(nil),
	}
	offlineKnowledgeOptions := buildOfflineKnowledgeOptions(cfg, backend, model, retriever, ragSvc, datasets, metrics)
	configureRAGShadow(ragSvc, cfg.Maintenance.OfflineKnowledgeOrganizer, offlineKnowledgeOptions)
	offlineKnowledgeSvc := offlineknowledge.NewService(backend.offlineKnowledgeRepo, offlineKnowledgeOptions)
	offlineScheduler := buildOfflineKnowledgeScheduler(cfg, offlineKnowledgeSvc, logger)
	closers := append([]func() error{}, backend.closers...)
	if offlineScheduler != nil && offlineScheduler.Enabled() {
		if err := offlineScheduler.Start(context.Background()); err != nil {
			for i := len(closers) - 1; i >= 0; i-- {
				if closers[i] != nil {
					_ = closers[i]()
				}
			}
			return nil, err
		}
		closers = append(closers, offlineScheduler.Stop)
	}
	tutorialCloneRunner := tutorial.NewCloneRunner(tutorialClones)
	if err := tutorialCloneRunner.Start(context.Background()); err != nil {
		for i := len(closers) - 1; i >= 0; i-- {
			if closers[i] != nil {
				_ = closers[i]()
			}
		}
		return nil, err
	}
	closers = append(closers, tutorialCloneRunner.Stop)
	tutorialRunRunner := tutorial.NewExperimentRunRunner(tutorialRuns)
	if err := tutorialRunRunner.Start(context.Background()); err != nil {
		for i := len(closers) - 1; i >= 0; i-- {
			if closers[i] != nil {
				_ = closers[i]()
			}
		}
		return nil, err
	}
	closers = append(closers, tutorialRunRunner.Stop)
	return &App{
		Config:              cfg,
		Logger:              logger,
		Auth:                authSvc,
		APIKeys:             apiKeys,
		KBStore:             backend.store,
		Ingest:              ingestSvc,
		RAG:                 ragSvc,
		Datasets:            datasets,
		Projects:            projects,
		Tutorials:           tutorials,
		TutorialClones:      tutorialClones,
		TutorialCloneRunner: tutorialCloneRunner,
		TutorialRuns:        tutorialRuns,
		TutorialRunRunner:   tutorialRunRunner,
		Eval:                evalRunner,
		EvaluationPolicy:    evaluationPolicySvc,
		Optimizer: &optimizer.Service{
			Repository: backend.optimizerRepo,
			Runner:     optimizerRunner,
		},
		OfflineKnowledge: offlineKnowledgeSvc,
		OfflineScheduler: offlineScheduler,
		Release:          releaseSvc,
		Pipeline:         pipelineSvc,
		PipelineCompiler: &pipelineCompiler,
		PipelineDebug:    pipelineDebug,
		ProductionQuery:  productionQuery,
		Metrics:          metrics,
		Traces:           backend.traceRepo,
		Postgres:         backend.pool,
		Qdrant:           backend.qdrant,
		closers:          closers,
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

func buildOfflineKnowledgeOptions(cfg config.Config, backend knowledgeBackend, model offlineKnowledgeChatModel, retriever kb.Retriever, ragSvc *rag.Service, datasets *dataset.Service, metrics *observability.Metrics) offlineknowledge.ServiceOptions {
	organizer := cfg.Maintenance.OfflineKnowledgeOrganizer
	quota := offlineknowledge.ToolQuota{
		MaxTokens:          organizer.MaxCodexTokensPerQuestion,
		MaxDeepSearchSteps: organizer.MaxCodexDeepSearchSteps,
		MaxRowsPerCall:     organizer.MaxToolRowsPerCall,
		MaxQPSPerTenant:    organizer.MaxToolQPSPerTenant,
		// Use the model timeout as the upper bound for each controlled Codex tool call.
		MaxTimeout: cfg.Ark.Timeout,
	}
	sourceReader := offlineknowledge.NewStoreSourceReader(backend.chunkSource)
	replayer := offlineknowledge.NewRetrieverRecallReplayer(retriever, offlineknowledge.NewChunkSourceMetadataReader(), cfg.RAG.ContextTopN)
	graphStore, _ := backend.store.(kb.GraphStore)
	var runtimeMetrics offlineknowledge.MetricsRecorder
	var codexToolMetrics offlineknowledge.CodexToolMetrics
	if metrics != nil {
		runtimeMetrics = metrics
		codexToolMetrics = metrics
	}
	shadowOptions := offlineknowledge.ShadowRetrieverOptions{
		Limit:                organizer.MaxClustersPerRun,
		EventSamplingRate:    organizer.ShadowEventSamplingRate,
		EventSamplingRateSet: true,
	}
	if metrics != nil {
		shadowOptions.DropMetric = metrics
		shadowOptions.HitMetric = metrics
	}
	feedbackSource := backend.offlineKnowledgeRepo
	if feedbackSource == nil {
		feedbackSource = offlineknowledge.NewMemoryRepository()
	}
	opts := offlineknowledge.ServiceOptions{
		HistorySource:     offlineknowledge.NewTraceHistoryExtractor(traceHistorySource{repo: backend.traceRepo}, feedbackSource),
		QuestionClusterer: offlineknowledge.NewDeterministicQuestionClusterer(time.Now),
		RecallReplayer:    replayer,
		SourceReader:      sourceReader,
		CodexAnalyzer:     buildOfflineKnowledgeCodexAnalyzer(cfg),
		CodexTools: offlineknowledge.NewCodexToolRegistry(offlineknowledge.CodexToolRegistryOptions{
			Retriever:   retriever,
			ChunkSource: backend.chunkSource,
			GraphStore:  graphStore,
			Repository:  backend.offlineKnowledgeRepo,
			EvalLookup:  offlineknowledge.EvalRepositoryToolLookup{Repository: backend.evalRepo},
			Replayer:    replayer,
			Quota:       quota,
			MaxSteps:    organizer.MaxCodexDeepSearchSteps,
			Audit:       backend.offlineKnowledgeRepo,
			Metrics:     codexToolMetrics,
			DefaultRows: organizer.MaxToolRowsPerCall,
		}),
		ShadowRetriever: offlineknowledge.NewShadowRetriever(backend.offlineKnowledgeRepo, shadowOptions),
		RegressionLimits: offlineknowledge.RegressionThresholds{
			MinRecallLift:        organizer.MinRecallLift,
			MinAnswerQualityLift: organizer.MinAnswerQualityLift,
			MaxLatencyDelta:      time.Duration(organizer.MaxLatencyDeltaMS) * time.Millisecond,
		},
		MaxQuestions: organizer.MaxQuestionsPerRun,
		MaxClusters:  organizer.MaxClustersPerRun,
		ToolQuota:    quota,
		Metrics:      runtimeMetrics,
	}
	opts.RegressionRunner = buildOfflineKnowledgeRegressionRunner(organizer, ragSvc, datasets, backend.evalRepo, opts)
	if !organizer.EvidenceValidationEnabled {
		opts.Validator = offlineknowledge.DisabledItemValidator{}
		return opts
	}
	var judge offlineknowledge.ConclusionJudge
	if organizer.ConclusionJudgeEnabled {
		judge = offlineknowledge.NewEvalConclusionJudge(eval.NewLLMJudge(judgeChatModelAdapter{
			Model:     model,
			ModelName: cfg.Ark.ChatModel,
		}, eval.JudgeConfig{
			Model:         cfg.Ark.ChatModel,
			Metrics:       []eval.JudgeMetric{eval.JudgeMetricGroundedness, eval.JudgeMetricCitationSupport},
			Timeout:       cfg.Ark.Timeout,
			MaxRetries:    cfg.Ark.RetryTimes,
			MaxJudgeCalls: 1,
		}), organizer.MinVerifyConfidence)
	} else {
		judge = offlineknowledge.DisabledConclusionJudge{}
	}
	opts.Validator = offlineknowledge.NewValidator(sourceReader, judge, offlineknowledge.ValidatorOptions{
		MinConfidence: organizer.MinVerifyConfidence,
	})
	return opts
}

func buildOfflineKnowledgeCodexAnalyzer(cfg config.Config) offlineknowledge.CodexAnalyzer {
	organizer := cfg.Maintenance.OfflineKnowledgeOrganizer
	command := strings.Fields(strings.TrimSpace(organizer.CodexCommand))
	return offlineknowledge.NewCodexRunnerAdapter(offlineknowledge.CodexRunnerConfig{
		Enabled:  organizer.CodexEnabled,
		Command:  command,
		Endpoint: strings.TrimSpace(organizer.CodexEndpoint),
		Timeout:  cfg.Ark.Timeout,
	})
}

func buildOfflineKnowledgeRegressionRunner(organizer config.OfflineKnowledgeOrganizerConfig, ragSvc *rag.Service, datasets *dataset.Service, repo eval.Repository, opts offlineknowledge.ServiceOptions) offlineknowledge.RegressionRunner {
	if !organizer.RegressionEvalEnabled {
		return offlineknowledge.DisabledRegressionRunner{}
	}
	if ragSvc == nil || datasets == nil {
		return offlineknowledge.UnavailableRegressionRunner{}
	}
	baselineRAG := cloneRAGForRegression(ragSvc, rag.ShadowOptions{})
	withOptimizationRAG := cloneRAGForRegression(ragSvc, rag.ShadowOptions{
		Enabled: true,
		Inject:  true,
		Limit:   organizer.MaxClustersPerRun,
	}, opts)
	return offlineknowledge.NewEvalRegressionRunner(offlineknowledge.EvalRegressionRunnerOptions{
		BaselineRunner: eval.Runner{
			RAG:        baselineRAG,
			Datasets:   datasets,
			Repository: repo,
		},
		WithOptimization: eval.Runner{
			RAG:        withOptimizationRAG,
			Datasets:   datasets,
			Repository: repo,
		},
		Datasets:                datasets,
		DatasetID:               organizer.RegressionDatasetID,
		BaselineProfile:         rag.ProfileRealtime,
		WithOptimizationProfile: rag.ProfileRealtime,
		TopK:                    cfgTopKForRegression(organizer),
	})
}

func cloneRAGForRegression(base *rag.Service, shadow rag.ShadowOptions, opts ...offlineknowledge.ServiceOptions) *rag.Service {
	if base == nil {
		return nil
	}
	cp := *base
	cp.Shadow = shadow
	cp.ShadowRetriever = nil
	cp.ShadowSourceReader = nil
	if shadow.Enabled && len(opts) > 0 {
		if opts[0].ShadowRetriever != nil {
			cp.ShadowRetriever = offlineKnowledgeShadowRetrieverAdapter{retriever: opts[0].ShadowRetriever}
		}
		if opts[0].SourceReader != nil {
			cp.ShadowSourceReader = offlineKnowledgeShadowSourceReaderAdapter{reader: opts[0].SourceReader}
		}
	}
	return &cp
}

func cfgTopKForRegression(organizer config.OfflineKnowledgeOrganizerConfig) int {
	if organizer.MaxClustersPerRun > 0 {
		return organizer.MaxClustersPerRun
	}
	return 0
}

func buildOfflineKnowledgeScheduler(cfg config.Config, svc *offlineknowledge.Service, logger *slog.Logger) *offlineknowledge.Scheduler {
	organizer := cfg.Maintenance.OfflineKnowledgeOrganizer
	if !organizer.Enabled {
		return nil
	}
	return offlineknowledge.NewScheduler(svc, offlineknowledge.SchedulerConfig{
		Enabled:            organizer.Enabled,
		Schedule:           organizer.Schedule,
		LookbackDays:       organizer.LookbackDays,
		MaxQuestionsPerRun: organizer.MaxQuestionsPerRun,
		MaxClustersPerRun:  organizer.MaxClustersPerRun,
		Targets:            offlineKnowledgeSchedulerTargets(organizer.Targets),
		ConfigJSON: map[string]any{
			"max_codex_concurrency":               organizer.MaxCodexConcurrency,
			"max_codex_deep_search_steps":         organizer.MaxCodexDeepSearchSteps,
			"max_codex_tokens_per_question":       organizer.MaxCodexTokensPerQuestion,
			"max_tool_qps_per_tenant":             organizer.MaxToolQPSPerTenant,
			"max_tool_rows_per_call":              organizer.MaxToolRowsPerCall,
			"max_replay_concurrency":              organizer.MaxReplayConcurrency,
			"max_eval_concurrency":                organizer.MaxEvalConcurrency,
			"min_question_occurrence":             organizer.MinQuestionOccurrence,
			"long_tail_sampling_rate":             organizer.LongTailSamplingRate,
			"explicit_negative_feedback_boost":    organizer.ExplicitNegativeFeedbackBoost,
			"evidence_validation_enabled":         organizer.EvidenceValidationEnabled,
			"conclusion_judge_enabled":            organizer.ConclusionJudgeEnabled,
			"shadow_retrieval_enabled":            organizer.ShadowRetrievalEnabled,
			"shadow_inject_enabled":               organizer.ShadowInjectEnabled,
			"auto_publish_enabled":                organizer.AutoPublishEnabled,
			"regression_eval_enabled":             organizer.RegressionEvalEnabled,
			"regression_dataset_id":               organizer.RegressionDatasetID,
			"full_regression_for_rewrite_enabled": organizer.FullRegressionForRewriteEnabled,
		},
	}, offlineknowledge.SchedulerOptions{Logger: logger})
}

func offlineKnowledgeSchedulerTargets(targets []config.OfflineKnowledgeOrganizerTargetConfig) []offlineknowledge.SchedulerTarget {
	out := make([]offlineknowledge.SchedulerTarget, 0, len(targets))
	for _, target := range targets {
		out = append(out, offlineknowledge.SchedulerTarget{
			TenantID: target.TenantID,
			KBID:     target.KBID,
		})
	}
	return out
}

type traceHistorySource struct {
	repo TraceRepository
}

func (s traceHistorySource) ListHistoryTraces(ctx context.Context, filter offlineknowledge.HistoryTraceFilter) ([]offlineknowledge.HistoryTrace, error) {
	if s.repo == nil {
		return nil, offlineknowledge.ErrHistorySourceRequired
	}
	traces, err := s.repo.ListTraces(ctx, postgres.TraceListFilter{
		TenantID: filter.TenantID,
		KBID:     filter.KBID,
		Since:    filter.Since,
		Until:    filter.Until,
		Limit:    filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]offlineknowledge.HistoryTrace, 0, len(traces))
	for _, trace := range traces {
		out = append(out, offlineknowledge.HistoryTrace{
			TenantID:        trace.TenantID,
			KBID:            trace.KBID,
			TraceID:         trace.ID,
			Query:           trace.Query,
			Answer:          trace.Answer,
			RetrievedChunks: append([]string(nil), trace.RetrievedChunks...),
			Latency:         time.Duration(trace.LatencyMS) * time.Millisecond,
			HasError:        trace.HasError,
			Error:           firstTraceSpanError(trace.NodeSpans),
			CreatedAt:       trace.CreatedAt,
		})
	}
	return out, nil
}

func firstTraceSpanError(spans []postgres.TraceNodeSpan) string {
	for _, span := range spans {
		if span.Error != "" {
			return span.Error
		}
	}
	return ""
}

type offlineKnowledgeChatModel interface {
	Chat(ctx context.Context, messages []ark.ChatMessage) (string, error)
}

type judgeChatModelAdapter struct {
	Model     offlineKnowledgeChatModel
	ModelName string
}

func (a judgeChatModelAdapter) Chat(ctx context.Context, prompt string) (eval.JudgeChatResponse, error) {
	if a.Model == nil {
		return eval.JudgeChatResponse{}, offlineknowledge.ErrConclusionUnavailable
	}
	content, err := a.Model.Chat(ctx, []ark.ChatMessage{{Role: "user", Content: prompt}})
	if err != nil {
		return eval.JudgeChatResponse{}, err
	}
	return eval.JudgeChatResponse{Content: content, Model: a.ModelName}, nil
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
	store                kb.KnowledgeBaseRepository
	indexer              kb.Indexer
	dense                kb.Retriever
	sparse               kb.Retriever
	cache                rag.SemanticCacheStore
	jobs                 ingest.JobStore
	traceStore           raggraph.TraceStore
	traceRepo            TraceRepository
	datasetRepo          dataset.Repository
	projectRepo          project.Repository
	tutorialCloneRepo    tutorial.CloneRepository
	apiKeyRepo           auth.APIKeyRepository
	evalRepo             eval.Repository
	evaluationPolicyRepo evaluationpolicy.Repository
	optimizerRepo        optimizer.Repository
	offlineKnowledgeRepo offlineknowledge.Repository
	releaseRepo          release.Repository
	pipelineRepo         pipeline.Repository
	chunkSource          kb.ChunkSource
	pool                 *pgxpool.Pool
	qdrant               *qdrantstore.Client
	closers              []func() error
}

type KnowledgeBaseVectorDeleter interface {
	DeleteKnowledgeBaseVectors(ctx context.Context, tenantID, kbID string) error
}

type KnowledgeBaseSemanticCacheDeleter interface {
	DeleteKnowledgeBaseSemanticCache(ctx context.Context, tenantID, kbID string) error
}

type knowledgeBaseStore struct {
	primary              kb.KnowledgeBaseRepository
	vectorDeleter        KnowledgeBaseVectorDeleter
	semanticCacheDeleter KnowledgeBaseSemanticCacheDeleter
}

func NewKnowledgeBaseStore(
	primary kb.KnowledgeBaseRepository,
	vectorDeleter KnowledgeBaseVectorDeleter,
	semanticCacheDeleter KnowledgeBaseSemanticCacheDeleter,
) kb.KnowledgeBaseRepository {
	return knowledgeBaseStore{
		primary:              primary,
		vectorDeleter:        vectorDeleter,
		semanticCacheDeleter: semanticCacheDeleter,
	}
}

func (s knowledgeBaseStore) PutKnowledgeBase(ctx context.Context, item kb.KnowledgeBase) error {
	return s.primary.PutKnowledgeBase(ctx, item)
}

func (s knowledgeBaseStore) ListKnowledgeBases(ctx context.Context, tenantID string) ([]kb.KnowledgeBase, error) {
	return s.primary.ListKnowledgeBases(ctx, tenantID)
}

func (s knowledgeBaseStore) ListKnowledgeBasesByProject(ctx context.Context, tenantID, projectID string) ([]kb.KnowledgeBase, error) {
	if scoped, ok := s.primary.(kb.ProjectKnowledgeBaseRepository); ok {
		return scoped.ListKnowledgeBasesByProject(ctx, tenantID, projectID)
	}
	items, err := s.primary.ListKnowledgeBases(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]kb.KnowledgeBase, 0, len(items))
	for _, item := range items {
		if item.ProjectID == projectID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s knowledgeBaseStore) GetKnowledgeBase(ctx context.Context, tenantID, id string) (kb.KnowledgeBase, bool, error) {
	return s.primary.GetKnowledgeBase(ctx, tenantID, id)
}

func (s knowledgeBaseStore) GetKnowledgeBaseByProject(ctx context.Context, tenantID, projectID, id string) (kb.KnowledgeBase, bool, error) {
	if scoped, ok := s.primary.(kb.ProjectKnowledgeBaseRepository); ok {
		return scoped.GetKnowledgeBaseByProject(ctx, tenantID, projectID, id)
	}
	item, found, err := s.primary.GetKnowledgeBase(ctx, tenantID, id)
	return item, found && item.ProjectID == projectID, err
}

func (s knowledgeBaseStore) DeleteKnowledgeBase(ctx context.Context, tenantID, id string) (bool, error) {
	if _, ok, err := s.primary.GetKnowledgeBase(ctx, tenantID, id); err != nil {
		return false, err
	} else if !ok {
		return false, nil
	}
	if s.semanticCacheDeleter != nil {
		if err := s.semanticCacheDeleter.DeleteKnowledgeBaseSemanticCache(ctx, tenantID, id); err != nil {
			return false, err
		}
	}
	if s.vectorDeleter != nil {
		if err := s.vectorDeleter.DeleteKnowledgeBaseVectors(ctx, tenantID, id); err != nil {
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
		projectRepo := newMemoryProjectRepository()
		if err := bootstrapMemory(ctx, store, defaultTenant); err != nil {
			return knowledgeBackend{}, err
		}
		now := time.Now().UTC()
		if err := projectRepo.CreateWithEnvironments(ctx, project.Project{
			ID: project.LegacyDefaultID(defaultTenant), TenantID: defaultTenant,
			Name: "Legacy Default", Description: "Compatibility project for pre-project resources.",
			CreatedAt: now, UpdatedAt: now,
		}, project.LegacyDefaultEnvironments(defaultTenant, now)); err != nil {
			return knowledgeBackend{}, err
		}
		return knowledgeBackend{
			store:                store,
			indexer:              store,
			dense:                kb.DenseRetriever{Store: store},
			sparse:               kb.SparseRetriever{Store: store},
			cache:                rag.NewSemanticCache(cfg.RAG.SemanticCacheMaxEntries),
			jobs:                 ingest.NewMemoryJobStore(),
			traceStore:           traceRepo,
			traceRepo:            traceRepo,
			datasetRepo:          dataset.NewMemoryRepository(),
			projectRepo:          projectRepo,
			tutorialCloneRepo:    tutorial.NewMemoryCloneRepository(),
			releaseRepo:          release.NewMemoryRepository(project.LegacyDefaultID(defaultTenant)),
			pipelineRepo:         pipeline.NewMemoryRepository(),
			apiKeyRepo:           auth.NewMemoryAPIKeyRepository(),
			evalRepo:             eval.NewMemoryRepository(),
			evaluationPolicyRepo: evaluationpolicy.NewMemoryRepository(),
			optimizerRepo:        optimizer.NewMemoryRepository(),
			offlineKnowledgeRepo: offlineknowledge.NewMemoryRepository(),
			chunkSource:          store,
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
	vectors := newPostgresVectorStore(qdrantClient, cfg.Qdrant.Collection, repo)
	indexer := kb.CompositeIndexer{Indexers: []kb.Indexer{repo, vectors}}
	cache := qdrantstore.SemanticCache{Client: qdrantClient, Collection: cfg.Qdrant.SemanticCacheCollection, Threshold: cfg.RAG.SemanticCacheThreshold}
	return knowledgeBackend{
		store:                NewKnowledgeBaseStore(repo, vectors, cache),
		indexer:              indexer,
		dense:                vectors,
		sparse:               postgres.NewFTSRetriever(repo),
		cache:                cache,
		jobs:                 repo,
		traceStore:           repo,
		traceRepo:            repo,
		datasetRepo:          repo,
		projectRepo:          postgres.NewProjectRepository(pool),
		tutorialCloneRepo:    postgres.NewTutorialCloneRepository(pool),
		releaseRepo:          repo,
		pipelineRepo:         repo,
		apiKeyRepo:           postgres.NewAPIKeyRepository(pool),
		evalRepo:             repo,
		evaluationPolicyRepo: repo,
		optimizerRepo:        repo,
		offlineKnowledgeRepo: repo,
		chunkSource:          repo,
		pool:                 pool,
		qdrant:               qdrantClient,
		closers: []func() error{
			qdrantClient.Conn.Close,
			func() error {
				pool.Close()
				return nil
			},
		},
	}, nil
}

func newPostgresVectorStore(client *qdrantstore.Client, collection string, repo *postgres.Repository) qdrantstore.VectorStore {
	return qdrantstore.VectorStore{
		Client:     client,
		Collection: collection,
		Visibility: repo,
	}
}

func bootstrapMemory(ctx context.Context, store *kb.MemoryStore, tenantID string) error {
	now := time.Now().UTC()
	return store.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:          "kb_default",
		TenantID:    tenantID,
		ProjectID:   project.LegacyDefaultID(tenantID),
		Name:        "Default Knowledge Base",
		Description: "默认知识库",
		Metadata:    map[string]string{"created_by": "bootstrap"},
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

type memoryProjectRepository struct {
	mu    sync.RWMutex
	items map[string]project.Project
}

func newMemoryProjectRepository() *memoryProjectRepository {
	return &memoryProjectRepository{items: make(map[string]project.Project)}
}
func (r *memoryProjectRepository) CreateWithEnvironments(_ context.Context, item project.Project, _ []project.Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
	return nil
}
func (r *memoryProjectRepository) List(_ context.Context, tenantID string) ([]project.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]project.Project, 0)
	for _, item := range r.items {
		if item.TenantID == tenantID {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *memoryProjectRepository) Get(_ context.Context, tenantID, projectID string) (project.Project, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[projectID]
	return item, ok && item.TenantID == tenantID, nil
}
func (r *memoryProjectRepository) Update(_ context.Context, item project.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
	return nil
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
	for name, check := range a.offlineKnowledgeReadinessChecks() {
		checks[name] = check
		a.observeDependencyCheck(name, check.Status, 0)
		if a.Config.Maintenance.OfflineKnowledgeOrganizer.Enabled && (check.Status == "unavailable" || check.Status == "error") {
			ready = false
		}
	}
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

func (a *App) offlineKnowledgeReadinessChecks() map[string]ReadinessCheck {
	organizer := a.Config.Maintenance.OfflineKnowledgeOrganizer
	checks := map[string]ReadinessCheck{}
	if a.OfflineKnowledge == nil {
		checks["offline_knowledge.service"] = ReadinessCheck{Status: "error", Error: "not configured"}
		return checks
	}
	checks["offline_knowledge.service"] = ReadinessCheck{Status: "ready"}
	if !organizer.CodexEnabled {
		checks["offline_knowledge.codex"] = ReadinessCheck{Status: "disabled"}
	} else if strings.TrimSpace(organizer.CodexCommand) == "" && strings.TrimSpace(organizer.CodexEndpoint) == "" {
		checks["offline_knowledge.codex"] = ReadinessCheck{Status: "unavailable", Error: offlineknowledge.ErrCodexUnavailable.Error()}
	} else {
		checks["offline_knowledge.codex"] = ReadinessCheck{Status: "configured"}
	}
	if !organizer.ConclusionJudgeEnabled {
		checks["offline_knowledge.judge"] = ReadinessCheck{Status: "disabled"}
	} else if a.modelProviderStatus() == "missing_credentials" {
		checks["offline_knowledge.judge"] = ReadinessCheck{Status: "unavailable", Error: offlineknowledge.ErrConclusionUnavailable.Error()}
	} else {
		checks["offline_knowledge.judge"] = ReadinessCheck{Status: "configured"}
	}
	if !organizer.RegressionEvalEnabled {
		checks["offline_knowledge.regression"] = ReadinessCheck{Status: "disabled"}
	} else if a.RAG == nil || a.Datasets == nil {
		checks["offline_knowledge.regression"] = ReadinessCheck{Status: "unavailable", Error: offlineknowledge.ErrRegressionUnavailable.Error()}
	} else {
		checks["offline_knowledge.regression"] = ReadinessCheck{Status: "configured"}
	}
	return checks
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
