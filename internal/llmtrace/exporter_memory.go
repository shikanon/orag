package llmtrace

import (
	"context"
	"sync"
)

type InMemoryExporter struct {
	mu           sync.RWMutex
	observations []LLMObservation
}

func NewInMemoryExporter() *InMemoryExporter {
	return &InMemoryExporter{
		observations: []LLMObservation{},
	}
}

func (e *InMemoryExporter) Export(_ context.Context, observation LLMObservation) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	obsCopy := observation
	if obsCopy.Metadata == nil {
		obsCopy.Metadata = map[string]string{}
	}
	if obsCopy.Tags == nil {
		obsCopy.Tags = []string{}
	}
	if obsCopy.ModelParameters == nil {
		obsCopy.ModelParameters = map[string]any{}
	}

	e.observations = append(e.observations, obsCopy)
	return nil
}

func (e *InMemoryExporter) Flush(_ context.Context) error {
	return nil
}

func (e *InMemoryExporter) Close(_ context.Context) error {
	return nil
}

func (e *InMemoryExporter) GetObservations() []LLMObservation {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]LLMObservation, len(e.observations))
	copy(result, e.observations)
	return result
}

func (e *InMemoryExporter) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.observations = []LLMObservation{}
}

var _ LLMTraceExporter = (*InMemoryExporter)(nil)
