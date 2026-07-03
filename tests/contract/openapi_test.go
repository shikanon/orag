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
		"OptimizeRequest",
		"OptimizeResult",
		"ReadinessResponse",
	} {
		if doc.Components.Schemas[schema] == nil {
			t.Fatalf("missing schema %s", schema)
		}
	}

	for _, route := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodDelete, "/v1/knowledge-bases/{id}", http.StatusNoContent},
		{http.MethodDelete, "/v1/knowledge-bases/{id}", http.StatusNotFound},
		{http.MethodDelete, "/v1/knowledge-bases/{id}", http.StatusInternalServerError},
		{http.MethodPost, "/v1/knowledge-bases/{id}/documents", http.StatusNotFound},
		{http.MethodPost, "/v1/knowledge-bases/{id}/documents:import", http.StatusNotFound},
		{http.MethodPost, "/v1/query", http.StatusNotFound},
		{http.MethodPost, "/v1/query:stream", http.StatusNotFound},
	} {
		op := doc.Paths.Find(route.path).GetOperation(route.method)
		if op.Responses.Get(route.status) == nil {
			t.Fatalf("%s %s missing %d response", route.method, route.path, route.status)
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

func hasBearerAuth(requirements openapi3.SecurityRequirements) bool {
	for _, req := range requirements {
		if _, ok := req["bearerAuth"]; ok {
			return true
		}
	}
	return false
}
