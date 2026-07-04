package contract

import (
	"context"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestOpenAPI(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/readyz"},
		{http.MethodGet, "/metrics"},
		{http.MethodGet, "/docs"},
		{http.MethodPost, "/v1/auth/login"},
		{http.MethodGet, "/v1/knowledge-bases"},
		{http.MethodPost, "/v1/knowledge-bases"},
		{http.MethodGet, "/v1/knowledge-bases/{id}"},
		{http.MethodDelete, "/v1/knowledge-bases/{id}"},
		{http.MethodPost, "/v1/knowledge-bases/{id}/documents"},
		{http.MethodPost, "/v1/knowledge-bases/{id}/documents:import"},
		{http.MethodGet, "/v1/ingestion-jobs/{id}"},
		{http.MethodPost, "/v1/query"},
		{http.MethodPost, "/v1/query:stream"},
		{http.MethodGet, "/v1/traces"},
		{http.MethodGet, "/v1/traces/{trace_id}"},
		{http.MethodPost, "/v1/datasets"},
		{http.MethodPost, "/v1/datasets/{id}/items"},
		{http.MethodPost, "/v1/evaluations"},
		{http.MethodGet, "/v1/evaluations/{id}"},
		{http.MethodPost, "/v1/optimizations"},
	} {
		item := doc.Paths.Find(route.path)
		if item == nil {
			t.Fatalf("missing path %s", route.path)
		}
		if item.GetOperation(route.method) == nil {
			t.Fatalf("missing operation %s %s", route.method, route.path)
		}
	}

	for _, schema := range []string{
		"ErrorResponse",
		"LoginRequest",
		"LoginResponse",
		"KnowledgeBase",
		"QueryRequest",
		"QueryResponse",
		"IngestionJob",
		"Dataset",
		"DatasetItem",
		"RunEvaluationRequest",
		"RunEvaluationResponse",
		"EvaluationMetrics",
		"OptimizeRequest",
		"OptimizeResult",
		"ReadinessResponse",
		"TraceListResponse",
		"TraceRecord",
		"TraceNodeSpan",
	} {
		if doc.Components.Schemas[schema] == nil {
			t.Fatalf("missing schema %s", schema)
		}
	}

	for path, item := range doc.Paths {
		if path == "/v1/auth/login" || len(path) < len("/v1/") || path[:len("/v1/")] != "/v1/" {
			continue
		}
		for method, op := range item.Operations() {
			if op.Security == nil || !hasBearerAuth(*op.Security) {
				t.Fatalf("%s %s missing bearerAuth security", method, path)
			}
		}
	}
}

func TestEvaluationSchemasExposeQualityMetrics(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}

	evalResp := requireSchema(t, doc, "RunEvaluationResponse")
	metricsRef, ok := evalResp.Properties["metrics"]
	if !ok {
		t.Fatal("RunEvaluationResponse missing metrics")
	}
	if metricsRef.Ref != "#/components/schemas/EvaluationMetrics" {
		t.Fatalf("metrics ref = %q, want EvaluationMetrics", metricsRef.Ref)
	}
	if evalResp.Description == "" {
		t.Fatal("RunEvaluationResponse missing pairwise accuracy semantics description")
	}

	evalMetrics := requireSchema(t, doc, "EvaluationMetrics")
	for _, name := range []string{
		"pairwise_accuracy",
		"ndcg_at_k",
		"recall_at_k",
		"mrr",
		"map",
		"coverage",
		"retrieval_failure_rate",
		"redundancy_rate",
		"duplicate_count",
		"deduped_top_k_count",
		"alpha_ndcg",
		"aspect_coverage",
		"latency_p95_ms",
		"cache_hit_rate",
	} {
		if _, ok := evalMetrics.Properties[name]; !ok {
			t.Fatalf("EvaluationMetrics missing %s", name)
		}
	}
	pairwise := evalMetrics.Properties["pairwise_accuracy"]
	if pairwise.Value == nil || pairwise.Value.Description == "" {
		t.Fatal("pairwise_accuracy missing primary metric description")
	}

	candidate := requireSchema(t, doc, "CandidateResult")
	for _, name := range []string{
		"score",
		"pairwise_accuracy",
		"ndcg_at_k",
		"recall_at_k",
		"mrr",
		"map",
		"retrieval_failure_rate",
		"redundancy_rate",
		"duplicate_count",
		"deduped_top_k_count",
		"alpha_ndcg",
		"aspect_coverage",
		"latency_p95_ms",
	} {
		if _, ok := candidate.Properties[name]; !ok {
			t.Fatalf("CandidateResult missing %s", name)
		}
	}
	score := candidate.Properties["score"]
	if score.Value == nil || score.Value.Description == "" {
		t.Fatal("CandidateResult.score missing primary metric description")
	}
}

func requireSchema(t *testing.T, doc *openapi3.T, name string) *openapi3.Schema {
	t.Helper()

	ref := doc.Components.Schemas[name]
	if ref == nil || ref.Value == nil {
		t.Fatalf("missing schema %s", name)
	}
	return ref.Value
}

func hasBearerAuth(requirements openapi3.SecurityRequirements) bool {
	for _, req := range requirements {
		if _, ok := req["bearerAuth"]; ok {
			return true
		}
	}
	return false
}
