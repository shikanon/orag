package contract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type grafanaDashboard struct {
	SchemaVersion int `json:"schemaVersion"`
	Templating    struct {
		List []struct {
			Name  string `json:"name"`
			Query string `json:"query"`
			Type  string `json:"type"`
		} `json:"list"`
	} `json:"templating"`
	Panels []struct {
		Title   string `json:"title"`
		Targets []struct {
			Expr string `json:"expr"`
		} `json:"targets"`
	} `json:"panels"`
}

func TestGrafanaDashboardUsesDocumentedMetrics(t *testing.T) {
	body, err := os.ReadFile("../../deployments/grafana/dashboards/orag-overview.json")
	if err != nil {
		t.Fatal(err)
	}
	var dashboard grafanaDashboard
	if err := json.Unmarshal(body, &dashboard); err != nil {
		t.Fatalf("parse Grafana dashboard: %v", err)
	}
	if dashboard.SchemaVersion < 41 {
		t.Fatalf("schemaVersion = %d, want >= 41", dashboard.SchemaVersion)
	}
	if len(dashboard.Templating.List) != 1 || dashboard.Templating.List[0].Name != "datasource" || dashboard.Templating.List[0].Type != "datasource" || dashboard.Templating.List[0].Query != "prometheus" {
		t.Fatalf("dashboard must expose one Prometheus datasource variable, got %#v", dashboard.Templating.List)
	}

	expressions := make(map[string]bool)
	titles := make(map[string]bool)
	for _, panel := range dashboard.Panels {
		titles[panel.Title] = true
		for _, target := range panel.Targets {
			expressions[target.Expr] = true
		}
	}
	for _, title := range []string{"API availability", "HTTP request and 5xx rate", "HTTP latency p95", "RAG outcomes and cache status", "RAG latency p95", "Dependency readiness checks", "Trace-store outcomes"} {
		if !titles[title] {
			t.Errorf("dashboard missing panel %q", title)
		}
	}
	for _, metric := range []string{"orag_up", "orag_http_requests_total", "orag_http_errors_total", "orag_http_request_latency_ms_bucket", "orag_rag_queries_total", "orag_rag_query_latency_ms_bucket", "orag_dependency_checks_total", "orag_trace_store_total"} {
		found := false
		for expression := range expressions {
			if strings.Contains(expression, metric) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("dashboard has no query for %s", metric)
		}
	}
	for expression := range expressions {
		for _, forbidden := range []string{"trace_id", "tenant", "prompt", "document", "user"} {
			if strings.Contains(strings.ToLower(expression), forbidden) {
				t.Errorf("dashboard expression %q contains forbidden high-cardinality field %q", expression, forbidden)
			}
		}
	}
}
