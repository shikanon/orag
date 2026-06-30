package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	oraghttp "github.com/shikanon/orag/internal/http"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
)

func TestIngestQueryWithPostgresQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	marker := fmt.Sprintf("oragint%d", time.Now().UnixNano())
	content := fmt.Sprintf("The marker %s describes ORAG using Qdrant vector retrieval and PostgreSQL sparse retrieval.", marker)
	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        testTenantID,
		KnowledgeBaseID: testKBID,
		SourceURI:       "integration://" + marker,
		Name:            marker + ".md",
		Content:         []byte(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.Status != ingest.JobStatusSucceeded || result.Job.DocumentID != result.Document.ID || result.Job.ChunkCount == 0 {
		t.Fatalf("unexpected ingest job: %#v", result.Job)
	}

	job, ok, err := app.Ingest.Jobs.GetJob(ctx, testTenantID, result.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || job.DocumentID != result.Document.ID || job.ChunkCount == 0 {
		t.Fatalf("stored job ok=%v job=%#v", ok, job)
	}

	req := rag.QueryRequest{
		TenantID:        testTenantID,
		KnowledgeBaseID: testKBID,
		Query:           "Which retrieval stores are described by marker " + marker + "?",
		Profile:         rag.ProfileRealtime,
		TopK:            8,
	}
	resp, err := app.RAG.Query(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Answer == "" || resp.TraceID == "" {
		t.Fatalf("unexpected query response: %#v", resp)
	}
	if !retrievedDocument(resp, result.Document.ID) {
		t.Fatalf("retrieved chunks do not contain document %s: %#v", result.Document.ID, resp.RetrievedChunks)
	}
	if len(resp.Citations) > 0 && !citedDocument(resp, result.Document.ID) {
		t.Fatalf("citations do not contain document %s: %#v", result.Document.ID, resp.Citations)
	}

	cached, err := app.RAG.Query(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if cached.Answer == "" || cached.TraceID == "" {
		t.Fatalf("unexpected cached query response: %#v", cached)
	}
	if cached.CacheStatus == "hit" && len(cached.Citations) == 0 {
		t.Fatalf("cache hit did not replay citations: %#v", cached)
	}
}

func TestIngestMissingKnowledgeBaseWithPostgresQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        testTenantID,
		KnowledgeBaseID: "kb_missing",
		SourceURI:       "integration://missing-kb",
		Name:            "missing.md",
		Content:         []byte("missing knowledge bases must not reach postgres foreign keys"),
	})
	if !errors.Is(err, ingest.ErrKnowledgeBaseNotFound) {
		t.Fatalf("Ingest() error = %v, want ErrKnowledgeBaseNotFound", err)
	}
	if result.Job.ID != "" {
		t.Fatalf("unexpected job for missing knowledge base: %#v", result.Job)
	}
}

func TestDeleteKnowledgeBaseWithPostgresQdrantCleansRetrievalState(t *testing.T) {
	app := newIntegrationApp(t)
	h := oraghttp.NewServer(app).Hertz().Engine
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	token := integrationLoginToken(t, h)
	marker := fmt.Sprintf("oragdel%d", time.Now().UnixNano())
	resp := performIntegrationJSON(t, h, "POST", "/v1/knowledge-bases", integrationJSON(t, map[string]any{
		"name":        "Delete " + marker,
		"description": "integration delete regression",
		"metadata":    map[string]string{"marker": marker},
	}), token)
	if resp.Code != 201 {
		t.Fatalf("create knowledge base status = %d body=%s", resp.Code, resp.Body)
	}
	var created kb.KnowledgeBase
	if err := json.Unmarshal([]byte(resp.Body), &created); err != nil {
		t.Fatal(err)
	}
	kbID := created.ID
	if kbID == "" {
		t.Fatalf("created knowledge base missing id: %#v", created)
	}
	if created.TenantID != testTenantID {
		t.Fatalf("created knowledge base tenant = %q, want %q", created.TenantID, testTenantID)
	}

	resp = performIntegrationJSON(t, h, "GET", "/v1/knowledge-bases/"+kbID, "", token)
	if resp.Code != 200 {
		t.Fatalf("get created knowledge base status = %d body=%s", resp.Code, resp.Body)
	}

	content := fmt.Sprintf("The marker %s verifies knowledge base deletion removes Qdrant vectors, semantic cache, PostgreSQL chunks, and ingestion jobs.", marker)
	resp = performIntegrationJSON(t, h, "POST", "/v1/knowledge-bases/"+kbID+"/documents:import", integrationJSON(t, map[string]any{
		"name":       marker + ".md",
		"source_uri": "integration://" + marker,
		"content":    content,
	}), token)
	if resp.Code != 202 {
		t.Fatalf("import status = %d body=%s", resp.Code, resp.Body)
	}
	var imported struct {
		Document struct {
			ID string `json:"id"`
		} `json:"document"`
		Job struct {
			ID         string           `json:"id"`
			Status     ingest.JobStatus `json:"status"`
			DocumentID string           `json:"document_id"`
			ChunkCount int              `json:"chunk_count"`
		} `json:"job"`
	}
	if err := json.Unmarshal([]byte(resp.Body), &imported); err != nil {
		t.Fatal(err)
	}
	if imported.Document.ID == "" || imported.Job.ID == "" || imported.Job.DocumentID != imported.Document.ID || imported.Job.ChunkCount == 0 || imported.Job.Status != ingest.JobStatusSucceeded {
		t.Fatalf("unexpected import response: %#v", imported)
	}

	queryBody := integrationJSON(t, map[string]any{
		"knowledge_base_id": kbID,
		"query":             "Which deletion stores are verified by marker " + marker + "?",
		"profile":           rag.ProfileRealtime,
		"top_k":             8,
	})
	resp = performIntegrationJSON(t, h, "POST", "/v1/query", queryBody, token)
	if resp.Code != 200 {
		t.Fatalf("pre-delete query status = %d body=%s", resp.Code, resp.Body)
	}
	var before rag.QueryResponse
	if err := json.Unmarshal([]byte(resp.Body), &before); err != nil {
		t.Fatal(err)
	}
	if !retrievedDocument(before, imported.Document.ID) || len(before.Citations) == 0 {
		t.Fatalf("pre-delete query did not retrieve document %s: %#v", imported.Document.ID, before)
	}

	resp = performIntegrationJSON(t, h, "POST", "/v1/query", queryBody, token)
	if resp.Code != 200 {
		t.Fatalf("pre-delete cached query status = %d body=%s", resp.Code, resp.Body)
	}
	var cached rag.QueryResponse
	if err := json.Unmarshal([]byte(resp.Body), &cached); err != nil {
		t.Fatal(err)
	}
	if cached.CacheStatus != "hit" {
		t.Fatalf("pre-delete query did not populate semantic cache: %#v", cached)
	}

	resp = performIntegrationJSON(t, h, "DELETE", "/v1/knowledge-bases/"+kbID, "", token)
	if resp.Code != 204 {
		t.Fatalf("delete status = %d body=%s", resp.Code, resp.Body)
	}

	resp = performIntegrationJSON(t, h, "GET", "/v1/knowledge-bases/"+kbID, "", token)
	assertIntegrationMissingKnowledgeBase(t, resp)

	resp = performIntegrationJSON(t, h, "GET", "/v1/knowledge-bases", "", token)
	if resp.Code != 200 {
		t.Fatalf("list knowledge bases status = %d body=%s", resp.Code, resp.Body)
	}
	if strings.Contains(resp.Body, kbID) {
		t.Fatalf("deleted knowledge base still appears in list: %s", resp.Body)
	}

	if _, ok, err := app.Ingest.Jobs.GetJob(ctx, testTenantID, imported.Job.ID); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatalf("ingestion job %s still exists after deleting knowledge base %s", imported.Job.ID, kbID)
	}

	resp = performIntegrationJSON(t, h, "POST", "/v1/query", queryBody, token)
	if resp.Code != 200 {
		t.Fatalf("post-delete query status = %d body=%s", resp.Code, resp.Body)
	}
	var after rag.QueryResponse
	if err := json.Unmarshal([]byte(resp.Body), &after); err != nil {
		t.Fatal(err)
	}
	if after.CacheStatus == "hit" || len(after.RetrievedChunks) != 0 || len(after.Citations) != 0 {
		t.Fatalf("post-delete query still returned retrieval state: %#v", after)
	}
	if !hasWarning(after.Warnings, "no_retrieved_context") {
		t.Fatalf("post-delete query warnings = %#v, want no_retrieved_context", after.Warnings)
	}

	resp = performIntegrationJSON(t, h, "POST", "/v1/knowledge-bases/"+kbID+"/documents:import", integrationJSON(t, map[string]any{
		"name":       marker + "-after-delete.md",
		"source_uri": "integration://" + marker + "/after-delete",
		"content":    "deleted knowledge base must reject new ingestion",
	}), token)
	assertIntegrationMissingKnowledgeBase(t, resp)

	resp = performIntegrationJSON(t, h, "GET", "/v1/ingestion-jobs/"+imported.Job.ID, "", token)
	if resp.Code != 404 {
		t.Fatalf("deleted ingestion job status = %d body=%s", resp.Code, resp.Body)
	}
}

type integrationHTTPResponse struct {
	Code int
	Body string
}

func integrationLoginToken(t *testing.T, h *route.Engine) string {
	t.Helper()
	resp := performIntegrationJSON(t, h, "POST", "/v1/auth/login", `{"username":"admin","password":"admin"}`, "")
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
		t.Fatal("login response missing access token")
	}
	return body.AccessToken
}

func performIntegrationJSON(t *testing.T, h *route.Engine, method, path, body, token string) integrationHTTPResponse {
	t.Helper()
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
	return integrationHTTPResponse{Code: result.StatusCode(), Body: string(result.Body())}
}

func integrationJSON(t *testing.T, v any) string {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func assertIntegrationMissingKnowledgeBase(t *testing.T, resp integrationHTTPResponse) {
	t.Helper()
	if resp.Code != 404 {
		t.Fatalf("missing knowledge base status = %d body=%s", resp.Code, resp.Body)
	}
	if !strings.Contains(resp.Body, `"code":"knowledge_base_not_found"`) {
		t.Fatalf("unexpected missing knowledge base body: %s", resp.Body)
	}
}

func retrievedDocument(resp rag.QueryResponse, documentID string) bool {
	for _, result := range resp.RetrievedChunks {
		if result.Chunk.DocumentID == documentID {
			return true
		}
	}
	return false
}

func hasWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

func citedDocument(resp rag.QueryResponse, documentID string) bool {
	for _, citation := range resp.Citations {
		if citation.DocumentID == documentID {
			return true
		}
	}
	return false
}
