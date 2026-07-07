package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime/multipart"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	"github.com/getkin/kin-openapi/openapi3"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/dataset"
	evalpkg "github.com/shikanon/orag/internal/eval"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/platform/logger"
	"github.com/shikanon/orag/internal/rag"
)

func TestLoginValidatesPassword(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	resp := performJSON(h, "POST", "/v1/auth/login", `{"username":"admin","password":"secret"}`, "")
	if resp.Code != 200 {
		t.Fatalf("login status = %d body=%s", resp.Code, resp.Body)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.AccessToken == "" {
		t.Fatal("missing access token")
	}

	resp = performJSON(h, "POST", "/v1/auth/login", `{"username":"admin","password":"wrong"}`, "")
	if resp.Code != 401 {
		t.Fatalf("wrong password status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"invalid_credentials"`) {
		t.Fatalf("unexpected body: %s", resp.Body)
	}
}

func TestAuthMiddlewareAndInvalidJSON(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	resp := performJSONWithTrace(h, "GET", "/v1/knowledge-bases", "", "", "trace_http_error")
	if resp.Code != 401 {
		t.Fatalf("missing token status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"trace_id":"trace_http_error"`) {
		t.Fatalf("error response missing request trace id: %s", resp.Body)
	}
	if got := resp.TraceIDHeader; got != "trace_http_error" {
		t.Fatalf("response trace header = %q, want trace_http_error", got)
	}

	resp = performJSON(h, "POST", "/v1/auth/login", `{`, "")
	if resp.Code != 400 {
		t.Fatalf("invalid json status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"invalid_json"`) {
		t.Fatalf("unexpected body: %s", resp.Body)
	}
}

func TestRalphLoopHTTPRuntimeIsPlannedOnly(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/ralph-loop", `{"task_spec_path":"tasks.md","task_id":"Task 1","mode":"focused","max_rounds":1}`, token, "trace_ralph_loop_planned_only")
	if resp.Code != 404 {
		t.Fatalf("ralph-loop status = %d, want 404 for planned-only runtime boundary body=%s", resp.Code, resp.Body)
	}
	if strings.Contains(resp.Body, `"verdict"`) || strings.Contains(resp.Body, `"status":"completed"`) {
		t.Fatalf("planned-only endpoint returned runnable Ralph Loop payload: %s", resp.Body)
	}
}

func TestCreateKnowledgeBaseMemoryBackendReturnsCreated(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"Docs","description":"Team docs","metadata":{"owner":"search"}}`, token)
	if resp.Code != 201 {
		t.Fatalf("create status = %d body=%s", resp.Code, resp.Body)
	}
	var created kb.KnowledgeBase
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.TenantID != "tenant_default" || created.Name != "Docs" {
		t.Fatalf("unexpected create response: %#v", created)
	}
}

func TestCreateKnowledgeBaseStorageError(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	app.KBStore = fakeKnowledgeBaseRepository{putErr: errors.New("insert failed")}

	resp := performJSONWithTrace(h, "POST", "/v1/knowledge-bases", `{"name":"Docs"}`, token, "trace_kb_create_error")
	if resp.Code == 201 {
		t.Fatalf("create status = 201, want storage error body=%s", resp.Body)
	}
	assertErrorResponse(t, resp, 500, "knowledge_base_create_failed", "trace_kb_create_error")
}

func TestKnowledgeBaseListStorageError(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	app.KBStore = fakeKnowledgeBaseRepository{listErr: errors.New("list failed")}

	resp := performJSONWithTrace(h, "GET", "/v1/knowledge-bases", "", token, "trace_kb_list_error")
	if resp.Code == 200 {
		t.Fatalf("list status = 200, want storage error body=%s", resp.Body)
	}
	if strings.Contains(resp.Body, `"items":[]`) {
		t.Fatalf("list storage error returned empty items: %s", resp.Body)
	}
	assertErrorResponse(t, resp, 500, "knowledge_base_list_failed", "trace_kb_list_error")
}

func TestKnowledgeBaseGetStorageError(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	app.KBStore = fakeKnowledgeBaseRepository{getErr: errors.New("lookup failed")}

	resp := performJSONWithTrace(h, "GET", "/v1/knowledge-bases/kb_default", "", token, "trace_kb_get_error")
	if resp.Code == 404 {
		t.Fatalf("get storage error status = 404, want 500 body=%s", resp.Body)
	}
	assertErrorResponse(t, resp, 500, "knowledge_base_lookup_failed", "trace_kb_get_error")
}

func TestKnowledgeBaseGetNotFound(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	app.KBStore = fakeKnowledgeBaseRepository{getFound: false}

	resp := performJSONWithTrace(h, "GET", "/v1/knowledge-bases/kb_missing", "", token, "trace_kb_not_found")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_kb_not_found")
}

func TestHTTPCompletionLogIncludesTraceAndErrorCodeWithoutSensitiveBody(t *testing.T) {
	var logs bytes.Buffer
	h, closeApp := newTestHertzWithLogger(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	defer closeApp()

	resp := performJSONWithTrace(h, "POST", "/v1/auth/login", `{"username":"admin","password":"raw-token prompt document model-response"`, "", "trace_http_log")
	if resp.Code != 400 {
		t.Fatalf("invalid json status = %d body=%s", resp.Code, resp.Body)
	}

	line := logs.String()
	for _, want := range []string{
		`"msg":"http_request_completed"`,
		`"method":"POST"`,
		`"route":"/v1/auth/login"`,
		`"path":"/v1/auth/login"`,
		`"status":400`,
		`"trace_id":"trace_http_log"`,
		`"error_code":"invalid_json"`,
		`"latency":`,
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("completion log missing %s: %s", want, line)
		}
	}
	for _, forbidden := range []string{"raw-token", "prompt document", "model-response"} {
		if strings.Contains(line, forbidden) {
			t.Fatalf("completion log leaked %q: %s", forbidden, line)
		}
	}
}

func TestQueryUsesRequestTraceID(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"kb_default","query":"hello"}`, token, "trace_query_success")
	if resp.Code != 200 {
		t.Fatalf("query status = %d body=%s", resp.Code, resp.Body)
	}
	var body rag.QueryResponse
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.TraceID != "trace_query_success" {
		t.Fatalf("query trace_id = %q, want trace_query_success", body.TraceID)
	}
}

func TestQueryDirectRouteResponseMatchesOpenAPI(t *testing.T) {
	t.Setenv("RAG_QUERY_ROUTER_ENABLED", "true")
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"kb_default","query":"你好"}`, token, "trace_query_direct_route")
	if resp.Code != 200 {
		t.Fatalf("query status = %d body=%s", resp.Code, resp.Body)
	}

	var body rag.QueryResponse
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body.CacheStatus != "bypass" {
		t.Fatalf("cache_status = %q, want bypass body=%s", body.CacheStatus, resp.Body)
	}
	if body.Route == nil || body.Route.Route != rag.QueryRouteDirect {
		t.Fatalf("route = %#v, want direct body=%s", body.Route, resp.Body)
	}

	var payload any
	if err := json.Unmarshal([]byte(resp.Body), &payload); err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("openapi validation failed: %v", err)
	}
	schemaRef := doc.Components.Schemas["QueryResponse"]
	if schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("OpenAPI missing QueryResponse schema")
	}
	if err := schemaRef.Value.VisitJSON(payload, openapi3.VisitAsResponse()); err != nil {
		t.Fatalf("direct route response does not match OpenAPI QueryResponse schema: %v body=%s", err, resp.Body)
	}
}

func TestQueryRepeatedTraceIDInvokesPipelinePerRequest(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &recordingPipeline{}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)
	traceID := "trace_query_reused"
	for _, query := range []string{"first repeated trace query", "second repeated trace query"} {
		resp := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"kb_default","query":"`+query+`"}`, token, traceID)
		if resp.Code != 200 {
			t.Fatalf("query status = %d body=%s", resp.Code, resp.Body)
		}
		var body rag.QueryResponse
		if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
			t.Fatal(err)
		}
		if body.TraceID != traceID {
			t.Fatalf("query trace_id = %q, want %q", body.TraceID, traceID)
		}
		if resp.TraceIDHeader != traceID {
			t.Fatalf("response trace header = %q, want %q", resp.TraceIDHeader, traceID)
		}
	}

	if len(pipeline.requests) != 2 {
		t.Fatalf("pipeline requests = %d, want 2", len(pipeline.requests))
	}
	for i, req := range pipeline.requests {
		if req.TraceID != traceID {
			t.Fatalf("pipeline request %d trace_id = %q, want %q", i+1, req.TraceID, traceID)
		}
	}
	if pipeline.requests[0].Query == pipeline.requests[1].Query {
		t.Fatalf("pipeline requests were not distinct: %#v", pipeline.requests)
	}
}

func TestTraceStatsReturnsTenantNodeStats(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"kb_default","query":"trace stats"}`, token, "trace_stats_http")
	if resp.Code != 200 {
		t.Fatalf("query status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/v1/traces:stats?limit=10", "", token)
	if resp.Code != 200 {
		t.Fatalf("trace stats status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"tenant_id":"tenant_default"`) || !strings.Contains(resp.Body, `"node_name":"init"`) {
		t.Fatalf("trace stats body = %s", resp.Body)
	}
}

type queryValidationCase struct {
	name    string
	body    string
	traceID string
}

func queryValidationCases(tracePrefix string) []queryValidationCase {
	return []queryValidationCase{
		{
			name:    "empty object",
			body:    `{}`,
			traceID: tracePrefix + "_empty_object",
		},
		{
			name:    "only query",
			body:    `{"query":"hello"}`,
			traceID: tracePrefix + "_only_query",
		},
		{
			name:    "only knowledge_base_id",
			body:    `{"knowledge_base_id":"kb_default"}`,
			traceID: tracePrefix + "_only_knowledge_base_id",
		},
		{
			name:    "blank knowledge_base_id",
			body:    `{"knowledge_base_id":"  ","query":"hello"}`,
			traceID: tracePrefix + "_blank_knowledge_base_id",
		},
		{
			name:    "blank query",
			body:    `{"knowledge_base_id":"kb_default","query":"  "}`,
			traceID: tracePrefix + "_blank_query",
		},
		{
			name:    "both blank strings",
			body:    `{"knowledge_base_id":"","query":""}`,
			traceID: tracePrefix + "_both_blank",
		},
		{
			name:    "invalid profile",
			body:    `{"knowledge_base_id":"kb_default","query":"hello","profile":"batch"}`,
			traceID: tracePrefix + "_invalid_profile",
		},
		{
			name:    "zero top_k",
			body:    `{"knowledge_base_id":"kb_default","query":"hello","top_k":0}`,
			traceID: tracePrefix + "_zero_top_k",
		},
		{
			name:    "negative top_k",
			body:    `{"knowledge_base_id":"kb_default","query":"hello","top_k":-1}`,
			traceID: tracePrefix + "_negative_top_k",
		},
		{
			name:    "too large top_k",
			body:    `{"knowledge_base_id":"kb_default","query":"hello","top_k":101}`,
			traceID: tracePrefix + "_too_large_top_k",
		},
	}
}

func TestQueryRejectsInvalidRequests(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &countingPipeline{}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)

	for _, tt := range queryValidationCases("trace_query_validation") {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONWithTrace(h, "POST", "/v1/query", tt.body, token, tt.traceID)
			assertErrorResponse(t, resp, 400, "invalid_request", tt.traceID)

			var body ErrorResponse
			if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "invalid_request" {
				t.Fatalf("error code = %q, want invalid_request body=%s", body.Error.Code, resp.Body)
			}
			if body.Error.TraceID != tt.traceID {
				t.Fatalf("error trace_id = %q, want %q body=%s", body.Error.TraceID, tt.traceID, resp.Body)
			}

			var topLevel map[string]json.RawMessage
			if err := json.Unmarshal([]byte(resp.Body), &topLevel); err != nil {
				t.Fatal(err)
			}
			if _, ok := topLevel["answer"]; ok {
				t.Fatalf("validation error returned query answer: %s", resp.Body)
			}
		})
	}

	if pipeline.calls != 0 {
		t.Fatalf("query pipeline called %d times for invalid requests", pipeline.calls)
	}
}

func TestQueryStreamRejectsInvalidRequests(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &countingPipeline{}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)
	for _, tt := range queryValidationCases("trace_query_stream_validation") {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONWithTrace(h, "POST", "/v1/query:stream", tt.body, token, tt.traceID)
			assertErrorResponse(t, resp, 400, "invalid_request", tt.traceID)
			if strings.Contains(resp.ContentType, "text/event-stream") {
				t.Fatalf("pre-stream validation content type = %q, want non-SSE body=%s", resp.ContentType, resp.Body)
			}

			var body ErrorResponse
			if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != "invalid_request" {
				t.Fatalf("error code = %q, want invalid_request body=%s", body.Error.Code, resp.Body)
			}
			if body.Error.TraceID != tt.traceID {
				t.Fatalf("error trace_id = %q, want %q body=%s", body.Error.TraceID, tt.traceID, resp.Body)
			}
		})
	}

	if pipeline.calls != 0 {
		t.Fatalf("query stream pipeline called %d times for invalid requests", pipeline.calls)
	}
}

func TestQueryRequiresExistingKnowledgeBase(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	otherToken, err := app.Auth.IssueToken("tenant_other", "user_other")
	if err != nil {
		t.Fatal(err)
	}
	pipeline := &countingPipeline{}
	app.RAG.Pipeline = pipeline

	for _, tt := range []struct {
		name    string
		path    string
		body    string
		token   string
		traceID string
	}{
		{
			name:    "json missing knowledge base",
			path:    "/v1/query",
			body:    `{"knowledge_base_id":"kb_missing_query","query":"hello"}`,
			token:   token,
			traceID: "trace_query_missing_kb",
		},
		{
			name:    "stream missing knowledge base",
			path:    "/v1/query:stream",
			body:    `{"knowledge_base_id":"kb_missing_query","query":"hello"}`,
			token:   token,
			traceID: "trace_query_stream_missing_kb",
		},
		{
			name:    "json cross tenant knowledge base",
			path:    "/v1/query",
			body:    `{"knowledge_base_id":"kb_default","query":"hello"}`,
			token:   otherToken,
			traceID: "trace_query_cross_tenant_kb",
		},
		{
			name:    "stream cross tenant knowledge base",
			path:    "/v1/query:stream",
			body:    `{"knowledge_base_id":"kb_default","query":"hello"}`,
			token:   otherToken,
			traceID: "trace_query_stream_cross_tenant_kb",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONWithTrace(h, "POST", tt.path, tt.body, tt.token, tt.traceID)
			assertErrorResponse(t, resp, 404, "knowledge_base_not_found", tt.traceID)
			if strings.Contains(resp.ContentType, "text/event-stream") {
				t.Fatalf("missing knowledge base response content type = %q, want JSON error body=%s", resp.ContentType, resp.Body)
			}
		})
	}

	if pipeline.calls != 0 {
		t.Fatalf("query pipeline called %d times for missing knowledge bases", pipeline.calls)
	}
}

func TestInvalidQueryRequestsDoNotIncrementRAGSuccessMetrics(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	tests := []struct {
		name string
		path string
		body string
	}{
		{
			name: "json",
			path: "/v1/query",
			body: `{"knowledge_base_id":"kb_default","query":""}`,
		},
		{
			name: "stream",
			path: "/v1/query:stream",
			body: `{"knowledge_base_id":"","query":"hello"}`,
		},
		{
			name: "invalid_profile",
			path: "/v1/query",
			body: `{"knowledge_base_id":"kb_default","query":"hello","profile":"batch"}`,
		},
		{
			name: "invalid_top_k",
			path: "/v1/query:stream",
			body: `{"knowledge_base_id":"kb_default","query":"hello","top_k":101}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceID := "trace_metrics_invalid_query_" + tt.name
			resp := performJSONWithTrace(h, "POST", tt.path, tt.body, token, traceID)
			assertErrorResponse(t, resp, 400, "invalid_request", traceID)
		})
	}

	resp := performJSON(h, "GET", "/metrics", "", "")
	if resp.Code != 200 {
		t.Fatalf("metrics status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, "orag_rag_queries_total 0\n") {
		t.Fatalf("invalid requests incremented total RAG queries: %s", resp.Body)
	}
	if strings.Contains(resp.Body, `outcome="success"`) {
		t.Fatalf("invalid requests incremented successful RAG query metrics: %s", resp.Body)
	}
}

func TestQueryStreamSSE(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/query:stream", `{"knowledge_base_id":"kb_default","query":"hello"}`, token)
	if resp.Code != 200 {
		t.Fatalf("query stream status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.ContentType, "text/event-stream") {
		t.Fatalf("content type = %q", resp.ContentType)
	}
	for _, event := range []string{"event: trace", "event: chunk", "event: citations", "event: done"} {
		if !strings.Contains(resp.Body, event) {
			t.Fatalf("sse body missing %q: %s", event, resp.Body)
		}
	}
}

func TestQueryStreamSSEErrorUsesRequestTraceID(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.RAG.Pipeline = failingPipeline{err: errors.New("boom")}

	token := loginToken(t, h)
	resp := performJSONWithTrace(h, "POST", "/v1/query:stream", `{"knowledge_base_id":"kb_default","query":"hello"}`, token, "trace_sse_error")
	if resp.Code != 500 {
		t.Fatalf("query stream status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.ContentType, "text/event-stream") {
		t.Fatalf("content type = %q", resp.ContentType)
	}
	if !strings.Contains(resp.Body, `"trace_id":"trace_sse_error"`) {
		t.Fatalf("sse error missing request trace id: %s", resp.Body)
	}
	resp = performJSON(h, "GET", "/metrics", "", "")
	if !strings.Contains(resp.Body, `orag_rag_errors_total{profile="default",error_code="query_failed"} 1`) {
		t.Fatalf("metrics missing rag error counter: %s", resp.Body)
	}
	if strings.Contains(resp.Body, "trace_sse_error") || strings.Contains(resp.Body, "query=") || strings.Contains(resp.Body, "session_id") {
		t.Fatalf("metrics contains high-cardinality request data: %s", resp.Body)
	}
}

func TestDatasetItemEvaluationAndOptimizationUseTokenTenant(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	tenantAToken := loginToken(t, h)
	tenantBToken, err := app.Auth.IssueToken("tenant_b", "user_b")
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSON(h, "POST", "/v1/datasets", `{"name":"regression","kind":"golden"}`, tenantAToken)
	if resp.Code != 201 {
		t.Fatalf("create dataset status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatalf("missing dataset id: %s", resp.Body)
	}

	evalBody := `{"dataset_id":"` + created.ID + `","knowledge_base_id":"kb_default","profile":"realtime"}`
	resp = performJSON(h, "POST", "/v1/evaluations", evalBody, tenantBToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant evaluation status = %d body=%s", resp.Code, resp.Body)
	}
	optimizeBody := `{"dataset_id":"` + created.ID + `","knowledge_base_id":"kb_default","profiles":["realtime"],"top_ks":[1]}`
	resp = performJSON(h, "POST", "/v1/optimizations", optimizeBody, tenantBToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant optimization status = %d body=%s", resp.Code, resp.Body)
	}

	itemBody := `{"query":"q","ground_truth":"a","relevant_doc_ids":["doc_1"]}`
	resp = performJSON(h, "POST", "/v1/datasets/"+created.ID+"/items", itemBody, tenantBToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant item create status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "POST", "/v1/datasets/"+created.ID+"/items", itemBody, tenantAToken)
	if resp.Code != 201 {
		t.Fatalf("tenant item create status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performJSON(h, "POST", "/v1/evaluations", evalBody, tenantAToken)
	if resp.Code != 202 {
		t.Fatalf("tenant evaluation status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestRunEvaluationRejectsMissingKnowledgeBaseBeforeRAG(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	pipeline := &countingPipeline{resp: rag.QueryResponse{Answer: "should not run", CacheStatus: "miss"}}
	app.RAG.Pipeline = pipeline

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "missing-kb-eval", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:       "q",
		GroundTruth: "a",
	}); err != nil {
		t.Fatal(err)
	}

	resp := performJSONWithTrace(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_missing","profile":"realtime"}`, token, "trace_eval_missing_kb")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_eval_missing_kb")
	if pipeline.calls != 0 {
		t.Fatalf("evaluation called RAG pipeline %d times for missing knowledge base", pipeline.calls)
	}
}

func TestGetEvaluationIncludesItemsJudgeAndPairwiseDetails(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.RAG.Pipeline = &countingPipeline{resp: rag.QueryResponse{
		Answer:      "qdrant answer",
		CacheStatus: "miss",
		LatencyMS:   5,
		RetrievedChunks: []kb.SearchResult{{
			Chunk: kb.Chunk{ID: "chk_1", DocumentID: "doc_1", Content: "qdrant evidence"},
		}},
		Citations: []rag.Citation{{ChunkID: "chk_1", DocumentID: "doc_1"}},
	}}
	app.Eval.Judge = httpFakeJudge{}
	app.Eval.QAG = httpFakeQAGJudge{}

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "judge-http", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:            "qdrant",
		GroundTruth:      "qdrant",
		RelevantDocIDs:   []string{"doc_1"},
		ExpectedEvidence: []string{"qdrant evidence"},
	}); err != nil {
		t.Fatal(err)
	}

	resp := performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profile":"realtime","judge":{"provider":"test","model":"judge"},"qag":{"provider":"test","model":"qag"}}`, token)
	if resp.Code != 202 {
		t.Fatalf("evaluation status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatalf("evaluation response missing id: %s", resp.Body)
	}

	resp = performJSON(h, "GET", "/v1/evaluations/"+created.ID+"?include_items=true&include_judge=true&include_pairwise=true", "", token)
	if resp.Code != 200 {
		t.Fatalf("detail status = %d body=%s", resp.Code, resp.Body)
	}
	var detail evalpkg.EvaluationDetail
	if err := json.Unmarshal([]byte(resp.Body), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Run.ID != created.ID || len(detail.Items) != 1 {
		t.Fatalf("detail run/items = %#v", detail)
	}
	if len(detail.JudgeRuns) != 2 || len(detail.JudgeResults) != 2 {
		t.Fatalf("judge detail = runs:%#v results:%#v", detail.JudgeRuns, detail.JudgeResults)
	}
	if detail.JudgeResults[0].RawResponse == "" || detail.JudgeResults[0].TokenUsage.TotalTokens == 0 {
		t.Fatalf("judge result missing raw/token: %#v", detail.JudgeResults)
	}
	if detail.Items[0].Metrics["qag_score"] != 1 {
		t.Fatalf("item metrics = %#v, want qag_score", detail.Items[0].Metrics)
	}
}

func TestGetEvaluationDefaultReturnsSummaryOnly(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "summary-only", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}
	app.RAG.Pipeline = &countingPipeline{resp: rag.QueryResponse{Answer: "a", CacheStatus: "miss"}}

	resp := performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profile":"realtime"}`, token)
	if resp.Code != 202 {
		t.Fatalf("evaluation status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	resp = performJSON(h, "GET", "/v1/evaluations/"+created.ID, "", token)
	if resp.Code != 200 {
		t.Fatalf("summary status = %d body=%s", resp.Code, resp.Body)
	}
	if strings.Contains(resp.Body, `"items"`) || strings.Contains(resp.Body, `"judge_results"`) {
		t.Fatalf("default evaluation response leaked details: %s", resp.Body)
	}
}

func TestOptimizationAsyncHTTPLifecycle(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.Optimizer.DisableAutoStart = true
	app.RAG.Pipeline = &countingPipeline{resp: rag.QueryResponse{Answer: "Qdrant", CacheStatus: "miss", LatencyMS: 7}}

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "async-optimizer", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{Query: "vector store?", GroundTruth: "Qdrant"}); err != nil {
		t.Fatal(err)
	}

	body := `{
		"dataset_id":"` + ds.ID + `",
		"knowledge_base_id":"kb_default",
		"profile":"realtime",
		"objective":{"maximize":"pairwise_accuracy"},
		"search_space":{"retrieval":{"dense_top_k":[1,2]}},
		"search":{"strategy":"grid","max_candidates":2},
		"budget":{"max_judge_calls":3},
		"selection_split":"eval",
		"holdout_split":"holdout"
	}`
	resp := performJSON(h, "POST", "/v1/optimizations", body, token)
	if resp.Code != 202 {
		t.Fatalf("submit status = %d body=%s", resp.Code, resp.Body)
	}
	var accepted struct {
		RunID     string `json:"run_id"`
		Status    string `json:"status"`
		PollURL   string `json:"poll_url"`
		CancelURL string `json:"cancel_url"`
		ResumeURL string `json:"resume_url"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.RunID == "" || accepted.Status != "queued" || accepted.PollURL == "" || accepted.CancelURL == "" || accepted.ResumeURL == "" {
		t.Fatalf("unexpected accepted response: %#v body=%s", accepted, resp.Body)
	}

	resp = performJSON(h, "GET", accepted.PollURL, "", token)
	if resp.Code != 200 {
		t.Fatalf("get status = %d body=%s", resp.Code, resp.Body)
	}
	var status struct {
		Run struct {
			ID                    string `json:"id"`
			Status                string `json:"status"`
			SampledCandidateCount int    `json:"sampled_candidate_count"`
			Objective             struct {
				Maximize string `json:"maximize"`
			} `json:"objective"`
			Runner map[string]any `json:"runner"`
		} `json:"run"`
		Candidates []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Config struct {
				Retrieval struct {
					DenseTopK int `json:"dense_top_k"`
				} `json:"retrieval"`
			} `json:"config"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &status); err != nil {
		t.Fatal(err)
	}
	if status.Run.ID != accepted.RunID || status.Run.Status != "queued" || status.Run.SampledCandidateCount != 2 || len(status.Candidates) != 2 {
		t.Fatalf("unexpected optimization status: %#v body=%s", status, resp.Body)
	}
	if status.Run.Objective.Maximize != "pairwise_accuracy" || status.Run.Runner["type"] != "internal_rag" {
		t.Fatalf("missing objective/runner metadata: %#v", status.Run)
	}

	resp = performJSON(h, "POST", accepted.CancelURL, `{"reason":"user requested"}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"status":"canceling"`) {
		t.Fatalf("cancel status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "POST", accepted.ResumeURL, `{}`, token)
	if resp.Code != 202 || !strings.Contains(resp.Body, `"status":"queued"`) {
		t.Fatalf("resume status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestOptimizationAcceptsLegacyProfilesTopKsShortcut(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	app.Optimizer.DisableAutoStart = true

	token := loginToken(t, h)
	ds, err := app.Datasets.Create(ctx, "tenant_default", "legacy-optimizer", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{Query: "q", GroundTruth: "a"}); err != nil {
		t.Fatal(err)
	}

	resp := performJSON(h, "POST", "/v1/optimizations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profiles":["high_precision"],"top_ks":[3,5]}`, token)
	if resp.Code != 202 {
		t.Fatalf("legacy submit status = %d body=%s", resp.Code, resp.Body)
	}
	var accepted struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &accepted); err != nil {
		t.Fatal(err)
	}
	resp = performJSON(h, "GET", "/v1/optimizations/"+accepted.RunID, "", token)
	if resp.Code != 200 {
		t.Fatalf("legacy status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"profile":"high_precision"`) || !strings.Contains(resp.Body, `"dense_top_k":3`) || !strings.Contains(resp.Body, `"dense_top_k":5`) {
		t.Fatalf("legacy shortcut did not map profile/top_ks into async run: %s", resp.Body)
	}
}

func TestHealthReadyAndMetrics(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	resp := performJSON(h, "GET", "/healthz", "", "")
	if resp.Code != 200 {
		t.Fatalf("health status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/readyz", "", "")
	if resp.Code != 200 {
		t.Fatalf("ready status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"storage":{"status":"ready"}`) {
		t.Fatalf("ready body = %s", resp.Body)
	}
	resp = performJSON(h, "GET", "/metrics", "", "")
	if resp.Code != 200 {
		t.Fatalf("metrics status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, "orag_http_requests_total") || !strings.Contains(resp.Body, "orag_rag_queries_total") {
		t.Fatalf("metrics body = %s", resp.Body)
	}
}

func TestIngestionJobLookupReturnsResultSummary(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/documents:import", `{"name":"demo.md","source_uri":"example://demo","content":"ORAG supports Qdrant and PostgreSQL retrieval."}`, token)
	if resp.Code != 202 {
		t.Fatalf("import status = %d body=%s", resp.Code, resp.Body)
	}
	var imported struct {
		Document struct {
			ID string `json:"id"`
		} `json:"document"`
		Job struct {
			ID         string `json:"id"`
			Status     string `json:"status"`
			DocumentID string `json:"document_id"`
			ChunkCount int    `json:"chunk_count"`
		} `json:"job"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &imported); err != nil {
		t.Fatal(err)
	}
	if imported.Job.ID == "" || imported.Job.DocumentID != imported.Document.ID || imported.Job.ChunkCount == 0 {
		t.Fatalf("unexpected import response: %#v", imported)
	}

	resp = performJSON(h, "GET", "/v1/ingestion-jobs/"+imported.Job.ID, "", token)
	if resp.Code != 200 {
		t.Fatalf("job status = %d body=%s", resp.Code, resp.Body)
	}
	var job struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		DocumentID string `json:"document_id"`
		ChunkCount int    `json:"chunk_count"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &job); err != nil {
		t.Fatal(err)
	}
	if job.ID != imported.Job.ID || job.Status != "succeeded" || job.DocumentID != imported.Document.ID || job.ChunkCount == 0 {
		t.Fatalf("unexpected job response: %#v", job)
	}
}

func TestDeleteKnowledgeBaseRemovesItFromGetListAndMemoryChunks(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases", `{"name":"delete me","description":"temporary"}`, token)
	if resp.Code != 201 {
		t.Fatalf("create status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatalf("create response missing id: %s", resp.Body)
	}

	marker := "deleted_kb_http_contract_marker"
	resp = performJSON(h, "POST", "/v1/knowledge-bases/"+created.ID+"/documents:import", `{"name":"delete.md","source_uri":"example://delete","content":"This document should be removed with marker `+marker+`."}`, token)
	if resp.Code != 202 {
		t.Fatalf("import status = %d body=%s", resp.Code, resp.Body)
	}
	chunkSource, ok := app.KBStore.(interface {
		Chunks(tenantID, kbID string) []kb.Chunk
	})
	if !ok {
		t.Fatal("test KB store does not expose chunks")
	}
	if chunks := chunkSource.Chunks("tenant_default", created.ID); len(chunks) == 0 {
		t.Fatal("expected imported chunks before delete")
	}

	resp = performJSON(h, "DELETE", "/v1/knowledge-bases/"+created.ID, "", token)
	if resp.Code != 204 {
		t.Fatalf("delete status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/v1/knowledge-bases/"+created.ID, "", token)
	if resp.Code != 404 {
		t.Fatalf("get after delete status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", "/v1/knowledge-bases", "", token)
	if resp.Code != 200 {
		t.Fatalf("list status = %d body=%s", resp.Code, resp.Body)
	}
	var listed struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &listed); err != nil {
		t.Fatal(err)
	}
	for _, item := range listed.Items {
		if item.ID == created.ID {
			t.Fatalf("deleted knowledge base still listed: %s", resp.Body)
		}
	}
	if chunks := chunkSource.Chunks("tenant_default", created.ID); len(chunks) != 0 {
		t.Fatalf("deleted knowledge base still has chunks: %#v", chunks)
	}

	stalePipeline := &countingPipeline{resp: rag.QueryResponse{
		Answer:      "stale deleted content " + marker,
		CacheStatus: "miss",
		Profile:     rag.ProfileRealtime,
		RetrievedChunks: []kb.SearchResult{{
			Chunk: kb.Chunk{
				TenantID:        "tenant_default",
				KnowledgeBaseID: created.ID,
				Content:         "stale deleted content " + marker,
			},
		}},
	}}
	app.RAG.Pipeline = stalePipeline
	resp = performJSONWithTrace(h, "POST", "/v1/query", `{"knowledge_base_id":"`+created.ID+`","query":"`+marker+`"}`, token, "trace_deleted_kb_query")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_deleted_kb_query")
	if stalePipeline.calls != 0 {
		t.Fatalf("query pipeline called %d times for deleted knowledge base", stalePipeline.calls)
	}

	resp = performJSON(h, "DELETE", "/v1/knowledge-bases/"+created.ID, "", token)
	if resp.Code != 404 {
		t.Fatalf("delete missing status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestImportDocumentRejectsMissingContent(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs

	for _, tt := range []struct {
		name    string
		body    string
		traceID string
	}{
		{
			name:    "missing content",
			body:    `{"name":"missing.md","source_uri":"test://missing-content"}`,
			traceID: "trace_import_missing_content",
		},
		{
			name:    "blank content",
			body:    `{"name":"blank.md","source_uri":"test://blank-content","content":"  "}`,
			traceID: "trace_import_blank_content",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := performJSONWithTrace(h, "POST", "/v1/knowledge-bases/kb_default/documents:import", tt.body, token, tt.traceID)
			assertErrorResponse(t, resp, 400, "invalid_request", tt.traceID)
			assertNoChunks(t, app, "kb_default")
		})
	}
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for invalid content", jobs.createCalls)
	}
}

func TestDocumentIngestionRequiresExistingKnowledgeBase(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing"

	resp := performJSON(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents:import", `{"name":"missing.md","source_uri":"test://missing","content":"orphan chunks must not be created"}`, token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)

	resp = performMultipartUpload(t, h, "/v1/knowledge-bases/"+missingKB+"/documents", "missing.md", "orphan chunks must not be created", token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if jobs.createCalls != 0 {
		t.Fatalf("upload created %d ingestion jobs for missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestImportDocumentRequiresExistingKnowledgeBase(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing_import"

	resp := performJSON(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents:import", `{"name":"missing.md","source_uri":"test://missing","content":"valid json must not be ingested"}`, token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if strings.Contains(resp.Body, `"code":"ingest_failed"`) {
		t.Fatalf("missing knowledge base returned ingest_failed: %s", resp.Body)
	}
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestUploadDocumentRequiresExistingKnowledgeBase(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing_upload"

	resp := performMultipartUpload(t, h, "/v1/knowledge-bases/"+missingKB+"/documents", "missing.md", "orphan chunks must not be created", token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if strings.Contains(resp.Body, `"code":"ingest_failed"`) {
		t.Fatalf("missing knowledge base returned ingest_failed: %s", resp.Body)
	}
	if jobs.createCalls != 0 {
		t.Fatalf("upload created %d ingestion jobs for missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestImportDocumentKnowledgeBaseLookupPrecedesInvalidJSON(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing_before_json_parse"

	resp := performJSONWithTrace(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents:import", `{`, token, "trace_import_invalid_json")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_import_invalid_json")
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for invalid json", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestUploadDocumentKnowledgeBaseLookupPrecedesMissingFile(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	missingKB := "kb_missing_before_multipart_parse"

	resp := performJSONWithTrace(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents", "", token, "trace_upload_missing_file")
	assertErrorResponse(t, resp, 404, "knowledge_base_not_found", "trace_upload_missing_file")
	if jobs.createCalls != 0 {
		t.Fatalf("upload created %d ingestion jobs for missing file", jobs.createCalls)
	}
	assertNoChunks(t, app, missingKB)
}

func TestDocumentIngestionMapsServiceMissingKnowledgeBaseTo404(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	token := loginToken(t, h)
	jobs := &countingJobStore{delegate: ingest.NewMemoryJobStore()}
	app.Ingest.Jobs = jobs
	app.Ingest.KnowledgeBases = kb.NewMemoryStore()

	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/documents:import", `{"name":"missing.md","source_uri":"test://missing","content":"service guard should map to 404"}`, token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if jobs.createCalls != 0 {
		t.Fatalf("import created %d ingestion jobs for service-level missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, "kb_default")

	resp = performMultipartUpload(t, h, "/v1/knowledge-bases/kb_default/documents", "missing.md", "service guard should map to 404", token)
	assertMissingKnowledgeBaseResponse(t, resp)
	if jobs.createCalls != 0 {
		t.Fatalf("upload created %d ingestion jobs for service-level missing knowledge base", jobs.createCalls)
	}
	assertNoChunks(t, app, "kb_default")
}

func TestResumableUploadCanContinueFromLastOffsetAndComplete(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	content := "ORAG resumable upload test content with enough searchable words."
	createBody := `{"name":"resume.md","source_uri":"test://resume.md","total_bytes":` + strconv.Itoa(len(content)) + `}`
	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/uploads", createBody, token)
	if resp.Code != 201 {
		t.Fatalf("create upload status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		ID            string `json:"id"`
		UploadURL     string `json:"upload_url"`
		CompleteURL   string `json:"complete_url"`
		ReceivedBytes int64  `json:"received_bytes"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.UploadURL == "" || created.CompleteURL == "" || created.ReceivedBytes != 0 {
		t.Fatalf("unexpected upload create response: %#v body=%s", created, resp.Body)
	}

	first := content[:12]
	resp = performRaw(h, "PUT", created.UploadURL, first, token, ut.Header{Key: "Upload-Offset", Value: "0"})
	if resp.Code != 200 || !strings.Contains(resp.Body, `"received_bytes":12`) {
		t.Fatalf("first chunk status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", created.UploadURL, "", token)
	if resp.Code != 200 || !strings.Contains(resp.Body, `"received_bytes":12`) {
		t.Fatalf("resume status lookup = %d body=%s", resp.Code, resp.Body)
	}
	resp = performRaw(h, "PUT", created.UploadURL, content[12:], token, ut.Header{Key: "Upload-Offset", Value: "12"})
	if resp.Code != 200 || !strings.Contains(resp.Body, `"received_bytes":`+strconv.Itoa(len(content))) {
		t.Fatalf("second chunk status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performJSON(h, "POST", created.CompleteURL, `{}`, token)
	if resp.Code != 202 {
		t.Fatalf("complete status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"status":"completed"`) || !strings.Contains(resp.Body, `"document"`) || !strings.Contains(resp.Body, `"job"`) {
		t.Fatalf("complete response missing upload/document/job: %s", resp.Body)
	}
}

func TestResumableUploadRejectsWrongOffsetWithCurrentOffset(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/uploads", `{"name":"offset.md","total_bytes":6}`, token)
	if resp.Code != 201 {
		t.Fatalf("create upload status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	resp = performRaw(h, "PUT", created.UploadURL, "abc", token, ut.Header{Key: "Upload-Offset", Value: "0"})
	if resp.Code != 200 {
		t.Fatalf("first chunk status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performRaw(h, "PUT", created.UploadURL, "def", token, ut.Header{Key: "Upload-Offset", Value: "0"})
	if resp.Code != 409 {
		t.Fatalf("wrong offset status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"upload_offset_mismatch"`) || !strings.Contains(resp.Body, `"received_bytes":3`) {
		t.Fatalf("wrong offset response missing current offset: %s", resp.Body)
	}
}

func TestResumableUploadCancelRemovesSession(t *testing.T) {
	h, closeApp := newTestHertz(t)
	defer closeApp()

	token := loginToken(t, h)
	resp := performJSON(h, "POST", "/v1/knowledge-bases/kb_default/uploads", `{"name":"cancel.md","total_bytes":6}`, token)
	if resp.Code != 201 {
		t.Fatalf("create upload status = %d body=%s", resp.Code, resp.Body)
	}
	var created struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}

	resp = performJSON(h, "DELETE", created.UploadURL, "", token)
	if resp.Code != 204 {
		t.Fatalf("cancel status = %d body=%s", resp.Code, resp.Body)
	}
	resp = performJSON(h, "GET", created.UploadURL, "", token)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"upload_not_found"`) {
		t.Fatalf("lookup canceled status = %d body=%s", resp.Code, resp.Body)
	}
}

func TestDatasetItemAndEvaluationRequireTenantOwnership(t *testing.T) {
	ctx := context.Background()
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	ds, err := app.Datasets.Create(ctx, "tenant_default", "golden", "golden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Datasets.AddItem(ctx, "tenant_default", ds.ID, dataset.Item{
		Query:       "qdrant vector",
		GroundTruth: "qdrant",
	}); err != nil {
		t.Fatal(err)
	}
	otherToken, err := app.Auth.IssueToken("tenant_other", "user_other")
	if err != nil {
		t.Fatal(err)
	}

	resp := performJSON(h, "POST", "/v1/datasets/"+ds.ID+"/items", `{"query":"cross tenant","ground_truth":"blocked"}`, otherToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant dataset item status = %d body=%s", resp.Code, resp.Body)
	}
	items, err := app.Datasets.Items(ctx, "tenant_default", ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("dataset items = %d, want original item only", len(items))
	}

	resp = performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profile":"realtime"}`, otherToken)
	if resp.Code != 404 || !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("cross-tenant evaluation status = %d body=%s", resp.Code, resp.Body)
	}
}

type testResponse struct {
	Code          int
	Body          string
	ContentType   string
	TraceIDHeader string
}

func newTestHertz(t *testing.T) (*route.Engine, func()) {
	h, _, closeApp := newTestHertzWithApp(t)
	return h, closeApp
}

func newTestHertzWithApp(t *testing.T) (*route.Engine, *core.App, func()) {
	return newTestHertzWithLoggerAndApp(t, logger.New(false))
}

func newTestHertzWithLogger(t *testing.T, logg *slog.Logger) (*route.Engine, func()) {
	h, _, closeApp := newTestHertzWithLoggerAndApp(t, logg)
	return h, closeApp
}

func newTestHertzWithLoggerAndApp(t *testing.T, logg *slog.Logger) (*route.Engine, *core.App, func()) {
	t.Helper()
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ADMIN_DEFAULT_USERNAME", "admin")
	t.Setenv("ADMIN_DEFAULT_PASSWORD", "secret")
	t.Setenv("PORT", "0")
	t.Setenv("ALLOW_DETERMINISTIC_MOCK", "true")
	t.Setenv("LLM_CHAT_PROVIDER", "mock")
	t.Setenv("LLM_EMBEDDING_PROVIDER", "mock")
	t.Setenv("LLM_RERANK_PROVIDER", "mock")
	t.Setenv("LLM_MULTIMODAL_PROVIDER", "mock")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	app, err := core.New(context.Background(), cfg, logg)
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(app).Hertz()
	return h.Engine, app, func() { _ = app.Close() }
}

func loginToken(t *testing.T, h *route.Engine) string {
	t.Helper()
	resp := performJSON(h, "POST", "/v1/auth/login", `{"username":"admin","password":"secret"}`, "")
	if resp.Code != 200 {
		t.Fatalf("login status = %d body=%s", resp.Code, resp.Body)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &body); err != nil {
		t.Fatal(err)
	}
	return body.AccessToken
}

func performJSON(h *route.Engine, method, path, body, token string) testResponse {
	return performJSONWithHeaders(h, method, path, body, token)
}

func performJSONWithTrace(h *route.Engine, method, path, body, token, traceID string) testResponse {
	return performJSONWithHeaders(h, method, path, body, token, ut.Header{Key: observability.TraceIDHeader, Value: traceID})
}

func performJSONWithHeaders(h *route.Engine, method, path, body, token string, extraHeaders ...ut.Header) testResponse {
	headers := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	headers = append(headers, extraHeaders...)
	var reqBody *ut.Body
	if body != "" {
		reqBody = &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
	}
	w := ut.PerformRequest(h, method, path, reqBody, headers...)
	result := w.Result()
	return testResponse{
		Code:          result.StatusCode(),
		Body:          string(result.Body()),
		ContentType:   string(result.Header.ContentType()),
		TraceIDHeader: result.Header.Get(observability.TraceIDHeader),
	}
}

func performRaw(h *route.Engine, method, path, body, token string, extraHeaders ...ut.Header) testResponse {
	headers := []ut.Header{{Key: "Content-Type", Value: "application/octet-stream"}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	headers = append(headers, extraHeaders...)
	w := ut.PerformRequest(h, method, path, &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}, headers...)
	result := w.Result()
	return testResponse{
		Code:          result.StatusCode(),
		Body:          string(result.Body()),
		ContentType:   string(result.Header.ContentType()),
		TraceIDHeader: result.Header.Get(observability.TraceIDHeader),
	}
}

func performMultipartUpload(t *testing.T, h *route.Engine, path, filename, content, token string) testResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	headers := []ut.Header{{Key: "Content-Type", Value: writer.FormDataContentType()}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	w := ut.PerformRequest(h, "POST", path, &ut.Body{Body: bytes.NewReader(body.Bytes()), Len: body.Len()}, headers...)
	result := w.Result()
	return testResponse{
		Code:          result.StatusCode(),
		Body:          string(result.Body()),
		ContentType:   string(result.Header.ContentType()),
		TraceIDHeader: result.Header.Get(observability.TraceIDHeader),
	}
}

func assertMissingKnowledgeBaseResponse(t *testing.T, resp testResponse) {
	t.Helper()
	if resp.Code != 404 {
		t.Fatalf("missing knowledge base status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"knowledge_base_not_found"`) {
		t.Fatalf("unexpected missing knowledge base body: %s", resp.Body)
	}
}

func assertErrorResponse(t *testing.T, resp testResponse, status int, code, traceID string) {
	t.Helper()
	if resp.Code != status {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, status, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"`+code+`"`) {
		t.Fatalf("error response missing code %q: %s", code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"trace_id":"`+traceID+`"`) {
		t.Fatalf("error response missing trace %q: %s", traceID, resp.Body)
	}
	if resp.TraceIDHeader != traceID {
		t.Fatalf("trace header = %q, want %q", resp.TraceIDHeader, traceID)
	}
}

func assertNoChunks(t *testing.T, app *core.App, kbID string) {
	t.Helper()
	chunks, ok := app.KBStore.(kb.ChunkSource)
	if !ok {
		t.Fatalf("test knowledge base store does not expose chunks")
	}
	if got := chunks.Chunks("tenant_default", kbID); len(got) != 0 {
		t.Fatalf("chunks created for missing knowledge base: %#v", got)
	}
}

type fakeKnowledgeBaseRepository struct {
	putErr    error
	listItems []kb.KnowledgeBase
	listErr   error
	getItem   kb.KnowledgeBase
	getFound  bool
	getErr    error
}

func (r fakeKnowledgeBaseRepository) PutKnowledgeBase(context.Context, kb.KnowledgeBase) error {
	return r.putErr
}

func (r fakeKnowledgeBaseRepository) ListKnowledgeBases(context.Context, string) ([]kb.KnowledgeBase, error) {
	return r.listItems, r.listErr
}

func (r fakeKnowledgeBaseRepository) GetKnowledgeBase(context.Context, string, string) (kb.KnowledgeBase, bool, error) {
	return r.getItem, r.getFound, r.getErr
}

func (r fakeKnowledgeBaseRepository) DeleteKnowledgeBase(context.Context, string, string) (bool, error) {
	return r.getFound, r.getErr
}

type countingJobStore struct {
	delegate    ingest.JobStore
	createCalls int
	updateCalls int
}

func (s *countingJobStore) CreateJob(ctx context.Context, job ingest.Job) (ingest.Job, error) {
	s.createCalls++
	return s.delegate.CreateJob(ctx, job)
}

func (s *countingJobStore) UpdateJob(ctx context.Context, job ingest.Job) error {
	s.updateCalls++
	return s.delegate.UpdateJob(ctx, job)
}

func (s *countingJobStore) GetJob(ctx context.Context, tenantID, id string) (ingest.Job, bool, error) {
	return s.delegate.GetJob(ctx, tenantID, id)
}

type countingPipeline struct {
	calls int
	resp  rag.QueryResponse
	err   error
}

func (p *countingPipeline) Invoke(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
	p.calls++
	return p.resp, p.err
}

type recordingPipeline struct {
	requests []rag.QueryRequest
}

func (p *recordingPipeline) Invoke(_ context.Context, req rag.QueryRequest) (rag.QueryResponse, error) {
	p.requests = append(p.requests, req)
	return rag.QueryResponse{
		Answer:      "ok",
		TraceID:     req.TraceID,
		CacheStatus: "miss",
		Profile:     rag.ProfileRealtime,
		LatencyMS:   1,
	}, nil
}

type failingPipeline struct {
	err error
}

func (p failingPipeline) Invoke(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
	return rag.QueryResponse{}, p.err
}

type httpFakeJudge struct{}

func (httpFakeJudge) Judge(_ context.Context, input evalpkg.JudgeInput) (evalpkg.JudgeOutput, error) {
	return evalpkg.JudgeOutput{
		Scores:      map[string]float64{"faithfulness": 1},
		Labels:      map[string]string{"faithfulness": "good"},
		Pass:        true,
		Rationale:   input.Query,
		RawResponse: `{"scores":{"faithfulness":1},"pass":true}`,
		ParsedJSON:  map[string]any{"scores": map[string]any{"faithfulness": float64(1)}},
		TokenUsage:  evalpkg.TokenUsage{PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3},
		CostUSD:     0.01,
	}, nil
}

type httpFakeQAGJudge struct{}

func (httpFakeQAGJudge) ScoreQAG(_ context.Context, input evalpkg.JudgeInput) (evalpkg.QAGOutput, error) {
	return evalpkg.QAGOutput{
		Score:       1,
		Metrics:     map[string]float64{"qag_score": 1, "qag_claim_coverage": 1, "qag_question_count": 1, "qag_unverifiable_rate": 0},
		Claims:      []evalpkg.QAGClaim{{Claim: input.Answer, Verdict: "supported"}},
		RawResponse: `{"score":1}`,
		ParsedJSON:  map[string]any{"score": float64(1)},
		TokenUsage:  evalpkg.TokenUsage{PromptTokens: 2, CompletionTokens: 2, TotalTokens: 4},
		CostUSD:     0.02,
	}, nil
}
