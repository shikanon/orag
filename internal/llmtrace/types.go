package llmtrace

import (
	"context"
	"time"
)

type ObservationType string

const (
	ObservationTypeGeneration ObservationType = "generation"
	ObservationTypeSpan       ObservationType = "span"
	ObservationTypeEvent      ObservationType = "event"
)

const (
	StatusSuccess = "success"
	StatusFailure = "failure"
)

const (
	LevelDebug   = "DEBUG"
	LevelInfo    = "INFO"
	LevelWarning = "WARNING"
	LevelError   = "ERROR"
)

type TokenUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Unit             string
}

type LLMObservation struct {
	TraceID         string
	SpanID          string
	ParentSpanID    string
	Name            string
	Type            ObservationType
	StartTime       time.Time
	EndTime         time.Time
	LatencyMs       int64
	Status          string
	Level           string
	Provider        string
	Model           string
	ModelParameters map[string]any
	Input           []byte
	Output          []byte
	Usage           TokenUsage
	Metadata        map[string]string
	ErrorCode       string
	ErrorMessage    string
	Tags            []string
}

type LLMTraceExporter interface {
	Export(ctx context.Context, observation LLMObservation) error
	Flush(ctx context.Context) error
	Close(ctx context.Context) error
}

type ExporterConfig struct {
	Enabled         bool
	RecordPrompts   bool
	HashIdentifiers bool
	BatchSize       int
	FlushInterval   time.Duration
	MaxQueueSize    int
}

func DefaultExporterConfig() ExporterConfig {
	return ExporterConfig{
		Enabled:         false,
		RecordPrompts:   false,
		HashIdentifiers: true,
		BatchSize:       100,
		FlushInterval:   5 * time.Second,
		MaxQueueSize:    10000,
	}
}
