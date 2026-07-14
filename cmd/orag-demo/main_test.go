package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestRunWalkthroughWritesSummaryAndReusesCompletedState(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/readyz":
			json.NewEncoder(w).Encode(map[string]any{"status": "ready"})
		case "/v1/auth/login":
			json.NewEncoder(w).Encode(map[string]any{"access_token": "token"})
		case "/v1/knowledge-bases":
			json.NewEncoder(w).Encode(map[string]any{"id": "kb_demo"})
		case "/v1/knowledge-bases/kb_demo/documents:import":
			json.NewEncoder(w).Encode(map[string]any{"document": map[string]any{"id": "doc_demo"}})
		case "/v1/query":
			json.NewEncoder(w).Encode(map[string]any{"answer": "ORAG is Go-native.", "trace_id": "trace_demo", "citations": []map[string]any{{"document_id": "doc_demo"}}})
		case "/v1/traces/trace_demo":
			json.NewEncoder(w).Encode(map[string]any{"trace_id": "trace_demo"})
		case "/v1/datasets":
			json.NewEncoder(w).Encode(map[string]any{"id": "ds_demo"})
		case "/v1/datasets/ds_demo/items":
			json.NewEncoder(w).Encode(map[string]any{"id": "dsi_demo"})
		case "/v1/evaluations":
			json.NewEncoder(w).Encode(map[string]any{"id": "eval_demo", "total": 1, "metrics": map[string]float64{"hit_rate": 1}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "walkthrough.json")
	cfg := demoConfig{BaseURL: server.URL, Username: "admin", Password: "admin", SummaryPath: path}
	if err := run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	firstRequests := requests.Load()
	if err := run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != firstRequests {
		t.Fatalf("completed walkthrough made new requests: before=%d after=%d", firstRequests, requests.Load())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var summary walkthroughSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Status != "completed" || summary.KnowledgeBaseID != "kb_demo" || summary.EvaluationID != "eval_demo" || summary.CitationCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}
