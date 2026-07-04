package observability

import (
	"context"
	"strings"
	"sync"

	"github.com/shikanon/orag/internal/platform/id"
)

const TraceIDHeader = "X-Trace-ID"

type traceIDContextKey struct{}

type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, Span)
}

type Span interface {
	End(err error)
}

type noopTracer struct{}

type noopSpan struct{}

var (
	tracerMu     sync.RWMutex
	activeTracer Tracer = noopTracer{}
)

func NewTraceID() string {
	return id.New("trace")
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDContextKey{}, traceID)
}

func TraceIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	traceID, ok := ctx.Value(traceIDContextKey{}).(string)
	traceID = strings.TrimSpace(traceID)
	return traceID, ok && traceID != ""
}

func EnsureTraceID(ctx context.Context) string {
	if traceID, ok := TraceIDFromContext(ctx); ok {
		return traceID
	}
	return NewTraceID()
}

func SetTracer(tracer Tracer) func() {
	if tracer == nil {
		tracer = noopTracer{}
	}
	tracerMu.Lock()
	previous := activeTracer
	activeTracer = tracer
	tracerMu.Unlock()

	return func() {
		tracerMu.Lock()
		activeTracer = previous
		tracerMu.Unlock()
	}
}

func StartSpan(ctx context.Context, name string) (context.Context, Span) {
	tracerMu.RLock()
	tracer := activeTracer
	tracerMu.RUnlock()
	if tracer == nil {
		tracer = noopTracer{}
	}
	spanCtx, span := tracer.StartSpan(ctx, name)
	if spanCtx == nil {
		spanCtx = ctx
	}
	if span == nil {
		span = noopSpan{}
	}
	return spanCtx, span
}

func (noopTracer) StartSpan(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, noopSpan{}
}

func (noopSpan) End(error) {}
