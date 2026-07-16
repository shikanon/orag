package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestOTLPMetricsSinkExportsCoreMetricsWithControlledAttributes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	defer func() {
		otel.SetMeterProvider(previousProvider)
		_ = provider.Shutdown(context.Background())
	}()

	sink, err := newOTLPMetricsSink("test.orag")
	if err != nil {
		t.Fatalf("newOTLPMetricsSink() error = %v", err)
	}
	metrics := NewMetrics()
	restore := metrics.setOTLPSink(sink)
	defer restore()

	metrics.ObserveHTTP("GET", "/v1/query?secret=redacted", 503, 17)
	metrics.ObserveRAGQuery("high_precision", "miss", "success", 25)
	metrics.IncRAGError("high_precision", "query_failed: trace_id=secret")
	metrics.ObserveDependencyCheck("postgres", "ready", 3)
	metrics.ObserveTraceStore("success", 2)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	metricNames := make(map[string]bool)
	for _, scope := range collected.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			metricNames[instrument.Name] = true
			assertNoSensitiveMetricAttributes(t, instrument)
		}
	}
	for _, name := range []string{
		"orag.http.requests", "orag.http.errors", "orag.http.request.duration",
		"orag.rag.queries", "orag.rag.query.duration", "orag.rag.errors",
		"orag.dependency.checks", "orag.dependency.check.duration",
		"orag.trace.store.attempts", "orag.trace.store.duration",
	} {
		if !metricNames[name] {
			t.Errorf("missing OTLP metric %q; got %#v", name, metricNames)
		}
	}
}

func TestConfigureOTLPMetricsExportsOnShutdownAndRestoresProvider(t *testing.T) {
	requests := make(chan struct {
		path string
		body []byte
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read OTLP request: %v", err)
		}
		requests <- struct {
			path string
			body []byte
		}{path: r.URL.Path, body: body}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	metrics := NewMetrics()
	previousProvider := otel.GetMeterProvider()
	closeMetrics, err := ConfigureOTLPMetrics(context.Background(), server.URL+"/v1/metrics", metrics)
	if err != nil {
		t.Fatalf("ConfigureOTLPMetrics() error = %v", err)
	}
	metrics.ObserveHTTP("GET", "/healthz", 200, 4)
	if err := closeMetrics(); err != nil {
		t.Fatalf("closeMetrics() error = %v", err)
	}
	if got := otel.GetMeterProvider(); got != previousProvider {
		t.Fatal("closeMetrics() did not restore the prior meter provider")
	}
	select {
	case request := <-requests:
		if request.path != "/v1/metrics" {
			t.Fatalf("OTLP metrics path = %q, want /v1/metrics", request.path)
		}
		if len(request.body) == 0 {
			t.Fatal("OTLP metrics request body is empty")
		}
	case <-time.After(time.Second):
		t.Fatal("OTLP metrics exporter did not flush on shutdown")
	}
	if err := closeMetrics(); err != nil {
		t.Fatalf("second closeMetrics() error = %v", err)
	}
}

func assertNoSensitiveMetricAttributes(t *testing.T, instrument metricdata.Metrics) {
	t.Helper()
	check := func(attributes []attribute.KeyValue) {
		for _, item := range attributes {
			for _, forbidden := range []string{"trace_id", "tenant", "prompt", "document", "user"} {
				if strings.Contains(strings.ToLower(string(item.Key)), forbidden) {
					t.Errorf("%s has forbidden attribute key %s", instrument.Name, item.Key)
				}
			}
			if strings.Contains(strings.ToLower(item.Value.AsString()), "secret") {
				t.Errorf("%s has a sensitive attribute value %s=%s", instrument.Name, item.Key, item.Value.AsString())
			}
		}
	}
	switch data := instrument.Data.(type) {
	case metricdata.Sum[int64]:
		for _, point := range data.DataPoints {
			check(point.Attributes.ToSlice())
		}
	case metricdata.Histogram[int64]:
		for _, point := range data.DataPoints {
			check(point.Attributes.ToSlice())
		}
	}
}
