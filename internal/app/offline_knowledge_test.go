package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/dataset"
	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/offlineknowledge"
	"github.com/shikanon/orag/internal/prompt"
	"github.com/shikanon/orag/internal/rag"
)

func TestBuildOfflineKnowledgeOptionsWiresRealSourceReaderAndJudge(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	if err := store.Store(ctx, kb.Document{
		ID:              "doc_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		ContentHash:     "v1",
		CreatedAt:       time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
	}, []kb.Chunk{{
		ID:              "chunk_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_1",
		Content:         "ORAG is a retrieval augmented generation framework.",
		Metadata:        map[string]string{"chunk_content_hash": "sha256:chunk_1"},
	}}); err != nil {
		t.Fatal(err)
	}

	opts := buildOfflineKnowledgeOptions(offlineKnowledgeAppConfig(true), knowledgeBackend{
		chunkSource: store,
	}, &offlineKnowledgeJudgeModel{
		response: `{"score":1,"claims":[{"claim":"ORAG is a retrieval augmented generation framework","question":"What is ORAG?","answer":"A retrieval augmented generation framework","verdict":"supported","evidence":"ORAG is a retrieval augmented generation framework"}]}`,
	}, kb.SparseRetriever{Store: store}, nil, nil, nil)
	if opts.Validator == nil {
		t.Fatal("Validator is nil, want wired validator")
	}

	err := opts.Validator.ValidateItem(ctx, "tenant_1", "kb_1", offlineKnowledgeAppValidatorItem())
	if err != nil {
		t.Fatalf("ValidateItem() error = %v, want nil", err)
	}
}

func TestBuildOfflineKnowledgeOptionsDisabledJudgeReturnsExplicitError(t *testing.T) {
	store := kb.NewMemoryStore()
	if err := store.Store(context.Background(), kb.Document{
		ID:              "doc_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		ContentHash:     "v1",
	}, []kb.Chunk{{
		ID:              "chunk_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_1",
		Content:         "ORAG is a retrieval augmented generation framework.",
		Metadata:        map[string]string{"chunk_content_hash": "sha256:chunk_1"},
	}}); err != nil {
		t.Fatal(err)
	}
	opts := buildOfflineKnowledgeOptions(offlineKnowledgeAppConfig(false), knowledgeBackend{chunkSource: store}, &offlineKnowledgeJudgeModel{}, kb.SparseRetriever{Store: store}, nil, nil, nil)

	err := opts.Validator.ValidateItem(context.Background(), "tenant_1", "kb_1", offlineKnowledgeAppValidatorItem())
	if err == nil || err != offlineknowledge.ErrConclusionDisabled {
		t.Fatalf("ValidateItem() error = %v, want %v", err, offlineknowledge.ErrConclusionDisabled)
	}
}

func TestBuildOfflineKnowledgeSchedulerDisabledReturnsNil(t *testing.T) {
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.Enabled = false
	svc := offlineKnowledgeAppService(t, cfg, kb.NewMemoryStore())

	scheduler := buildOfflineKnowledgeScheduler(cfg, svc, nil)
	if scheduler != nil {
		t.Fatal("buildOfflineKnowledgeScheduler() returned scheduler, want nil when disabled")
	}
}

func TestBuildOfflineKnowledgeOptionsWiresRealRuntimeDependencies(t *testing.T) {
	store := kb.NewMemoryStore()
	repo := offlineknowledge.NewMemoryRepository()
	traceRepo := newMemoryTraceRepository()
	cfg := offlineKnowledgeAppConfig(true)
	opts := buildOfflineKnowledgeOptions(cfg, knowledgeBackend{
		store:                store,
		offlineKnowledgeRepo: repo,
		traceRepo:            traceRepo,
		chunkSource:          store,
		evalRepo:             nil,
	}, &offlineKnowledgeJudgeModel{
		response: `{"score":1,"claims":[{"claim":"ORAG is a retrieval augmented generation framework","question":"What is ORAG?","answer":"A retrieval augmented generation framework","verdict":"supported","evidence":"ORAG is a retrieval augmented generation framework"}]}`,
	}, kb.SparseRetriever{Store: store}, nil, nil, nil)

	if opts.HistorySource == nil || opts.QuestionClusterer == nil || opts.RecallReplayer == nil || opts.SourceReader == nil || opts.Validator == nil {
		t.Fatalf("core dependencies not fully wired: %#v", opts)
	}
	history, ok := opts.HistorySource.(*offlineknowledge.TraceHistoryExtractor)
	if !ok {
		t.Fatalf("HistorySource type = %T, want *TraceHistoryExtractor", opts.HistorySource)
	}
	if history.NegativeFeedback == nil {
		t.Fatal("HistorySource NegativeFeedback is nil, want repository-backed source")
	}
	if history.NegativeFeedback != repo {
		t.Fatalf("HistorySource NegativeFeedback = %T, want offline knowledge repository", history.NegativeFeedback)
	}
	if opts.CodexAnalyzer == nil || opts.CodexTools == nil || opts.ShadowRetriever == nil || opts.RegressionRunner == nil {
		t.Fatalf("runtime dependencies not fully wired: %#v", opts)
	}
	if _, err := opts.CodexAnalyzer.AnalyzeCodex(context.Background(), offlineKnowledgeCodexRequest()); !errors.Is(err, offlineknowledge.ErrCodexDisabled) {
		t.Fatalf("AnalyzeCodex() error = %v, want %v", err, offlineknowledge.ErrCodexDisabled)
	}
	if _, err := opts.RegressionRunner.RunRegression(context.Background(), offlineknowledge.RegressionRequest{}); !errors.Is(err, offlineknowledge.ErrRegressionUnavailable) {
		t.Fatalf("RunRegression() error = %v, want %v", err, offlineknowledge.ErrRegressionUnavailable)
	}
}

func TestBuildOfflineKnowledgeOptionsWiresCodexToolAuditSink(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	if err := store.Store(ctx, kb.Document{
		ID:              "doc_audit",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		ContentHash:     "v1",
		CreatedAt:       time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
	}, []kb.Chunk{{
		ID:              "chunk_audit",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_audit",
		Content:         "Codex tool audit sink is wired by App.",
	}}); err != nil {
		t.Fatal(err)
	}
	repo := offlineknowledge.NewMemoryRepository()
	opts := buildOfflineKnowledgeOptions(offlineKnowledgeAppConfig(true), knowledgeBackend{
		store:                store,
		offlineKnowledgeRepo: repo,
		traceRepo:            newMemoryTraceRepository(),
		chunkSource:          store,
		evalRepo:             nil,
	}, &offlineKnowledgeJudgeModel{}, kb.SparseRetriever{Store: store}, nil, nil, nil)

	if opts.CodexTools == nil {
		t.Fatal("CodexTools is nil, want registry with audit sink")
	}
	if _, err := opts.CodexTools.Execute(ctx, offlineknowledge.CodexToolCall{
		SessionID: "session_app_audit",
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		Tool:      offlineknowledge.ReadOnlyToolSearchChunksByText,
		Query:     "audit sink",
		MaxRows:   2,
		Timeout:   time.Second,
	}); err != nil {
		t.Fatalf("CodexTools.Execute() error = %v", err)
	}
	events, err := repo.ListCodexToolAuditEvents(ctx, offlineknowledge.CodexToolAuditFilter{
		TenantID:  "tenant_1",
		KBID:      "kb_1",
		SessionID: "session_app_audit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !events[0].Allowed || events[0].Tool != offlineknowledge.ReadOnlyToolSearchChunksByText {
		t.Fatalf("audit events = %#v, want App-wired successful tool audit", events)
	}
}

func TestBuildOfflineKnowledgeOptionsWiresRealRegressionRunnerWhenEnabled(t *testing.T) {
	ctx := context.Background()
	dsRepo := dataset.NewMemoryRepository()
	if _, err := dsRepo.CreateDataset(ctx, dataset.Dataset{
		ID:       "ds_regression",
		TenantID: "tenant_1",
		Name:     "offline regression",
		Kind:     "golden",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dsRepo.AddDatasetItem(ctx, "tenant_1", dataset.Item{
		ID:             "item_1",
		DatasetID:      "ds_regression",
		Query:          "What is ORAG?",
		GroundTruth:    "ORAG framework",
		RelevantDocIDs: []string{"doc_1"},
	}); err != nil {
		t.Fatal(err)
	}
	dsSvc := dataset.NewService(dsRepo)
	evalRepo := eval.NewMemoryRepository()
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.RegressionDatasetID = "ds_regression"
	pipeline := &appRecordingRegressionPipeline{}
	ragSvc := &rag.Service{Pipeline: pipeline}
	opts := buildOfflineKnowledgeOptions(cfg, knowledgeBackend{
		store:                kb.NewMemoryStore(),
		offlineKnowledgeRepo: offlineknowledge.NewMemoryRepository(),
		traceRepo:            newMemoryTraceRepository(),
		chunkSource:          kb.NewMemoryStore(),
		evalRepo:             evalRepo,
	}, &offlineKnowledgeJudgeModel{}, kb.SparseRetriever{}, ragSvc, dsSvc, nil)

	result, err := opts.RegressionRunner.RunRegression(ctx, offlineknowledge.RegressionRequest{
		TenantID: "tenant_1",
		Item: offlineknowledge.OptimizationItem{
			ID:                "item_regression_app",
			TenantID:          "tenant_1",
			KBID:              "kb_1",
			QuestionClusterID: "cluster_must_not_be_used_as_dataset",
			ItemType:          offlineknowledge.ItemTypeAnswer,
		},
	})
	if err != nil {
		t.Fatalf("RunRegression() error = %v", err)
	}
	if !result.Passed || !result.FullDatasetUsed {
		t.Fatalf("regression result = %#v, want passed with full dataset", result)
	}
	if len(pipeline.requests) != 2 {
		t.Fatalf("pipeline requests = %d, want baseline and candidate", len(pipeline.requests))
	}
	if pipeline.requests[0].Profile != rag.ProfileRealtime || pipeline.requests[1].Profile != rag.ProfileRealtime {
		t.Fatalf("pipeline profiles = %q/%q, want same realtime", pipeline.requests[0].Profile, pipeline.requests[1].Profile)
	}
	if pipeline.requests[0].ScopedShadowItemID != "" || pipeline.requests[1].ScopedShadowItemID != "item_regression_app" {
		t.Fatalf("pipeline scoped ids = %q/%q, want empty/current item", pipeline.requests[0].ScopedShadowItemID, pipeline.requests[1].ScopedShadowItemID)
	}
	if !result.ProfileNeutrality.SameProfile || !result.ProfileNeutrality.OptimizationLiftOnly {
		t.Fatalf("profile neutrality = %#v, want neutral optimization lift", result.ProfileNeutrality)
	}
	if _, found, err := evalRepo.GetEvaluationDetail(ctx, "tenant_1", "missing", eval.EvaluationDetailOptions{}); err != nil || found {
		t.Fatalf("unexpected eval repo behavior found=%v err=%v", found, err)
	}
}

func TestBuildOfflineKnowledgeOptionsRegressionRunnerRequiresConfiguredDataset(t *testing.T) {
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.RegressionDatasetID = ""
	opts := buildOfflineKnowledgeOptions(cfg, knowledgeBackend{
		store:                kb.NewMemoryStore(),
		offlineKnowledgeRepo: offlineknowledge.NewMemoryRepository(),
		traceRepo:            newMemoryTraceRepository(),
		chunkSource:          kb.NewMemoryStore(),
		evalRepo:             eval.NewMemoryRepository(),
	}, &offlineKnowledgeJudgeModel{}, kb.SparseRetriever{}, &rag.Service{Pipeline: appRegressionPipeline{}}, dataset.NewService(), nil)

	_, err := opts.RegressionRunner.RunRegression(context.Background(), offlineknowledge.RegressionRequest{
		TenantID: "tenant_1",
		Item: offlineknowledge.OptimizationItem{
			ID:                "item_missing_dataset",
			TenantID:          "tenant_1",
			KBID:              "kb_1",
			QuestionClusterID: "cluster_legacy_should_not_be_used",
			ItemType:          offlineknowledge.ItemTypeAnswer,
		},
	})
	if !errors.Is(err, offlineknowledge.ErrRegressionDatasetRequired) {
		t.Fatalf("RunRegression() error = %v, want %v", err, offlineknowledge.ErrRegressionDatasetRequired)
	}
}

func TestBuildOfflineKnowledgeOptionsUsesDisabledRegressionRunnerWhenDisabled(t *testing.T) {
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.RegressionEvalEnabled = false
	opts := buildOfflineKnowledgeOptions(cfg, knowledgeBackend{}, &offlineKnowledgeJudgeModel{}, kb.SparseRetriever{}, &rag.Service{}, dataset.NewService(), nil)

	_, err := opts.RegressionRunner.RunRegression(context.Background(), offlineknowledge.RegressionRequest{})
	if !errors.Is(err, offlineknowledge.ErrRegressionDisabled) {
		t.Fatalf("RunRegression() error = %v, want %v", err, offlineknowledge.ErrRegressionDisabled)
	}
}

func TestConfigureRAGShadowPassesOrganizerConfig(t *testing.T) {
	store := kb.NewMemoryStore()
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.ShadowRetrievalEnabled = true
	cfg.Maintenance.OfflineKnowledgeOrganizer.ShadowInjectEnabled = true
	cfg.Maintenance.OfflineKnowledgeOrganizer.MaxClustersPerRun = 5
	opts := buildOfflineKnowledgeOptions(cfg, knowledgeBackend{
		store:                store,
		offlineKnowledgeRepo: offlineknowledge.NewMemoryRepository(),
		traceRepo:            newMemoryTraceRepository(),
		chunkSource:          store,
	}, &offlineKnowledgeJudgeModel{}, kb.SparseRetriever{Store: store}, nil, nil, nil)
	service := &rag.Service{}

	configureRAGShadow(service, cfg.Maintenance.OfflineKnowledgeOrganizer, opts)

	if !service.Shadow.Enabled || !service.Shadow.Inject || service.Shadow.Limit != 5 {
		t.Fatalf("rag shadow config = %#v, want enabled inject limit 5", service.Shadow)
	}
	if service.ShadowRetriever == nil || service.ShadowSourceReader == nil {
		t.Fatalf("rag shadow dependencies not configured: retriever=%T source=%T", service.ShadowRetriever, service.ShadowSourceReader)
	}
}

func TestConfigureRAGShadowKeepsDisabledRetrievalOff(t *testing.T) {
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.ShadowRetrievalEnabled = false
	cfg.Maintenance.OfflineKnowledgeOrganizer.ShadowInjectEnabled = true
	service := &rag.Service{}

	configureRAGShadow(service, cfg.Maintenance.OfflineKnowledgeOrganizer, offlineknowledge.ServiceOptions{})

	if service.Shadow.Enabled {
		t.Fatalf("rag shadow enabled = true, want false when config disables retrieval")
	}
	if !service.Shadow.Inject {
		t.Fatalf("rag shadow inject = false, want raw config preserved")
	}
}

func TestConfiguredRAGShadowWritesEventWithoutChangingDefaultAnswer(t *testing.T) {
	ctx := context.Background()
	store := kb.NewMemoryStore()
	if err := store.Store(ctx, kb.Document{
		ID:              "doc_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		ContentHash:     "v1",
	}, []kb.Chunk{{
		ID:              "chunk_1",
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		DocumentID:      "doc_1",
		Content:         "ORAG is a retrieval augmented generation framework.",
		Metadata:        map[string]string{"chunk_content_hash": "sha256:chunk_1"},
	}}); err != nil {
		t.Fatal(err)
	}
	repo := offlineknowledge.NewMemoryRepository()
	item := offlineKnowledgeAppValidatorItem()
	item.Status = offlineknowledge.ItemStatusShadowEnabled
	if err := repo.CreateOptimizationItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.ShadowRetrievalEnabled = true
	cfg.Maintenance.OfflineKnowledgeOrganizer.ShadowInjectEnabled = false
	cfg.Maintenance.OfflineKnowledgeOrganizer.ShadowEventSamplingRate = 1
	opts := buildOfflineKnowledgeOptions(cfg, knowledgeBackend{
		store:                store,
		offlineKnowledgeRepo: repo,
		traceRepo:            newMemoryTraceRepository(),
		chunkSource:          store,
	}, &offlineKnowledgeJudgeModel{}, kb.SparseRetriever{Store: store}, nil, nil, nil)
	service := &rag.Service{
		Retriever:       appRAGRetriever{},
		Model:           appRAGModel{},
		Packer:          rag.ContextPacker{MaxTokens: 512, TopN: 4},
		PromptStrategy:  prompt.NewStrategy("auto"),
		DefaultProfile:  rag.ProfileRealtime,
		NoContextAnswer: "no context",
		TopK:            4,
	}
	configureRAGShadow(service, cfg.Maintenance.OfflineKnowledgeOrganizer, opts)

	resp, err := service.Execute(ctx, rag.QueryRequest{
		TenantID:        "tenant_1",
		KnowledgeBaseID: "kb_1",
		Query:           "What is ORAG?",
		TraceID:         "trace_shadow_app",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if resp.Answer != "online answer [chunk_1]" {
		t.Fatalf("answer = %q, want online answer unchanged", resp.Answer)
	}
	events, err := repo.ListShadowEvents(ctx, offlineknowledge.ShadowRetrievalEventFilter{
		TenantID: "tenant_1",
		KBID:     "kb_1",
		TraceID:  "trace_shadow_app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ItemID != item.ID || events[0].Injected {
		t.Fatalf("shadow events = %#v, want one non-injected event for item", events)
	}
}

func TestBuildOfflineKnowledgeSchedulerEnabledStartsAndStops(t *testing.T) {
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.Enabled = true
	cfg.Maintenance.OfflineKnowledgeOrganizer.Schedule = "0 2 * * *"
	cfg.Maintenance.OfflineKnowledgeOrganizer.LookbackDays = 7
	cfg.Maintenance.OfflineKnowledgeOrganizer.Targets = []config.OfflineKnowledgeOrganizerTargetConfig{
		{TenantID: "tenant_1", KBID: "kb_1"},
	}
	svc := offlineKnowledgeAppService(t, cfg, kb.NewMemoryStore())

	scheduler := buildOfflineKnowledgeScheduler(cfg, svc, nil)
	if scheduler == nil {
		t.Fatal("buildOfflineKnowledgeScheduler() = nil, want scheduler when enabled")
	}
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !scheduler.IsRunning() {
		t.Fatal("IsRunning() = false, want true after Start")
	}
	if err := scheduler.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if scheduler.IsRunning() {
		t.Fatal("IsRunning() = true, want false after Stop")
	}
}

func TestBuildOfflineKnowledgeSchedulerEnabledWithEmptyTargetsFailsStart(t *testing.T) {
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.Enabled = true
	cfg.Maintenance.OfflineKnowledgeOrganizer.Schedule = "0 2 * * *"
	cfg.Maintenance.OfflineKnowledgeOrganizer.LookbackDays = 7
	cfg.Maintenance.OfflineKnowledgeOrganizer.Targets = nil
	svc := offlineKnowledgeAppService(t, cfg, kb.NewMemoryStore())

	scheduler := buildOfflineKnowledgeScheduler(cfg, svc, nil)
	if scheduler == nil {
		t.Fatal("buildOfflineKnowledgeScheduler() = nil, want explicit scheduler that fails on Start")
	}
	err := scheduler.Start(context.Background())
	if !errors.Is(err, offlineknowledge.ErrSchedulerTargetRequired) {
		t.Fatalf("Start() error = %v, want %v", err, offlineknowledge.ErrSchedulerTargetRequired)
	}
}

func TestBuildOfflineKnowledgeSchedulerConfigUsesOrganizerTargetsAndLimits(t *testing.T) {
	cfg := offlineKnowledgeAppConfig(true)
	cfg.Maintenance.OfflineKnowledgeOrganizer.Enabled = true
	cfg.Maintenance.OfflineKnowledgeOrganizer.Schedule = "15 3 * * *"
	cfg.Maintenance.OfflineKnowledgeOrganizer.LookbackDays = 2
	cfg.Maintenance.OfflineKnowledgeOrganizer.MaxQuestionsPerRun = 11
	cfg.Maintenance.OfflineKnowledgeOrganizer.MaxClustersPerRun = 7
	cfg.Maintenance.OfflineKnowledgeOrganizer.Targets = []config.OfflineKnowledgeOrganizerTargetConfig{
		{TenantID: "tenant_1", KBID: "kb_1"},
		{TenantID: "tenant_2", KBID: "kb_2"},
	}
	svc := offlineKnowledgeAppService(t, cfg, kb.NewMemoryStore())
	scheduler := buildOfflineKnowledgeScheduler(cfg, svc, nil)

	results := scheduler.Trigger(context.Background(), time.Date(2026, 7, 8, 3, 15, 10, 0, time.UTC))
	if len(results) != 2 {
		t.Fatalf("Trigger() results = %d, want 2", len(results))
	}
	first := results[0].Request
	second := results[1].Request
	if first.TenantID != "tenant_1" || first.KBID != "kb_1" || second.TenantID != "tenant_2" || second.KBID != "kb_2" {
		t.Fatalf("Trigger() targets = %s/%s, %s/%s", first.TenantID, first.KBID, second.TenantID, second.KBID)
	}
	if first.MaxQuestions != 11 || first.MaxClusters != 7 || second.MaxQuestions != 11 || second.MaxClusters != 7 {
		t.Fatalf("Trigger() limits = %d/%d and %d/%d, want 11/7 for both", first.MaxQuestions, first.MaxClusters, second.MaxQuestions, second.MaxClusters)
	}
	if got := first.WindowEnd.Sub(first.WindowStart); got != 48*time.Hour {
		t.Fatalf("Trigger() window size = %s, want 48h", got)
	}
	if first.ConfigHash == "" || second.ConfigHash == "" {
		t.Fatal("Trigger() ConfigHash is empty")
	}
}

func offlineKnowledgeAppService(t *testing.T, cfg config.Config, store *kb.MemoryStore) *offlineknowledge.Service {
	t.Helper()
	repo := offlineknowledge.NewMemoryRepository()
	opts := buildOfflineKnowledgeOptions(cfg, knowledgeBackend{
		store:                store,
		offlineKnowledgeRepo: repo,
		traceRepo:            newMemoryTraceRepository(),
		chunkSource:          store,
	}, &offlineKnowledgeJudgeModel{}, kb.SparseRetriever{Store: store}, nil, nil, nil)
	return offlineknowledge.NewService(repo, opts)
}

func offlineKnowledgeCodexRequest() offlineknowledge.CodexAnalyzeRequest {
	return offlineknowledge.CodexAnalyzeRequest{
		TenantID:          "tenant_1",
		KBID:              "kb_1",
		CanonicalQuestion: "What is ORAG?",
		Constraints: offlineknowledge.CodexConstraints{
			ReadOnlyTools: []offlineknowledge.ReadOnlyToolName{offlineknowledge.ReadOnlyToolSearchChunksByText},
			Quota: offlineknowledge.ToolQuota{
				MaxTokens:          100,
				MaxRowsPerCall:     10,
				MaxQPSPerTenant:    5,
				MaxDeepSearchSteps: 4,
			},
			AllowedItemTypes: []offlineknowledge.ItemType{offlineknowledge.ItemTypeAnswer},
			AllowedActions:   []offlineknowledge.RecommendedAction{offlineknowledge.RecommendedActionCreateAnswerItem},
		},
	}
}

func offlineKnowledgeAppConfig(conclusionJudgeEnabled bool) config.Config {
	return config.Config{
		Ark: config.ArkConfig{
			ChatModel:  "judge-model",
			Timeout:    time.Second,
			RetryTimes: 0,
		},
		Maintenance: config.MaintenanceConfig{
			OfflineKnowledgeOrganizer: config.OfflineKnowledgeOrganizerConfig{
				EvidenceValidationEnabled: true,
				ConclusionJudgeEnabled:    conclusionJudgeEnabled,
				MinVerifyConfidence:       0.8,
				MaxCodexTokensPerQuestion: 1000,
				MaxCodexDeepSearchSteps:   4,
				MaxToolRowsPerCall:        10,
				MaxToolQPSPerTenant:       5,
				MaxQuestionsPerRun:        10,
				MaxClustersPerRun:         10,
				RegressionEvalEnabled:     true,
				RegressionDatasetID:       "ds_regression",
			},
		},
	}
}

func offlineKnowledgeAppValidatorItem() offlineknowledge.OptimizationItem {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	return offlineknowledge.OptimizationItem{
		ID:                "item_1",
		TenantID:          "tenant_1",
		RunID:             "run_1",
		KBID:              "kb_1",
		QuestionClusterID: "cluster_1",
		ItemType:          offlineknowledge.ItemTypeAnswer,
		Status:            offlineknowledge.ItemStatusCandidate,
		CanonicalQuestion: "What is ORAG?",
		FinalAnswer:       "ORAG is a retrieval augmented generation framework.",
		RecallQuality:     offlineknowledge.RecallQualityMiss,
		FailureType:       offlineknowledge.FailureTypeSemanticGap,
		Confidence:        0.9,
		SourceFingerprints: []offlineknowledge.SourceFingerprint{
			{DocID: "doc_1", DocVersion: "v1", ChunkID: "chunk_1", ChunkContentHash: "sha256:chunk_1"},
		},
		Evidence: []offlineknowledge.Evidence{
			{ChunkID: "chunk_1", DocID: "doc_1", Quote: "ORAG is a retrieval augmented generation framework", Supports: "definition"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

type offlineKnowledgeJudgeModel struct {
	response string
}

func (m *offlineKnowledgeJudgeModel) Chat(context.Context, []ark.ChatMessage) (string, error) {
	return m.response, nil
}

type appRAGRetriever struct{}

func (appRAGRetriever) Retrieve(_ context.Context, req kb.SearchRequest) ([]kb.SearchResult, error) {
	return []kb.SearchResult{{
		Chunk: kb.Chunk{
			ID:              "chunk_1",
			TenantID:        req.TenantID,
			KnowledgeBaseID: req.KnowledgeBaseID,
			DocumentID:      "doc_1",
			Content:         "online source chunk",
			SourceURI:       "memory://online",
		},
		Score: 1,
		Rank:  1,
		From:  "app_test",
	}}, nil
}

type appRAGModel struct{}

func (appRAGModel) Chat(context.Context, []ark.ChatMessage) (string, error) {
	return "online answer [chunk_1]", nil
}

func (appRAGModel) Embed(_ context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i := range texts {
		out[i] = []float64{1, 2}
	}
	return out, nil
}

func (appRAGModel) Rerank(_ context.Context, _ string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
	if topN <= 0 || topN > len(docs) {
		topN = len(docs)
	}
	out := make([]ark.RerankResult, 0, topN)
	for i := 0; i < topN; i++ {
		out = append(out, ark.RerankResult{Index: i, Score: 1})
	}
	return out, nil
}

type appRegressionPipeline struct{}

func (appRegressionPipeline) Invoke(_ context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
	return rag.QueryResponse{
		Answer: "ORAG framework answer",
		Citations: []rag.Citation{{
			ChunkID:    "chunk_1",
			DocumentID: "doc_1",
		}},
		RetrievedChunks: []kb.SearchResult{{
			Chunk: kb.Chunk{ID: "chunk_1", DocumentID: "doc_1"},
		}},
		TraceID:   req.TraceID,
		LatencyMS: 5,
	}, nil
}

type appRecordingRegressionPipeline struct {
	requests []rag.QueryRequest
}

func (p *appRecordingRegressionPipeline) Invoke(ctx context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
	p.requests = append(p.requests, req)
	return appRegressionPipeline{}.Invoke(ctx, req)
}
