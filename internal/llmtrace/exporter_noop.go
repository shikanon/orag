package llmtrace

import "context"

type NoopExporter struct{}

func NewNoopExporter() *NoopExporter {
	return &NoopExporter{}
}

func (e *NoopExporter) Export(_ context.Context, _ LLMObservation) error {
	return nil
}

func (e *NoopExporter) Flush(_ context.Context) error {
	return nil
}

func (e *NoopExporter) Close(_ context.Context) error {
	return nil
}

var _ LLMTraceExporter = (*NoopExporter)(nil)
