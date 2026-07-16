package http

import (
	"testing"

	"github.com/shikanon/orag/internal/execution"
)

func TestExecutionOperationClassifiesExpensiveWritePaths(t *testing.T) {
	tests := []struct {
		route  string
		method string
		want   execution.Operation
		ok     bool
	}{
		{"/v1/knowledge-bases/:id/documents", "POST", execution.Ingestion, true},
		{"/v1/knowledge-bases/:id/documents:import", "POST", execution.Ingestion, true},
		{"/v1/uploads/*action", "POST", execution.Ingestion, true},
		{"/v1/query", "POST", execution.Query, true},
		{"/v1/query:stream", "POST", execution.Query, true},
		{"/v1/evaluations", "POST", execution.Evaluation, true},
		{"/v1/projects/:project_id/releases:promote", "POST", execution.Release, true},
		{"/v1/projects/:project_id/environments/production/rollback", "POST", execution.Release, true},
		{"/v1/projects/:project_id/versions", "POST", execution.Release, true},
		{"/v1/query", "GET", "", false},
		{"/v1/knowledge-bases", "POST", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.route+tt.method, func(t *testing.T) {
			got, ok := executionOperation(tt.route, tt.method)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("executionOperation(%q, %q) = (%q, %v), want (%q, %v)", tt.route, tt.method, got, ok, tt.want, tt.ok)
			}
		})
	}
}
