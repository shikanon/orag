package observability

import (
	"context"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// metricSink mirrors the core runtime metrics to an optional standard exporter.
// Implementations must not add high-cardinality or user-provided attributes.
type metricSink interface {
	ObserveHTTP(httpMetricLabels, int64)
	ObserveRAGQuery(ragMetricLabels, int64)
	ObserveRAGError(ragErrorLabels)
	ObserveDependencyCheck(dependencyMetricLabels, int64)
	ObserveTraceStore(traceStoreMetricLabels, int64)
}

// ConfigureOTLPMetrics installs a batch OTLP/HTTP metrics exporter when endpoint
// is set. It exports the core HTTP, RAG, readiness, and trace-store series with
// the same normalized, low-cardinality dimensions as the Prometheus endpoint.
func ConfigureOTLPMetrics(ctx context.Context, endpoint string, metrics *Metrics) (func() error, error) {
	endpoint = strings.TrimSpace(endpoint)
	if metrics == nil || endpoint == "" {
		return func() error { return nil }, nil
	}
	exporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(resource.NewWithAttributes("", attribute.String("service.name", "orag"))),
	)
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	sink, err := newOTLPMetricsSink("github.com/shikanon/orag")
	if err != nil {
		otel.SetMeterProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
		return nil, err
	}
	restore := metrics.setOTLPSink(sink)
	var shutdownOnce sync.Once
	var shutdownErr error
	return func() error {
		shutdownOnce.Do(func() {
			restore()
			otel.SetMeterProvider(previousProvider)
			shutdownErr = provider.Shutdown(context.Background())
		})
		return shutdownErr
	}, nil
}

func (m *Metrics) setOTLPSink(sink metricSink) func() {
	m.mu.Lock()
	previous := m.otlpSink
	m.otlpSink = sink
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		m.otlpSink = previous
		m.mu.Unlock()
	}
}

type otlpMetricsSink struct {
	httpRequests      otelmetric.Int64Counter
	httpErrors        otelmetric.Int64Counter
	httpLatency       otelmetric.Int64Histogram
	ragQueries        otelmetric.Int64Counter
	ragLatency        otelmetric.Int64Histogram
	ragErrors         otelmetric.Int64Counter
	dependencyChecks  otelmetric.Int64Counter
	dependencyLatency otelmetric.Int64Histogram
	traceStore        otelmetric.Int64Counter
	traceStoreLatency otelmetric.Int64Histogram
}

func newOTLPMetricsSink(name string) (*otlpMetricsSink, error) {
	meter := otel.Meter(name)
	requests, err := meter.Int64Counter("orag.http.requests", otelmetric.WithDescription("Total HTTP requests"))
	if err != nil {
		return nil, err
	}
	httpErrors, err := meter.Int64Counter("orag.http.errors", otelmetric.WithDescription("Total HTTP error responses"))
	if err != nil {
		return nil, err
	}
	httpLatency, err := meter.Int64Histogram("orag.http.request.duration", otelmetric.WithUnit("ms"), otelmetric.WithDescription("HTTP request latency"))
	if err != nil {
		return nil, err
	}
	ragQueries, err := meter.Int64Counter("orag.rag.queries", otelmetric.WithDescription("Total RAG queries"))
	if err != nil {
		return nil, err
	}
	ragLatency, err := meter.Int64Histogram("orag.rag.query.duration", otelmetric.WithUnit("ms"), otelmetric.WithDescription("RAG query latency"))
	if err != nil {
		return nil, err
	}
	ragErrors, err := meter.Int64Counter("orag.rag.errors", otelmetric.WithDescription("Total failed RAG queries"))
	if err != nil {
		return nil, err
	}
	dependencyChecks, err := meter.Int64Counter("orag.dependency.checks", otelmetric.WithDescription("Total dependency readiness checks"))
	if err != nil {
		return nil, err
	}
	dependencyLatency, err := meter.Int64Histogram("orag.dependency.check.duration", otelmetric.WithUnit("ms"), otelmetric.WithDescription("Dependency readiness check latency"))
	if err != nil {
		return nil, err
	}
	traceStore, err := meter.Int64Counter("orag.trace.store.attempts", otelmetric.WithDescription("Total trace-store attempts"))
	if err != nil {
		return nil, err
	}
	traceStoreLatency, err := meter.Int64Histogram("orag.trace.store.duration", otelmetric.WithUnit("ms"), otelmetric.WithDescription("Trace-store latency"))
	if err != nil {
		return nil, err
	}
	return &otlpMetricsSink{
		httpRequests: requests, httpErrors: httpErrors, httpLatency: httpLatency,
		ragQueries: ragQueries, ragLatency: ragLatency, ragErrors: ragErrors,
		dependencyChecks: dependencyChecks, dependencyLatency: dependencyLatency,
		traceStore: traceStore, traceStoreLatency: traceStoreLatency,
	}, nil
}

func (s *otlpMetricsSink) ObserveHTTP(labels httpMetricLabels, latencyMS int64) {
	attrs := otelmetric.WithAttributes(
		attribute.String("http.request.method", labels.Method),
		attribute.String("http.route", labels.Route),
		attribute.String("http.response.status_code", labels.Status),
		attribute.String("orag.http.status_class", labels.StatusClass),
	)
	s.httpRequests.Add(context.Background(), 1, attrs)
	if labels.StatusClass == "4xx" || labels.StatusClass == "5xx" {
		s.httpErrors.Add(context.Background(), 1, attrs)
	}
	s.httpLatency.Record(context.Background(), nonNegative(latencyMS), attrs)
}

func (s *otlpMetricsSink) ObserveRAGQuery(labels ragMetricLabels, latencyMS int64) {
	attrs := otelmetric.WithAttributes(
		attribute.String("orag.rag.profile", labels.Profile),
		attribute.String("orag.cache.status", labels.CacheStatus),
		attribute.String("orag.outcome", labels.Outcome),
	)
	s.ragQueries.Add(context.Background(), 1, attrs)
	s.ragLatency.Record(context.Background(), nonNegative(latencyMS), attrs)
}

func (s *otlpMetricsSink) ObserveRAGError(labels ragErrorLabels) {
	s.ragErrors.Add(context.Background(), 1, otelmetric.WithAttributes(
		attribute.String("orag.rag.profile", labels.Profile),
		attribute.String("error.type", labels.ErrorCode),
	))
}

func (s *otlpMetricsSink) ObserveDependencyCheck(labels dependencyMetricLabels, latencyMS int64) {
	attrs := otelmetric.WithAttributes(
		attribute.String("orag.dependency", labels.Dependency),
		attribute.String("orag.outcome", labels.Status),
	)
	s.dependencyChecks.Add(context.Background(), 1, attrs)
	s.dependencyLatency.Record(context.Background(), nonNegative(latencyMS), attrs)
}

func (s *otlpMetricsSink) ObserveTraceStore(labels traceStoreMetricLabels, latencyMS int64) {
	attrs := otelmetric.WithAttributes(attribute.String("orag.outcome", labels.Outcome))
	s.traceStore.Add(context.Background(), 1, attrs)
	s.traceStoreLatency.Record(context.Background(), nonNegative(latencyMS), attrs)
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
