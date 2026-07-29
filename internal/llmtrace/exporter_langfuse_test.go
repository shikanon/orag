package llmtrace

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLangfuseExporter_ImplementsInterface(t *testing.T) {
	var _ LLMTraceExporter = (*LangfuseExporter)(nil)
}

func TestLangfuseExporter_DefaultConfigNoPrompts(t *testing.T) {
	var receivedBody []byte
	var receivedAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-no-prompts")

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	if len(batch.Batch) != 1 {
		t.Fatalf("expected 1 observation in batch, got %d", len(batch.Batch))
	}

	got := batch.Batch[0]

	if got.Input != nil {
		t.Error("expected Input to be nil when RecordPrompts=false")
	}
	if got.Output != nil {
		t.Error("expected Output to be nil when RecordPrompts=false")
	}

	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-test:sk-test"))
	if receivedAuth != expectedAuth {
		t.Errorf("expected auth %q, got %q", expectedAuth, receivedAuth)
	}
}

func TestLangfuseExporter_TokenUsageMapping(t *testing.T) {
	var receivedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true
	config.HashIdentifiers = false

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-usage")

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	if len(batch.Batch) != 1 {
		t.Fatalf("expected 1 observation in batch, got %d", len(batch.Batch))
	}

	got := batch.Batch[0]

	if got.Usage.PromptTokens != 100 {
		t.Errorf("expected PromptTokens 100, got %d", got.Usage.PromptTokens)
	}
	if got.Usage.CompletionTokens != 200 {
		t.Errorf("expected CompletionTokens 200, got %d", got.Usage.CompletionTokens)
	}
	if got.Usage.TotalTokens != 300 {
		t.Errorf("expected TotalTokens 300, got %d", got.Usage.TotalTokens)
	}
}

func TestLangfuseExporter_QueueFullDropsObservations(t *testing.T) {
	var requestCount int
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := ExporterConfig{
		Enabled:         true,
		RecordPrompts:   false,
		HashIdentifiers: true,
		BatchSize:       10,
		FlushInterval:   10 * time.Second,
		MaxQueueSize:    5,
	}

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()

	for i := 0; i < 20; i++ {
		obs := newTestObservation("test-drop")
		obs.SpanID = string(rune('a' + i%26))
		if err := exporter.Export(ctx, obs); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	if len(exporter.queue) > 5 {
		t.Errorf("expected queue size <= 5, got %d", len(exporter.queue))
	}
}

func TestLangfuseExporter_NoKeysDegradesGracefully(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when keys are missing")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true

	exporter, err := NewLangfuseExporter(ts.URL, "", "", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-no-keys")

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
}

func TestLangfuseExporter_DisabledNoExport(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when exporter is disabled")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = false

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-disabled")

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
}

func TestLangfuseExporter_RequestFormat(t *testing.T) {
	var receivedBody []byte
	var receivedMethod string
	var receivedPath string
	var receivedContentType string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true
	config.HashIdentifiers = false
	config.RecordPrompts = true

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-format")

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("expected method POST, got %s", receivedMethod)
	}
	if receivedPath != "/api/public/ingestion" {
		t.Errorf("expected path /api/public/ingestion, got %s", receivedPath)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedContentType)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	if len(batch.Batch) != 1 {
		t.Fatalf("expected 1 observation in batch, got %d", len(batch.Batch))
	}

	got := batch.Batch[0]

	if got.ID != "span-123" {
		t.Errorf("expected id span-123, got %s", got.ID)
	}
	if got.TraceID != "trace-abc" {
		t.Errorf("expected traceId trace-abc, got %s", got.TraceID)
	}
	if got.ParentObservationID != "span-parent" {
		t.Errorf("expected parentObservationId span-parent, got %s", got.ParentObservationID)
	}
	if got.Type != "GENERATION" {
		t.Errorf("expected type GENERATION, got %s", got.Type)
	}
	if got.Name != "test-format" {
		t.Errorf("expected name test-format, got %s", got.Name)
	}
	if got.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", got.Model)
	}
	if got.Input != "test input" {
		t.Errorf("expected input 'test input', got %v", got.Input)
	}
	if got.Output != "test output" {
		t.Errorf("expected output 'test output', got %v", got.Output)
	}
	if got.Latency != 1000 {
		t.Errorf("expected latency 1000, got %f", got.Latency)
	}

	if len(got.Metadata) != 2 {
		t.Errorf("expected 2 metadata entries, got %d", len(got.Metadata))
	}
	if got.Metadata["project_id"] != "proj-1" {
		t.Errorf("expected metadata project_id proj-1, got %s", got.Metadata["project_id"])
	}

	if len(got.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(got.Tags))
	}
}

func TestLangfuseExporter_HashIdentifiers(t *testing.T) {
	var receivedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true
	config.HashIdentifiers = true

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-hash")

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	got := batch.Batch[0]

	expectedTraceID := hashString("trace-abc")
	expectedSpanID := hashString("span-123")
	expectedParentID := hashString("span-parent")

	if got.TraceID != expectedTraceID {
		t.Errorf("expected hashed traceId %q, got %q", expectedTraceID, got.TraceID)
	}
	if got.ID != expectedSpanID {
		t.Errorf("expected hashed id %q, got %q", expectedSpanID, got.ID)
	}
	if got.ParentObservationID != expectedParentID {
		t.Errorf("expected hashed parentObservationId %q, got %q", expectedParentID, got.ParentObservationID)
	}
}

func TestLangfuseExporter_BatchSending(t *testing.T) {
	var mu sync.Mutex
	var totalReceived int
	var batchSizes []int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var batch langfuseIngestionBatch
		json.Unmarshal(body, &batch)

		mu.Lock()
		totalReceived += len(batch.Batch)
		batchSizes = append(batchSizes, len(batch.Batch))
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := ExporterConfig{
		Enabled:         true,
		RecordPrompts:   false,
		HashIdentifiers: false,
		BatchSize:       5,
		FlushInterval:   10 * time.Second,
		MaxQueueSize:    100,
	}

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()

	for i := 0; i < 12; i++ {
		obs := newTestObservation("test-batch")
		obs.SpanID = string(rune('a' + i))
		if err := exporter.Export(ctx, obs); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if totalReceived != 12 {
		t.Errorf("expected total 12 observations received, got %d", totalReceived)
	}
}

func TestLangfuseExporter_ObservationTypes(t *testing.T) {
	var receivedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true
	config.HashIdentifiers = false
	config.BatchSize = 10

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()

	types := []ObservationType{ObservationTypeGeneration, ObservationTypeSpan, ObservationTypeEvent}
	for _, typ := range types {
		obs := newTestObservation("test-type-" + string(typ))
		obs.Type = typ
		if err := exporter.Export(ctx, obs); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	expectedTypes := []string{"GENERATION", "SPAN", "EVENT"}
	for i, expected := range expectedTypes {
		if batch.Batch[i].Type != expected {
			t.Errorf("expected type %q, got %q", expected, batch.Batch[i].Type)
		}
	}
}

func TestLangfuseExporter_StartAndClose(t *testing.T) {
	var mu sync.Mutex
	var totalReceived int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var batch langfuseIngestionBatch
		json.Unmarshal(body, &batch)

		mu.Lock()
		totalReceived += len(batch.Batch)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := ExporterConfig{
		Enabled:         true,
		RecordPrompts:   false,
		HashIdentifiers: false,
		BatchSize:       10,
		FlushInterval:   50 * time.Millisecond,
		MaxQueueSize:    100,
	}

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	exporter.Start(ctx)

	for i := 0; i < 5; i++ {
		obs := newTestObservation("test-start")
		obs.SpanID = string(rune('a' + i))
		if err := exporter.Export(ctx, obs); err != nil {
			t.Fatalf("Export failed: %v", err)
		}
	}

	time.Sleep(200 * time.Millisecond)

	if err := exporter.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if totalReceived != 5 {
		t.Errorf("expected 5 observations received, got %d", totalReceived)
	}
}

func TestLangfuseExporter_ConcurrencySafe(t *testing.T) {
	var mu sync.Mutex
	var totalReceived int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var batch langfuseIngestionBatch
		json.Unmarshal(body, &batch)

		mu.Lock()
		totalReceived += len(batch.Batch)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := ExporterConfig{
		Enabled:         true,
		RecordPrompts:   false,
		HashIdentifiers: false,
		BatchSize:       50,
		FlushInterval:   10 * time.Millisecond,
		MaxQueueSize:    1000,
	}

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	exporter.Start(ctx)

	var wg sync.WaitGroup
	numGoroutines := 10
	perGoroutine := 20

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
	time.Sleep(200 * time.Millisecond)

	if err := exporter.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	expected := numGoroutines * perGoroutine
	if totalReceived != expected {
		t.Errorf("expected %d observations received, got %d", expected, totalReceived)
	}
}

func TestLangfuseExporter_RetryOnFailure(t *testing.T) {
	var mu sync.Mutex
	var requestCount int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		currentCount := requestCount
		mu.Unlock()

		if currentCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := ExporterConfig{
		Enabled:         true,
		RecordPrompts:   false,
		HashIdentifiers: false,
		BatchSize:       10,
		FlushInterval:   10 * time.Second,
		MaxQueueSize:    100,
	}

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	obs := newTestObservation("test-retry")
	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	err = exporter.Flush(ctx)

	mu.Lock()
	defer mu.Unlock()

	if requestCount < 3 {
		t.Errorf("expected at least 3 requests (retries), got %d", requestCount)
	}

	if err != nil {
		t.Logf("Flush returned error (may be expected with fast timeout): %v", err)
	}
}

func TestLangfuseExporter_BaseURLTrailingSlash(t *testing.T) {
	var receivedPath string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true

	exporter, err := NewLangfuseExporter(ts.URL+"/", "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-slash")

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if receivedPath != "/api/public/ingestion" {
		t.Errorf("expected path /api/public/ingestion, got %s", receivedPath)
	}
}

func TestLangfuseExporter_LatencyCalculation(t *testing.T) {
	var receivedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true
	config.HashIdentifiers = false

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()

	obs := newTestObservation("test-latency-zero")
	obs.LatencyMs = 0
	obs.StartTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	obs.EndTime = time.Date(2025, 1, 1, 0, 0, 2, 500000000, time.UTC)

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	got := batch.Batch[0]
	expectedLatency := float64(2500)

	if got.Latency != expectedLatency {
		t.Errorf("expected latency %f, got %f", expectedLatency, got.Latency)
	}
}

func TestLangfuseExporter_FailureStatus(t *testing.T) {
	var receivedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true
	config.HashIdentifiers = false

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-failure")
	obs.Status = StatusFailure
	obs.ErrorMessage = "something went wrong"

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	got := batch.Batch[0]

	if got.Level != "ERROR" {
		t.Errorf("expected level ERROR, got %s", got.Level)
	}
	if got.StatusMessage != "something went wrong" {
		t.Errorf("expected statusMessage 'something went wrong', got %s", got.StatusMessage)
	}
}

func TestLangfuseExporter_CloseWithoutStart(t *testing.T) {
	config := DefaultExporterConfig()
	config.Enabled = true

	exporter, err := NewLangfuseExporter("http://localhost", "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	err = exporter.Close(ctx)
	if err != nil {
		t.Fatalf("Close without Start should not fail: %v", err)
	}
}

func TestLangfuseExporter_DoubleStart(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := ExporterConfig{
		Enabled:         true,
		RecordPrompts:   false,
		HashIdentifiers: false,
		BatchSize:       10,
		FlushInterval:   100 * time.Millisecond,
		MaxQueueSize:    100,
	}

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	exporter.Start(ctx)
	exporter.Start(ctx)

	if err := exporter.Close(ctx); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestHashString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"test", func() string {
			h := sha256.Sum256([]byte("test"))
			return hex.EncodeToString(h[:])
		}()},
	}

	for _, tt := range tests {
		got := hashString(tt.input)
		if got != tt.expected {
			t.Errorf("hashString(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestLangfuseExporter_EmptyParentSpanID(t *testing.T) {
	var receivedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true
	config.HashIdentifiers = true

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-empty-parent")
	obs.ParentSpanID = ""

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	got := batch.Batch[0]

	if got.ParentObservationID != "" {
		t.Errorf("expected empty parentObservationId, got %q", got.ParentObservationID)
	}

	bodyStr := string(receivedBody)
	if strings.Contains(bodyStr, "parentObservationId") {
		t.Error("parentObservationId should be omitted when empty")
	}
}

func TestLangfuseExporter_ModelParameters(t *testing.T) {
	var receivedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := DefaultExporterConfig()
	config.Enabled = true
	config.HashIdentifiers = false

	exporter, err := NewLangfuseExporter(ts.URL, "pk-test", "sk-test", config)
	if err != nil {
		t.Fatalf("NewLangfuseExporter failed: %v", err)
	}

	ctx := context.Background()
	obs := newTestObservation("test-model-params")

	if err := exporter.Export(ctx, obs); err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if err := exporter.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	var batch langfuseIngestionBatch
	if err := json.Unmarshal(receivedBody, &batch); err != nil {
		t.Fatalf("failed to unmarshal request body: %v", err)
	}

	got := batch.Batch[0]

	if got.ModelParameters == nil {
		t.Fatal("expected ModelParameters to be set")
	}

	temp, ok := got.ModelParameters["temperature"]
	if !ok {
		t.Error("expected temperature in modelParameters")
	}
	if tempFloat, ok := temp.(float64); ok {
		if tempFloat != 0.7 {
			t.Errorf("expected temperature 0.7, got %f", tempFloat)
		}
	} else {
		t.Errorf("expected temperature to be float64, got %T", temp)
	}
}
