package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
	qdrant "github.com/qdrant/go-client/qdrant"
	core "github.com/shikanon/orag/internal/app"
	oraghttp "github.com/shikanon/orag/internal/http"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/kb"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/storage/postgres"
	qdrantstore "github.com/shikanon/orag/internal/storage/qdrant"
	"google.golang.org/grpc"
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

func TestFailedQdrantIngestKeepsPostgresChunksUnsearchable(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	kbID := fmt.Sprintf("kb_failed_qdrant_%d", time.Now().UnixNano())
	now := time.Now().UTC()
	if err := app.KBStore.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:          kbID,
		TenantID:    testTenantID,
		Name:        "integration failed qdrant",
		Description: "temporary integration test knowledge base",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = app.KBStore.DeleteKnowledgeBase(context.Background(), testTenantID, kbID) })

	repo := postgres.NewRepository(app.Postgres)
	repo.StageChunks = true
	qdrantErr := errors.New("qdrant upsert failed")
	vectors := qdrantstore.VectorStore{
		Client:     &qdrantstore.Client{Points: failingPointsClient{err: qdrantErr}},
		Collection: app.Config.Qdrant.Collection,
	}
	app.Ingest.Indexer = kb.CompositeIndexer{Indexers: []kb.Indexer{repo, vectors}}

	marker := fmt.Sprintf("failedqdrantmarker%d", time.Now().UnixNano())
	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        testTenantID,
		KnowledgeBaseID: kbID,
		SourceURI:       "integration://" + marker,
		Name:            marker + ".md",
		Content:         []byte("The marker " + marker + " must remain hidden after a failed qdrant write."),
	})
	if !errors.Is(err, qdrantErr) {
		t.Fatalf("Ingest() error = %v, want %v", err, qdrantErr)
	}
	if result.Job.Status != ingest.JobStatusFailed {
		t.Fatalf("job status = %q", result.Job.Status)
	}
	job, ok, err := app.Ingest.Jobs.GetJob(ctx, testTenantID, result.Job.ID)
	if err != nil || !ok {
		t.Fatalf("job lookup ok=%v err=%v", ok, err)
	}
	if job.Status != ingest.JobStatusFailed {
		t.Fatalf("stored job status = %q", job.Status)
	}
	if count := countPostgresRows(t, ctx, app, "chunks", kbID); count == 0 {
		t.Fatal("expected staged postgres chunks after qdrant failure")
	}
	if count := countSearchablePostgresChunks(t, ctx, app, kbID); count != 0 {
		t.Fatalf("searchable postgres chunks after qdrant failure = %d", count)
	}

	results, err := postgres.NewFTSRetriever(repo).Retrieve(ctx, kb.SearchRequest{
		TenantID:        testTenantID,
		KnowledgeBaseID: kbID,
		Query:           marker,
		TopK:            8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("failed ingest chunks are searchable through postgres FTS: %#v", results)
	}
}

func TestHTTPIngestMissingKnowledgeBaseWithPostgresQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	h := oraghttp.NewServer(app).Hertz().Engine
	token := loginHTTPToken(t, h, app.Config.Auth.AdminDefaultUsername, app.Config.Auth.AdminDefaultPassword)
	missingKB := fmt.Sprintf("kb_missing_http_%d", time.Now().UnixNano())

	status, body := performIntegrationJSON(h, "POST", "/v1/knowledge-bases/"+missingKB+"/documents:import", `{"name":"missing.md","source_uri":"integration://missing-http","content":"missing knowledge bases must return 404"}`, token)
	assertMissingKBHTTPResponse(t, status, body)
	assertNoStoredChunks(t, app.KBStore, missingKB)

	status, body = performIntegrationUpload(t, h, "/v1/knowledge-bases/"+missingKB+"/documents", "missing.md", "missing knowledge bases must return 404", token)
	assertMissingKBHTTPResponse(t, status, body)
	assertNoStoredChunks(t, app.KBStore, missingKB)
}

func TestDeleteKnowledgeBaseCleansPostgresAndQdrant(t *testing.T) {
	app := newIntegrationApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	kbID := fmt.Sprintf("kb_delete_%d", time.Now().UnixNano())
	now := time.Now().UTC()
	if err := app.KBStore.PutKnowledgeBase(ctx, kb.KnowledgeBase{
		ID:          kbID,
		TenantID:    testTenantID,
		Name:        "integration delete",
		Description: "temporary integration test knowledge base",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := app.Ingest.Ingest(ctx, ingest.Request{
		TenantID:        testTenantID,
		KnowledgeBaseID: kbID,
		SourceURI:       "integration://" + kbID,
		Name:            kbID + ".md",
		Content:         []byte("This marker verifies knowledge base deletion clears vectors and rows."),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.ChunkCount == 0 {
		t.Fatalf("expected chunks before delete: %#v", result.Job)
	}
	if count := countPostgresRows(t, ctx, app, "chunks", kbID); count == 0 {
		t.Fatal("expected postgres chunks before delete")
	}
	if count := countQdrantPoints(t, ctx, app, kbID); count == 0 {
		t.Fatal("expected qdrant points before delete")
	}
	cacheVector := make([]float64, app.Config.Ark.EmbeddingDimensions)
	cacheVector[0] = 0.42
	if err := app.RAG.Cache.Store(ctx, rag.SemanticCacheEntry{
		TenantID:        testTenantID,
		KnowledgeBaseID: kbID,
		Query:           "cache cleanup " + kbID,
		Vector:          cacheVector,
		Profile:         rag.ProfileRealtime,
		TopK:            8,
		Response: rag.QueryResponse{
			Answer:  "cached answer for deleted knowledge base",
			Profile: rag.ProfileRealtime,
			RetrievedChunks: []kb.SearchResult{{
				Chunk: kb.Chunk{
					ID:              "cached_" + kbID,
					TenantID:        testTenantID,
					KnowledgeBaseID: kbID,
					DocumentID:      result.Document.ID,
					Content:         "cached deleted content",
				},
			}},
		},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if count := countQdrantSemanticCachePoints(t, ctx, app, kbID); count == 0 {
		t.Fatal("expected qdrant semantic cache points before delete")
	}

	deleted, err := app.KBStore.DeleteKnowledgeBase(ctx, testTenantID, kbID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("DeleteKnowledgeBase deleted=false, want true")
	}
	if _, ok, err := app.KBStore.GetKnowledgeBase(testTenantID, kbID); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("deleted knowledge base is still readable")
	}
	for _, table := range []string{"chunks", "documents", "ingestion_jobs", "knowledge_bases"} {
		if count := countPostgresRows(t, ctx, app, table, kbID); count != 0 {
			t.Fatalf("%s rows after delete = %d", table, count)
		}
	}
	if count := countQdrantPoints(t, ctx, app, kbID); count != 0 {
		t.Fatalf("qdrant points after delete = %d", count)
	}
	if count := countQdrantSemanticCachePoints(t, ctx, app, kbID); count != 0 {
		t.Fatalf("qdrant semantic cache points after delete = %d", count)
	}
}

func countPostgresRows(t *testing.T, ctx context.Context, app *core.App, table, kbID string) int {
	t.Helper()
	var where string
	switch table {
	case "chunks", "documents", "ingestion_jobs":
		where = "tenant_id=$1 AND knowledge_base_id=$2"
	case "knowledge_bases":
		where = "tenant_id=$1 AND id=$2"
	default:
		t.Fatalf("unexpected table %q", table)
	}
	var count int
	if err := app.Postgres.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE %s", table, where), testTenantID, kbID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func countQdrantPoints(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	return countQdrantPointsInCollection(t, ctx, app, app.Config.Qdrant.Collection, kbID)
}

func countQdrantSemanticCachePoints(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	return countQdrantPointsInCollection(t, ctx, app, app.Config.Qdrant.SemanticCacheCollection, kbID)
}

func countQdrantPointsInCollection(t *testing.T, ctx context.Context, app *core.App, collection, kbID string) int {
	t.Helper()
	exact := true
	resp, err := app.Qdrant.Points.Count(ctx, &qdrant.CountPoints{
		CollectionName: collection,
		Filter:         integrationKnowledgeBaseFilter(kbID),
		Exact:          &exact,
	})
	if err != nil {
		t.Fatal(err)
	}
	return int(resp.GetResult().GetCount())
}

func countSearchablePostgresChunks(t *testing.T, ctx context.Context, app *core.App, kbID string) int {
	t.Helper()
	var count int
	if err := app.Postgres.QueryRow(ctx, `
		SELECT count(*) FROM chunks
		WHERE tenant_id=$1 AND knowledge_base_id=$2 AND searchable`,
		testTenantID, kbID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func integrationKnowledgeBaseFilter(kbID string) *qdrant.Filter {
	return &qdrant.Filter{Must: []*qdrant.Condition{
		integrationMatchKeyword("tenant_id", testTenantID),
		integrationMatchKeyword("knowledge_base_id", kbID),
	}}
}

func integrationMatchKeyword(key, value string) *qdrant.Condition {
	return &qdrant.Condition{ConditionOneOf: &qdrant.Condition_Field{Field: &qdrant.FieldCondition{
		Key: key,
		Match: &qdrant.Match{MatchValue: &qdrant.Match_Keyword{
			Keyword: value,
		}},
	}}}
}

func retrievedDocument(resp rag.QueryResponse, documentID string) bool {
	for _, result := range resp.RetrievedChunks {
		if result.Chunk.DocumentID == documentID {
			return true
		}
	}
	return false
}

type failingPointsClient struct {
	err error
}

func (c failingPointsClient) Upsert(context.Context, *qdrant.UpsertPoints, ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	return nil, c.err
}

func (c failingPointsClient) Search(context.Context, *qdrant.SearchPoints, ...grpc.CallOption) (*qdrant.SearchResponse, error) {
	return nil, c.err
}

func (c failingPointsClient) Delete(context.Context, *qdrant.DeletePoints, ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	return nil, c.err
}

func (c failingPointsClient) Count(context.Context, *qdrant.CountPoints, ...grpc.CallOption) (*qdrant.CountResponse, error) {
	return nil, c.err
}

func loginHTTPToken(t *testing.T, h *route.Engine, username, password string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		t.Fatal(err)
	}
	status, resp := performIntegrationJSON(h, "POST", "/v1/auth/login", string(body), "")
	if status != 200 {
		t.Fatalf("login status = %d body=%s", status, resp)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.AccessToken == "" {
		t.Fatal("missing access token")
	}
	return parsed.AccessToken
}

func performIntegrationJSON(h *route.Engine, method, path, body, token string) (int, string) {
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
	return result.StatusCode(), string(result.Body())
}

func performIntegrationUpload(t *testing.T, h *route.Engine, path, filename, content, token string) (int, string) {
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
	return result.StatusCode(), string(result.Body())
}

func assertMissingKBHTTPResponse(t *testing.T, status int, body string) {
	t.Helper()
	if status != 404 {
		t.Fatalf("missing knowledge base status = %d body=%s", status, body)
	}
	if !strings.Contains(body, `"code":"knowledge_base_not_found"`) {
		t.Fatalf("unexpected missing knowledge base body: %s", body)
	}
	if strings.Contains(body, `"code":"ingest_failed"`) {
		t.Fatalf("missing knowledge base returned ingest_failed: %s", body)
	}
}

func assertNoStoredChunks(t *testing.T, store any, kbID string) {
	t.Helper()
	chunks, ok := store.(kb.ChunkSource)
	if !ok {
		t.Fatalf("store does not expose chunks")
	}
	if got := chunks.Chunks(testTenantID, kbID); len(got) != 0 {
		t.Fatalf("chunks created for missing knowledge base: %#v", got)
	}
}

func citedDocument(resp rag.QueryResponse, documentID string) bool {
	for _, citation := range resp.Citations {
		if citation.DocumentID == documentID {
			return true
		}
	}
	return false
}
