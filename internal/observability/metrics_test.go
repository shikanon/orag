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
	metrics.ObserveOfflineKnowledgeRun("completed")
	metrics.ObserveOfflineKnowledgeRun("custom-run-status")
	metrics.AddOfflineKnowledgeExtractedQuestions(11)
	metrics.SetOfflineKnowledgeClusters(4)
	metrics.ObserveOfflineKnowledgeReplay("success", 8)
	metrics.ObserveOfflineKnowledgeReplay("custom-replay-outcome", 2)
	metrics.ObserveOfflineKnowledgeCodexAnalysis("success", 5)
	metrics.ObserveOfflineKnowledgeCodexAnalysis("custom-codex-outcome", 3)
	metrics.ObserveOfflineKnowledgeEvidenceValidation("success", 6)
	metrics.ObserveOfflineKnowledgeEvidenceValidation("custom-validation-outcome", 1)
	metrics.SetOptimizationItems("verified", 3)
	metrics.SetOptimizationItems("custom-item-status", 2)
	metrics.IncOptimizationItemStatusTotal("verified")
	metrics.IncOptimizationItemStatusTotal("verified")
	metrics.IncOptimizationItemStatusTotal("verified")
	metrics.ObserveOptimizationRevalidate("success", 2)
	metrics.ObserveOptimizationRevalidate("custom-revalidate-outcome", 1)
	metrics.ObserveOptimizationShadowHit(true, 80)
	metrics.ObserveOptimizationShadowHit(false, 120)
	metrics.IncOptimizationShadowWriteDropped("rate_limited")
	metrics.IncOptimizationShadowWriteDropped("trace_id=trace_123")
	metrics.SetOptimizationQualityLift(0.07, 0.04, 0.03)
	metrics.IncOptimizationHallucinationRisk("evidence_insufficient")
	metrics.IncOptimizationHallucinationRisk("query=bad")

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
		`offline_knowledge_runs_total{status="completed"} 1`,
		`offline_knowledge_runs_total{status="other"} 1`,
		`offline_knowledge_extracted_questions_total 11`,
		`offline_knowledge_clusters 4`,
		`offline_knowledge_replay_total{outcome="success"} 8`,
		`offline_knowledge_replay_total{outcome="error"} 2`,
		`offline_knowledge_codex_analysis_total{outcome="success"} 1`,
		`offline_knowledge_codex_analysis_total{outcome="error"} 1`,
		`offline_knowledge_codex_analysis_errors_total 1`,
		`offline_knowledge_deep_search_steps_total 8`,
		`offline_knowledge_evidence_validation_total{outcome="success"} 6`,
		`offline_knowledge_evidence_validation_errors_total 1`,
		`optimization_items{status="verified"} 3`,
		`optimization_items{status="other"} 2`,
		`optimization_items_verified_total 3`,
		`optimization_revalidate_total{outcome="success"} 2`,
		`optimization_revalidate_errors_total 1`,
		`optimization_shadow_hit_total{injected="true"} 1`,
		`optimization_shadow_hit_total{injected="false"} 1`,
		`optimization_shadow_write_dropped_total{reason="rate_limited"} 1`,
		`optimization_shadow_write_dropped_total{reason="other"} 1`,
		`optimization_shadow_latency_seconds_bucket{injected="true",le="0.100"} 1`,
		`optimization_shadow_latency_seconds_sum{injected="true"} 0.080`,
		`optimization_recall_lift 0.070000`,
		`optimization_answer_quality_lift 0.040000`,
		`optimization_citation_coverage_lift 0.030000`,
		`optimization_hallucination_risk_total{reason="evidence_insufficient"} 1`,
		`optimization_hallucination_risk_total{reason="other"} 1`,
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
		"custom-run-status",
		"custom-replay-outcome",
		"custom-codex-outcome",
		"custom-validation-outcome",
		"custom-item-status",
		"custom-revalidate-outcome",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics body contains high-cardinality field %q:\n%s", forbidden, body)
		}
	}
}

func TestOptimizationItemStatusCountersAreIndependentFromGauge(t *testing.T) {
	metrics := NewMetrics()
	metrics.IncOptimizationItemStatusTotal("verified")
	metrics.IncOptimizationItemStatusTotal("verified")
	metrics.IncOptimizationItemStatusTotal("verified")
	metrics.SetOptimizationItems("verified", 3)
	metrics.SetOptimizationItems("verified", 1)

	body := metrics.Render()
	required := []string{
		`optimization_items{status="verified"} 1`,
		`optimization_items_verified_total 3`,
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}
