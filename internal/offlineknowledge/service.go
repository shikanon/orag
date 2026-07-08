package offlineknowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const AllKnowledgeBases = "__all__"

var (
	ErrServiceRepositoryRequired = errors.New("offline knowledge repository is required")
	ErrHistorySourceRequired     = errors.New("offline knowledge history source is required")
	ErrQuestionClustererRequired = errors.New("offline knowledge question clusterer is required")
	ErrRecallReplayerRequired    = errors.New("offline knowledge recall replayer is required")
	ErrCodexAnalyzerRequired     = errors.New("offline knowledge codex analyzer is required")
	ErrValidatorRequired         = errors.New("offline knowledge validator is required")
	ErrRegressionRunnerRequired  = errors.New("offline knowledge regression runner is required")
	ErrValidatorDisabled         = errors.New("offline knowledge validator is disabled")
	ErrRegressionDisabled        = errors.New("offline knowledge regression runner is disabled")
	ErrRegressionUnavailable     = errors.New("offline knowledge regression runner is unavailable")
	ErrRunNotFound               = errors.New("offline knowledge run not found")
	ErrRunExecutionConflict      = errors.New("offline knowledge run cannot be executed from current status")
	ErrOptimizationItemNotFound  = errors.New("offline knowledge optimization item not found")
	ErrInvalidItemTransition     = errors.New("offline knowledge invalid optimization item status transition")
)

type HistorySource interface {
	ExtractHistory(ctx context.Context, request HistoryRequest) ([]HistorySignal, error)
}

type QuestionClusterer interface {
	ClusterQuestions(ctx context.Context, request ClusterRequest) ([]QuestionCluster, error)
}

type RecallReplayer interface {
	ReplayRecall(ctx context.Context, cluster QuestionCluster) (RecallReplayResult, error)
}

type RegressionRunner interface {
	RunRegression(ctx context.Context, request RegressionRequest) (RegressionResult, error)
}

type ItemValidator interface {
	ValidateItem(ctx context.Context, tenantID, kbID string, item OptimizationItem) error
}

type MetricsRecorder interface {
	ObserveOfflineKnowledgeRun(status string)
	AddOfflineKnowledgeExtractedQuestions(count int64)
	SetOfflineKnowledgeClusters(count int64)
	ObserveOfflineKnowledgeReplay(outcome string, count int64)
	ObserveOfflineKnowledgeCodexAnalysis(outcome string, deepSearchSteps int64)
	ObserveOfflineKnowledgeEvidenceValidation(outcome string, count int64)
	IncOptimizationItemStatusTotal(status string)
	ObserveOptimizationRevalidate(outcome string, count int64)
	SetOptimizationQualityLift(recallLift, answerQualityLift, citationCoverageLift float64)
	IncOptimizationHallucinationRisk(reason string)
}

type HistoryRequest struct {
	TenantID string
	KBID     string
	Window   TimeWindow
	Limit    int
}

type TimeWindow struct {
	Start time.Time
	End   time.Time
}

type HistorySignal struct {
	TenantID                 string
	KBID                     string
	Query                    string
	TraceID                  string
	Answer                   string
	RetrievedChunks          []string
	Latency                  time.Duration
	HasError                 bool
	Error                    string
	ExplicitNegativeFeedback bool
	NegativeFeedbackReason   string
	LongTail                 bool
	LongTailPriority         int
	Metadata                 map[string]any
}

type ClusterRequest struct {
	Run     OfflineKnowledgeRun
	Signals []HistorySignal
}

type RecallReplayResult struct {
	BaselineRecallResults []BaselineRecallItem
	TraceSummaries        []TraceSummary
	SourceFingerprints    []SourceFingerprint
	Metadata              map[string]any
}

type RegressionResult struct {
	RecallLift           float64       `json:"recall_lift"`
	AnswerQualityLift    float64       `json:"answer_quality_lift"`
	CitationCoverageLift float64       `json:"citation_coverage_lift"`
	LatencyDelta         time.Duration `json:"-"`
	LatencyDeltaMS       int64         `json:"latency_delta_ms"`
	TokenCostDelta       float64       `json:"token_cost_delta"`
	HallucinationRisk    float64       `json:"hallucination_risk"`
	FullDatasetUsed      bool          `json:"full_dataset_used"`
	Passed               bool          `json:"passed"`
}

type RegressionThresholds struct {
	MinRecallLift           float64       `json:"min_recall_lift,omitempty"`
	MinAnswerQualityLift    float64       `json:"min_answer_quality_lift,omitempty"`
	MinCitationCoverageLift float64       `json:"min_citation_coverage_lift,omitempty"`
	MaxLatencyDelta         time.Duration `json:"max_latency_delta,omitempty"`
	MaxTokenCostDelta       float64       `json:"max_token_cost_delta,omitempty"`
	MaxHallucinationRisk    float64       `json:"max_hallucination_risk,omitempty"`
}

type RegressionRequest struct {
	TenantID            string               `json:"tenant_id"`
	ItemID              string               `json:"item_id"`
	Item                OptimizationItem     `json:"item"`
	Thresholds          RegressionThresholds `json:"thresholds"`
	FullDatasetRequired bool                 `json:"full_dataset_required"`
	RequestedAt         time.Time            `json:"requested_at"`
}

type EvalReport struct {
	Result              RegressionResult     `json:"result"`
	Thresholds          RegressionThresholds `json:"thresholds"`
	Passed              bool                 `json:"passed"`
	Reasons             []string             `json:"reasons,omitempty"`
	FullDatasetRequired bool                 `json:"full_dataset_required"`
	FullDatasetUsed     bool                 `json:"full_dataset_used"`
	EvaluatedAt         time.Time            `json:"evaluated_at"`
}

type ServiceOptions struct {
	HistorySource     HistorySource
	QuestionClusterer QuestionClusterer
	RecallReplayer    RecallReplayer
	SourceReader      SourceReader
	CodexAnalyzer     CodexAnalyzer
	CodexTools        *CodexToolRegistry
	Validator         ItemValidator
	RegressionRunner  RegressionRunner
	ShadowRetriever   *ShadowRetriever
	RegressionLimits  RegressionThresholds
	ToolQuota         ToolQuota
	MaxQuestions      int
	MaxClusters       int
	Metrics           MetricsRecorder
	Now               func() time.Time
}

type Service struct {
	repo       Repository
	history    HistorySource
	clusterer  QuestionClusterer
	replayer   RecallReplayer
	source     SourceReader
	codex      CodexAnalyzer
	codexTools *CodexToolRegistry
	validator  ItemValidator
	regression RegressionRunner
	shadow     *ShadowRetriever
	thresholds RegressionThresholds
	quota      ToolQuota
	maxHistory int
	maxCluster int
	metrics    MetricsRecorder
	now        func() time.Time
}

type RunRequest struct {
	TenantID     string
	KBID         string
	WindowStart  time.Time
	WindowEnd    time.Time
	ConfigHash   string
	ConfigJSON   map[string]any
	MaxQuestions int
	MaxClusters  int
}

type RunResult struct {
	Run              OfflineKnowledgeRun
	Deduplicated     bool
	ProcessedCluster int
	CreatedItems     []OptimizationItem
}

type RevalidateResult struct {
	Item      OptimizationItem `json:"item"`
	Updated   bool             `json:"updated"`
	Skipped   bool             `json:"skipped"`
	OldStatus ItemStatus       `json:"old_status"`
	NewStatus ItemStatus       `json:"new_status"`
	Error     error            `json:"error,omitempty"`
}

type BulkRevalidateRequest struct {
	TenantID          string
	KBID              string
	Status            ItemStatus
	SourceFingerprint SourceFingerprint
	SourceDocID       string
	SourceChunkID     string
	SourceContentHash string
	Limit             int
}

type BulkRevalidateResult struct {
	Matched int                `json:"matched"`
	Updated int                `json:"updated"`
	Skipped int                `json:"skipped"`
	Results []RevalidateResult `json:"results"`
}

func NewService(repo Repository, opts ServiceOptions) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		repo:       repo,
		history:    opts.HistorySource,
		clusterer:  opts.QuestionClusterer,
		replayer:   opts.RecallReplayer,
		source:     opts.SourceReader,
		codex:      opts.CodexAnalyzer,
		codexTools: opts.CodexTools,
		validator:  opts.Validator,
		regression: opts.RegressionRunner,
		shadow:     opts.ShadowRetriever,
		thresholds: opts.RegressionLimits,
		quota:      opts.ToolQuota,
		maxHistory: opts.MaxQuestions,
		maxCluster: opts.MaxClusters,
		metrics:    opts.Metrics,
		now:        now,
	}
}

func (s *Service) CreateRun(ctx context.Context, request RunRequest) (OfflineKnowledgeRun, bool, error) {
	if s.repo == nil {
		return OfflineKnowledgeRun{}, false, ErrServiceRepositoryRequired
	}
	kbID := normalizeKBID(request.KBID)
	configHash, err := resolveConfigHash(request.ConfigHash, request.ConfigJSON)
	if err != nil {
		return OfflineKnowledgeRun{}, false, err
	}
	runs, err := s.repo.ListRuns(ctx, RunFilter{TenantID: request.TenantID, KBID: kbID})
	if err != nil {
		return OfflineKnowledgeRun{}, false, err
	}
	if run, ok := findMatchingRun(runs, request.WindowStart, request.WindowEnd, configHash); ok {
		return run, true, nil
	}

	now := s.now()
	run := OfflineKnowledgeRun{
		ID:          stableID("run", request.TenantID, kbID, request.WindowStart.UTC().Format(time.RFC3339Nano), request.WindowEnd.UTC().Format(time.RFC3339Nano), configHash),
		TenantID:    request.TenantID,
		KBID:        kbID,
		Status:      RunStatusPending,
		WindowStart: request.WindowStart,
		WindowEnd:   request.WindowEnd,
		ConfigHash:  configHash,
		ConfigJSON:  copyMap(request.ConfigJSON),
		StartedAt:   now,
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		if errors.Is(err, ErrRunConflict) {
			runs, listErr := s.repo.ListRuns(ctx, RunFilter{TenantID: request.TenantID, KBID: kbID})
			if listErr != nil {
				return OfflineKnowledgeRun{}, false, listErr
			}
			if existing, ok := findMatchingRun(runs, request.WindowStart, request.WindowEnd, configHash); ok {
				return existing, true, nil
			}
		}
		return OfflineKnowledgeRun{}, false, err
	}
	if s.metrics != nil {
		s.metrics.ObserveOfflineKnowledgeRun(string(run.Status))
	}
	return run, false, nil
}

func (s *Service) GetRun(ctx context.Context, tenantID, runID string) (OfflineKnowledgeRun, bool, error) {
	if s.repo == nil {
		return OfflineKnowledgeRun{}, false, ErrServiceRepositoryRequired
	}
	return s.repo.GetRun(ctx, tenantID, runID)
}

func (s *Service) ListRuns(ctx context.Context, filter RunFilter) ([]OfflineKnowledgeRun, error) {
	if s.repo == nil {
		return nil, ErrServiceRepositoryRequired
	}
	return s.repo.ListRuns(ctx, filter)
}

func (s *Service) ListQuestionClusters(ctx context.Context, filter QuestionClusterFilter) ([]QuestionCluster, error) {
	if s.repo == nil {
		return nil, ErrServiceRepositoryRequired
	}
	return s.repo.ListQuestionClusters(ctx, filter)
}

func (s *Service) GetOptimizationItem(ctx context.Context, tenantID, itemID string) (OptimizationItem, bool, error) {
	if s.repo == nil {
		return OptimizationItem{}, false, ErrServiceRepositoryRequired
	}
	return s.repo.GetOptimizationItem(ctx, tenantID, itemID)
}

func (s *Service) ListOptimizationItems(ctx context.Context, filter OptimizationItemFilter) ([]OptimizationItem, error) {
	if s.repo == nil {
		return nil, ErrServiceRepositoryRequired
	}
	return s.repo.ListOptimizationItems(ctx, filter)
}

func (s *Service) TransitionOptimizationItem(ctx context.Context, tenantID, itemID string, next ItemStatus) (OptimizationItem, error) {
	if s.repo == nil {
		return OptimizationItem{}, ErrServiceRepositoryRequired
	}
	item, found, err := s.repo.GetOptimizationItem(ctx, tenantID, itemID)
	if err != nil {
		return OptimizationItem{}, err
	}
	if !found {
		return OptimizationItem{}, ErrOptimizationItemNotFound
	}
	if item.Status == next {
		return item, nil
	}
	if !CanTransition(item.Status, next) {
		return OptimizationItem{}, ErrInvalidItemTransition
	}
	item.Status = next
	item.UpdatedAt = s.now()
	if next == ItemStatusPublished {
		item.PublishedAt = item.UpdatedAt
	}
	if ok, err := s.repo.UpdateOptimizationItem(ctx, item); err != nil {
		return OptimizationItem{}, err
	} else if !ok {
		return OptimizationItem{}, ErrOptimizationItemNotFound
	}
	if err := s.appendStatusEvent(ctx, item, "status_changed"); err != nil {
		return OptimizationItem{}, err
	}
	if s.metrics != nil {
		s.metrics.IncOptimizationItemStatusTotal(string(item.Status))
	}
	return item, nil
}

func (s *Service) RunOnce(ctx context.Context, request RunRequest) (RunResult, error) {
	if err := s.requireRunDependencies(); err != nil {
		return RunResult{}, err
	}
	run, deduped, err := s.CreateRun(ctx, request)
	if err != nil {
		return RunResult{}, err
	}
	if deduped {
		if s.metrics != nil {
			s.metrics.ObserveOfflineKnowledgeRun("deduplicated")
		}
		return RunResult{Run: run, Deduplicated: true}, nil
	}
	return s.executeRun(ctx, run, request)
}

func (s *Service) ExecuteRun(ctx context.Context, tenantID, runID string) (RunResult, error) {
	if err := s.requireRunDependencies(); err != nil {
		return RunResult{}, err
	}
	run, found, err := s.repo.GetRun(ctx, tenantID, runID)
	if err != nil {
		return RunResult{}, err
	}
	if !found {
		return RunResult{}, ErrRunNotFound
	}
	if run.Status != RunStatusPending && run.Status != RunStatusFailed {
		return RunResult{}, ErrRunExecutionConflict
	}
	return s.executeRun(ctx, run, RunRequest{
		TenantID:     run.TenantID,
		KBID:         run.KBID,
		WindowStart:  run.WindowStart,
		WindowEnd:    run.WindowEnd,
		ConfigHash:   run.ConfigHash,
		ConfigJSON:   copyMap(run.ConfigJSON),
		MaxQuestions: s.maxHistory,
		MaxClusters:  s.maxCluster,
	})
}

func (s *Service) executeRun(ctx context.Context, run OfflineKnowledgeRun, request RunRequest) (RunResult, error) {
	run.Status = RunStatusRunning
	run.Error = ""
	if _, err := s.repo.UpdateRun(ctx, run); err != nil {
		return RunResult{}, err
	}
	if s.metrics != nil {
		s.metrics.ObserveOfflineKnowledgeRun(string(run.Status))
	}
	signals, err := s.history.ExtractHistory(ctx, HistoryRequest{
		TenantID: request.TenantID,
		KBID:     run.KBID,
		Window:   TimeWindow{Start: request.WindowStart, End: request.WindowEnd},
		Limit:    firstPositive(request.MaxQuestions, s.maxHistory),
	})
	if err != nil {
		return s.failRun(ctx, run, err)
	}
	if s.metrics != nil {
		s.metrics.AddOfflineKnowledgeExtractedQuestions(int64(len(signals)))
	}
	clusters, err := s.clusterer.ClusterQuestions(ctx, ClusterRequest{Run: run, Signals: signals})
	if err != nil {
		return s.failRun(ctx, run, err)
	}
	clusters = limitQuestionClusters(clusters, firstPositive(request.MaxClusters, s.maxCluster))
	if s.metrics != nil {
		s.metrics.SetOfflineKnowledgeClusters(int64(len(clusters)))
	}

	result := RunResult{Run: run}
	for _, cluster := range clusters {
		item, created, err := s.ProcessCluster(ctx, run.ID, cluster)
		if err != nil {
			return s.failRun(ctx, run, err)
		}
		result.ProcessedCluster++
		if created {
			result.CreatedItems = append(result.CreatedItems, item)
		}
	}
	run.Status = RunStatusCompleted
	run.FinishedAt = s.now()
	if _, err := s.repo.UpdateRun(ctx, run); err != nil {
		return RunResult{}, err
	}
	if s.metrics != nil {
		s.metrics.ObserveOfflineKnowledgeRun(string(run.Status))
	}
	result.Run = run
	return result, nil
}

func (s *Service) ProcessCluster(ctx context.Context, runID string, cluster QuestionCluster) (OptimizationItem, bool, error) {
	if err := s.requireClusterDependencies(); err != nil {
		return OptimizationItem{}, false, err
	}
	run, found, err := s.repo.GetRun(ctx, cluster.TenantID, runID)
	if err != nil {
		return OptimizationItem{}, false, err
	}
	if !found {
		return OptimizationItem{}, false, ErrRunNotFound
	}
	cluster = normalizeCluster(run, cluster, s.now())
	if err := s.repo.UpsertQuestionCluster(ctx, cluster); err != nil {
		return OptimizationItem{}, false, err
	}
	replay, err := s.replayer.ReplayRecall(ctx, cluster)
	if err != nil {
		if s.metrics != nil {
			s.metrics.ObserveOfflineKnowledgeReplay("error", 1)
		}
		return OptimizationItem{}, false, err
	}
	if s.metrics != nil {
		s.metrics.ObserveOfflineKnowledgeReplay("success", 1)
	}
	response, err := s.codex.AnalyzeCodex(ctx, CodexAnalyzeRequest{
		TenantID:              run.TenantID,
		KBID:                  run.KBID,
		CanonicalQuestion:     cluster.CanonicalQuestion,
		SampleQuestions:       append([]string(nil), cluster.SampleQuestions...),
		BaselineRecallResults: append([]BaselineRecallItem(nil), replay.BaselineRecallResults...),
		TraceSummaries:        append([]TraceSummary(nil), replay.TraceSummaries...),
		Metadata:              copyMap(replay.Metadata),
		Constraints: CodexConstraints{
			ReadOnlyTools:      allowedReadOnlyTools(),
			Quota:              s.quota,
			RequireEvidence:    true,
			MaxDeepSearchSteps: s.quota.MaxDeepSearchSteps,
			AllowedItemTypes:   []ItemType{ItemTypeAnswer, ItemTypeQueryRewrite, ItemTypeKnowledgeGap},
			AllowedActions: []RecommendedAction{
				RecommendedActionCreateAnswerItem,
				RecommendedActionCreateQueryRewriteItem,
				RecommendedActionCreateKnowledgeGapItem,
				RecommendedActionNeedsReview,
				RecommendedActionReject,
			},
		},
	})
	if err != nil {
		if s.metrics != nil {
			s.metrics.ObserveOfflineKnowledgeCodexAnalysis("error", 0)
		}
		return OptimizationItem{}, false, err
	}
	if err := ValidateCodexResponse(response, s.quota); err != nil {
		if s.metrics != nil {
			s.metrics.ObserveOfflineKnowledgeCodexAnalysis("error", int64(len(response.DeepSearchSteps)))
		}
		return OptimizationItem{}, false, err
	}
	if s.metrics != nil {
		s.metrics.ObserveOfflineKnowledgeCodexAnalysis("success", int64(len(response.DeepSearchSteps)))
	}

	item := s.buildItem(run, cluster, response, replay.SourceFingerprints)
	switch response.RecommendedAction {
	case RecommendedActionReject:
		item.Status = ItemStatusRejected
	case RecommendedActionCreateKnowledgeGapItem:
		item.Status = ItemStatusKnowledgeGap
	case RecommendedActionNeedsReview:
		item.Status = ItemStatusNeedsReview
	default:
		if s.validator == nil {
			return OptimizationItem{}, false, ErrValidatorRequired
		}
		item.Status = ItemStatusEvidenceValidating
		if err := s.validator.ValidateItem(ctx, run.TenantID, run.KBID, item); err != nil {
			if s.metrics != nil {
				s.metrics.ObserveOfflineKnowledgeEvidenceValidation("error", 1)
				s.recordValidationRisk(err)
			}
			item.Status = ItemStatusRejected
		} else {
			if s.metrics != nil {
				s.metrics.ObserveOfflineKnowledgeEvidenceValidation("success", 1)
			}
			item.Status = ItemStatusVerified
		}
	}
	item.UpdatedAt = s.now()
	if err := s.repo.CreateOptimizationItem(ctx, item); err != nil {
		return OptimizationItem{}, false, err
	}
	if err := s.appendStatusEvent(ctx, item, "created"); err != nil {
		return OptimizationItem{}, false, err
	}
	if s.metrics != nil {
		s.metrics.IncOptimizationItemStatusTotal(string(item.Status))
	}
	return item, true, nil
}

func (s *Service) RevalidateItem(ctx context.Context, tenantID, itemID string) (RevalidateResult, error) {
	if s.repo == nil {
		return RevalidateResult{}, ErrServiceRepositoryRequired
	}
	if s.validator == nil {
		return RevalidateResult{}, ErrValidatorRequired
	}
	item, found, err := s.repo.GetOptimizationItem(ctx, tenantID, itemID)
	if err != nil {
		return RevalidateResult{}, err
	}
	if !found {
		return RevalidateResult{}, ErrOptimizationItemNotFound
	}
	result := RevalidateResult{Item: item, OldStatus: item.Status, NewStatus: item.Status}
	if item.Status != ItemStatusStale {
		result.Skipped = true
		return result, nil
	}
	if ok, err := s.repo.UpdateOptimizationItemStatus(ctx, tenantID, itemID, ItemStatusEvidenceValidating, s.now()); err != nil {
		return RevalidateResult{}, err
	} else if !ok {
		return RevalidateResult{}, ErrOptimizationItemNotFound
	}

	next := ItemStatusVerified
	validateErr := s.validator.ValidateItem(ctx, item.TenantID, item.KBID, item)
	if validateErr != nil {
		if s.metrics != nil {
			s.metrics.ObserveOfflineKnowledgeEvidenceValidation("error", 1)
			s.recordValidationRisk(validateErr)
		}
		if errors.Is(validateErr, ErrStaleFingerprint) || errors.Is(validateErr, ErrSourceNotFound) {
			next = ItemStatusDeprecated
		} else {
			next = ItemStatusRejected
		}
	} else if s.metrics != nil {
		s.metrics.ObserveOfflineKnowledgeEvidenceValidation("success", 1)
	}
	item.Status = next
	item.UpdatedAt = s.now()
	if ok, err := s.repo.UpdateOptimizationItem(ctx, item); err != nil {
		return RevalidateResult{}, err
	} else if !ok {
		return RevalidateResult{}, ErrOptimizationItemNotFound
	}
	if err := s.appendStatusEvent(ctx, item, "revalidated"); err != nil {
		return RevalidateResult{}, err
	}
	if s.metrics != nil {
		s.metrics.IncOptimizationItemStatusTotal(string(next))
	}
	result.Item = item
	result.Updated = true
	result.NewStatus = next
	result.Error = validateErr
	return result, nil
}

func (s *Service) RunRegressionForItem(ctx context.Context, tenantID, itemID string) (OptimizationItem, error) {
	if s.repo == nil {
		return OptimizationItem{}, ErrServiceRepositoryRequired
	}
	if s.regression == nil {
		return OptimizationItem{}, ErrRegressionRunnerRequired
	}
	item, found, err := s.repo.GetOptimizationItem(ctx, tenantID, itemID)
	if err != nil {
		return OptimizationItem{}, err
	}
	if !found {
		return OptimizationItem{}, ErrOptimizationItemNotFound
	}
	if !canRunRegressionForStatus(item.Status) {
		return OptimizationItem{}, ErrInvalidItemTransition
	}

	request := RegressionRequest{
		TenantID:            tenantID,
		ItemID:              itemID,
		Item:                item,
		Thresholds:          s.thresholds,
		FullDatasetRequired: item.ItemType == ItemTypeQueryRewrite,
		RequestedAt:         s.now(),
	}
	regression, err := s.regression.RunRegression(ctx, request)
	if err != nil {
		return OptimizationItem{}, err
	}
	report := s.evaluateRegression(regression, request)
	next := ItemStatusRegressionFailed
	if report.Passed {
		next = ItemStatusRegressionPassed
	}
	if item.Status != next && !CanTransition(item.Status, next) {
		return OptimizationItem{}, ErrInvalidItemTransition
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return OptimizationItem{}, err
	}
	item.Status = next
	item.UpdatedAt = s.now()
	item.EvalReportJSON = reportJSON
	if ok, err := s.repo.UpdateOptimizationItem(ctx, item); err != nil {
		return OptimizationItem{}, err
	} else if !ok {
		return OptimizationItem{}, ErrOptimizationItemNotFound
	}
	if err := s.appendRegressionEvent(ctx, item, report); err != nil {
		return OptimizationItem{}, err
	}
	if s.metrics != nil {
		s.metrics.SetOptimizationQualityLift(report.Result.RecallLift, report.Result.AnswerQualityLift, report.Result.CitationCoverageLift)
		if report.Result.HallucinationRisk > 0 {
			s.metrics.IncOptimizationHallucinationRisk("evidence_insufficient")
		}
		s.metrics.IncOptimizationItemStatusTotal(string(item.Status))
	}
	return item, nil
}

func (s *Service) BulkRevalidate(ctx context.Context, request BulkRevalidateRequest) (BulkRevalidateResult, error) {
	if s.repo == nil {
		return BulkRevalidateResult{}, ErrServiceRepositoryRequired
	}
	status := request.Status
	if status == "" {
		status = ItemStatusStale
	}
	items, err := s.repo.ListOptimizationItems(ctx, OptimizationItemFilter{
		TenantID: request.TenantID,
		KBID:     normalizeOptionalKBID(request.KBID),
		Status:   status,
		Limit:    request.Limit,
	})
	if err != nil {
		return BulkRevalidateResult{}, err
	}
	out := BulkRevalidateResult{}
	for _, item := range items {
		if !matchesSourceFilter(item, request) {
			continue
		}
		out.Matched++
		result, err := s.RevalidateItem(ctx, item.TenantID, item.ID)
		if err != nil {
			if s.metrics != nil {
				s.metrics.ObserveOptimizationRevalidate("error", 1)
			}
			return out, err
		}
		out.Results = append(out.Results, result)
		if result.Updated {
			out.Updated++
		}
		if result.Skipped {
			out.Skipped++
		}
	}
	if s.metrics != nil {
		s.metrics.ObserveOptimizationRevalidate("success", int64(out.Matched))
	}
	return out, nil
}

func (s *Service) evaluateRegression(result RegressionResult, request RegressionRequest) EvalReport {
	result = normalizeRegressionResult(result)
	report := EvalReport{
		Result:              result,
		Thresholds:          request.Thresholds,
		FullDatasetRequired: request.FullDatasetRequired,
		FullDatasetUsed:     result.FullDatasetUsed,
		EvaluatedAt:         s.now(),
	}
	if !result.Passed {
		report.Reasons = append(report.Reasons, "runner_reported_failed")
	}
	thresholds := request.Thresholds
	if thresholds.MinRecallLift > 0 && result.RecallLift < thresholds.MinRecallLift {
		report.Reasons = append(report.Reasons, "recall_lift_below_threshold")
	}
	if thresholds.MinAnswerQualityLift > 0 && result.AnswerQualityLift < thresholds.MinAnswerQualityLift {
		report.Reasons = append(report.Reasons, "answer_quality_lift_below_threshold")
	}
	if thresholds.MinCitationCoverageLift > 0 && result.CitationCoverageLift < thresholds.MinCitationCoverageLift {
		report.Reasons = append(report.Reasons, "citation_coverage_lift_below_threshold")
	}
	if thresholds.MaxLatencyDelta > 0 && result.LatencyDelta > thresholds.MaxLatencyDelta {
		report.Reasons = append(report.Reasons, "latency_delta_above_threshold")
	}
	if thresholds.MaxTokenCostDelta > 0 && result.TokenCostDelta > thresholds.MaxTokenCostDelta {
		report.Reasons = append(report.Reasons, "token_cost_delta_above_threshold")
	}
	if thresholds.MaxHallucinationRisk > 0 && result.HallucinationRisk > thresholds.MaxHallucinationRisk {
		report.Reasons = append(report.Reasons, "hallucination_risk_above_threshold")
	}
	if request.FullDatasetRequired && !result.FullDatasetUsed {
		report.Reasons = append(report.Reasons, "full_dataset_required")
	}
	report.Passed = len(report.Reasons) == 0
	return report
}

func normalizeRegressionResult(result RegressionResult) RegressionResult {
	if result.LatencyDelta == 0 && result.LatencyDeltaMS != 0 {
		result.LatencyDelta = time.Duration(result.LatencyDeltaMS) * time.Millisecond
	}
	if result.LatencyDeltaMS == 0 && result.LatencyDelta != 0 {
		result.LatencyDeltaMS = result.LatencyDelta.Milliseconds()
	}
	return result
}

func canRunRegressionForStatus(status ItemStatus) bool {
	return status == ItemStatusShadowEnabled || status == ItemStatusRegressionFailed
}

func (s *Service) requireRunDependencies() error {
	if s.history == nil {
		return ErrHistorySourceRequired
	}
	if s.clusterer == nil {
		return ErrQuestionClustererRequired
	}
	return s.requireClusterDependencies()
}

func (s *Service) requireClusterDependencies() error {
	if s.repo == nil {
		return ErrServiceRepositoryRequired
	}
	if s.replayer == nil {
		return ErrRecallReplayerRequired
	}
	if s.codex == nil {
		return ErrCodexAnalyzerRequired
	}
	return nil
}

func (s *Service) failRun(ctx context.Context, run OfflineKnowledgeRun, cause error) (RunResult, error) {
	run.Status = RunStatusFailed
	run.Error = cause.Error()
	run.FinishedAt = s.now()
	_, updateErr := s.repo.UpdateRun(ctx, run)
	if updateErr != nil {
		return RunResult{}, updateErr
	}
	if s.metrics != nil {
		s.metrics.ObserveOfflineKnowledgeRun(string(run.Status))
	}
	return RunResult{Run: run}, cause
}

func (s *Service) recordValidationRisk(err error) {
	if s.metrics == nil || err == nil {
		return
	}
	switch {
	case errors.Is(err, ErrConclusionRejected), errors.Is(err, ErrConclusionDisabled), errors.Is(err, ErrConclusionUnavailable):
		s.metrics.IncOptimizationHallucinationRisk("judge_failed")
	case errors.Is(err, ErrMissingEvidence), errors.Is(err, ErrMissingQuote), errors.Is(err, ErrMissingFingerprint), errors.Is(err, ErrQuoteNotContained):
		s.metrics.IncOptimizationHallucinationRisk("evidence_insufficient")
	default:
	}
}

func (s *Service) buildItem(run OfflineKnowledgeRun, cluster QuestionCluster, response CodexAnalyzeResponse, fingerprints []SourceFingerprint) OptimizationItem {
	now := s.now()
	evidence := append([]Evidence(nil), response.Evidence...)
	return OptimizationItem{
		ID:                 stableID("item", run.TenantID, run.KBID, cluster.ID, string(response.ItemType), response.FinalAnswer),
		TenantID:           run.TenantID,
		RunID:              run.ID,
		KBID:               run.KBID,
		QuestionClusterID:  cluster.ID,
		ItemType:           response.ItemType,
		Status:             ItemStatusCandidate,
		CanonicalQuestion:  cluster.CanonicalQuestion,
		FinalAnswer:        response.FinalAnswer,
		RecallQuality:      response.RecallQuality,
		FailureType:        response.FailureType,
		Confidence:         response.Confidence,
		SourceFingerprints: selectFingerprints(fingerprints, evidence),
		Evidence:           evidence,
		DeepSearchSteps:    append([]DeepSearchStep(nil), response.DeepSearchSteps...),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func (s *Service) appendStatusEvent(ctx context.Context, item OptimizationItem, eventType string) error {
	return s.repo.AppendItemEvent(ctx, OptimizationItemEvent{
		ID:        stableID("event", item.ID, eventType, string(item.Status), item.UpdatedAt.UTC().Format(time.RFC3339Nano)),
		TenantID:  item.TenantID,
		ItemID:    item.ID,
		EventType: eventType,
		Payload:   map[string]any{"status": string(item.Status)},
		CreatedAt: s.now(),
	})
}

func (s *Service) appendRegressionEvent(ctx context.Context, item OptimizationItem, report EvalReport) error {
	payload := map[string]any{
		"status":                string(item.Status),
		"passed":                report.Passed,
		"reasons":               append([]string(nil), report.Reasons...),
		"full_dataset_required": report.FullDatasetRequired,
		"full_dataset_used":     report.FullDatasetUsed,
	}
	return s.repo.AppendItemEvent(ctx, OptimizationItemEvent{
		ID:        stableID("event", item.ID, "regression_evaluated", string(item.Status), item.UpdatedAt.UTC().Format(time.RFC3339Nano)),
		TenantID:  item.TenantID,
		ItemID:    item.ID,
		EventType: "regression_evaluated",
		Payload:   payload,
		CreatedAt: s.now(),
	})
}

func normalizeCluster(run OfflineKnowledgeRun, cluster QuestionCluster, now time.Time) QuestionCluster {
	cluster.TenantID = run.TenantID
	cluster.RunID = run.ID
	cluster.KBID = run.KBID
	if cluster.CanonicalQuestion == "" && len(cluster.SampleQuestions) > 0 {
		cluster.CanonicalQuestion = cluster.SampleQuestions[0]
	}
	if cluster.NormalizedQuestion == "" {
		cluster.NormalizedQuestion = strings.ToLower(strings.TrimSpace(cluster.CanonicalQuestion))
	}
	if cluster.QuestionHash == "" {
		cluster.QuestionHash = shortHash(cluster.NormalizedQuestion)
	}
	if cluster.ID == "" {
		cluster.ID = stableID("cluster", run.TenantID, run.KBID, cluster.QuestionHash)
	}
	if cluster.OccurrenceCount == 0 {
		cluster.OccurrenceCount = len(cluster.SampleQuestions)
	}
	if cluster.CreatedAt.IsZero() {
		cluster.CreatedAt = now
	}
	return cluster
}

func allowedReadOnlyTools() []ReadOnlyToolName {
	return []ReadOnlyToolName{
		ReadOnlyToolSearchChunksByText,
		ReadOnlyToolSearchChunksVector,
		ReadOnlyToolGetChunkNeighbors,
		ReadOnlyToolGetDocumentChunks,
		ReadOnlyToolGetGraphChunks,
		ReadOnlyToolLookupEvalResults,
		ReadOnlyToolLookupExistingItem,
		ReadOnlyToolReplayRecall,
	}
}

func normalizeKBID(kbID string) string {
	if strings.TrimSpace(kbID) == "" {
		return AllKnowledgeBases
	}
	return kbID
}

func normalizeOptionalKBID(kbID string) string {
	if strings.TrimSpace(kbID) == "" {
		return ""
	}
	return normalizeKBID(kbID)
}

func resolveConfigHash(configHash string, configJSON map[string]any) (string, error) {
	if configHash != "" {
		return configHash, nil
	}
	data, err := json.Marshal(configJSON)
	if err != nil {
		return "", err
	}
	return "sha256:" + shortHash(string(data)), nil
}

func stableID(prefix string, parts ...string) string {
	return prefix + "_" + shortHash(strings.Join(parts, "\x00"))
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func selectFingerprints(fingerprints []SourceFingerprint, evidence []Evidence) []SourceFingerprint {
	if len(evidence) == 0 {
		return append([]SourceFingerprint(nil), fingerprints...)
	}
	byChunk := make(map[string]SourceFingerprint, len(fingerprints))
	for _, fingerprint := range fingerprints {
		byChunk[fingerprint.ChunkID] = fingerprint
	}
	out := make([]SourceFingerprint, 0, len(evidence))
	seen := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		fingerprint, ok := byChunk[item.ChunkID]
		if !ok {
			continue
		}
		if _, exists := seen[fingerprint.ChunkID]; exists {
			continue
		}
		out = append(out, fingerprint)
		seen[fingerprint.ChunkID] = struct{}{}
	}
	return out
}

func matchesSourceFilter(item OptimizationItem, request BulkRevalidateRequest) bool {
	filter := request.SourceFingerprint
	if filter.DocID == "" {
		filter.DocID = request.SourceDocID
	}
	if filter.ChunkID == "" {
		filter.ChunkID = request.SourceChunkID
	}
	if filter.ChunkContentHash == "" {
		filter.ChunkContentHash = request.SourceContentHash
	}
	if filter.DocID == "" && filter.ChunkID == "" && filter.DocVersion == "" && filter.ChunkContentHash == "" {
		return true
	}
	for _, fingerprint := range item.SourceFingerprints {
		if filter.DocID != "" && fingerprint.DocID != filter.DocID {
			continue
		}
		if filter.ChunkID != "" && fingerprint.ChunkID != filter.ChunkID {
			continue
		}
		if filter.DocVersion != "" && fingerprint.DocVersion != filter.DocVersion {
			continue
		}
		if filter.ChunkContentHash != "" && fingerprint.ChunkContentHash != filter.ChunkContentHash {
			continue
		}
		return true
	}
	return false
}

func limitQuestionClusters(clusters []QuestionCluster, limit int) []QuestionCluster {
	if limit <= 0 || len(clusters) <= limit {
		return clusters
	}
	return clusters[:limit]
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func findMatchingRun(runs []OfflineKnowledgeRun, windowStart, windowEnd time.Time, configHash string) (OfflineKnowledgeRun, bool) {
	for _, run := range runs {
		if run.WindowStart.Equal(windowStart) &&
			run.WindowEnd.Equal(windowEnd) &&
			run.ConfigHash == configHash {
			return run, true
		}
	}
	return OfflineKnowledgeRun{}, false
}
