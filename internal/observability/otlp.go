package observability

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// OTelTracer adapts the application tracer contract without exporting query,
// prompt, document, or model-output content.
type OTelTracer struct{ tracer oteltrace.Tracer }

// ConfigureOTLP installs a batch OTLP/HTTP exporter only when endpoint is set.
func ConfigureOTLP(ctx context.Context, endpoint string) (func() error, error) {
	if strings.TrimSpace(endpoint) == "" {
		return func() error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	provider := trace.NewTracerProvider(trace.WithBatcher(exporter))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	restore := SetTracer(NewOTelTracer("github.com/shikanon/orag"))
	var shutdownOnce sync.Once
	var shutdownErr error
	return func() error {
		shutdownOnce.Do(func() {
			restore()
			otel.SetTracerProvider(previousProvider)
			shutdownErr = provider.Shutdown(context.Background())
		})
		return shutdownErr
	}, nil
}

func NewOTelTracer(name string) OTelTracer {
	return OTelTracer{tracer: otel.Tracer(name)}
}

func (t OTelTracer) StartSpan(ctx context.Context, name string) (context.Context, Span) {
	if traceID, ok := TraceIDFromContext(ctx); ok {
		ctx, span := t.tracer.Start(ctx, name, oteltrace.WithAttributes(attribute.String("orag.trace_id", traceID)))
		return ctx, otelSpan{span}
	}
	ctx, span := t.tracer.Start(ctx, name)
	return ctx, otelSpan{span}
}

type otelSpan struct{ span oteltrace.Span }

func (s otelSpan) End(err error) {
	if err != nil {
		s.span.RecordError(err)
		s.span.SetAttributes(attribute.String("error.type", strings.TrimSpace(fmt.Sprintf("%T", err))))
	}
	s.span.End()
}
