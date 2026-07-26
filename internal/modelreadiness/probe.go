package modelreadiness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/audit"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/platform/clock"
	"github.com/shikanon/orag/internal/platform/id"
)

type Capability string

const (
	CapabilityChat         Capability = "chat"
	CapabilityEmbedding    Capability = "embedding"
	CapabilityRerank       Capability = "rerank"
	CapabilityMultimodal   Capability = "multimodal"
	CapabilityProviderAuth Capability = "provider_auth"
)

type ProbeStatus string

const (
	ProbeStatusPass    ProbeStatus = "pass"
	ProbeStatusFail    ProbeStatus = "fail"
	ProbeStatusSkipped ProbeStatus = "skipped"
)

type ErrorCategory string

const (
	ErrorCategoryAuth              ErrorCategory = "auth"
	ErrorCategoryModelNotFound     ErrorCategory = "model_not_found"
	ErrorCategoryQuota             ErrorCategory = "quota"
	ErrorCategoryRateLimit         ErrorCategory = "rate_limit"
	ErrorCategoryNetworkTimeout    ErrorCategory = "network_timeout"
	ErrorCategoryDimensionMismatch ErrorCategory = "dimension_mismatch"
	ErrorCategoryUnknown           ErrorCategory = "unknown"
)

type ProbeResult struct {
	Capability    string
	Status        ProbeStatus
	LatencyMs     int64
	Model         string
	Dimensions    int
	ErrorCategory ErrorCategory
	ErrorMessage  string
	Warnings      []string
}

type ReadinessRun struct {
	ID            string
	TenantID      string
	ActorType     string
	ActorID       string
	TraceID       string
	Provider      string
	StartedAt     time.Time
	CompletedAt   time.Time
	Results       []ProbeResult
	OverallStatus ProbeStatus
}

type ProbeConfig struct {
	TenantID           string
	ActorType          string
	ActorID            string
	TraceID            string
	Provider           string
	ChatModel          string
	EmbeddingModel     string
	ExpectedDimensions int
	RerankModel        string
	MultimodalModel    string
	Timeout            time.Duration
	SkipChat           bool
	SkipEmbedding      bool
	SkipRerank         bool
	SkipMultimodal     bool
	SkipProviderAuth   bool
}

type modelClient interface {
	Chat(ctx context.Context, messages []ark.ChatMessage) (string, error)
	Embed(ctx context.Context, texts []string) ([][]float64, error)
	Rerank(ctx context.Context, query string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error)
	MultimodalParse(ctx context.Context, name string, content []byte) (string, error)
}

type auditService interface {
	Record(ctx context.Context, event audit.AuditEvent) error
}

type ProbeService struct {
	modelClient  modelClient
	auditService auditService
	clock        clock.Clock
	mu           sync.Mutex
	runs         map[string]ReadinessRun
}

func NewProbeService(modelClient modelClient, auditService auditService, timer clock.Clock) *ProbeService {
	if timer == nil {
		timer = clock.RealClock{}
	}
	return &ProbeService{
		modelClient:  modelClient,
		auditService: auditService,
		clock:        timer,
		runs:         map[string]ReadinessRun{},
	}
}

func (s *ProbeService) RunProbes(ctx context.Context, cfg ProbeConfig) (*ReadinessRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	run := &ReadinessRun{
		ID:        id.New("readiness"),
		TenantID:  cfg.TenantID,
		ActorType: cfg.ActorType,
		ActorID:   cfg.ActorID,
		TraceID:   cfg.TraceID,
		Provider:  cfg.Provider,
		StartedAt: s.clock.Now(),
		Results:   make([]ProbeResult, 0),
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]ProbeResult, 0)

	probes := []struct {
		capability Capability
		skip       bool
		probe      func(context.Context, ProbeConfig) ProbeResult
	}{
		{CapabilityChat, cfg.SkipChat, s.probeChat},
		{CapabilityEmbedding, cfg.SkipEmbedding, s.probeEmbedding},
		{CapabilityRerank, cfg.SkipRerank, s.probeRerank},
		{CapabilityMultimodal, cfg.SkipMultimodal, s.probeMultimodal},
		{CapabilityProviderAuth, cfg.SkipProviderAuth, s.probeProviderAuth},
	}

	for _, p := range probes {
		if p.skip {
			results = append(results, ProbeResult{
				Capability: string(p.capability),
				Status:     ProbeStatusSkipped,
			})
			continue
		}
		wg.Add(1)
		go func(cap Capability, probeFn func(context.Context, ProbeConfig) ProbeResult) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			result := probeFn(probeCtx, cfg)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(p.capability, p.probe)
	}

	wg.Wait()

	run.Results = results
	run.CompletedAt = s.clock.Now()
	run.OverallStatus = computeOverallStatus(results)

	s.recordAuditEvent(ctx, run)
	s.runs[run.ID] = *run

	return run, nil
}

// GetRun returns a probe run only when it belongs to the requesting tenant.
func (s *ProbeService) GetRun(_ context.Context, tenantID, runID string) (*ReadinessRun, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || tenantID == "" || run.TenantID != tenantID {
		return nil, false, nil
	}
	copyRun := run
	copyRun.Results = append([]ProbeResult(nil), run.Results...)
	return &copyRun, true, nil
}

func (s *ProbeService) probeChat(ctx context.Context, cfg ProbeConfig) ProbeResult {
	result := ProbeResult{
		Capability: string(CapabilityChat),
		Model:      cfg.ChatModel,
		Warnings:   make([]string, 0),
	}

	start := s.clock.Now()
	resp, err := s.modelClient.Chat(ctx, []ark.ChatMessage{
		{Role: "user", Content: "ping"},
	})
	latency := s.clock.Now().Sub(start).Milliseconds()
	result.LatencyMs = latency

	if err != nil {
		result.Status = ProbeStatusFail
		result.ErrorCategory = classifyError(err)
		result.ErrorMessage = sanitizeErrorMessage(err.Error())
		return result
	}

	if strings.TrimSpace(resp) == "" {
		result.Status = ProbeStatusFail
		result.ErrorCategory = ErrorCategoryUnknown
		result.ErrorMessage = "empty response"
		return result
	}

	result.Status = ProbeStatusPass
	return result
}

func (s *ProbeService) probeEmbedding(ctx context.Context, cfg ProbeConfig) ProbeResult {
	result := ProbeResult{
		Capability: string(CapabilityEmbedding),
		Model:      cfg.EmbeddingModel,
		Warnings:   make([]string, 0),
	}

	start := s.clock.Now()
	embeddings, err := s.modelClient.Embed(ctx, []string{"hello world"})
	latency := s.clock.Now().Sub(start).Milliseconds()
	result.LatencyMs = latency

	if err != nil {
		result.Status = ProbeStatusFail
		result.ErrorCategory = classifyError(err)
		result.ErrorMessage = sanitizeErrorMessage(err.Error())
		return result
	}

	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		result.Status = ProbeStatusFail
		result.ErrorCategory = ErrorCategoryUnknown
		result.ErrorMessage = "empty embedding response"
		return result
	}

	dims := len(embeddings[0])
	result.Dimensions = dims

	if cfg.ExpectedDimensions > 0 && dims != cfg.ExpectedDimensions {
		result.Status = ProbeStatusFail
		result.ErrorCategory = ErrorCategoryDimensionMismatch
		result.ErrorMessage = fmt.Sprintf("expected %d dimensions, got %d", cfg.ExpectedDimensions, dims)
		return result
	}

	result.Status = ProbeStatusPass
	return result
}

func (s *ProbeService) probeRerank(ctx context.Context, cfg ProbeConfig) ProbeResult {
	result := ProbeResult{
		Capability: string(CapabilityRerank),
		Model:      cfg.RerankModel,
		Warnings:   make([]string, 0),
	}

	start := s.clock.Now()
	results, err := s.modelClient.Rerank(ctx, "what is machine learning?", []ark.RerankDocument{
		{ID: "1", Content: "Machine learning is a subset of artificial intelligence."},
		{ID: "2", Content: "The weather today is sunny and warm."},
	}, 2)
	latency := s.clock.Now().Sub(start).Milliseconds()
	result.LatencyMs = latency

	if err != nil {
		result.Status = ProbeStatusFail
		result.ErrorCategory = classifyError(err)
		result.ErrorMessage = sanitizeErrorMessage(err.Error())
		return result
	}

	if len(results) == 0 {
		result.Status = ProbeStatusFail
		result.ErrorCategory = ErrorCategoryUnknown
		result.ErrorMessage = "empty rerank response"
		return result
	}

	result.Status = ProbeStatusPass
	return result
}

func (s *ProbeService) probeMultimodal(ctx context.Context, cfg ProbeConfig) ProbeResult {
	result := ProbeResult{
		Capability: string(CapabilityMultimodal),
		Model:      cfg.MultimodalModel,
		Warnings:   make([]string, 0),
		Status:     ProbeStatusSkipped,
	}

	if cfg.MultimodalModel == "" {
		result.Warnings = append(result.Warnings, "multimodal model not configured")
		return result
	}

	start := s.clock.Now()
	resp, err := s.modelClient.MultimodalParse(ctx, "test.txt", []byte("hello world"))
	latency := s.clock.Now().Sub(start).Milliseconds()
	result.LatencyMs = latency

	if err != nil {
		result.Status = ProbeStatusFail
		result.ErrorCategory = classifyError(err)
		result.ErrorMessage = sanitizeErrorMessage(err.Error())
		return result
	}

	if strings.TrimSpace(resp) == "" {
		result.Status = ProbeStatusFail
		result.ErrorCategory = ErrorCategoryUnknown
		result.ErrorMessage = "empty multimodal response"
		return result
	}

	result.Status = ProbeStatusPass
	return result
}

func (s *ProbeService) probeProviderAuth(ctx context.Context, cfg ProbeConfig) ProbeResult {
	result := ProbeResult{
		Capability: string(CapabilityProviderAuth),
		Warnings:   make([]string, 0),
	}

	start := s.clock.Now()
	_, err := s.modelClient.Chat(ctx, []ark.ChatMessage{
		{Role: "user", Content: "ping"},
	})
	latency := s.clock.Now().Sub(start).Milliseconds()
	result.LatencyMs = latency

	if err != nil {
		result.Status = ProbeStatusFail
		result.ErrorCategory = classifyError(err)
		result.ErrorMessage = sanitizeErrorMessage(err.Error())
		return result
	}

	result.Status = ProbeStatusPass
	return result
}

func classifyError(err error) ErrorCategory {
	if err == nil {
		return ErrorCategoryUnknown
	}

	errStr := strings.ToLower(err.Error())

	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "connection") {
		return ErrorCategoryNetworkTimeout
	}

	if strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "invalid_api_key") ||
		strings.Contains(errStr, "invalid api key") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "auth") {
		return ErrorCategoryAuth
	}

	if strings.Contains(errStr, "model_not_found") ||
		strings.Contains(errStr, "model not found") ||
		strings.Contains(errStr, "no such model") ||
		strings.Contains(errStr, "does not exist") {
		return ErrorCategoryModelNotFound
	}

	if strings.Contains(errStr, "quota_exceeded") ||
		strings.Contains(errStr, "insufficient_quota") ||
		strings.Contains(errStr, "quota exceeded") ||
		strings.Contains(errStr, "insufficient quota") ||
		strings.Contains(errStr, "billing") ||
		strings.Contains(errStr, "credit") {
		return ErrorCategoryQuota
	}

	if strings.Contains(errStr, "rate_limit") ||
		strings.Contains(errStr, "too_many_requests") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "429") {
		return ErrorCategoryRateLimit
	}

	return ErrorCategoryUnknown
}

func sanitizeErrorMessage(msg string) string {
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}

func computeOverallStatus(results []ProbeResult) ProbeStatus {
	hasFail := false
	allSkipped := true

	for _, r := range results {
		if r.Status == ProbeStatusFail {
			hasFail = true
		}
		if r.Status != ProbeStatusSkipped {
			allSkipped = false
		}
	}

	if allSkipped {
		return ProbeStatusSkipped
	}
	if hasFail {
		return ProbeStatusFail
	}
	return ProbeStatusPass
}

func (s *ProbeService) recordAuditEvent(ctx context.Context, run *ReadinessRun) {
	if s.auditService == nil {
		return
	}

	metadata := map[string]string{
		"provider":       run.Provider,
		"overall_status": string(run.OverallStatus),
		"run_id":         run.ID,
	}

	for _, r := range run.Results {
		key := r.Capability + "_status"
		metadata[key] = string(r.Status)
		if r.ErrorCategory != "" {
			metadata[r.Capability+"_error_category"] = string(r.ErrorCategory)
		}
	}

	event := audit.AuditEvent{
		ID:           id.New("audit"),
		TenantID:     run.TenantID,
		TraceID:      run.TraceID,
		Action:       audit.ActionModelReadinessTested,
		ResourceType: audit.ResourceTypeModel,
		ResourceID:   run.Provider,
		Outcome:      outcomeFromStatus(run.OverallStatus),
		Metadata:     metadata,
		CreatedAt:    s.clock.Now(),
		ActorType:    actorTypeOrSystem(run.ActorType),
		ActorID:      run.ActorID,
	}

	_ = s.auditService.Record(ctx, event)
}

func actorTypeOrSystem(value string) string {
	if value == "" {
		return audit.ActorTypeSystem
	}
	return value
}

func outcomeFromStatus(status ProbeStatus) string {
	switch status {
	case ProbeStatusPass:
		return audit.OutcomeSuccess
	case ProbeStatusFail:
		return audit.OutcomeFailure
	default:
		return audit.OutcomeSuccess
	}
}
