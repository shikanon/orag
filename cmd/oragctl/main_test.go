package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
)

func TestRunTraceLookupFound(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	getter := fakeTraceGetter{
		trace: postgres.TraceRecord{
			ID:         "trace_1",
			TenantID:   "tenant_1",
			Profile:    rag.Profile("realtime"),
			LatencyMS:  42,
			CreatedAt:  createdAt,
			HasError:   true,
			ErrorCount: 1,
			NodeSpans: []postgres.TraceNodeSpan{
				{ID: "span_1", NodeName: "generate", LatencyMS: 42, Error: "llm timeout", CreatedAt: createdAt},
			},
		},
		found: true,
	}
	var out bytes.Buffer

	if err := runTraceLookup(context.Background(), getter, "trace_1", &out); err != nil {
		t.Fatalf("runTraceLookup() error = %v", err)
	}
	var got traceLookupResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, out.String())
	}
	if !got.Found || got.Trace == nil || got.Trace.ID != "trace_1" || !got.Trace.HasError {
		t.Fatalf("runTraceLookup() output = %#v", got)
	}
	if got.TraceID != "" {
		t.Fatalf("found output should not duplicate trace_id at top level: %#v", got)
	}
}

func TestRunTraceLookupNotFound(t *testing.T) {
	var out bytes.Buffer

	if err := runTraceLookup(context.Background(), fakeTraceGetter{}, "missing_trace", &out); err != nil {
		t.Fatalf("runTraceLookup() error = %v", err)
	}
	var got traceLookupResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, out.String())
	}
	if got.Found || got.Trace != nil || got.TraceID != "missing_trace" {
		t.Fatalf("runTraceLookup() output = %#v", got)
	}
}

type fakeTraceGetter struct {
	trace postgres.TraceRecord
	found bool
	err   error
}

func (f fakeTraceGetter) GetTrace(context.Context, string) (postgres.TraceRecord, bool, error) {
	return f.trace, f.found, f.err
}
