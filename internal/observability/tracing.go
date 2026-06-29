package observability

import (
	"context"
	"strings"

	"github.com/shikanon/orag/internal/platform/id"
)

const TraceIDHeader = "X-Trace-ID"

type traceIDContextKey struct{}

type Span struct {
	Name string
}

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

func StartSpan(ctx context.Context, name string) (context.Context, Span) {
	return ctx, Span{Name: name}
}

func (Span) End(error) {}
