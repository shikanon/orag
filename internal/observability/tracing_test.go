package observability

import (
	"context"
	"errors"
	"testing"
)

func TestStartSpanUsesInjectedTracerAndKeepsTraceID(t *testing.T) {
	tracer := &recordingTracer{}
	restore := SetTracer(tracer)
	defer restore()

	ctx := WithTraceID(context.Background(), "trace_existing")
	spanCtx, span := StartSpan(ctx, "hybrid_retrieve")
	wantErr := errors.New("retrieval unavailable")
	span.End(wantErr)

	if got, ok := TraceIDFromContext(spanCtx); !ok || got != "trace_existing" {
		t.Fatalf("trace_id from span context = %q, %v; want trace_existing, true", got, ok)
	}
	if len(tracer.names) != 1 || tracer.names[0] != "hybrid_retrieve" {
		t.Fatalf("started spans = %#v, want hybrid_retrieve", tracer.names)
	}
	if len(tracer.ended) != 1 || !errors.Is(tracer.ended[0], wantErr) {
		t.Fatalf("ended errors = %#v, want retrieval unavailable", tracer.ended)
	}
}

func TestStartSpanDefaultsToNoop(t *testing.T) {
	restore := SetTracer(nil)
	defer restore()

	ctx := WithTraceID(context.Background(), "trace_noop")
	spanCtx, span := StartSpan(ctx, "init")
	span.End(nil)

	if got, ok := TraceIDFromContext(spanCtx); !ok || got != "trace_noop" {
		t.Fatalf("trace_id from noop span context = %q, %v; want trace_noop, true", got, ok)
	}
}

type recordingTracer struct {
	names []string
	ended []error
}

func (t *recordingTracer) StartSpan(ctx context.Context, name string) (context.Context, Span) {
	t.names = append(t.names, name)
	return ctx, recordingSpan{tracer: t}
}

type recordingSpan struct {
	tracer *recordingTracer
}

func (s recordingSpan) End(err error) {
	s.tracer.ended = append(s.tracer.ended, err)
}
