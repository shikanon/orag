package http

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/platform/logger"
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

	resp := performJSON(h, "GET", "/v1/knowledge-bases", "", "")
	if resp.Code != 401 {
		t.Fatalf("missing token status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"trace_id"`) {
		t.Fatalf("error response missing trace id: %s", resp.Body)
	}

	resp = performJSON(h, "POST", "/v1/auth/login", `{`, "")
	if resp.Code != 400 {
		t.Fatalf("invalid json status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"invalid_json"`) {
		t.Fatalf("unexpected body: %s", resp.Body)
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

type testResponse struct {
	Code        int
	Body        string
	ContentType string
}

func newTestHertz(t *testing.T) (*route.Engine, func()) {
	t.Helper()
	t.Setenv("STORAGE_BACKEND", "memory")
	t.Setenv("ADMIN_DEFAULT_USERNAME", "admin")
	t.Setenv("ADMIN_DEFAULT_PASSWORD", "secret")
	t.Setenv("PORT", "0")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	app, err := core.New(context.Background(), cfg, logger.New(false))
	if err != nil {
		t.Fatal(err)
	}
	h := NewServer(app).Hertz()
	return h.Engine, func() { _ = app.Close() }
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
	headers := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
	if token != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: "Bearer " + token})
	}
	var reqBody *ut.Body
	if body != "" {
		reqBody = &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
	}
	w := ut.PerformRequest(h, method, path, reqBody, headers...)
	result := w.Result()
	return testResponse{
		Code:        result.StatusCode(),
		Body:        string(result.Body()),
		ContentType: string(result.Header.ContentType()),
	}
}
