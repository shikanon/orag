package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/eval"
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

func TestDatasetItemAndEvaluationCrossTenantReturn404(t *testing.T) {
	h, app, closeApp := newTestHertzWithApp(t)
	defer closeApp()

	tenantAToken := loginToken(t, h)
	tenantBToken, err := app.Auth.IssueToken("tenant_b", "user_b")
	if err != nil {
		t.Fatal(err)
	}
	evalRepo := &countingEvalRepository{delegate: app.Eval.Repository}
	app.Eval.Repository = evalRepo

	resp := performJSON(h, "POST", "/v1/datasets", `{"name":"tenant a regression","kind":"golden"}`, tenantAToken)
	if resp.Code != 201 {
		t.Fatalf("create dataset status = %d body=%s", resp.Code, resp.Body)
	}
	var ds struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &ds); err != nil {
		t.Fatal(err)
	}
	if ds.ID == "" {
		t.Fatalf("missing dataset id in response: %s", resp.Body)
	}

	resp = performJSON(h, "POST", "/v1/datasets/"+ds.ID+"/items", `{"query":"tenant b write","ground_truth":"blocked"}`, tenantBToken)
	if resp.Code != 404 {
		t.Fatalf("cross-tenant item status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("unexpected cross-tenant item body: %s", resp.Body)
	}
	items, err := app.Datasets.Items(context.Background(), "tenant_default", ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("cross-tenant item was persisted: %#v", items)
	}

	resp = performJSON(h, "POST", "/v1/evaluations", `{"dataset_id":"`+ds.ID+`","knowledge_base_id":"kb_default","profile":"realtime"}`, tenantBToken)
	if resp.Code != 404 {
		t.Fatalf("cross-tenant evaluation status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"dataset_not_found"`) {
		t.Fatalf("unexpected cross-tenant evaluation body: %s", resp.Body)
	}
	if evalRepo.runCreates != 0 || evalRepo.resultCreates != 0 {
		t.Fatalf("cross-tenant evaluation persisted rows: runs=%d results=%d", evalRepo.runCreates, evalRepo.resultCreates)
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

type countingEvalRepository struct {
	delegate      eval.Repository
	runCreates    int
	resultCreates int
}

func (r *countingEvalRepository) StoreEvaluationRun(ctx context.Context, tenantID string, result eval.RunResult) error {
	r.runCreates++
	return r.delegate.StoreEvaluationRun(ctx, tenantID, result)
}

func (r *countingEvalRepository) StoreEvaluationResult(ctx context.Context, runID, datasetItemID, answer string, metrics map[string]float64) error {
	r.resultCreates++
	return r.delegate.StoreEvaluationResult(ctx, runID, datasetItemID, answer, metrics)
}

func (r *countingEvalRepository) GetEvaluationRun(ctx context.Context, tenantID, id string) (eval.RunResult, bool, error) {
	return r.delegate.GetEvaluationRun(ctx, tenantID, id)
}

type failingPipeline struct {
	err error
}

func (p failingPipeline) Invoke(context.Context, rag.QueryRequest) (rag.QueryResponse, error) {
	return rag.QueryResponse{}, p.err
}
