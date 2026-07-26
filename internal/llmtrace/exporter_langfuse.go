package llmtrace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/platform/clock"
)

type LangfuseExporter struct {
	config    ExporterConfig
	baseURL   string
	publicKey string
	secretKey string
	client    *http.Client
	queue     chan LLMObservation
	wg        sync.WaitGroup
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	clock     clock.Clock
	logger    *slog.Logger
	dropped   int64
}

type langfuseIngestionBatch struct {
	Batch []langfuseObservation `json:"batch"`
}

type langfuseObservation struct {
	ID                  string            `json:"id"`
	TraceID             string            `json:"traceId"`
	ParentObservationID string            `json:"parentObservationId,omitempty"`
	Type                string            `json:"type"`
	Name                string            `json:"name"`
	StartTime           string            `json:"startTime"`
	EndTime             string            `json:"endTime"`
	Latency             float64           `json:"latency"`
	Model               string            `json:"model,omitempty"`
	ModelParameters     map[string]any    `json:"modelParameters,omitempty"`
	Input               any               `json:"input,omitempty"`
	Output              any               `json:"output,omitempty"`
	Usage               langfuseUsage     `json:"usage,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	Tags                []string          `json:"tags,omitempty"`
	Level               string            `json:"level,omitempty"`
	StatusMessage       string            `json:"statusMessage,omitempty"`
	Version             string            `json:"version,omitempty"`
}

type langfuseUsage struct {
	PromptTokens     int `json:"promptTokens,omitempty"`
	CompletionTokens int `json:"completionTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

func NewLangfuseExporter(baseURL, publicKey, secretKey string, config ExporterConfig) (*LangfuseExporter, error) {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}
	if config.MaxQueueSize <= 0 {
		config.MaxQueueSize = 10000
	}

	baseURL = strings.TrimRight(baseURL, "/")

	e := &LangfuseExporter{
		config:    config,
		baseURL:   baseURL,
		publicKey: publicKey,
		secretKey: secretKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		queue:  make(chan LLMObservation, config.MaxQueueSize),
		stopCh: make(chan struct{}),
		clock:  clock.RealClock{},
		logger: slog.Default(),
	}

	return e, nil
}

func (e *LangfuseExporter) SetClock(c clock.Clock) {
	e.clock = c
}

func (e *LangfuseExporter) SetHTTPClient(client *http.Client) {
	e.client = client
}

func (e *LangfuseExporter) SetLogger(logger *slog.Logger) {
	if logger != nil {
		e.logger = logger
	}
}

func (e *LangfuseExporter) Start(ctx context.Context) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()

	e.wg.Add(1)
	go e.runFlushLoop(ctx)
}

func (e *LangfuseExporter) runFlushLoop(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.flushAll(ctx)
			return
		case <-e.stopCh:
			e.flushAll(ctx)
			return
		case <-ticker.C:
			e.flushBatch(ctx)
		}
	}
}

func (e *LangfuseExporter) Export(ctx context.Context, observation LLMObservation) error {
	if !e.config.Enabled {
		return nil
	}
	if e.publicKey == "" || e.secretKey == "" {
		return nil
	}

	select {
	case e.queue <- observation:
	default:
		e.logger.Warn("langfuse exporter queue full, dropping observation",
			"queue_size", len(e.queue),
			"max_queue_size", e.config.MaxQueueSize,
		)
	}

	return nil
}

func (e *LangfuseExporter) Flush(ctx context.Context) error {
	return e.flushBatch(ctx)
}

func (e *LangfuseExporter) flushBatch(ctx context.Context) error {
	batch := e.drainBatch(e.config.BatchSize)
	if len(batch) == 0 {
		return nil
	}

	return e.sendBatch(ctx, batch)
}

func (e *LangfuseExporter) flushAll(ctx context.Context) {
	for {
		batch := e.drainBatch(e.config.BatchSize)
		if len(batch) == 0 {
			return
		}
		if err := e.sendBatch(ctx, batch); err != nil {
			e.logger.Error("failed to send batch during flushAll", "error", err)
		}
	}
}

func (e *LangfuseExporter) drainBatch(maxSize int) []LLMObservation {
	batch := make([]LLMObservation, 0, maxSize)

	for i := 0; i < maxSize; i++ {
		select {
		case obs := <-e.queue:
			batch = append(batch, obs)
		default:
			return batch
		}
	}

	return batch
}

func (e *LangfuseExporter) sendBatch(ctx context.Context, batch []LLMObservation) error {
	ingestionBatch := langfuseIngestionBatch{
		Batch: make([]langfuseObservation, 0, len(batch)),
	}

	for _, obs := range batch {
		langfuseObs := e.mapObservation(obs)
		ingestionBatch.Batch = append(ingestionBatch.Batch, langfuseObs)
	}

	body, err := json.Marshal(ingestionBatch)
	if err != nil {
		e.logger.Error("failed to marshal langfuse ingestion batch", "error", err)
		return err
	}

	url := e.baseURL + "/api/public/ingestion"

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := e.sendRequest(ctx, url, body)
		if err == nil {
			return nil
		}

		lastErr = err
		e.logger.Warn("langfuse ingestion request failed",
			"attempt", attempt+1,
			"error", err,
		)
	}

	e.logger.Error("langfuse ingestion failed after retries",
		"batch_size", len(batch),
		"error", lastErr,
	)

	return lastErr
}

func (e *LangfuseExporter) sendRequest(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.SetBasicAuth(e.publicKey, e.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("langfuse API returned status %d: %s", resp.StatusCode, string(respBody))
}

func (e *LangfuseExporter) mapObservation(obs LLMObservation) langfuseObservation {
	result := langfuseObservation{
		Name:      obs.Name,
		StartTime: obs.StartTime.UTC().Format(time.RFC3339Nano),
		EndTime:   obs.EndTime.UTC().Format(time.RFC3339Nano),
		Level:     obs.Level,
	}

	traceID := obs.TraceID
	spanID := obs.SpanID
	parentSpanID := obs.ParentSpanID

	if e.config.HashIdentifiers {
		traceID = hashString(traceID)
		spanID = hashString(spanID)
		if parentSpanID != "" {
			parentSpanID = hashString(parentSpanID)
		}
	}

	result.TraceID = traceID
	result.ID = spanID
	result.ParentObservationID = parentSpanID

	switch obs.Type {
	case ObservationTypeGeneration:
		result.Type = "GENERATION"
	case ObservationTypeSpan:
		result.Type = "SPAN"
	case ObservationTypeEvent:
		result.Type = "EVENT"
	default:
		result.Type = "SPAN"
	}

	latency := obs.LatencyMs
	if latency == 0 && !obs.EndTime.IsZero() && !obs.StartTime.IsZero() {
		latency = obs.EndTime.Sub(obs.StartTime).Milliseconds()
	}
	result.Latency = float64(latency)

	if obs.Model != "" {
		result.Model = obs.Model
	}
	if len(obs.ModelParameters) > 0 {
		result.ModelParameters = obs.ModelParameters
	}

	if e.config.RecordPrompts {
		if len(obs.Input) > 0 {
			result.Input = string(obs.Input)
		}
		if len(obs.Output) > 0 {
			result.Output = string(obs.Output)
		}
	}

	if obs.Usage.PromptTokens > 0 || obs.Usage.CompletionTokens > 0 || obs.Usage.TotalTokens > 0 {
		result.Usage = langfuseUsage{
			PromptTokens:     obs.Usage.PromptTokens,
			CompletionTokens: obs.Usage.CompletionTokens,
			TotalTokens:      obs.Usage.TotalTokens,
		}
	}

	if len(obs.Metadata) > 0 {
		result.Metadata = obs.Metadata
	}

	if len(obs.Tags) > 0 {
		result.Tags = obs.Tags
	}

	if obs.Status == StatusFailure {
		result.Level = "ERROR"
		if obs.ErrorMessage != "" {
			result.StatusMessage = obs.ErrorMessage
		}
	}

	return result
}

func (e *LangfuseExporter) Close(ctx context.Context) error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = false
	close(e.stopCh)
	e.mu.Unlock()

	e.wg.Wait()
	return nil
}

func hashString(s string) string {
	if s == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

var _ LLMTraceExporter = (*LangfuseExporter)(nil)
