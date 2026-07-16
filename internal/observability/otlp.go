package observability

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// OTelTracer adapts the application tracer contract without exporting query,
// prompt, document, or model-output content.
type OTelTracer struct{ tracer oteltrace.Tracer }

// ConfigureOTLP installs a batch OTLP/HTTP exporter only when endpoint is set.
// Sampling controls only the optional OTLP export; application trace persistence
// remains independent. serviceName defaults to "orag".
func ConfigureOTLP(ctx context.Context, endpoint string, sampleRatio float64, serviceName string) (func() error, error) {
	if strings.TrimSpace(endpoint) == "" {
		return func() error { return nil }, nil
	}
	if math.IsNaN(sampleRatio) || math.IsInf(sampleRatio, 0) || sampleRatio < 0 || sampleRatio > 1 {
		return nil, fmt.Errorf("OTLP trace sample ratio must be in [0, 1]")
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		serviceName = "orag"
	}
	provider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithSampler(newOTLPSampler(sampleRatio)),
		trace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", serviceName))),
	)
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	restore := SetTracer(NewOTelTracer("github.com/shikanon/orag"))
	var shutdownOnce sync.Once
	var shutdownErr error
	return func() error {
		shutdownOnce.Do(func() {
			restore()
			otel.SetTracerProvider(previousProvider)
			otel.SetTextMapPropagator(previousPropagator)
			shutdownErr = provider.Shutdown(context.Background())
		})
		return shutdownErr
	}, nil
}

func newOTLPSampler(sampleRatio float64) trace.Sampler {
	return trace.ParentBased(trace.TraceIDRatioBased(sampleRatio))
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
