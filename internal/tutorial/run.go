package tutorial

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/ingest/chunker"
	"github.com/shikanon/orag/internal/platform/id"
	"github.com/shikanon/orag/internal/rag"
)

var (
	ErrExperimentRunNotFound  = errors.New("tutorial experiment run not found")
	ErrExperimentRunKey       = errors.New("tutorial experiment run idempotency key is required")
	ErrRuntimeUnavailable     = errors.New("tutorial Pack has no runnable runtime declaration")
	ErrPackNotInstalled       = errors.New("tutorial Pack is not installed")
	ErrExperimentRunCancelled = errors.New("tutorial experiment run is cancelled")
	ErrExperimentRunVariant   = errors.New("tutorial experiment run variant is invalid")
	ErrBaselineRequired       = errors.New("tutorial candidate requires a compatible completed baseline")
)

type ExperimentRunStage string

const (
	ExperimentRunStageIndex    ExperimentRunStage = "index_private_pack"
	ExperimentRunStageEvaluate ExperimentRunStage = "run_evaluation"
	ExperimentRunStageComplete ExperimentRunStage = "completed"
)

type ExperimentRunStatus string

const (
	ExperimentRunQueued          ExperimentRunStatus = "queued"
	ExperimentRunRunning         ExperimentRunStatus = "running"
	ExperimentRunCancelRequested ExperimentRunStatus = "cancel_requested"
	ExperimentRunCancelled       ExperimentRunStatus = "cancelled"
	ExperimentRunFailed          ExperimentRunStatus = "failed"
	ExperimentRunCompleted       ExperimentRunStatus = "completed"
)

type ExperimentRun struct {
	ID                         string               `json:"id"`
	TenantID                   string               `json:"tenant_id"`
	ProjectID                  string               `json:"project_id"`
	ExperimentID               string               `json:"experiment_id"`
	Variant                    string               `json:"variant"`
	BaselineRunID              string               `json:"baseline_run_id,omitempty"`
	ComparisonFingerprint      string               `json:"comparison_fingerprint,omitempty"`
	DefinitionFingerprint      string               `json:"definition_fingerprint,omitempty"`
	PackManifestSHA256         string               `json:"pack_manifest_sha256,omitempty"`
	RuntimeEnvironmentSHA256   string               `json:"runtime_environment_sha256,omitempty"`
	BuildRevision              string               `json:"build_revision,omitempty"`
	KnowledgeBaseID            string               `json:"knowledge_base_id,omitempty"`
	DatasetID                  string               `json:"dataset_id,omitempty"`
	Profile                    string               `json:"profile,omitempty"`
	TopK                       int                  `json:"top_k,omitempty"`
	ParserMethod               string               `json:"parser_method,omitempty"`
	ChunkSizeTokens            int                  `json:"chunk_size_tokens,omitempty"`
	ChunkOverlapTokens         int                  `json:"chunk_overlap_tokens,omitempty"`
	ContextualRetrievalEnabled bool                 `json:"contextual_retrieval_enabled"`
	RetrievalStrategy          string               `json:"retrieval_strategy"`
	ReusedBaselineIndex        bool                 `json:"reused_baseline_index"`
	QueryExpansionMode         string               `json:"query_expansion_mode"`
	MultiQueryCount            int                  `json:"multi_query_count"`
	RerankEnabled              bool                 `json:"rerank_enabled"`
	GraphRetrievalEnabled      bool                 `json:"graph_retrieval_enabled"`
	ContextPackTopN            int                  `json:"context_pack_top_n"`
	ContextPackMaxTokens       int                  `json:"context_pack_max_tokens"`
	IndexedChunkCount          int                  `json:"indexed_chunk_count,omitempty"`
	AverageChunkTokens         float64              `json:"average_chunk_tokens,omitempty"`
	ContextualizedChunkCount   int                  `json:"contextualized_chunk_count,omitempty"`
	AverageContextTokens       float64              `json:"average_context_tokens,omitempty"`
	Stage                      ExperimentRunStage   `json:"stage"`
	Status                     ExperimentRunStatus  `json:"status"`
	EvaluationRunID            string               `json:"evaluation_run_id,omitempty"`
	FailureCode                string               `json:"failure_code,omitempty"`
	Events                     []ExperimentRunEvent `json:"events"`
	CreatedAt                  time.Time            `json:"created_at"`
	UpdatedAt                  time.Time            `json:"updated_at"`
}

type ExperimentRunEvent struct {
	Stage      ExperimentRunStage `json:"stage"`
	Outcome    string             `json:"outcome"`
	DetailCode string             `json:"detail_code,omitempty"`
	OccurredAt time.Time          `json:"occurred_at"`
}

type ExperimentRunRepository interface {
	CreateOrGetRun(context.Context, ExperimentRun, string) (ExperimentRun, bool, error)
	GetExperimentRun(context.Context, string, string) (ExperimentRun, bool, error)
	FindCompletedBaseline(context.Context, string, string, string, string) (ExperimentRun, bool, error)
	AcquireExperimentRun(context.Context, string, string, time.Time) (ExperimentRun, bool, error)
	AdvanceExperimentRun(context.Context, string, string, ExperimentRunStage, ExperimentRunStage, time.Time) (ExperimentRun, bool, error)
	RecordExperimentRunIndexStats(context.Context, string, string, ExperimentRunIndexStats, time.Time) (ExperimentRun, bool, error)
	CompleteExperimentRun(context.Context, string, string, string, time.Time) (ExperimentRun, bool, error)
	FailExperimentRun(context.Context, string, string, ExperimentRunStage, string, time.Time) (ExperimentRun, bool, error)
	CancelExperimentRun(context.Context, string, string, time.Time) (ExperimentRun, bool, error)
	MarkExperimentRunCancelled(context.Context, string, string, time.Time) (ExperimentRun, bool, error)
	RecoverExperimentRuns(context.Context, time.Time) ([]ExperimentRun, error)
}

type RuntimeIngestor interface {
	Ingest(context.Context, ingest.Request) (ingest.Result, error)
}

type RuntimeEvaluator interface {
	Run(context.Context, eval.RunRequest) (eval.RunResult, error)
}

type LiveRunService struct {
	repo                ExperimentRunRepository
	experiments         CloneRepository
	ingest              RuntimeIngestor
	evaluator           RuntimeEvaluator
	private             PrivateStore
	candidateIngestors  map[string]RuntimeIngestor
	candidateEvaluators map[string]RuntimeEvaluator
	runtimeEnvironment  RuntimeEnvironment
	now                 func() time.Time
	newID               func(string) string
}

func NewLiveRunService(repo ExperimentRunRepository, experiments CloneRepository, now func() time.Time) *LiveRunService {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &LiveRunService{repo: repo, experiments: experiments, now: now, newID: id.New}
}

func (s *LiveRunService) Configure(ingestor RuntimeIngestor, evaluator RuntimeEvaluator, private PrivateStore) {
	if s == nil {
		return
	}
	s.ingest = ingestor
	s.evaluator = evaluator
	s.private = private
}

// ConfigureCandidateIngestors installs only app-owned variant ingestors and a
// redacted runtime description used to prevent comparisons across model or
// evaluator changes. The browser cannot influence either value.
func (s *LiveRunService) ConfigureCandidateIngestors(environment RuntimeEnvironment, ingestors map[string]RuntimeIngestor) {
	if s == nil {
		return
	}
	s.runtimeEnvironment = environment
	s.candidateIngestors = make(map[string]RuntimeIngestor, len(ingestors))
	for candidateID, ingestor := range ingestors {
		s.candidateIngestors[candidateID] = ingestor
	}
}

// ConfigureCandidateEvaluators installs retrievers whose behavior is scoped to
// a tutorial candidate. Only the application may select these evaluators.
func (s *LiveRunService) ConfigureCandidateEvaluators(evaluators map[string]RuntimeEvaluator) {
	if s == nil {
		return
	}
	s.candidateEvaluators = make(map[string]RuntimeEvaluator, len(evaluators))
	for candidateID, evaluator := range evaluators {
		s.candidateEvaluators[candidateID] = evaluator
	}
}

func (s *LiveRunService) Start(ctx context.Context, subject Subject, projectID, idempotencyKey string) (ExperimentRun, bool, error) {
	return s.StartVariant(ctx, subject, projectID, "baseline", idempotencyKey)
}

func (s *LiveRunService) StartVariant(ctx context.Context, subject Subject, projectID, variant, idempotencyKey string) (ExperimentRun, bool, error) {
	if s == nil || s.repo == nil || s.experiments == nil {
		return ExperimentRun{}, false, errors.New("tutorial live run service is unavailable")
	}
	subject = normalizeSubject(subject)
	projectID, variant, idempotencyKey = strings.TrimSpace(projectID), strings.TrimSpace(variant), strings.TrimSpace(idempotencyKey)
	if variant == "" {
		variant = "baseline"
	}
	if subject.TenantID == "" || projectID == "" || idempotencyKey == "" || len(idempotencyKey) > 200 {
		return ExperimentRun{}, false, ErrExperimentRunKey
	}
	experiment, found, err := s.experiments.GetExperiment(ctx, subject.TenantID, projectID)
	if err != nil {
		return ExperimentRun{}, false, err
	}
	if !found {
		return ExperimentRun{}, false, ErrCloneExperimentAbsent
	}
	if experiment.PackStatus != PackStatusInstalled {
		return ExperimentRun{}, false, ErrPackNotInstalled
	}
	definition, err := s.runtimeDefinition(experiment, variant)
	if err != nil {
		return ExperimentRun{}, false, err
	}
	baselineRunID := ""
	var baseline ExperimentRun
	if variant != "baseline" {
		var found bool
		baseline, found, err = s.repo.FindCompletedBaseline(ctx, subject.TenantID, projectID, experiment.ID, definition.comparisonFingerprint)
		if err != nil {
			return ExperimentRun{}, false, err
		}
		if !found {
			return ExperimentRun{}, false, ErrBaselineRequired
		}
		baselineRunID = baseline.ID
		if definition.reuseBaselineIndex && baseline.KnowledgeBaseID != definition.knowledgeBaseID {
			return ExperimentRun{}, false, ErrBaselineRequired
		}
	}
	now := s.now().UTC()
	stage := ExperimentRunStageIndex
	events := []ExperimentRunEvent{{Stage: ExperimentRunStageIndex, Outcome: "queued", OccurredAt: now}}
	if definition.reuseBaselineIndex {
		stage = ExperimentRunStageEvaluate
		events = []ExperimentRunEvent{{Stage: ExperimentRunStageEvaluate, Outcome: "queued", OccurredAt: now}}
	}
	run := ExperimentRun{
		ID: s.newID("terun"), TenantID: subject.TenantID, ProjectID: projectID, ExperimentID: experiment.ID,
		Variant: variant, BaselineRunID: baselineRunID, ComparisonFingerprint: definition.comparisonFingerprint,
		DefinitionFingerprint: definition.definitionFingerprint, KnowledgeBaseID: definition.knowledgeBaseID,
		PackManifestSHA256: definition.packManifestSHA256, RuntimeEnvironmentSHA256: definition.runtimeEnvironmentSHA256, BuildRevision: definition.buildRevision,
		DatasetID: definition.datasetID, Profile: definition.profile, TopK: definition.topK, ParserMethod: definition.parserMethod,
		ChunkSizeTokens: definition.chunkSizeTokens, ChunkOverlapTokens: definition.chunkOverlapTokens,
		ContextualRetrievalEnabled: definition.contextualRetrievalEnabled,
		RetrievalStrategy:          definition.retrievalStrategy,
		ReusedBaselineIndex:        definition.reuseBaselineIndex,
		QueryExpansionMode:         definition.queryExpansionMode,
		MultiQueryCount:            definition.multiQueryCount,
		RerankEnabled:              definition.rerankEnabled,
		GraphRetrievalEnabled:      definition.graphRetrievalEnabled,
		ContextPackTopN:            definition.contextPackTopN,
		ContextPackMaxTokens:       definition.contextPackMaxTokens,
		IndexedChunkCount:          baseline.IndexedChunkCount,
		AverageChunkTokens:         baseline.AverageChunkTokens,
		ContextualizedChunkCount:   baseline.ContextualizedChunkCount,
		AverageContextTokens:       baseline.AverageContextTokens,
		Stage:                      stage, Status: ExperimentRunQueued,
		Events:    events,
		CreatedAt: now, UpdatedAt: now,
	}
	return s.repo.CreateOrGetRun(ctx, run, idempotencyKey)
}

func (s *LiveRunService) Get(ctx context.Context, subject Subject, runID string) (ExperimentRun, error) {
	if s == nil || s.repo == nil {
		return ExperimentRun{}, ErrExperimentRunNotFound
	}
	subject = normalizeSubject(subject)
	run, found, err := s.repo.GetExperimentRun(ctx, subject.TenantID, strings.TrimSpace(runID))
	if err != nil {
		return ExperimentRun{}, err
	}
	if !found {
		return ExperimentRun{}, ErrExperimentRunNotFound
	}
	return run, nil
}

func (s *LiveRunService) Cancel(ctx context.Context, subject Subject, runID string) (ExperimentRun, error) {
	if s == nil || s.repo == nil {
		return ExperimentRun{}, ErrExperimentRunNotFound
	}
	subject = normalizeSubject(subject)
	run, changed, err := s.repo.CancelExperimentRun(ctx, subject.TenantID, strings.TrimSpace(runID), s.now().UTC())
	if err != nil {
		return ExperimentRun{}, err
	}
	if !changed {
		return ExperimentRun{}, ErrExperimentRunNotFound
	}
	return run, nil
}

func (s *LiveRunService) RecoverPending(ctx context.Context) ([]ExperimentRun, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("tutorial live run service is unavailable")
	}
	return s.repo.RecoverExperimentRuns(ctx, s.now().UTC())
}

// Execute performs only server-derived baseline work. It never receives a
// resource ID, profile, source URL, or object-storage coordinate from a client.
func (s *LiveRunService) Execute(ctx context.Context, tenantID, runID string) error {
	if s == nil || s.repo == nil || s.experiments == nil || s.evaluator == nil || s.private == nil {
		return errors.New("tutorial live run service is not configured")
	}
	for {
		run, acquired, err := s.repo.AcquireExperimentRun(ctx, tenantID, runID, s.now().UTC())
		if err != nil || !acquired {
			return err
		}
		experiment, found, err := s.experiments.GetExperiment(ctx, tenantID, run.ProjectID)
		if err != nil || !found {
			if err == nil {
				err = ErrCloneExperimentAbsent
			}
			return s.fail(ctx, run, err)
		}
		definition, err := s.runtimeDefinition(experiment, run.Variant)
		if err != nil {
			return s.fail(ctx, run, err)
		}
		if !definition.matches(run) && !run.isLegacyBaseline() {
			return s.fail(ctx, run, ErrRuntimeUnavailable)
		}
		switch run.Stage {
		case ExperimentRunStageIndex:
			ingestor, err := s.ingestorFor(run, definition)
			if err != nil {
				return s.fail(ctx, run, err)
			}
			stats, err := s.index(ctx, experiment, run, ingestor)
			if err != nil {
				return s.fail(ctx, run, err)
			}
			if _, recorded, err := s.repo.RecordExperimentRunIndexStats(ctx, tenantID, run.ID, stats.persisted(), s.now().UTC()); err != nil || !recorded {
				if err != nil {
					return s.fail(ctx, run, err)
				}
				return s.finishCancellationIfRequested(ctx, run)
			}
			if _, advanced, err := s.repo.AdvanceExperimentRun(ctx, tenantID, run.ID, ExperimentRunStageIndex, ExperimentRunStageEvaluate, s.now().UTC()); err != nil || !advanced {
				if err != nil {
					return err
				}
				return s.finishCancellationIfRequested(ctx, run)
			}
		case ExperimentRunStageEvaluate:
			evaluator := s.evaluator
			if candidateEvaluator := s.candidateEvaluators[run.Variant]; candidateEvaluator != nil {
				evaluator = candidateEvaluator
			}
			if evaluator == nil {
				return s.fail(ctx, run, ErrRuntimeUnavailable)
			}
			result, err := evaluator.Run(ctx, eval.RunRequest{
				TenantID: tenantID, ProjectID: experiment.ProjectID, DatasetID: run.DatasetID,
				KnowledgeBaseID: run.KnowledgeBaseID, Profile: rag.Profile(run.Profile), TopK: run.TopK,
			})
			if err != nil {
				return s.fail(ctx, run, err)
			}
			if _, completed, err := s.repo.CompleteExperimentRun(ctx, tenantID, run.ID, result.ID, s.now().UTC()); err != nil || !completed {
				if err != nil {
					return err
				}
				return s.finishCancellationIfRequested(ctx, run)
			}
			return nil
		default:
			return s.fail(ctx, run, fmt.Errorf("unsupported tutorial experiment run stage %q", run.Stage))
		}
	}
}

type indexStats struct {
	chunkCount               int
	tokenCount               int
	contextualizedChunkCount int
	contextTokenCount        int
}

// ExperimentRunIndexStats is the atomically persisted measurement from an
// actual indexing pass. These values are never supplied by an API client.
type ExperimentRunIndexStats struct {
	ChunkCount               int
	AverageChunkTokens       float64
	ContextualizedChunkCount int
	AverageContextTokens     float64
}

func (s indexStats) averageChunkTokens() float64 {
	if s.chunkCount == 0 {
		return 0
	}
	return float64(s.tokenCount) / float64(s.chunkCount)
}

func (s indexStats) averageContextTokens() float64 {
	if s.contextualizedChunkCount == 0 {
		return 0
	}
	return float64(s.contextTokenCount) / float64(s.contextualizedChunkCount)
}

func (s indexStats) persisted() ExperimentRunIndexStats {
	return ExperimentRunIndexStats{
		ChunkCount: s.chunkCount, AverageChunkTokens: s.averageChunkTokens(),
		ContextualizedChunkCount: s.contextualizedChunkCount, AverageContextTokens: s.averageContextTokens(),
	}
}

func (s *LiveRunService) index(ctx context.Context, experiment Experiment, run ExperimentRun, ingestor RuntimeIngestor) (indexStats, error) {
	stats := indexStats{}
	for _, document := range experiment.PackManifest.Runtime.Documents {
		object, found := packObject(experiment.PackManifest, document.ObjectPath)
		if !found {
			return indexStats{}, ErrRuntimeUnavailable
		}
		content, err := s.private.ReadVerified(ctx, PrivateObject{
			TenantID: experiment.TenantID, ProjectID: experiment.ProjectID, JobID: experiment.CloneJobID,
			Object: VerifiedObject{PackObject: object},
		})
		if err != nil {
			return indexStats{}, err
		}
		result, err := ingestor.Ingest(ctx, ingest.Request{
			TenantID: experiment.TenantID, KnowledgeBaseID: run.KnowledgeBaseID,
			SourceURI: "tutorial://" + experiment.TemplateID + "/" + experiment.TemplateVersion + "/" + object.Path,
			Name:      document.Name, Content: content,
		})
		if err != nil {
			return indexStats{}, err
		}
		for _, chunk := range result.Chunks {
			stats.chunkCount++
			stats.tokenCount += chunker.TokenCount(chunk.Content)
			if contextualText := strings.TrimSpace(chunk.ContextualText); contextualText != "" {
				stats.contextualizedChunkCount++
				stats.contextTokenCount += chunker.TokenCount(contextualText)
			}
		}
	}
	return stats, nil
}

func (s *LiveRunService) ingestorFor(run ExperimentRun, definition runtimeDefinition) (RuntimeIngestor, error) {
	if run.ParserMethod != definition.parserMethod || run.ChunkSizeTokens != definition.chunkSizeTokens || run.ChunkOverlapTokens != definition.chunkOverlapTokens {
		return nil, ErrRuntimeUnavailable
	}
	if run.Variant == "baseline" && s.ingest != nil {
		return s.ingest, nil
	}
	if ingestor := s.candidateIngestors[run.Variant]; ingestor != nil {
		return ingestor, nil
	}
	return nil, ErrRuntimeUnavailable
}

func (s *LiveRunService) fail(ctx context.Context, run ExperimentRun, cause error) error {
	_, transitioned, transitionErr := s.repo.FailExperimentRun(ctx, run.TenantID, run.ID, run.Stage, experimentRunFailureCode(cause), s.now().UTC())
	if transitionErr != nil {
		return transitionErr
	}
	if !transitioned {
		return s.finishCancellationIfRequested(ctx, run)
	}
	return cause
}

func (s *LiveRunService) finishCancellationIfRequested(ctx context.Context, run ExperimentRun) error {
	_, cancelled, err := s.repo.MarkExperimentRunCancelled(ctx, run.TenantID, run.ID, s.now().UTC())
	if err != nil {
		return err
	}
	if cancelled {
		return ErrExperimentRunCancelled
	}
	return nil
}

func packObject(manifest Manifest, path string) (PackObject, bool) {
	for _, object := range manifest.Objects {
		if object.Path == path {
			return object, true
		}
	}
	for _, object := range manifest.VisualAssets {
		if object.Path == path {
			return object, true
		}
	}
	for _, object := range manifest.TemporalAssets {
		if object.Path == path {
			return object, true
		}
	}
	return PackObject{}, false
}

func experimentRunFailureCode(err error) string {
	switch {
	case errors.Is(err, ErrRuntimeUnavailable):
		return "runtime_unavailable"
	case errors.Is(err, ErrBaselineRequired):
		return "baseline_required"
	case errors.Is(err, ErrPrivateStoreRead):
		return "private_pack_unavailable"
	case errors.Is(err, ErrExperimentRunCancelled):
		return "cancelled"
	default:
		return "live_run_failed"
	}
}
