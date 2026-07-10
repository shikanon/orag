package offlineknowledge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestServiceCreateRunDeduplicatesByWindowAndConfig(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	svc := NewService(repo, ServiceOptions{Now: fixedServiceNow})
	req := serviceRunRequest()
	req.KBID = ""

	first, deduped, err := svc.CreateRun(ctx, req)
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if deduped {
		t.Fatalf("CreateRun() deduped = true, want false")
	}
	second, deduped, err := svc.CreateRun(ctx, req)
	if err != nil {
		t.Fatalf("CreateRun() duplicate error = %v", err)
	}
	if !deduped {
		t.Fatalf("CreateRun() duplicate deduped = false, want true")
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate run ID = %q, want %q", second.ID, first.ID)
	}
	if second.KBID != AllKnowledgeBases {
		t.Fatalf("duplicate run KBID = %q, want %q", second.KBID, AllKnowledgeBases)
	}
	runs, err := repo.ListRuns(ctx, RunFilter{TenantID: req.TenantID, KBID: AllKnowledgeBases})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("ListRuns() len = %d, want 1", len(runs))
	}
}

func TestServiceCreateRunDeduplicatesAfterRepositoryConflict(t *testing.T) {
	ctx := context.Background()
	existing := OfflineKnowledgeRun{
		ID:          "run_existing",
		TenantID:    "tenant_1",
		KBID:        "kb_1",
		Status:      RunStatusPending,
		WindowStart: serviceRunRequest().WindowStart,
		WindowEnd:   serviceRunRequest().WindowEnd,
		ConfigHash:  "config_hash_1",
		StartedAt:   fixedServiceNow().Add(-time.Minute),
	}
	repo := &serviceConflictRunRepository{
		MemoryRepository: NewMemoryRepository(),
		hideNextList:     true,
		conflict:         true,
	}
	if err := repo.MemoryRepository.CreateRun(ctx, existing); err != nil {
		t.Fatal(err)
	}
	svc := NewService(repo, ServiceOptions{Now: fixedServiceNow})

	got, deduped, err := svc.CreateRun(ctx, serviceRunRequest())
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if !deduped {
		t.Fatalf("CreateRun() deduped = false, want true")
	}
	if got.ID != existing.ID {
		t.Fatalf("CreateRun() ID = %q, want existing %q", got.ID, existing.ID)
	}
	if repo.createCalls != 1 || repo.listCalls != 2 {
		t.Fatalf("repo calls create=%d list=%d, want create=1 list=2", repo.createCalls, repo.listCalls)
	}
}

func TestServiceRunOnceCreatesVerifiedItem(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	history := &serviceFakeHistorySource{signals: serviceSignals()}
	clusterer := &serviceFakeClusterer{clusters: []QuestionCluster{serviceCluster()}}
	replayer := &serviceFakeRecallReplayer{result: serviceReplayResult()}
	codex := &serviceFakeCodexAnalyzer{response: serviceAnswerResponse()}
	validator := &serviceFakeValidator{}
	svc := NewService(repo, ServiceOptions{
		HistorySource:     history,
		QuestionClusterer: clusterer,
		RecallReplayer:    replayer,
		CodexAnalyzer:     codex,
		Validator:         validator,
		ToolQuota:         testCodexQuota(),
		Now:               fixedServiceNow,
	})

	result, err := svc.RunOnce(ctx, serviceRunRequest())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Run.Status != RunStatusCompleted {
		t.Fatalf("RunOnce() status = %q, want %q", result.Run.Status, RunStatusCompleted)
	}
	if len(result.CreatedItems) != 1 {
		t.Fatalf("RunOnce() created items = %d, want 1", len(result.CreatedItems))
	}
	item := result.CreatedItems[0]
	if item.Status != ItemStatusVerified {
		t.Fatalf("created item status = %q, want %q", item.Status, ItemStatusVerified)
	}
	if validator.calls != 1 {
		t.Fatalf("validator calls = %d, want 1", validator.calls)
	}
	if !clusterer.got.Signals[0].ExplicitNegativeFeedback || !clusterer.got.Signals[1].LongTail {
		t.Fatalf("ClusterQuestions() signals = %#v, want negative feedback and long-tail signals preserved", clusterer.got.Signals)
	}
	if codex.got.BaselineRecallResults[0].ChunkID != "chunk_1" {
		t.Fatalf("AnalyzeCodex() baseline = %#v, want replayed recall", codex.got.BaselineRecallResults)
	}
	if !replayer.got.CreatedAt.Equal(fixedServiceNow()) {
		t.Fatalf("ReplayRecall() cluster CreatedAt = %s, want injected clock %s", replayer.got.CreatedAt, fixedServiceNow())
	}
	items, err := repo.ListOptimizationItems(ctx, OptimizationItemFilter{TenantID: "tenant_1", KBID: "kb_1", Status: ItemStatusVerified})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].SourceFingerprints[0].ChunkContentHash != "sha256:chunk_1" {
		t.Fatalf("stored verified items = %#v", items)
	}
}

func TestServiceRunOnceRecordsRuntimeMetrics(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	metrics := &serviceMetricsSpy{}
	svc := NewService(repo, ServiceOptions{
		HistorySource:     &serviceFakeHistorySource{signals: serviceSignals()},
		QuestionClusterer: &serviceFakeClusterer{clusters: []QuestionCluster{serviceCluster()}},
		RecallReplayer:    &serviceFakeRecallReplayer{result: serviceReplayResult()},
		CodexAnalyzer:     &serviceFakeCodexAnalyzer{response: serviceAnswerResponse()},
		Validator:         &serviceFakeValidator{},
		ToolQuota:         testCodexQuota(),
		Metrics:           metrics,
		Now:               fixedServiceNow,
	})

	result, err := svc.RunOnce(ctx, serviceRunRequest())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Run.Status != RunStatusCompleted {
		t.Fatalf("RunOnce() status = %q, want completed", result.Run.Status)
	}
	if metrics.runs[string(RunStatusPending)] != 1 ||
		metrics.runs[string(RunStatusRunning)] != 1 ||
		metrics.runs[string(RunStatusCompleted)] != 1 {
		t.Fatalf("run metrics = %#v, want pending/running/completed", metrics.runs)
	}
	if metrics.extracted != int64(len(serviceSignals())) || metrics.clusters != 1 {
		t.Fatalf("history metrics extracted=%d clusters=%d", metrics.extracted, metrics.clusters)
	}
	if metrics.replay["success"] != 1 || metrics.codex["success"] != 1 || metrics.validation["success"] != 1 {
		t.Fatalf("path metrics replay=%#v codex=%#v validation=%#v", metrics.replay, metrics.codex, metrics.validation)
	}
	if metrics.statusTotals[string(ItemStatusVerified)] != 1 {
		t.Fatalf("status totals = %#v, want verified=1", metrics.statusTotals)
	}
}

func TestServiceRunOnceStoresRejectedItemWhenValidatorFails(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	validator := &serviceFakeValidator{err: ErrConclusionRejected}
	svc := serviceReadyForRun(repo, serviceAnswerResponse(), validator)

	result, err := svc.RunOnce(ctx, serviceRunRequest())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(result.CreatedItems) != 1 {
		t.Fatalf("created items = %d, want 1", len(result.CreatedItems))
	}
	if result.CreatedItems[0].Status != ItemStatusRejected {
		t.Fatalf("created item status = %q, want %q", result.CreatedItems[0].Status, ItemStatusRejected)
	}
	if validator.calls != 1 {
		t.Fatalf("validator calls = %d, want 1", validator.calls)
	}
}

func TestServiceRunOnceCreatesKnowledgeGapWithoutValidator(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	validator := &serviceFakeValidator{}
	response := serviceAnswerResponse()
	response.ItemType = ItemTypeKnowledgeGap
	response.RecommendedAction = RecommendedActionCreateKnowledgeGapItem
	response.RecallQuality = RecallQualityNoAnswerInKB
	response.FailureType = FailureTypeKnowledgeGap
	response.FinalAnswer = ""
	response.Evidence = nil
	svc := serviceReadyForRun(repo, response, validator)

	result, err := svc.RunOnce(ctx, serviceRunRequest())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(result.CreatedItems) != 1 {
		t.Fatalf("created items = %d, want 1", len(result.CreatedItems))
	}
	if result.CreatedItems[0].Status != ItemStatusKnowledgeGap {
		t.Fatalf("created item status = %q, want %q", result.CreatedItems[0].Status, ItemStatusKnowledgeGap)
	}
	if validator.calls != 0 {
		t.Fatalf("validator calls = %d, want 0", validator.calls)
	}
}

func TestServiceRevalidateItemMovesStaleToVerified(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	item := serviceStoredItem("item_stale", ItemStatusStale, "chunk_1", "sha256:chunk_1")
	if err := repo.CreateOptimizationItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	validator := &serviceFakeValidator{}
	svc := NewService(repo, ServiceOptions{Validator: validator, Now: fixedServiceNow})

	result, err := svc.RevalidateItem(ctx, "tenant_1", "item_stale")
	if err != nil {
		t.Fatalf("RevalidateItem() error = %v", err)
	}
	if !result.Updated || result.NewStatus != ItemStatusVerified {
		t.Fatalf("RevalidateItem() result = %#v, want updated verified", result)
	}
	stored, found, err := repo.GetOptimizationItem(ctx, "tenant_1", "item_stale")
	if err != nil || !found {
		t.Fatalf("GetOptimizationItem() found=%v err=%v", found, err)
	}
	if stored.Status != ItemStatusVerified {
		t.Fatalf("stored item status = %q, want %q", stored.Status, ItemStatusVerified)
	}
}

func TestServiceBulkRevalidateFiltersByStatusAndSourceHash(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	items := []OptimizationItem{
		serviceStoredItem("item_match", ItemStatusStale, "chunk_1", "sha256:match"),
		serviceStoredItem("item_other_hash", ItemStatusStale, "chunk_2", "sha256:other"),
		serviceStoredItem("item_verified", ItemStatusVerified, "chunk_3", "sha256:match"),
	}
	for _, item := range items {
		if err := repo.CreateOptimizationItem(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewService(repo, ServiceOptions{Validator: &serviceFakeValidator{}, Now: fixedServiceNow})

	result, err := svc.BulkRevalidate(ctx, BulkRevalidateRequest{
		TenantID:          "tenant_1",
		KBID:              "kb_1",
		Status:            ItemStatusStale,
		SourceContentHash: "sha256:match",
	})
	if err != nil {
		t.Fatalf("BulkRevalidate() error = %v", err)
	}
	if result.Matched != 1 || result.Updated != 1 {
		t.Fatalf("BulkRevalidate() result = %#v, want one matched and updated", result)
	}
	match, _, _ := repo.GetOptimizationItem(ctx, "tenant_1", "item_match")
	other, _, _ := repo.GetOptimizationItem(ctx, "tenant_1", "item_other_hash")
	verified, _, _ := repo.GetOptimizationItem(ctx, "tenant_1", "item_verified")
	if match.Status != ItemStatusVerified {
		t.Fatalf("matched status = %q, want %q", match.Status, ItemStatusVerified)
	}
	if other.Status != ItemStatusStale {
		t.Fatalf("other hash status = %q, want %q", other.Status, ItemStatusStale)
	}
	if verified.Status != ItemStatusVerified {
		t.Fatalf("verified status = %q, want unchanged %q", verified.Status, ItemStatusVerified)
	}
}

func TestServiceRunRegressionForItemPassesAndStoresReportEvent(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	item := serviceRegressionItem("item_regression_pass", ItemStatusShadowEnabled, ItemTypeAnswer)
	if err := repo.CreateOptimizationItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	runner := &serviceFakeRegressionRunner{result: RegressionResult{
		RecallLift:           0.22,
		AnswerQualityLift:    0.18,
		CitationCoverageLift: 0.12,
		LatencyDelta:         20 * time.Millisecond,
		TokenCostDelta:       0.03,
		HallucinationRisk:    0.02,
		Passed:               true,
	}}
	svc := NewService(repo, ServiceOptions{
		RegressionRunner: runner,
		RegressionLimits: RegressionThresholds{
			MinRecallLift:        0.1,
			MinAnswerQualityLift: 0.1,
			MaxLatencyDelta:      100 * time.Millisecond,
			MaxHallucinationRisk: 0.1,
		},
		Now: fixedServiceNow,
	})

	got, err := svc.RunRegressionForItem(ctx, "tenant_1", item.ID)
	if err != nil {
		t.Fatalf("RunRegressionForItem() error = %v", err)
	}
	if got.Status != ItemStatusRegressionPassed {
		t.Fatalf("RunRegressionForItem() status = %q, want %q", got.Status, ItemStatusRegressionPassed)
	}
	if runner.calls != 1 || runner.got.ItemID != item.ID || runner.got.FullDatasetRequired {
		t.Fatalf("runner request = %#v calls=%d, want item request without full dataset", runner.got, runner.calls)
	}
	report := decodeEvalReport(t, got.EvalReportJSON)
	if !report.Passed || len(report.Reasons) != 0 {
		t.Fatalf("eval report = %#v, want passed without reasons", report)
	}
	if report.ScopedItemID != item.ID || report.Result.ScopedItemID != item.ID {
		t.Fatalf("scoped item metadata report=%q result=%q, want %q", report.ScopedItemID, report.Result.ScopedItemID, item.ID)
	}
	if !report.ProfileNeutrality.SameProfile || !report.ProfileNeutrality.OptimizationLiftOnly {
		t.Fatalf("profile neutrality = %#v, want same profile optimization lift only", report.ProfileNeutrality)
	}
	rawReport := decodeEvalReportMap(t, got.EvalReportJSON)
	result, ok := rawReport["result"].(map[string]any)
	if !ok {
		t.Fatalf("eval report result = %#v, want object", rawReport["result"])
	}
	if rawReport["scoped_item_id"] != item.ID {
		t.Fatalf("eval report scoped_item_id = %#v, want %q", rawReport["scoped_item_id"], item.ID)
	}
	profileNeutrality, ok := rawReport["profile_neutrality"].(map[string]any)
	if !ok || profileNeutrality["same_profile"] != true || profileNeutrality["optimization_lift_only"] != true {
		t.Fatalf("eval report profile_neutrality = %#v, want neutral metadata", rawReport["profile_neutrality"])
	}
	if result["recall_lift"] != 0.22 ||
		result["answer_quality_lift"] != 0.18 ||
		result["citation_coverage_lift"] != 0.12 ||
		result["latency_delta_ms"] != float64(20) ||
		result["token_cost_delta"] != 0.03 ||
		result["hallucination_risk"] != 0.02 {
		t.Fatalf("eval report result = %#v, want persisted regression metrics", result)
	}
	events, err := repo.ListItemEvents(ctx, OptimizationItemEventFilter{TenantID: "tenant_1", ItemID: item.ID, EventType: "regression_evaluated"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Payload["status"] != string(ItemStatusRegressionPassed) || events[0].Payload["passed"] != true {
		t.Fatalf("regression events = %#v, want passed event", events)
	}
}

func TestServiceRunRegressionForItemFailsDueRecallLift(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	item := serviceRegressionItem("item_regression_lift_fail", ItemStatusShadowEnabled, ItemTypeAnswer)
	if err := repo.CreateOptimizationItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	svc := NewService(repo, ServiceOptions{
		RegressionRunner: &serviceFakeRegressionRunner{result: RegressionResult{
			RecallLift: 0.03,
			Passed:     true,
		}},
		RegressionLimits: RegressionThresholds{MinRecallLift: 0.1},
		Now:              fixedServiceNow,
	})

	got, err := svc.RunRegressionForItem(ctx, "tenant_1", item.ID)
	if err != nil {
		t.Fatalf("RunRegressionForItem() error = %v", err)
	}
	if got.Status != ItemStatusRegressionFailed {
		t.Fatalf("RunRegressionForItem() status = %q, want %q", got.Status, ItemStatusRegressionFailed)
	}
	report := decodeEvalReport(t, got.EvalReportJSON)
	if report.Passed || !containsString(report.Reasons, "recall_lift_below_threshold") {
		t.Fatalf("eval report = %#v, want recall lift failure", report)
	}
}

func TestServiceRunRegressionForItemFailsDueLatencyAndRisk(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	item := serviceRegressionItem("item_regression_latency_risk_fail", ItemStatusShadowEnabled, ItemTypeAnswer)
	if err := repo.CreateOptimizationItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	svc := NewService(repo, ServiceOptions{
		RegressionRunner: &serviceFakeRegressionRunner{result: RegressionResult{
			LatencyDelta:      250 * time.Millisecond,
			HallucinationRisk: 0.35,
			Passed:            true,
		}},
		RegressionLimits: RegressionThresholds{
			MaxLatencyDelta:      100 * time.Millisecond,
			MaxHallucinationRisk: 0.2,
		},
		Now: fixedServiceNow,
	})

	got, err := svc.RunRegressionForItem(ctx, "tenant_1", item.ID)
	if err != nil {
		t.Fatalf("RunRegressionForItem() error = %v", err)
	}
	if got.Status != ItemStatusRegressionFailed {
		t.Fatalf("RunRegressionForItem() status = %q, want %q", got.Status, ItemStatusRegressionFailed)
	}
	report := decodeEvalReport(t, got.EvalReportJSON)
	if report.Passed ||
		!containsString(report.Reasons, "latency_delta_above_threshold") ||
		!containsString(report.Reasons, "hallucination_risk_above_threshold") {
		t.Fatalf("eval report = %#v, want latency and hallucination risk failures", report)
	}
}

func TestServiceRunRegressionForQueryRewriteRequiresFullDataset(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	item := serviceRegressionItem("item_regression_rewrite_fail", ItemStatusShadowEnabled, ItemTypeQueryRewrite)
	if err := repo.CreateOptimizationItem(ctx, item); err != nil {
		t.Fatal(err)
	}
	runner := &serviceFakeRegressionRunner{result: RegressionResult{
		RecallLift:      0.5,
		FullDatasetUsed: false,
		Passed:          true,
	}}
	svc := NewService(repo, ServiceOptions{RegressionRunner: runner, Now: fixedServiceNow})

	got, err := svc.RunRegressionForItem(ctx, "tenant_1", item.ID)
	if err != nil {
		t.Fatalf("RunRegressionForItem() error = %v", err)
	}
	if !runner.got.FullDatasetRequired {
		t.Fatalf("runner FullDatasetRequired = false, want true for query rewrite item")
	}
	if got.Status != ItemStatusRegressionFailed {
		t.Fatalf("RunRegressionForItem() status = %q, want %q", got.Status, ItemStatusRegressionFailed)
	}
	report := decodeEvalReport(t, got.EvalReportJSON)
	if report.Passed || !report.FullDatasetRequired || report.FullDatasetUsed || !containsString(report.Reasons, "full_dataset_required") {
		t.Fatalf("eval report = %#v, want full dataset failure", report)
	}
}

func TestServiceRunRegressionForItemRequiresRunner(t *testing.T) {
	svc := NewService(NewMemoryRepository(), ServiceOptions{Now: fixedServiceNow})

	_, err := svc.RunRegressionForItem(context.Background(), "tenant_1", "item_missing_runner")
	if !errors.Is(err, ErrRegressionRunnerRequired) {
		t.Fatalf("RunRegressionForItem() error = %v, want %v", err, ErrRegressionRunnerRequired)
	}
}

func serviceReadyForRun(repo *MemoryRepository, response CodexAnalyzeResponse, validator *serviceFakeValidator) *Service {
	return NewService(repo, ServiceOptions{
		HistorySource:     &serviceFakeHistorySource{signals: serviceSignals()},
		QuestionClusterer: &serviceFakeClusterer{clusters: []QuestionCluster{serviceCluster()}},
		RecallReplayer:    &serviceFakeRecallReplayer{result: serviceReplayResult()},
		CodexAnalyzer:     &serviceFakeCodexAnalyzer{response: response},
		Validator:         validator,
		ToolQuota:         testCodexQuota(),
		Now:               fixedServiceNow,
	})
}

func serviceRunRequest() RunRequest {
	return RunRequest{
		TenantID:    "tenant_1",
		KBID:        "kb_1",
		WindowStart: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		ConfigHash:  "config_hash_1",
		ConfigJSON:  map[string]any{"lookback_days": float64(1)},
	}
}

func fixedServiceNow() time.Time {
	return time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
}

func serviceSignals() []HistorySignal {
	return []HistorySignal{
		{
			TenantID:                 "tenant_1",
			KBID:                     "kb_1",
			Query:                    "What is ORAG?",
			TraceID:                  "trace_negative",
			ExplicitNegativeFeedback: true,
			NegativeFeedbackReason:   "answer did not cite source",
		},
		{
			TenantID:               "tenant_1",
			KBID:                   "kb_1",
			Query:                  "Rare ORAG deployment question",
			TraceID:                "trace_long_tail",
			LongTail:               true,
			LongTailPriority:       1,
			RetrievedChunks:        []string{"chunk_9"},
			Latency:                time.Second,
			NegativeFeedbackReason: "",
		},
	}
}

func serviceCluster() QuestionCluster {
	return QuestionCluster{
		ID:                 "cluster_1",
		TenantID:           "tenant_1",
		KBID:               "kb_1",
		CanonicalQuestion:  "What is ORAG?",
		NormalizedQuestion: "what is orag?",
		QuestionHash:       "question_hash_1",
		OccurrenceCount:    2,
		SampleQuestions:    []string{"What is ORAG?", "Rare ORAG deployment question"},
		TraceIDs:           []string{"trace_negative", "trace_long_tail"},
	}
}

func serviceReplayResult() RecallReplayResult {
	return RecallReplayResult{
		BaselineRecallResults: []BaselineRecallItem{
			{TraceID: "trace_negative", ChunkID: "chunk_1", DocID: "doc_1", Rank: 1, Score: 0.91, Matched: true},
		},
		TraceSummaries: []TraceSummary{
			{TraceID: "trace_negative", Query: "What is ORAG?", RetrievedChunks: []string{"chunk_1"}},
		},
		SourceFingerprints: []SourceFingerprint{
			{DocID: "doc_1", DocVersion: "v1", ChunkID: "chunk_1", ChunkContentHash: "sha256:chunk_1"},
		},
		Metadata: map[string]any{"replay": "offline"},
	}
}

func serviceAnswerResponse() CodexAnalyzeResponse {
	response := validCodexAnalyzeResponse()
	response.Evidence = []Evidence{
		{ChunkID: "chunk_1", DocID: "doc_1", Quote: "ORAG is a retrieval augmented generation framework", Supports: "definition"},
	}
	return response
}

func serviceStoredItem(id string, status ItemStatus, chunkID, hash string) OptimizationItem {
	item := validValidatorItem()
	item.ID = id
	item.Status = status
	item.SourceFingerprints = []SourceFingerprint{
		{DocID: "doc_1", DocVersion: "v1", ChunkID: chunkID, ChunkContentHash: hash},
	}
	item.Evidence = []Evidence{
		{ChunkID: chunkID, DocID: "doc_1", Quote: "ORAG is a retrieval augmented generation framework", Supports: "definition"},
	}
	return item
}

type serviceFakeHistorySource struct {
	signals []HistorySignal
	got     HistoryRequest
	err     error
}

func (s *serviceFakeHistorySource) ExtractHistory(_ context.Context, request HistoryRequest) ([]HistorySignal, error) {
	s.got = request
	return append([]HistorySignal(nil), s.signals...), s.err
}

type serviceFakeClusterer struct {
	clusters []QuestionCluster
	got      ClusterRequest
	err      error
}

func (c *serviceFakeClusterer) ClusterQuestions(_ context.Context, request ClusterRequest) ([]QuestionCluster, error) {
	c.got = request
	return append([]QuestionCluster(nil), c.clusters...), c.err
}

type serviceFakeRecallReplayer struct {
	result RecallReplayResult
	got    QuestionCluster
	err    error
}

func (r *serviceFakeRecallReplayer) ReplayRecall(_ context.Context, cluster QuestionCluster) (RecallReplayResult, error) {
	r.got = cluster
	return r.result, r.err
}

type serviceFakeCodexAnalyzer struct {
	response CodexAnalyzeResponse
	got      CodexAnalyzeRequest
	err      error
}

func (a *serviceFakeCodexAnalyzer) AnalyzeCodex(_ context.Context, request CodexAnalyzeRequest) (CodexAnalyzeResponse, error) {
	a.got = request
	return a.response, a.err
}

type serviceFakeValidator struct {
	calls int
	err   error
}

func (v *serviceFakeValidator) ValidateItem(_ context.Context, _, _ string, _ OptimizationItem) error {
	v.calls++
	if v.err != nil {
		return v.err
	}
	return nil
}

func serviceRegressionItem(id string, status ItemStatus, itemType ItemType) OptimizationItem {
	item := serviceStoredItem(id, status, "chunk_1", "sha256:chunk_1")
	item.ItemType = itemType
	return item
}

func decodeEvalReport(t *testing.T, data []byte) EvalReport {
	t.Helper()
	var report EvalReport
	if len(data) == 0 {
		t.Fatalf("EvalReportJSON is empty")
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal EvalReportJSON: %v", err)
	}
	return report
}

func decodeEvalReportMap(t *testing.T, data []byte) map[string]any {
	t.Helper()
	if len(data) == 0 {
		t.Fatalf("EvalReportJSON is empty")
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal EvalReportJSON map: %v", err)
	}
	return report
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type serviceFakeRegressionRunner struct {
	result RegressionResult
	got    RegressionRequest
	calls  int
	err    error
}

func (r *serviceFakeRegressionRunner) RunRegression(_ context.Context, request RegressionRequest) (RegressionResult, error) {
	r.calls++
	r.got = request
	if r.err != nil {
		return RegressionResult{}, r.err
	}
	return r.result, nil
}

type serviceMetricsSpy struct {
	runs         map[string]int64
	replay       map[string]int64
	codex        map[string]int64
	validation   map[string]int64
	revalidate   map[string]int64
	statusTotals map[string]int64
	risks        map[string]int64
	extracted    int64
	clusters     int64
	recallLift   float64
	answerLift   float64
	citationLift float64
}

func (m *serviceMetricsSpy) ensure() {
	if m.runs == nil {
		m.runs = map[string]int64{}
		m.replay = map[string]int64{}
		m.codex = map[string]int64{}
		m.validation = map[string]int64{}
		m.revalidate = map[string]int64{}
		m.statusTotals = map[string]int64{}
		m.risks = map[string]int64{}
	}
}

func (m *serviceMetricsSpy) ObserveOfflineKnowledgeRun(status string) {
	m.ensure()
	m.runs[status]++
}

func (m *serviceMetricsSpy) AddOfflineKnowledgeExtractedQuestions(count int64) {
	m.extracted += count
}

func (m *serviceMetricsSpy) SetOfflineKnowledgeClusters(count int64) {
	m.clusters = count
}

func (m *serviceMetricsSpy) ObserveOfflineKnowledgeReplay(outcome string, count int64) {
	m.ensure()
	m.replay[outcome] += count
}

func (m *serviceMetricsSpy) ObserveOfflineKnowledgeCodexAnalysis(outcome string, _ int64) {
	m.ensure()
	m.codex[outcome]++
}

func (m *serviceMetricsSpy) ObserveOfflineKnowledgeEvidenceValidation(outcome string, count int64) {
	m.ensure()
	m.validation[outcome] += count
}

func (m *serviceMetricsSpy) IncOptimizationItemStatusTotal(status string) {
	m.ensure()
	m.statusTotals[status]++
}

func (m *serviceMetricsSpy) ObserveOptimizationRevalidate(outcome string, count int64) {
	m.ensure()
	m.revalidate[outcome] += count
}

func (m *serviceMetricsSpy) SetOptimizationQualityLift(recallLift, answerQualityLift, citationCoverageLift float64) {
	m.recallLift = recallLift
	m.answerLift = answerQualityLift
	m.citationLift = citationCoverageLift
}

func (m *serviceMetricsSpy) IncOptimizationHallucinationRisk(reason string) {
	m.ensure()
	m.risks[reason]++
}

type serviceConflictRunRepository struct {
	*MemoryRepository
	hideNextList bool
	conflict     bool
	createCalls  int
	listCalls    int
}

func (r *serviceConflictRunRepository) ListRuns(ctx context.Context, filter RunFilter) ([]OfflineKnowledgeRun, error) {
	r.listCalls++
	if r.hideNextList {
		r.hideNextList = false
		return nil, nil
	}
	return r.MemoryRepository.ListRuns(ctx, filter)
}

func (r *serviceConflictRunRepository) CreateRun(context.Context, OfflineKnowledgeRun) error {
	r.createCalls++
	if r.conflict {
		r.conflict = false
		return ErrRunConflict
	}
	return nil
}
