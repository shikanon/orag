// Command orag-demo runs the deterministic HTTP walkthrough used by Compose
// and release smoke tests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type demoConfig struct {
	BaseURL     string
	Username    string
	Password    string
	SummaryPath string
	WaitTimeout time.Duration
	HTTPClient  *http.Client
}

type walkthroughSummary struct {
	Status          string             `json:"status"`
	LastStep        string             `json:"last_step"`
	KnowledgeBaseID string             `json:"knowledge_base_id,omitempty"`
	DocumentID      string             `json:"document_id,omitempty"`
	TraceID         string             `json:"trace_id,omitempty"`
	DatasetID       string             `json:"dataset_id,omitempty"`
	EvaluationID    string             `json:"evaluation_id,omitempty"`
	CitationCount   int                `json:"citation_count"`
	Metrics         map[string]float64 `json:"metrics,omitempty"`
	Error           string             `json:"error,omitempty"`
	CompletedAt     time.Time          `json:"completed_at,omitempty"`
}

type apiClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func main() {
	waitTimeout, err := time.ParseDuration(env("ORAG_DEMO_WAIT_TIMEOUT", "2m"))
	if err != nil {
		log.Fatalf("invalid ORAG_DEMO_WAIT_TIMEOUT: %v", err)
	}
	cfg := demoConfig{
		BaseURL:     env("ORAG_DEMO_BASE_URL", "http://orag-api:8080"),
		Username:    env("ORAG_DEMO_USERNAME", "admin"),
		Password:    env("ORAG_DEMO_PASSWORD", "admin"),
		SummaryPath: env("ORAG_DEMO_SUMMARY", "/demo/walkthrough.json"),
		WaitTimeout: waitTimeout,
	}
	if err := run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ORAG deterministic walkthrough completed: %s\n", cfg.SummaryPath)
}

func run(ctx context.Context, cfg demoConfig) error {
	if existing, err := readSummary(cfg.SummaryPath); err == nil && existing.Status == "completed" {
		return nil
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Username) == "" || cfg.Password == "" {
		return errors.New("base URL, username, and password are required")
	}
	if cfg.WaitTimeout <= 0 {
		cfg.WaitTimeout = 2 * time.Minute
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	client := &apiClient{baseURL: strings.TrimRight(cfg.BaseURL, "/"), client: cfg.HTTPClient}
	summary := walkthroughSummary{Status: "running", LastStep: "wait_for_readiness"}
	fail := func(step string, err error) error {
		summary.Status = "failed"
		summary.LastStep = step
		summary.Error = err.Error()
		_ = writeSummary(cfg.SummaryPath, summary)
		return fmt.Errorf("walkthrough failed after %s: %w", step, err)
	}

	if err := client.waitReady(ctx, cfg.WaitTimeout); err != nil {
		return fail(summary.LastStep, err)
	}
	summary.LastStep = "login"
	var login struct {
		AccessToken string `json:"access_token"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/auth/login", map[string]string{"username": cfg.Username, "password": cfg.Password}, &login); err != nil {
		return fail(summary.LastStep, err)
	}
	if login.AccessToken == "" {
		return fail(summary.LastStep, errors.New("login response has no access_token"))
	}
	client.token = login.AccessToken

	summary.LastStep = "create_knowledge_base"
	var knowledgeBase struct {
		ID string `json:"id"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/knowledge-bases", map[string]any{"name": "ORAG deterministic walkthrough", "description": "Created by the no-key Compose demo"}, &knowledgeBase); err != nil {
		return fail(summary.LastStep, err)
	}
	summary.KnowledgeBaseID = knowledgeBase.ID

	summary.LastStep = "ingest_document"
	var ingestion struct {
		Document struct {
			ID string `json:"id"`
		} `json:"document"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/knowledge-bases/"+knowledgeBase.ID+"/documents:import", map[string]any{
		"name":       "orag-walkthrough.md",
		"source_uri": "demo://orag/walkthrough",
		"content":    "ORAG is a Go-native RAG service and evaluation control plane. It combines ingestion, retrieval, cited generation, traces, and reproducible evaluation.",
	}, &ingestion); err != nil {
		return fail(summary.LastStep, err)
	}
	summary.DocumentID = ingestion.Document.ID

	summary.LastStep = "query"
	var query struct {
		TraceID   string            `json:"trace_id"`
		Citations []json.RawMessage `json:"citations"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/query", map[string]any{
		"knowledge_base_id": knowledgeBase.ID,
		"query":             "What is ORAG and which workflow does it provide?",
		"profile":           "realtime",
	}, &query); err != nil {
		return fail(summary.LastStep, err)
	}
	if len(query.Citations) == 0 {
		return fail(summary.LastStep, errors.New("query returned no citations"))
	}
	summary.TraceID = query.TraceID
	summary.CitationCount = len(query.Citations)

	summary.LastStep = "lookup_trace"
	var trace json.RawMessage
	if err := client.request(ctx, http.MethodGet, "/v1/traces/"+query.TraceID, nil, &trace); err != nil {
		return fail(summary.LastStep, err)
	}

	summary.LastStep = "create_dataset"
	var dataset struct {
		ID string `json:"id"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/datasets", map[string]string{"name": "ORAG walkthrough evaluation", "kind": "retrieval"}, &dataset); err != nil {
		return fail(summary.LastStep, err)
	}
	summary.DatasetID = dataset.ID

	summary.LastStep = "add_dataset_item"
	if err := client.request(ctx, http.MethodPost, "/v1/datasets/"+dataset.ID+"/items", map[string]any{
		"query":            "What is ORAG?",
		"ground_truth":     "ORAG is a Go-native RAG service and evaluation control plane.",
		"relevant_doc_ids": []string{ingestion.Document.ID},
		"split":            "eval",
	}, &json.RawMessage{}); err != nil {
		return fail(summary.LastStep, err)
	}

	summary.LastStep = "run_evaluation"
	var evaluation struct {
		ID      string             `json:"id"`
		Metrics map[string]float64 `json:"metrics"`
	}
	if err := client.request(ctx, http.MethodPost, "/v1/evaluations", map[string]any{
		"dataset_id":        dataset.ID,
		"knowledge_base_id": knowledgeBase.ID,
		"profile":           "realtime",
		"split":             "eval",
	}, &evaluation); err != nil {
		return fail(summary.LastStep, err)
	}
	summary.EvaluationID = evaluation.ID
	summary.Metrics = evaluation.Metrics
	summary.Status = "completed"
	summary.LastStep = "completed"
	summary.CompletedAt = time.Now().UTC()
	return writeSummary(cfg.SummaryPath, summary)
}

func (c *apiClient) waitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var response struct {
			Status string `json:"status"`
		}
		if err := c.request(ctx, http.MethodGet, "/readyz", nil, &response); err == nil && response.Status == "ready" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("API was not ready within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (c *apiClient) request(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("%s %s returned %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(message)))
	}
	if output == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}

func readSummary(path string) (walkthroughSummary, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return walkthroughSummary{}, err
	}
	var summary walkthroughSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		return walkthroughSummary{}, err
	}
	return summary, nil
}

func writeSummary(path string, summary walkthroughSummary) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
