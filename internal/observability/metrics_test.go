package observability

import (
	"strings"
	"testing"
)

func TestMetricsRenderUsesControlledLabels(t *testing.T) {
	metrics := NewMetrics()
	metrics.ObserveHTTPRequest("GET", "/v1/knowledge-bases/:id", 404)
	metrics.ObserveHTTPLatency("GET", "/v1/knowledge-bases/:id?trace_id=trace_123", 404, 42)
	metrics.ObserveRAGQuery("realtime", "miss", "success", 120)
	metrics.ObserveRAGQuery("high_precision", "hit", "success", 20)
	metrics.ObserveRAGQuery("custom-profile-value", "custom-cache-value", "error", 300)
	metrics.IncRAGError("custom-profile-value", "query_failed: trace_id=trace_123")
	metrics.ObserveDependencyCheck("postgres", "ready", 12)
	metrics.ObserveDependencyCheck("custom-dependency", "custom-status", 13)
	metrics.ObserveTraceStore("success", 3)
	metrics.ObserveTraceStore("custom-outcome", 7)

	body := metrics.Render()
	required := []string{
		"# HELP orag_http_requests_total Total HTTP requests",
		"# TYPE orag_http_requests_total counter",
		`orag_http_requests_total{method="GET",route="/v1/knowledge-bases/:id",status="404",status_class="4xx"} 1`,
		`orag_http_errors_total{method="GET",route="/v1/knowledge-bases/:id",status="404",status_class="4xx"} 1`,
		`orag_http_request_latency_ms_bucket{method="GET",route="/v1/knowledge-bases/:id",status_class="4xx",le="50"} 1`,
		`orag_http_request_latency_ms_sum{method="GET",route="/v1/knowledge-bases/:id",status_class="4xx"} 42`,
		`orag_http_request_latency_ms_count{method="GET",route="/v1/knowledge-bases/:id",status_class="4xx"} 1`,
		`orag_rag_queries_total{profile="realtime",cache_status="miss",outcome="success"} 1`,
		`orag_rag_queries_total{profile="other",cache_status="unknown",outcome="error"} 1`,
		`orag_rag_errors_total{profile="other",error_code="other"} 1`,
		`orag_rag_query_latency_ms_bucket{profile="realtime",cache_status="miss",outcome="success",le="250"} 1`,
		`orag_rag_query_latency_ms_bucket{profile="realtime",cache_status="miss",outcome="success",le="+Inf"} 1`,
		`orag_rag_query_latency_ms_sum{profile="realtime",cache_status="miss",outcome="success"} 120`,
		`orag_rag_query_latency_ms_count{profile="realtime",cache_status="miss",outcome="success"} 1`,
		`orag_dependency_checks_total{dependency="postgres",status="ready"} 1`,
		`orag_dependency_checks_total{dependency="other",status="error"} 1`,
		`orag_dependency_check_latency_ms_sum{dependency="postgres",status="ready"} 12`,
		`orag_trace_store_total{outcome="success"} 1`,
		`orag_trace_store_total{outcome="error"} 1`,
		`orag_trace_store_latency_ms_sum{outcome="error"} 7`,
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}

	for _, forbidden := range []string{
		"trace_id=",
		"session_id",
		"tenant_id",
		"query=",
		"trace_123",
		"knowledge_base_id",
		"custom-dependency",
		"custom-status",
		"custom-outcome",
		"custom-profile-value",
		"custom-cache-value",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics body contains high-cardinality field %q:\n%s", forbidden, body)
		}
	}
}
