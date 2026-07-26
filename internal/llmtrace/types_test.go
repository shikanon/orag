package llmtrace

import (
	"context"
	"sync"
	"testing"
	"time"
)

func newTestObservation(name string) LLMObservation {
	return LLMObservation{
		TraceID:      "trace-abc",
		SpanID:       "span-123",
		ParentSpanID: "span-parent",
		Name:         name,
		Type:         ObservationTypeGeneration,
		StartTime:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:      time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC),
		LatencyMs:    1000,
		Status:       StatusSuccess,
		Level:        LevelInfo,
		Provider:     "openai",
		Model:        "gpt-4",
		ModelParameters: map[string]any{
			"temperature": 0.7,
			"max_tokens":  1024,
		},
		Input:  []byte("test input"),
		Output: []byte("test output"),
		Usage: TokenUsage{
			PromptTokens:     100,
			CompletionTokens: 200,
			TotalTokens:      300,
			Unit:             "tokens",
		},
		Metadata: map[string]string{
			"project_id": "proj-1",
			"user_id":    "user-1",
		},
		Tags: []string{"test", "generation"},
	}
}

func TestDefaultExporterConfig(t *testing.T) {
	cfg := DefaultExporterConfig()

	if cfg.Enabled {
		t.Error("expected Enabled to be false by default")
	}
	if cfg.RecordPrompts {
		t.Error("expected RecordPrompts to be false by default")
	}
	if !cfg.HashIdentifiers {
		t.Error("expected HashIdentifiers to be true by default")
	}
	if cfg.BatchSize != 100 {
		t.Errorf("expected BatchSize 100, got %d", cfg.BatchSize)
	}
	if cfg.FlushInterval != 5*time.Second {
		t.Errorf("expected FlushInterval 5s, got %v", cfg.FlushInterval)
	}
	if cfg.MaxQueueSize != 10000 {
		t.Errorf("expected MaxQueueSize 10000, got %d", cfg.MaxQueueSize)
	}
}

func TestNoopExporter_NoSideEffects(t *testing.T) {
	exporter := NewNoopExporter()
	ctx := context.Background()

	obs := newTestObservation("test-noop")

	err := exporter.Export(ctx, obs)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	err = exporter.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	err = exporter.Close(ctx)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestNoopExporter_ImplementsInterface(t *testing.T) {
	var _ LLMTraceExporter = (*NoopExporter)(nil)
}

func TestInMemoryExporter_ExportAndGet(t *testing.T) {
	exporter := NewInMemoryExporter()
	ctx := context.Background()

	obs := newTestObservation("test-memory")

	err := exporter.Export(ctx, obs)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	observations := exporter.GetObservations()
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}

	got := observations[0]
	if got.TraceID != "trace-abc" {
		t.Errorf("expected TraceID trace-abc, got %s", got.TraceID)
	}
	if got.SpanID != "span-123" {
		t.Errorf("expected SpanID span-123, got %s", got.SpanID)
	}
	if got.ParentSpanID != "span-parent" {
		t.Errorf("expected ParentSpanID span-parent, got %s", got.ParentSpanID)
	}
	if got.Name != "test-memory" {
		t.Errorf("expected Name test-memory, got %s", got.Name)
	}
	if got.Type != ObservationTypeGeneration {
		t.Errorf("expected Type generation, got %s", got.Type)
	}
	if got.LatencyMs != 1000 {
		t.Errorf("expected LatencyMs 1000, got %d", got.LatencyMs)
	}
	if got.Status != StatusSuccess {
		t.Errorf("expected Status success, got %s", got.Status)
	}
	if got.Level != LevelInfo {
		t.Errorf("expected Level INFO, got %s", got.Level)
	}
	if got.Provider != "openai" {
		t.Errorf("expected Provider openai, got %s", got.Provider)
	}
	if got.Model != "gpt-4" {
		t.Errorf("expected Model gpt-4, got %s", got.Model)
	}
	if got.Usage.PromptTokens != 100 {
		t.Errorf("expected PromptTokens 100, got %d", got.Usage.PromptTokens)
	}
	if got.Usage.CompletionTokens != 200 {
		t.Errorf("expected CompletionTokens 200, got %d", got.Usage.CompletionTokens)
	}
	if got.Usage.TotalTokens != 300 {
		t.Errorf("expected TotalTokens 300, got %d", got.Usage.TotalTokens)
	}
	if got.Usage.Unit != "tokens" {
		t.Errorf("expected Unit tokens, got %s", got.Usage.Unit)
	}
}

func TestInMemoryExporter_Clear(t *testing.T) {
	exporter := NewInMemoryExporter()
	ctx := context.Background()

	obs := newTestObservation("test-clear")
	exporter.Export(ctx, obs)

	if len(exporter.GetObservations()) != 1 {
		t.Fatal("expected 1 observation before clear")
	}

	exporter.Clear()

	if len(exporter.GetObservations()) != 0 {
		t.Fatal("expected 0 observations after clear")
	}
}

func TestInMemoryExporter_FlushAndClose(t *testing.T) {
	exporter := NewInMemoryExporter()
	ctx := context.Background()

	err := exporter.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	err = exporter.Close(ctx)
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestInMemoryExporter_ImplementsInterface(t *testing.T) {
	var _ LLMTraceExporter = (*InMemoryExporter)(nil)
}

func TestInMemoryExporter_NilFieldsInitialized(t *testing.T) {
	exporter := NewInMemoryExporter()
	ctx := context.Background()

	obs := LLMObservation{
		TraceID: "trace-1",
		Name:    "test-nil",
		Type:    ObservationTypeSpan,
	}

	err := exporter.Export(ctx, obs)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	got := exporter.GetObservations()[0]

	if got.Metadata == nil {
		t.Error("expected Metadata to be initialized")
	}
	if got.Tags == nil {
		t.Error("expected Tags to be initialized")
	}
	if got.ModelParameters == nil {
		t.Error("expected ModelParameters to be initialized")
	}
}

func TestInMemoryExporter_ConcurrencySafe(t *testing.T) {
	exporter := NewInMemoryExporter()
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 100
	perGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				obs := newTestObservation("test-concurrent")
				obs.TraceID = string(rune('A' + idx%26))
				exporter.Export(ctx, obs)
			}
		}(i)
	}

	wg.Wait()

	observations := exporter.GetObservations()
	expected := numGoroutines * perGoroutine
	if len(observations) != expected {
		t.Errorf("expected %d observations, got %d", expected, len(observations))
	}
}

func TestObservationTypeConstants(t *testing.T) {
	if ObservationTypeGeneration != "generation" {
		t.Errorf("expected ObservationTypeGeneration to be 'generation', got '%s'", ObservationTypeGeneration)
	}
	if ObservationTypeSpan != "span" {
		t.Errorf("expected ObservationTypeSpan to be 'span', got '%s'", ObservationTypeSpan)
	}
	if ObservationTypeEvent != "event" {
		t.Errorf("expected ObservationTypeEvent to be 'event', got '%s'", ObservationTypeEvent)
	}
}

func TestStatusConstants(t *testing.T) {
	if StatusSuccess != "success" {
		t.Errorf("expected StatusSuccess to be 'success', got '%s'", StatusSuccess)
	}
	if StatusFailure != "failure" {
		t.Errorf("expected StatusFailure to be 'failure', got '%s'", StatusFailure)
	}
}

func TestLevelConstants(t *testing.T) {
	if LevelDebug != "DEBUG" {
		t.Errorf("expected LevelDebug to be 'DEBUG', got '%s'", LevelDebug)
	}
	if LevelInfo != "INFO" {
		t.Errorf("expected LevelInfo to be 'INFO', got '%s'", LevelInfo)
	}
	if LevelWarning != "WARNING" {
		t.Errorf("expected LevelWarning to be 'WARNING', got '%s'", LevelWarning)
	}
	if LevelError != "ERROR" {
		t.Errorf("expected LevelError to be 'ERROR', got '%s'", LevelError)
	}
}

func TestLLMObservation_AllRequiredFields(t *testing.T) {
	obs := newTestObservation("required-fields-test")

	if obs.TraceID == "" {
		t.Error("TraceID is required")
	}
	if obs.Provider == "" {
		t.Error("Provider is required")
	}
	if obs.Model == "" {
		t.Error("Model is required")
	}
	if obs.LatencyMs == 0 {
		t.Error("LatencyMs is required")
	}
	if obs.Usage.TotalTokens == 0 && obs.Usage.Unit == "" {
		t.Log("Note: TokenUsage may be zero for non-generation observations")
	}
}

func TestInMemoryExporter_GetObservationsReturnsCopy(t *testing.T) {
	exporter := NewInMemoryExporter()
	ctx := context.Background()

	obs := newTestObservation("copy-test")
	exporter.Export(ctx, obs)

	first := exporter.GetObservations()
	second := exporter.GetObservations()

	if &first[0] == &second[0] {
		t.Error("GetObservations should return a copy, not the same slice")
	}

	first[0].Name = "modified"
	if second[0].Name == "modified" {
		t.Error("modifying returned slice should not affect internal state")
	}
}
