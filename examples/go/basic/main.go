// Package main demonstrates how another Go service can call ORAG through its
// public HTTP/OpenAPI surface without importing ORAG internal packages.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	} `json:"error"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type KnowledgeBase struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type IngestionResponse struct {
	Document struct {
		ID string `json:"id"`
	} `json:"document"`
	Chunks int          `json:"chunks"`
	Job    IngestionJob `json:"job"`
}

type IngestionJob struct {
	ID              string `json:"id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Status          string `json:"status"`
	SourceURI       string `json:"source_uri"`
	DocumentID      string `json:"document_id"`
	ChunkCount      int    `json:"chunk_count"`
	Error           string `json:"error"`
}

type QueryResponse struct {
	Answer          string `json:"answer"`
	TraceID         string `json:"trace_id"`
	CacheStatus     string `json:"cache_status"`
	Profile         string `json:"profile"`
	LatencyMillis   int64  `json:"latency_ms"`
	RetrievedChunks []struct {
		Score float64 `json:"score"`
		Rank  int     `json:"rank"`
		From  string  `json:"from"`
		Chunk struct {
			ID        string `json:"id"`
			SourceURI string `json:"source_uri"`
			Content   string `json:"content"`
		} `json:"chunk"`
	} `json:"retrieved_chunks"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Login(ctx context.Context, username, password string) (*LoginResponse, error) {
	var resp LoginResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, &resp)
	if err != nil {
		return nil, err
	}
	c.Token = resp.AccessToken
	return &resp, nil
}

func (c *Client) CreateKnowledgeBase(ctx context.Context, name, description string) (*KnowledgeBase, error) {
	var resp KnowledgeBase
	err := c.doJSON(ctx, http.MethodPost, "/v1/knowledge-bases", map[string]string{
		"name":        name,
		"description": description,
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ImportDocument(ctx context.Context, kbID, name, sourceURI, content string) (*IngestionResponse, error) {
	var resp IngestionResponse
	path := fmt.Sprintf("/v1/knowledge-bases/%s/documents:import", url.PathEscape(kbID))
	err := c.doJSON(ctx, http.MethodPost, path, map[string]string{
		"name":       name,
		"source_uri": sourceURI,
		"content":    content,
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetIngestionJob(ctx context.Context, jobID string) (*IngestionJob, error) {
	var resp IngestionJob
	path := fmt.Sprintf("/v1/ingestion-jobs/%s", url.PathEscape(jobID))
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Query(ctx context.Context, kbID, question, profile string) (*QueryResponse, error) {
	var resp QueryResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/query", map[string]string{
		"knowledge_base_id": kbID,
		"query":             question,
		"profile":           profile,
	}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr ErrorResponse
		if err := json.Unmarshal(respBody, &apiErr); err == nil && apiErr.Error.Code != "" {
			return fmt.Errorf("orag api error: status=%d code=%s message=%s trace_id=%s",
				resp.StatusCode, apiErr.Error.Code, apiErr.Error.Message, apiErr.Error.TraceID)
		}
		return fmt.Errorf("orag api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	baseURL := env("ORAG_BASE_URL", "http://localhost:8080")
	username := env("ORAG_USERNAME", "admin")
	password := env("ORAG_PASSWORD", "admin")
	profile := env("ORAG_PROFILE", "realtime")

	client := NewClient(baseURL)
	if _, err := client.Login(ctx, username, password); err != nil {
		exitf("login: %v", err)
	}

	kb, err := client.CreateKnowledgeBase(ctx, env("ORAG_KB_NAME", "Go SDK Example KB"), "created by examples/go/basic")
	if err != nil {
		exitf("create knowledge base: %v", err)
	}
	fmt.Printf("knowledge_base_id=%s\n", kb.ID)

	ingestion, err := client.ImportDocument(
		ctx,
		kb.ID,
		env("ORAG_DOC_NAME", "orag-go-sdk-example.md"),
		env("ORAG_DOC_SOURCE_URI", "example://go-sdk/orag"),
		env("ORAG_DOC_CONTENT", "ORAG 是 Go RAG 框架，支持知识库入库、混合检索、重排、问答和评估。"),
	)
	if err != nil {
		exitf("import document: %v", err)
	}
	fmt.Printf("document_id=%s job_id=%s chunks=%d status=%s\n",
		ingestion.Document.ID, ingestion.Job.ID, ingestion.Chunks, ingestion.Job.Status)

	job, err := waitIngestion(ctx, client, ingestion.Job.ID)
	if err != nil {
		exitf("wait ingestion: %v", err)
	}
	fmt.Printf("ingestion_status=%s chunk_count=%d\n", job.Status, job.ChunkCount)

	query, err := client.Query(ctx, kb.ID, env("ORAG_QUERY", "ORAG 支持哪些能力？"), profile)
	if err != nil {
		exitf("query: %v", err)
	}
	fmt.Printf("answer=%s\ntrace_id=%s cache_status=%s profile=%s latency_ms=%d retrieved_chunks=%d\n",
		query.Answer, query.TraceID, query.CacheStatus, query.Profile, query.LatencyMillis, len(query.RetrievedChunks))
}

func waitIngestion(ctx context.Context, client *Client, jobID string) (*IngestionJob, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		job, err := client.GetIngestionJob(ctx, jobID)
		if err != nil {
			return nil, err
		}
		switch job.Status {
		case "succeeded":
			return job, nil
		case "failed":
			return nil, fmt.Errorf("job %s failed: %s", job.ID, job.Error)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
