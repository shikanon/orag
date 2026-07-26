package modelreadiness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/audit"
	"github.com/shikanon/orag/internal/llm/ark"
	"github.com/shikanon/orag/internal/platform/clock"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type mockModelClient struct {
	mu              sync.Mutex
	chatFn          func(ctx context.Context, messages []ark.ChatMessage) (string, error)
	embedFn         func(ctx context.Context, texts []string) ([][]float64, error)
	rerankFn        func(ctx context.Context, query string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error)
	multimodalFn    func(ctx context.Context, name string, content []byte) (string, error)
	chatCalls       int
	embedCalls      int
	rerankCalls     int
	multimodalCalls int
	lastChatInput   []ark.ChatMessage
	lastEmbedInput  []string
}

func (m *mockModelClient) Chat(ctx context.Context, messages []ark.ChatMessage) (string, error) {
	m.mu.Lock()
	m.chatCalls++
	m.lastChatInput = messages
	m.mu.Unlock()
	if m.chatFn != nil {
		return m.chatFn(ctx, messages)
	}
	return "pong", nil
}

func (m *mockModelClient) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	m.mu.Lock()
	m.embedCalls++
	m.lastEmbedInput = texts
	m.mu.Unlock()
	if m.embedFn != nil {
		return m.embedFn(ctx, texts)
	}
	return [][]float64{make([]float64, 1024)}, nil
}

func (m *mockModelClient) Rerank(ctx context.Context, query string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
	m.mu.Lock()
	m.rerankCalls++
	m.mu.Unlock()
	if m.rerankFn != nil {
		return m.rerankFn(ctx, query, docs, topN)
	}
	results := make([]ark.RerankResult, len(docs))
	for i := range docs {
		results[i] = ark.RerankResult{Index: i, Score: 1.0 - float64(i)*0.1}
	}
	return results, nil
}

func (m *mockModelClient) MultimodalParse(ctx context.Context, name string, content []byte) (string, error) {
	m.mu.Lock()
	m.multimodalCalls++
	m.mu.Unlock()
	if m.multimodalFn != nil {
		return m.multimodalFn(ctx, name, content)
	}
	return "parsed content", nil
}

type mockAuditService struct {
	mu     sync.Mutex
	events []audit.AuditEvent
}

func (m *mockAuditService) Record(ctx context.Context, event audit.AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockAuditService) GetEvents() []audit.AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]audit.AuditEvent, len(m.events))
	copy(result, m.events)
	return result
}

func TestNewProbeService(t *testing.T) {
	mockClient := &mockModelClient{}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()

	svc := NewProbeService(mockClient, mockAudit, fc)
	if svc == nil {
		t.Fatal("expected non-nil ProbeService")
	}
	if svc.modelClient != mockClient {
		t.Error("expected modelClient to be set")
	}
	if svc.auditService != mockAudit {
		t.Error("expected auditService to be set")
	}
	if svc.clock != fc {
		t.Error("expected clock to be set")
	}
}

func TestProbeChat_Success(t *testing.T) {
	mockClient := &mockModelClient{
		chatFn: func(ctx context.Context, messages []ark.ChatMessage) (string, error) {
			return "pong", nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:      "test-provider",
		ChatModel:     "gpt-test",
		SkipEmbedding: true,
		SkipRerank:    true,
		SkipMultimodal: true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	if run == nil {
		t.Fatal("expected non-nil ReadinessRun")
	}
	if run.Provider != "test-provider" {
		t.Errorf("expected provider 'test-provider', got '%s'", run.Provider)
	}

	var chatResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityChat) {
			chatResult = &run.Results[i]
			break
		}
	}

	if chatResult == nil {
		t.Fatal("expected chat result")
	}
	if chatResult.Status != ProbeStatusPass {
		t.Errorf("expected status 'pass', got '%s'", chatResult.Status)
	}
	if chatResult.Model != "gpt-test" {
		t.Errorf("expected model 'gpt-test', got '%s'", chatResult.Model)
	}
	if chatResult.LatencyMs < 0 {
		t.Error("expected non-negative latency")
	}
	if chatResult.ErrorCategory != "" {
		t.Errorf("expected empty error category, got '%s'", chatResult.ErrorCategory)
	}

	if mockClient.chatCalls != 1 {
		t.Errorf("expected 1 chat call, got %d", mockClient.chatCalls)
	}
}

func TestProbeChat_Failure(t *testing.T) {
	mockClient := &mockModelClient{
		chatFn: func(ctx context.Context, messages []ark.ChatMessage) (string, error) {
			return "", errors.New("invalid_api_key: authentication failed")
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		ChatModel:        "gpt-test",
		SkipEmbedding:    true,
		SkipRerank:       true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var chatResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityChat) {
			chatResult = &run.Results[i]
			break
		}
	}

	if chatResult == nil {
		t.Fatal("expected chat result")
	}
	if chatResult.Status != ProbeStatusFail {
		t.Errorf("expected status 'fail', got '%s'", chatResult.Status)
	}
	if chatResult.ErrorCategory != ErrorCategoryAuth {
		t.Errorf("expected error category 'auth', got '%s'", chatResult.ErrorCategory)
	}
	if chatResult.ErrorMessage == "" {
		t.Error("expected non-empty error message")
	}
}

func TestProbeChat_EmptyResponse(t *testing.T) {
	mockClient := &mockModelClient{
		chatFn: func(ctx context.Context, messages []ark.ChatMessage) (string, error) {
			return "   ", nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		ChatModel:        "gpt-test",
		SkipEmbedding:    true,
		SkipRerank:       true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var chatResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityChat) {
			chatResult = &run.Results[i]
			break
		}
	}

	if chatResult == nil {
		t.Fatal("expected chat result")
	}
	if chatResult.Status != ProbeStatusFail {
		t.Errorf("expected status 'fail', got '%s'", chatResult.Status)
	}
}

func TestProbeEmbedding_Success(t *testing.T) {
	mockClient := &mockModelClient{
		embedFn: func(ctx context.Context, texts []string) ([][]float64, error) {
			return [][]float64{make([]float64, 1024)}, nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:           "test-provider",
		EmbeddingModel:     "embedding-test",
		ExpectedDimensions: 1024,
		SkipChat:           true,
		SkipRerank:         true,
		SkipMultimodal:     true,
		SkipProviderAuth:   true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var embedResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityEmbedding) {
			embedResult = &run.Results[i]
			break
		}
	}

	if embedResult == nil {
		t.Fatal("expected embedding result")
	}
	if embedResult.Status != ProbeStatusPass {
		t.Errorf("expected status 'pass', got '%s'", embedResult.Status)
	}
	if embedResult.Dimensions != 1024 {
		t.Errorf("expected 1024 dimensions, got %d", embedResult.Dimensions)
	}
}

func TestProbeEmbedding_DimensionMismatch(t *testing.T) {
	mockClient := &mockModelClient{
		embedFn: func(ctx context.Context, texts []string) ([][]float64, error) {
			return [][]float64{make([]float64, 768)}, nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:           "test-provider",
		EmbeddingModel:     "embedding-test",
		ExpectedDimensions: 1024,
		SkipChat:           true,
		SkipRerank:         true,
		SkipMultimodal:     true,
		SkipProviderAuth:   true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var embedResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityEmbedding) {
			embedResult = &run.Results[i]
			break
		}
	}

	if embedResult == nil {
		t.Fatal("expected embedding result")
	}
	if embedResult.Status != ProbeStatusFail {
		t.Errorf("expected status 'fail', got '%s'", embedResult.Status)
	}
	if embedResult.ErrorCategory != ErrorCategoryDimensionMismatch {
		t.Errorf("expected error category 'dimension_mismatch', got '%s'", embedResult.ErrorCategory)
	}
	if embedResult.Dimensions != 768 {
		t.Errorf("expected 768 dimensions, got %d", embedResult.Dimensions)
	}
}

func TestProbeEmbedding_EmptyResponse(t *testing.T) {
	mockClient := &mockModelClient{
		embedFn: func(ctx context.Context, texts []string) ([][]float64, error) {
			return [][]float64{{}}, nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		EmbeddingModel:   "embedding-test",
		SkipChat:         true,
		SkipRerank:       true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var embedResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityEmbedding) {
			embedResult = &run.Results[i]
			break
		}
	}

	if embedResult == nil {
		t.Fatal("expected embedding result")
	}
	if embedResult.Status != ProbeStatusFail {
		t.Errorf("expected status 'fail', got '%s'", embedResult.Status)
	}
}

func TestProbeRerank_Success(t *testing.T) {
	mockClient := &mockModelClient{
		rerankFn: func(ctx context.Context, query string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
			return []ark.RerankResult{
				{Index: 0, Score: 0.9},
				{Index: 1, Score: 0.1},
			}, nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		RerankModel:      "rerank-test",
		SkipChat:         true,
		SkipEmbedding:    true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var rerankResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityRerank) {
			rerankResult = &run.Results[i]
			break
		}
	}

	if rerankResult == nil {
		t.Fatal("expected rerank result")
	}
	if rerankResult.Status != ProbeStatusPass {
		t.Errorf("expected status 'pass', got '%s'", rerankResult.Status)
	}
	if rerankResult.Model != "rerank-test" {
		t.Errorf("expected model 'rerank-test', got '%s'", rerankResult.Model)
	}
}

func TestProbeRerank_Failure(t *testing.T) {
	mockClient := &mockModelClient{
		rerankFn: func(ctx context.Context, query string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
			return nil, errors.New("model not found")
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		RerankModel:      "rerank-test",
		SkipChat:         true,
		SkipEmbedding:    true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var rerankResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityRerank) {
			rerankResult = &run.Results[i]
			break
		}
	}

	if rerankResult == nil {
		t.Fatal("expected rerank result")
	}
	if rerankResult.Status != ProbeStatusFail {
		t.Errorf("expected status 'fail', got '%s'", rerankResult.Status)
	}
	if rerankResult.ErrorCategory != ErrorCategoryModelNotFound {
		t.Errorf("expected error category 'model_not_found', got '%s'", rerankResult.ErrorCategory)
	}
}

func TestProbeMultimodal_SkippedWhenNoModel(t *testing.T) {
	mockClient := &mockModelClient{}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		SkipChat:         true,
		SkipEmbedding:    true,
		SkipRerank:       true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var mmResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityMultimodal) {
			mmResult = &run.Results[i]
			break
		}
	}

	if mmResult == nil {
		t.Fatal("expected multimodal result")
	}
	if mmResult.Status != ProbeStatusSkipped {
		t.Errorf("expected status 'skipped', got '%s'", mmResult.Status)
	}
	if len(mmResult.Warnings) == 0 {
		t.Error("expected warning about missing model config")
	}
}

func TestProbeMultimodal_Success(t *testing.T) {
	mockClient := &mockModelClient{
		multimodalFn: func(ctx context.Context, name string, content []byte) (string, error) {
			return "parsed content", nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		MultimodalModel:  "mm-test",
		SkipChat:         true,
		SkipEmbedding:    true,
		SkipRerank:       true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var mmResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityMultimodal) {
			mmResult = &run.Results[i]
			break
		}
	}

	if mmResult == nil {
		t.Fatal("expected multimodal result")
	}
	if mmResult.Status != ProbeStatusPass {
		t.Errorf("expected status 'pass', got '%s'", mmResult.Status)
	}
}

func TestProbeProviderAuth_Success(t *testing.T) {
	mockClient := &mockModelClient{
		chatFn: func(ctx context.Context, messages []ark.ChatMessage) (string, error) {
			return "ok", nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:       "test-provider",
		SkipChat:       true,
		SkipEmbedding:  true,
		SkipRerank:     true,
		SkipMultimodal: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var authResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityProviderAuth) {
			authResult = &run.Results[i]
			break
		}
	}

	if authResult == nil {
		t.Fatal("expected provider_auth result")
	}
	if authResult.Status != ProbeStatusPass {
		t.Errorf("expected status 'pass', got '%s'", authResult.Status)
	}
}

func TestProbeProviderAuth_Failure(t *testing.T) {
	mockClient := &mockModelClient{
		chatFn: func(ctx context.Context, messages []ark.ChatMessage) (string, error) {
			return "", errors.New("401 unauthorized: invalid api key")
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:       "test-provider",
		SkipChat:       true,
		SkipEmbedding:  true,
		SkipRerank:     true,
		SkipMultimodal: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	var authResult *ProbeResult
	for i := range run.Results {
		if run.Results[i].Capability == string(CapabilityProviderAuth) {
			authResult = &run.Results[i]
			break
		}
	}

	if authResult == nil {
		t.Fatal("expected provider_auth result")
	}
	if authResult.Status != ProbeStatusFail {
		t.Errorf("expected status 'fail', got '%s'", authResult.Status)
	}
	if authResult.ErrorCategory != ErrorCategoryAuth {
		t.Errorf("expected error category 'auth', got '%s'", authResult.ErrorCategory)
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{"nil error", nil, ErrorCategoryUnknown},
		{"auth 401", errors.New("401 unauthorized"), ErrorCategoryAuth},
		{"auth 403", errors.New("403 forbidden"), ErrorCategoryAuth},
		{"auth invalid_api_key", errors.New("invalid_api_key: bad key"), ErrorCategoryAuth},
		{"auth unauthorized", errors.New("unauthorized access"), ErrorCategoryAuth},
		{"model not found", errors.New("model_not_found: gpt-x"), ErrorCategoryModelNotFound},
		{"model not found text", errors.New("model not found"), ErrorCategoryModelNotFound},
		{"quota exceeded", errors.New("quota_exceeded: limit reached"), ErrorCategoryQuota},
		{"quota insufficient", errors.New("insufficient_quota"), ErrorCategoryQuota},
		{"rate limit", errors.New("rate_limit: too fast"), ErrorCategoryRateLimit},
		{"rate limit 429", errors.New("status 429: too many requests"), ErrorCategoryRateLimit},
		{"too many requests", errors.New("too_many_requests"), ErrorCategoryRateLimit},
		{"network timeout", errors.New("timeout: connection timed out"), ErrorCategoryNetworkTimeout},
		{"network connection", errors.New("connection refused"), ErrorCategoryNetworkTimeout},
		{"deadline exceeded", context.DeadlineExceeded, ErrorCategoryNetworkTimeout},
		{"unknown error", errors.New("something went wrong"), ErrorCategoryUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestComputeOverallStatus(t *testing.T) {
	tests := []struct {
		name     string
		results  []ProbeResult
		expected ProbeStatus
	}{
		{
			name: "all pass",
			results: []ProbeResult{
				{Status: ProbeStatusPass},
				{Status: ProbeStatusPass},
			},
			expected: ProbeStatusPass,
		},
		{
			name: "one fail",
			results: []ProbeResult{
				{Status: ProbeStatusPass},
				{Status: ProbeStatusFail},
			},
			expected: ProbeStatusFail,
		},
		{
			name: "all fail",
			results: []ProbeResult{
				{Status: ProbeStatusFail},
				{Status: ProbeStatusFail},
			},
			expected: ProbeStatusFail,
		},
		{
			name: "all skipped",
			results: []ProbeResult{
				{Status: ProbeStatusSkipped},
				{Status: ProbeStatusSkipped},
			},
			expected: ProbeStatusSkipped,
		},
		{
			name: "mix pass and skipped",
			results: []ProbeResult{
				{Status: ProbeStatusPass},
				{Status: ProbeStatusSkipped},
			},
			expected: ProbeStatusPass,
		},
		{
			name: "mix fail and skipped",
			results: []ProbeResult{
				{Status: ProbeStatusFail},
				{Status: ProbeStatusSkipped},
			},
			expected: ProbeStatusFail,
		},
		{
			name:     "empty results",
			results:  []ProbeResult{},
			expected: ProbeStatusSkipped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeOverallStatus(tt.results)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestAuditEventRecorded(t *testing.T) {
	mockClient := &mockModelClient{}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		ChatModel:        "gpt-test",
		EmbeddingModel:   "embed-test",
		SkipRerank:       true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	_, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	events := mockAudit.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}

	event := events[0]
	if event.Action != audit.ActionModelReadinessTested {
		t.Errorf("expected action '%s', got '%s'", audit.ActionModelReadinessTested, event.Action)
	}
	if event.ResourceType != audit.ResourceTypeModel {
		t.Errorf("expected resource type '%s', got '%s'", audit.ResourceTypeModel, event.ResourceType)
	}
	if event.ResourceID != "test-provider" {
		t.Errorf("expected resource id 'test-provider', got '%s'", event.ResourceID)
	}
	if event.Outcome != audit.OutcomeSuccess {
		t.Errorf("expected outcome '%s', got '%s'", audit.OutcomeSuccess, event.Outcome)
	}
	if event.ActorType != audit.ActorTypeSystem {
		t.Errorf("expected actor type '%s', got '%s'", audit.ActorTypeSystem, event.ActorType)
	}
	if event.Metadata == nil {
		t.Fatal("expected non-nil metadata")
	}
	if event.Metadata["provider"] != "test-provider" {
		t.Errorf("expected metadata provider 'test-provider', got '%s'", event.Metadata["provider"])
	}
	if event.Metadata["overall_status"] != string(ProbeStatusPass) {
		t.Errorf("expected metadata overall_status 'pass', got '%s'", event.Metadata["overall_status"])
	}
}

func TestAuditEventFailureOutcome(t *testing.T) {
	mockClient := &mockModelClient{
		chatFn: func(ctx context.Context, messages []ark.ChatMessage) (string, error) {
			return "", errors.New("401 unauthorized")
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		ChatModel:        "gpt-test",
		SkipEmbedding:    true,
		SkipRerank:       true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	_, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	events := mockAudit.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}

	if events[0].Outcome != audit.OutcomeFailure {
		t.Errorf("expected outcome '%s', got '%s'", audit.OutcomeFailure, events[0].Outcome)
	}
}

func TestNoSensitiveInfoInResults(t *testing.T) {
	mockClient := &mockModelClient{
		chatFn: func(ctx context.Context, messages []ark.ChatMessage) (string, error) {
			return "pong with secret-key sk-12345", nil
		},
		embedFn: func(ctx context.Context, texts []string) ([][]float64, error) {
			return [][]float64{make([]float64, 1024)}, nil
		},
	}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		ChatModel:        "gpt-test",
		EmbeddingModel:   "embed-test",
		SkipRerank:       true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	for _, r := range run.Results {
		if strings.Contains(r.ErrorMessage, "sk-") {
			t.Errorf("result contains potential API key: %s", r.ErrorMessage)
		}
		if strings.Contains(r.ErrorMessage, "ping") {
			t.Errorf("result contains prompt content: %s", r.ErrorMessage)
		}
		if strings.Contains(r.ErrorMessage, "pong") {
			t.Errorf("result contains response content: %s", r.ErrorMessage)
		}
	}

	events := mockAudit.GetEvents()
	for _, e := range events {
		for k, v := range e.Metadata {
			if strings.Contains(v, "sk-") || strings.Contains(v, "ping") || strings.Contains(v, "pong") {
				t.Errorf("audit metadata contains sensitive info: %s=%s", k, v)
			}
		}
	}
}

func TestSanitizeErrorMessage(t *testing.T) {
	longMsg := strings.Repeat("a", 1000)
	result := sanitizeErrorMessage(longMsg)
	if len(result) != 500 {
		t.Errorf("expected 500 chars, got %d", len(result))
	}

	shortMsg := "short error"
	result = sanitizeErrorMessage(shortMsg)
	if result != shortMsg {
		t.Errorf("expected '%s', got '%s'", shortMsg, result)
	}
}

func TestSkipProbes(t *testing.T) {
	mockClient := &mockModelClient{}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	cfg := ProbeConfig{
		Provider:         "test-provider",
		SkipChat:         true,
		SkipEmbedding:    true,
		SkipRerank:       true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	if run.OverallStatus != ProbeStatusSkipped {
		t.Errorf("expected overall status 'skipped', got '%s'", run.OverallStatus)
	}

	for _, r := range run.Results {
		if r.Status != ProbeStatusSkipped {
			t.Errorf("expected all skipped, but %s is '%s'", r.Capability, r.Status)
		}
	}

	if mockClient.chatCalls != 0 {
		t.Errorf("expected 0 chat calls, got %d", mockClient.chatCalls)
	}
	if mockClient.embedCalls != 0 {
		t.Errorf("expected 0 embed calls, got %d", mockClient.embedCalls)
	}
	if mockClient.rerankCalls != 0 {
		t.Errorf("expected 0 rerank calls, got %d", mockClient.rerankCalls)
	}
	if mockClient.multimodalCalls != 0 {
		t.Errorf("expected 0 multimodal calls, got %d", mockClient.multimodalCalls)
	}
}

func TestConcurrencySafety(t *testing.T) {
	mockClient := &mockModelClient{}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg := ProbeConfig{
				Provider:         fmt.Sprintf("provider-%d", idx),
				ChatModel:        fmt.Sprintf("model-%d", idx),
				SkipEmbedding:    true,
				SkipRerank:       true,
				SkipMultimodal:   true,
				SkipProviderAuth: true,
			}
			_, err := svc.RunProbes(context.Background(), cfg)
			if err != nil {
				t.Errorf("RunProbes failed for goroutine %d: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	events := mockAudit.GetEvents()
	if len(events) != numGoroutines {
		t.Errorf("expected %d audit events, got %d", numGoroutines, len(events))
	}
}

func TestReadinessRunTimestamps(t *testing.T) {
	mockClient := &mockModelClient{}
	mockAudit := &mockAuditService{}
	fc := newFakeClock()
	svc := NewProbeService(mockClient, mockAudit, fc)

	startTime := fc.Now()

	cfg := ProbeConfig{
		Provider:         "test-provider",
		ChatModel:        "gpt-test",
		SkipEmbedding:    true,
		SkipRerank:       true,
		SkipMultimodal:   true,
		SkipProviderAuth: true,
	}

	run, err := svc.RunProbes(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProbes failed: %v", err)
	}

	if run.ID == "" {
		t.Error("expected non-empty ID")
	}
	if !strings.HasPrefix(run.ID, "readiness_") {
		t.Errorf("expected ID to start with 'readiness_', got '%s'", run.ID)
	}
	if run.StartedAt.Before(startTime) {
		t.Error("StartedAt should not be before start time")
	}
	if run.CompletedAt.Before(run.StartedAt) {
		t.Error("CompletedAt should not be before StartedAt")
	}
}

func TestCapabilityConstants(t *testing.T) {
	if CapabilityChat != "chat" {
		t.Errorf("expected CapabilityChat to be 'chat', got '%s'", CapabilityChat)
	}
	if CapabilityEmbedding != "embedding" {
		t.Errorf("expected CapabilityEmbedding to be 'embedding', got '%s'", CapabilityEmbedding)
	}
	if CapabilityRerank != "rerank" {
		t.Errorf("expected CapabilityRerank to be 'rerank', got '%s'", CapabilityRerank)
	}
	if CapabilityMultimodal != "multimodal" {
		t.Errorf("expected CapabilityMultimodal to be 'multimodal', got '%s'", CapabilityMultimodal)
	}
	if CapabilityProviderAuth != "provider_auth" {
		t.Errorf("expected CapabilityProviderAuth to be 'provider_auth', got '%s'", CapabilityProviderAuth)
	}
}

func TestProbeStatusConstants(t *testing.T) {
	if ProbeStatusPass != "pass" {
		t.Errorf("expected ProbeStatusPass to be 'pass', got '%s'", ProbeStatusPass)
	}
	if ProbeStatusFail != "fail" {
		t.Errorf("expected ProbeStatusFail to be 'fail', got '%s'", ProbeStatusFail)
	}
	if ProbeStatusSkipped != "skipped" {
		t.Errorf("expected ProbeStatusSkipped to be 'skipped', got '%s'", ProbeStatusSkipped)
	}
}

func TestErrorCategoryConstants(t *testing.T) {
	if ErrorCategoryAuth != "auth" {
		t.Errorf("expected ErrorCategoryAuth to be 'auth', got '%s'", ErrorCategoryAuth)
	}
	if ErrorCategoryModelNotFound != "model_not_found" {
		t.Errorf("expected ErrorCategoryModelNotFound to be 'model_not_found', got '%s'", ErrorCategoryModelNotFound)
	}
	if ErrorCategoryQuota != "quota" {
		t.Errorf("expected ErrorCategoryQuota to be 'quota', got '%s'", ErrorCategoryQuota)
	}
	if ErrorCategoryRateLimit != "rate_limit" {
		t.Errorf("expected ErrorCategoryRateLimit to be 'rate_limit', got '%s'", ErrorCategoryRateLimit)
	}
	if ErrorCategoryNetworkTimeout != "network_timeout" {
		t.Errorf("expected ErrorCategoryNetworkTimeout to be 'network_timeout', got '%s'", ErrorCategoryNetworkTimeout)
	}
	if ErrorCategoryDimensionMismatch != "dimension_mismatch" {
		t.Errorf("expected ErrorCategoryDimensionMismatch to be 'dimension_mismatch', got '%s'", ErrorCategoryDimensionMismatch)
	}
	if ErrorCategoryUnknown != "unknown" {
		t.Errorf("expected ErrorCategoryUnknown to be 'unknown', got '%s'", ErrorCategoryUnknown)
	}
}

var _ clock.Clock = (*fakeClock)(nil)
var _ modelClient = (*mockModelClient)(nil)
var _ auditService = (*mockAuditService)(nil)
